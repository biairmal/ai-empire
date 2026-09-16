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

	origin := newRemote(t, tmp, "origin")

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

	var proj api.Project
	do(t, owner, "POST", "/projects", api.CreateProject{Slug: "demo", Name: "Demo", AutonomyLevel: "high"}, &proj)
	// No repository yet → tasks are refused.
	expectStatus(t, owner, "POST", "/tasks", api.CreateTask{Project: "demo", Title: "x"}, 400)
	do(t, owner, "POST", "/projects/demo/repositories", api.CreateRepository{
		Name: "app", RepoURL: origin, Stack: "go", TestCommand: testCmd(),
	}, nil)

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

// M1.8: clients, multi-repo projects, cross-project dependencies (spec §11A, §11B).
func TestClientsAndRepositories(t *testing.T) {
	dbURL := os.Getenv("EMPIRE_TEST_DATABASE_URL")
	if dbURL == "" {
		t.Skip("EMPIRE_TEST_DATABASE_URL not set")
	}
	ctx := context.Background()
	pool := freshDB(t, ctx, dbURL)
	tmp := t.TempDir()
	sdkGit, beGit, feGit, otherGit := newRemote(t, tmp, "sdk"), newRemote(t, tmp, "be"), newRemote(t, tmp, "fe"), newRemote(t, tmp, "other")

	kdir := filepath.Join(tmp, "knowledge")
	writeFile(t, kdir, "global/principles.md", "GLOBAL")
	writeFile(t, kdir, "stacks/go/go.md", "GO-STACK")
	writeFile(t, kdir, "stacks/node/node.md", "NODE-STACK")
	writeFile(t, kdir, "clients/acme/conventions.md", "ACME-CLIENT")
	writeFile(t, kdir, "clients/globex/conventions.md", "GLOBEX-CLIENT")
	writeFile(t, kdir, "projects/guest/requirements/prd.md", "GUEST-PRD")

	srv, err := controlplane.New(pool, controlplane.Config{
		OwnerToken: "owner-t", WorkerToken: "worker-t", KnowledgeDir: kdir, StaleAfter: time.Minute,
	})
	if err != nil {
		t.Fatal(err)
	}
	hs := httptest.NewServer(srv.Handler())
	defer hs.Close()
	owner := client.New(hs.URL, "owner-t")

	do(t, owner, "POST", "/clients", api.Client{Slug: "acme", Name: "Acme", DefaultAutonomyLevel: "medium"}, nil)
	do(t, owner, "POST", "/clients", api.Client{Slug: "globex", Name: "Globex"}, nil)
	expectStatus(t, owner, "POST", "/clients", api.Client{Slug: "123", Name: "digits only"}, 400)

	var guest api.Project
	do(t, owner, "POST", "/projects", api.CreateProject{Slug: "guest", Name: "Guest", Client: "acme"}, &guest)
	if guest.AutonomyLevel != "medium" || guest.ClientID == nil {
		t.Fatalf("guest = %+v, want acme's default autonomy", guest)
	}
	do(t, owner, "POST", "/projects", api.CreateProject{Slug: "sdk", Name: "SDK", Client: "acme"}, nil)
	do(t, owner, "POST", "/projects", api.CreateProject{Slug: "other", Name: "Other", Client: "globex"}, nil)
	do(t, owner, "POST", "/projects", api.CreateProject{Slug: "mine", Name: "Mine"}, nil)
	expectStatus(t, owner, "POST", "/projects", api.CreateProject{Slug: "x", Name: "X", Client: "nobody"}, 400)

	repo := func(project, name, url, stack string) {
		t.Helper()
		do(t, owner, "POST", "/projects/"+project+"/repositories", api.CreateRepository{
			Name: name, RepoURL: url, Stack: stack, TestCommand: testCmd(),
		}, nil)
	}
	repo("guest", "backend", beGit, "go")
	repo("guest", "frontend", feGit, "node")
	repo("sdk", "sdk", sdkGit, "go")
	repo("other", "other", otherGit, "go")
	repo("mine", "mine", otherGit, "go")
	expectStatus(t, owner, "POST", "/projects/guest/repositories", api.CreateRepository{Name: "backend", RepoURL: "x", Stack: "go"}, 409)

	// Repository choice.
	expectStatus(t, owner, "POST", "/tasks", api.CreateTask{Project: "guest", Title: "which repo?"}, 400)
	expectStatus(t, owner, "POST", "/tasks", api.CreateTask{Project: "guest", Repository: "nope", Title: "x"}, 400)

	// Chain across projects within one client: sdk → backend → frontend.
	var s1, b1, f1 api.Task
	do(t, owner, "POST", "/tasks", api.CreateTask{Project: "sdk", Title: "QR token helper"}, &s1)
	do(t, owner, "POST", "/tasks", api.CreateTask{Project: "guest", Repository: "backend", Title: "QR endpoint", DependsOn: []int64{s1.ID}}, &b1)
	do(t, owner, "POST", "/tasks", api.CreateTask{
		Project: "guest", Repository: "frontend", Title: "QR scanner", DependsOn: []int64{b1.ID},
		ContextDocs: []string{"requirements/prd.md"},
	}, &f1)
	if f1.RequiredCapabilities[0] != "node" {
		t.Errorf("frontend caps = %v, want repository stack", f1.RequiredCapabilities)
	}
	// Never across clients, and never from a client project into a personal one.
	expectStatus(t, owner, "POST", "/tasks", api.CreateTask{Project: "other", Title: "x", DependsOn: []int64{s1.ID}}, 400)
	expectStatus(t, owner, "POST", "/tasks", api.CreateTask{Project: "mine", Title: "x", DependsOn: []int64{s1.ID}}, 400)

	w, err := worker.New(client.New(hs.URL, "worker-t"), worker.Config{
		Name: "w", Capabilities: []string{"go", "node"}, Workspaces: filepath.Join(tmp, "ws"), Agent: worker.Fake{},
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := w.Register(ctx); err != nil {
		t.Fatal(err)
	}

	for _, tk := range []struct {
		task    api.Task
		gateFor string
	}{{s1, "sdk:main"}, {b1, "backend:main"}, {f1, "frontend:main"}} {
		step(t, w, true)  // implement → gate
		step(t, w, false) // dependents stay blocked while this one waits
		var pending []api.ApprovalRequest
		do(t, owner, "GET", "/approvals?status=PENDING_APPROVAL", nil, &pending)
		if len(pending) != 1 || pending[0].SubjectRef != strconv.FormatInt(tk.task.ID, 10) || !strings.Contains(pending[0].Summary, tk.gateFor) {
			t.Fatalf("pending approvals = %+v, want one for task %d into %s", pending, tk.task.ID, tk.gateFor)
		}
		// M1.9: the human sees what the agent says it did.
		if !strings.Contains(pending[0].Summary, "Agent summary:\nfake agent wrote:") {
			t.Errorf("approval summary lacks agent summary:\n%s", pending[0].Summary)
		}
		if runs := taskDetail(t, owner, tk.task.ID).Runs; len(runs) != 1 || !strings.HasPrefix(runs[0].Summary, "fake agent wrote:") {
			t.Errorf("run summary = %+v", runs)
		}
		do(t, owner, "POST", "/approvals/"+strconv.FormatInt(pending[0].ID, 10)+"/approve", api.Decision{}, nil)
		step(t, w, true) // merge
		if s := taskDetail(t, owner, tk.task.ID).Task.Status; s != task.Completed {
			t.Fatalf("task %d: %s", tk.task.ID, s)
		}
	}

	// Each change landed only in its own repository.
	for _, remote := range []string{sdkGit, beGit, feGit} {
		out, err := exec.Command("git", "--git-dir", remote, "show", "main:empire-fake-agent.txt").Output()
		if err != nil || strings.Count(string(out), "\n") != 1 {
			t.Errorf("%s main = %q, %v", remote, out, err)
		}
	}
	if _, err := os.Stat(filepath.Join(tmp, "ws", "guest", "backend", "_base")); err != nil {
		t.Errorf("workspace layout: %v", err)
	}

	// Frontend context: node stack + acme + its doc; never Go or another client.
	runs := taskDetail(t, owner, f1.ID).Runs
	want := []string{"global/principles.md", "stacks/node/node.md", "clients/acme/conventions.md", "projects/guest/requirements/prd.md"}
	if len(runs) != 1 || !slices.Equal(runs[0].ContextFiles, want) {
		t.Errorf("frontend context = %v, want %v", runs, want)
	}
	backendFiles := taskDetail(t, owner, b1.ID).Runs[0].ContextFiles
	if !slices.Contains(backendFiles, "stacks/go/go.md") || slices.Contains(backendFiles, "stacks/node/node.md") {
		t.Errorf("backend context = %v", backendFiles)
	}

	// Moving projects between clients must not split a dependency chain.
	expectStatus(t, owner, "POST", "/projects/sdk/client", api.MoveProject{Client: "globex"}, 409)
	expectStatus(t, owner, "POST", "/projects/guest/client", api.MoveProject{}, 409)
	var moved api.Project
	do(t, owner, "POST", "/projects/other/client", api.MoveProject{Client: "acme"}, &moved)
	if moved.ClientID == nil || *moved.ClientID != *guest.ClientID || moved.AutonomyLevel != "conservative" {
		t.Errorf("moved = %+v", moved)
	}
	do(t, owner, "POST", "/projects/mine/client", api.MoveProject{Client: "globex"}, nil)
	var entries []api.AuditEntry
	do(t, owner, "GET", "/audit", nil, &entries)
	moves := 0
	for _, e := range entries {
		if e.Action == "project.move_client" {
			moves++
		}
	}
	if moves != 2 {
		t.Errorf("project.move_client audit rows = %d, want 2", moves)
	}

	// M1.9: edit projects and repositories, audited with before/after.
	str := func(s string) *string { return &s }
	var p api.Project
	do(t, owner, "PATCH", "/projects/other", api.UpdateProject{AutonomyLevel: str("medium")}, &p)
	if p.AutonomyLevel != "medium" || p.Name != "Other" {
		t.Errorf("project after update = %+v", p)
	}
	expectStatus(t, owner, "PATCH", "/projects/other", api.UpdateProject{AutonomyLevel: str("reckless")}, 400)
	expectStatus(t, owner, "PATCH", "/projects/other", api.UpdateProject{Name: str("")}, 400)

	var rp api.Repository
	do(t, owner, "PATCH", "/projects/guest/repositories/backend", api.UpdateRepository{TestCommand: str("")}, &rp)
	if rp.TestCommand != "" || rp.Stack != "go" || rp.RepoURL != beGit {
		t.Errorf("repository after update = %+v", rp)
	}
	do(t, owner, "PATCH", "/projects/guest/repositories/backend", api.UpdateRepository{DefaultBranch: str("develop")}, &rp)
	if rp.DefaultBranch != "develop" || rp.TestCommand != "" {
		t.Errorf("repository after second update = %+v", rp)
	}
	expectStatus(t, owner, "PATCH", "/projects/guest/repositories/backend", api.UpdateRepository{Stack: str("")}, 400)
	expectStatus(t, owner, "PATCH", "/projects/guest/repositories/nope", api.UpdateRepository{Stack: str("go")}, 404)
	expectStatus(t, client.New(hs.URL, "worker-t"), "PATCH", "/projects/guest", api.UpdateProject{AutonomyLevel: str("high")}, 403)

	entries = nil
	do(t, owner, "GET", "/audit?target=project:"+strconv.FormatInt(guest.ID, 10), nil, &entries)
	updates := 0
	for _, e := range entries {
		if e.Action == "repository.update" && e.Payload["before"] != nil && e.Payload["after"] != nil {
			updates++
		}
	}
	if updates != 2 {
		t.Errorf("repository.update audit rows = %d, want 2", updates)
	}
}

func newRemote(t *testing.T, dir, name string) string {
	t.Helper()
	origin := filepath.Join(dir, name+".git")
	mustGit(t, dir, "init", "--bare", "-b", "main", origin)
	seed := filepath.Join(dir, name+"-seed")
	mustGit(t, dir, "clone", origin, seed)
	os.WriteFile(filepath.Join(seed, "README.md"), []byte(name+"\n"), 0o644)
	mustGit(t, seed, "add", ".")
	mustGit(t, seed, "-c", "user.name=t", "-c", "user.email=t@t", "commit", "-m", "init")
	mustGit(t, seed, "push", "origin", "HEAD:main")
	return origin
}

func testCmd() string {
	if runtime.GOOS == "windows" {
		return "type empire-fake-agent.txt"
	}
	return "cat empire-fake-agent.txt"
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
