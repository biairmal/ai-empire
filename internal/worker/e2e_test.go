package worker_test

// End-to-end V1 check: control plane + worker + real Postgres + real git, fake agent.
// Needs EMPIRE_TEST_DATABASE_URL (make test sets it); skipped otherwise.

import (
	"context"
	"errors"
	"net/http/httptest"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"

	"aiempire/internal/api"
	"aiempire/internal/client"
	"aiempire/internal/controlplane"
	"aiempire/internal/task"
	"aiempire/internal/worker"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

func TestV1EndToEnd(t *testing.T) {
	dbURL := os.Getenv("EMPIRE_TEST_DATABASE_URL")
	if dbURL == "" {
		t.Skip("EMPIRE_TEST_DATABASE_URL not set")
	}
	ctx := context.Background()
	pool := freshDB(t, ctx, dbURL)
	tmp := t.TempDir()

	// Remote repo with one commit on main.
	origin := filepath.Join(tmp, "origin.git")
	mustGit(t, tmp, "init", "--bare", "-b", "main", origin)
	seed := filepath.Join(tmp, "seed")
	mustGit(t, tmp, "clone", origin, seed)
	os.WriteFile(filepath.Join(seed, "README.md"), []byte("demo\n"), 0o644)
	mustGit(t, seed, "add", ".")
	mustGit(t, seed, "-c", "user.name=t", "-c", "user.email=t@t", "commit", "-m", "init")
	mustGit(t, seed, "push", "origin", "HEAD:main")

	// Knowledge repo.
	kdir := filepath.Join(tmp, "knowledge")
	writeFile(t, kdir, "global/principles.md", "Simple over clever.")
	writeFile(t, kdir, "stacks/dotnet/efcore.md", "DOTNET ONLY")
	writeFile(t, kdir, "projects/demo/requirements/prd.md", "Health endpoint PRD")

	srv, err := controlplane.New(pool, controlplane.Config{
		OwnerToken: "owner-t", WorkerToken: "worker-t", KnowledgeDir: kdir, StaleAfter: time.Minute,
	})
	if err != nil {
		t.Fatal(err)
	}
	hs := httptest.NewServer(srv.Handler())
	defer hs.Close()
	owner := client.New(hs.URL, "owner-t")

	testCmd := "cat empire-fake-agent.txt"
	if runtime.GOOS == "windows" {
		testCmd = "type empire-fake-agent.txt"
	}
	var proj api.Project
	do(t, owner, "POST", "/projects", api.Project{
		Slug: "demo", Name: "Demo", RepoURL: origin, Stack: "go", AutonomyLevel: "high", TestCommand: testCmd,
	}, &proj)

	var t1 api.Task
	do(t, owner, "POST", "/tasks", api.CreateTask{
		Project: "demo", Title: "add /health endpoint", ContextDocs: []string{"requirements/prd.md"},
	}, &t1)
	expectStatus(t, owner, "POST", "/tasks", api.CreateTask{Project: "demo", Title: "x", ContextDocs: []string{"../other/secret.md"}}, 400)

	w, err := worker.New(client.New(hs.URL, "worker-t"), worker.Config{
		Name: "test-worker", Capabilities: []string{"go"}, Workspaces: filepath.Join(tmp, "ws"), Agent: worker.Fake{},
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := w.Register(ctx); err != nil {
		t.Fatal(err)
	}
	workerClient := client.New(hs.URL, "worker-t")
	workerClient.WorkerID = 1

	// 1. Implement → parked at the merge gate.
	step(t, w, true)
	d := taskDetail(t, owner, t1.ID)
	if d.Task.Status != task.WaitingForHuman || d.Task.Stage != task.StageMerge {
		t.Fatalf("after implement: %s/%s (%s)", d.Task.Status, d.Task.Stage, d.Task.LastError)
	}
	if len(d.Runs) != 1 || !slices.Contains(d.Runs[0].ContextFiles, "projects/demo/requirements/prd.md") ||
		slices.Contains(d.Runs[0].ContextFiles, "stacks/dotnet/efcore.md") {
		t.Fatalf("runs = %+v", d.Runs)
	}
	if len(d.Approvals) != 1 || d.Approvals[0].Gate != "merge_protected" || d.Approvals[0].Status != "PENDING_APPROVAL" {
		t.Fatalf("approvals = %+v", d.Approvals)
	}
	if _, err := exec.Command("git", "--git-dir", origin, "rev-parse", "--verify", "ai/task-1").Output(); err != nil {
		t.Fatalf("branch not pushed: %v", err)
	}
	if _, err := exec.Command("git", "--git-dir", origin, "show", "main:empire-fake-agent.txt").Output(); err == nil {
		t.Fatal("main changed before approval")
	}

	// AI can't approve: worker token is rejected on decision endpoints.
	expectStatus(t, workerClient, "POST", "/approvals/1/approve", api.Decision{}, 403)
	// Nothing to claim while waiting.
	step(t, w, false)

	// 2. Request changes → rework on the same branch → new gate.
	do(t, owner, "POST", "/approvals/1/request-changes", api.Decision{Comment: "also log requests"}, nil)
	expectStatus(t, owner, "POST", "/approvals/1/approve", api.Decision{}, 409) // already decided
	step(t, w, true)
	d = taskDetail(t, owner, t1.ID)
	if d.Task.Status != task.WaitingForHuman || len(d.Approvals) != 2 || d.Task.Feedback != "also log requests" {
		t.Fatalf("after rework: %+v", d)
	}

	// 3. Approve → merge exactly the approved commit → COMPLETED.
	approvedSHA := d.Approvals[1].SubjectVersion
	do(t, owner, "POST", "/approvals/2/approve", api.Decision{Comment: "ship it"}, nil)
	step(t, w, true)
	d = taskDetail(t, owner, t1.ID)
	if d.Task.Status != task.Completed {
		t.Fatalf("after merge: %s (%s)", d.Task.Status, d.Task.LastError)
	}
	out, err := exec.Command("git", "--git-dir", origin, "show", "main:empire-fake-agent.txt").Output()
	if err != nil || strings.Count(string(out), "\n") != 2 {
		t.Fatalf("main content = %q, %v", out, err)
	}
	if err := exec.Command("git", "--git-dir", origin, "merge-base", "--is-ancestor", approvedSHA, "main").Run(); err != nil {
		t.Fatalf("approved commit not on main: %v", err)
	}

	// 4. Crash recovery: a claimed task is requeued when its worker re-registers or goes silent.
	var t2, t3 api.Task
	do(t, owner, "POST", "/tasks", api.CreateTask{Project: "demo", Title: "second"}, &t2)
	do(t, owner, "POST", "/tasks", api.CreateTask{Project: "demo", Title: "third", DependsOn: []int64{t2.ID}}, &t3)
	var cl *api.Claim
	do(t, workerClient, "POST", "/workers/1/claim", nil, &cl)
	if cl == nil || cl.Task.ID != t2.ID {
		t.Fatalf("claim = %+v", cl)
	}
	if err := w.Register(ctx); err != nil { // "restart"
		t.Fatal(err)
	}
	if s := taskDetail(t, owner, t2.ID).Task.Status; s != task.Pending {
		t.Fatalf("after restart: %s", s)
	}
	cl = nil
	do(t, workerClient, "POST", "/workers/1/claim", nil, &cl)
	pool.Exec(ctx, `UPDATE workers SET last_heartbeat_at = now() - interval '1 hour'`)
	if err := srv.ReapOnce(ctx); err != nil {
		t.Fatal(err)
	}
	if s := taskDetail(t, owner, t2.ID).Task.Status; s != task.Pending {
		t.Fatalf("after reap: %s", s)
	}
	if err := w.Register(ctx); err != nil { // back online
		t.Fatal(err)
	}

	// 5. Cancel; a task with an unfinished dependency is never claimed.
	do(t, owner, "POST", "/tasks/2/cancel", nil, nil)
	expectStatus(t, owner, "POST", "/tasks/2/cancel", nil, 409)
	step(t, w, false)

	// 6. Audit trail is complete and append-only.
	var entries []api.AuditEntry
	do(t, owner, "GET", "/audit?"+url.Values{"target": {"task:1"}}.Encode(), nil, &entries)
	actions := map[string]int{}
	for _, e := range entries {
		actions[e.Action]++
	}
	for _, a := range []string{"task.create", "task.status", "agent_run.start", "agent_run.finish", "policy.allow"} {
		if actions[a] == 0 {
			t.Errorf("audit missing %s: %v", a, actions)
		}
	}
	if _, err := pool.Exec(ctx, `DELETE FROM audit_log`); err == nil {
		t.Error("audit_log delete should fail")
	}
	var decisions int
	pool.QueryRow(ctx, `SELECT count(*) FROM approval_decisions`).Scan(&decisions)
	if decisions != 2 {
		t.Errorf("decisions = %d", decisions)
	}
}

func freshDB(t *testing.T, ctx context.Context, dbURL string) *pgxpool.Pool {
	t.Helper()
	u, err := url.Parse(dbURL)
	if err != nil {
		t.Fatal(err)
	}
	name := strings.TrimPrefix(u.Path, "/")
	admin := *u
	admin.Path = "/postgres"
	conn, err := pgx.Connect(ctx, admin.String())
	if err != nil {
		t.Skipf("postgres not reachable (%v); run `make up`", err)
	}
	defer conn.Close(ctx)
	ident := pgx.Identifier{name}.Sanitize()
	for _, q := range []string{"DROP DATABASE IF EXISTS " + ident + " WITH (FORCE)", "CREATE DATABASE " + ident} {
		if _, err := conn.Exec(ctx, q); err != nil {
			t.Fatal(err)
		}
	}
	pool, err := pgxpool.New(ctx, dbURL)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	files, _ := filepath.Glob("../../migrations/*.up.sql")
	slices.Sort(files)
	for _, f := range files {
		sql, _ := os.ReadFile(f)
		if _, err := pool.Exec(ctx, string(sql)); err != nil {
			t.Fatalf("%s: %v", f, err)
		}
	}
	return pool
}

func step(t *testing.T, w *worker.Worker, wantWorked bool) {
	t.Helper()
	worked, err := w.Step(context.Background())
	if err != nil || worked != wantWorked {
		t.Fatalf("step: worked=%v err=%v, want worked=%v", worked, err, wantWorked)
	}
}

func do(t *testing.T, c *client.Client, method, path string, in, out any) {
	t.Helper()
	if err := c.Do(context.Background(), method, path, in, out); err != nil {
		t.Fatalf("%s %s: %v", method, path, err)
	}
}

func expectStatus(t *testing.T, c *client.Client, method, path string, in any, code int) {
	t.Helper()
	err := c.Do(context.Background(), method, path, in, nil)
	var ce *client.Error
	if !errors.As(err, &ce) || ce.Status != code {
		t.Fatalf("%s %s: got %v, want HTTP %d", method, path, err, code)
	}
}

func taskDetail(t *testing.T, c *client.Client, id int64) api.TaskDetail {
	t.Helper()
	var d api.TaskDetail
	do(t, c, "GET", "/tasks/"+strconv.FormatInt(id, 10), nil, &d)
	return d
}

func mustGit(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
}

func writeFile(t *testing.T, root, rel, body string) {
	t.Helper()
	p := filepath.Join(root, rel)
	os.MkdirAll(filepath.Dir(p), 0o755)
	if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}
