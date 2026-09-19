package worker_test

// End-to-end V2 check: Hermes over MCP, confirm-code approvals, notifications, per-worker tokens.

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"aiempire/internal/api"
	"aiempire/internal/client"
	"aiempire/internal/controlplane"
	"aiempire/internal/mcp"
	"aiempire/internal/task"
	"aiempire/internal/worker"
)

func TestV2EndToEnd(t *testing.T) {
	dbURL := os.Getenv("EMPIRE_TEST_DATABASE_URL")
	if dbURL == "" {
		t.Skip("EMPIRE_TEST_DATABASE_URL not set")
	}
	ctx := context.Background()
	pool := freshDB(t, ctx, dbURL)
	tmp := t.TempDir()
	kdir := filepath.Join(tmp, "knowledge")
	writeFile(t, kdir, "global/principles.md", "Simple over clever.")

	// A fake ntfy that records what it was sent.
	var mu sync.Mutex
	var sent []string
	ntfy := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		mu.Lock()
		sent = append(sent, r.Header.Get("Title")+"\n"+string(body))
		mu.Unlock()
	}))
	defer ntfy.Close()

	srv, err := controlplane.New(pool, controlplane.Config{
		OwnerToken: "owner-t", WorkerToken: "worker-t", HermesToken: "hermes-t",
		NotifyURL: ntfy.URL, KnowledgeDir: kdir, StaleAfter: time.Minute,
	})
	if err != nil {
		t.Fatal(err)
	}
	notified := func() string {
		t.Helper()
		if err := srv.NotifyOnce(ctx); err != nil {
			t.Fatal(err)
		}
		mu.Lock()
		defer mu.Unlock()
		return strings.Join(sent, "\n---\n")
	}
	hs := httptest.NewServer(srv.Handler())
	defer hs.Close()
	owner := client.New(hs.URL, "owner-t")
	hermes := client.New(hs.URL, "hermes-t")
	shared := client.New(hs.URL, "worker-t")

	for _, slug := range []string{"demo", "other"} {
		do(t, owner, "POST", "/projects", api.CreateProject{Slug: slug, Name: strings.ToUpper(slug[:1]) + slug[1:] + " App", AutonomyLevel: "high"}, nil)
		do(t, owner, "POST", "/projects/"+slug+"/repositories", api.CreateRepository{
			Name: "app", RepoURL: newRemote(t, tmp, slug), Stack: "go", TestCommand: testCmd(),
		}, nil)
	}
	var other api.Task
	do(t, owner, "POST", "/tasks", api.CreateTask{Project: "other", Title: "not for the remote worker"}, &other)

	// Hermes operates, but only within the operator role; workers can't read the API.
	expectStatus(t, hermes, "POST", "/projects", api.CreateProject{Slug: "x", Name: "x"}, 403)
	expectStatus(t, shared, "GET", "/tasks", nil, 403)

	// M2.1/M2.2: MCP tools, project resolved by name.
	m := &mcp.Server{C: hermes}
	if out := rpc(t, m, "initialize", `{"protocolVersion":"2025-06-18"}`); !strings.Contains(out, "Hermes") {
		t.Fatalf("initialize = %s", out)
	}
	if out := rpc(t, m, "tools/list", `{}`); !strings.Contains(out, "decide_approval") {
		t.Fatalf("tools/list = %s", out)
	}
	if out := call(t, m, "create_task", `{"project":"demo app","title":"add /health endpoint"}`); !strings.Contains(out, `"id": 2`) {
		t.Fatalf("create_task = %s", out)
	}
	var audit []api.AuditEntry
	do(t, owner, "GET", "/audit?target=task:2", nil, &audit)
	if len(audit) == 0 || audit[len(audit)-1].ActorType != "hermes" {
		t.Fatalf("task.create audit = %+v", audit)
	}

	// M2.4: a per-worker token limited to one project.
	var tok api.WorkerToken
	do(t, owner, "POST", "/workers", api.CreateWorker{Name: "remote", Projects: []string{"demo"}}, &tok)
	expectStatus(t, shared, "POST", "/workers/register", api.RegisterWorker{Name: "remote"}, 403) // no hijacking
	remote := client.New(hs.URL, tok.Token)
	expectStatus(t, remote, "POST", "/workers/register", api.RegisterWorker{Name: "someone-else"}, 403)
	w, err := worker.New(remote, worker.Config{
		Name: "remote", Capabilities: []string{"go"}, Workspaces: filepath.Join(tmp, "ws"), Agent: worker.Fake{},
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := w.Register(ctx); err != nil {
		t.Fatal(err)
	}
	shared.WorkerID = tok.Worker.ID
	expectStatus(t, shared, "POST", "/workers/"+strconv.FormatInt(tok.Worker.ID, 10)+"/claim", nil, 403) // shared token can't pose as it

	step(t, w, true) // the demo task (id 2), not the older task of "other"
	d := taskDetail(t, owner, 2)
	if d.Task.Status != task.WaitingForHuman || len(d.Runs) != 1 || !strings.Contains(d.Runs[0].LogTail, "fake agent wrote") {
		t.Fatalf("after implement: %+v", d)
	}
	step(t, w, false)
	if s := call(t, m, "status", `{}`); !strings.Contains(s, "approval #1") || !strings.Contains(s, "remote online") {
		t.Fatalf("status = %s", s)
	}

	// M2.3: the approval pings the owner with the confirm code; Hermes needs it to relay a decision.
	msgs := notified()
	code := regexp.MustCompile(`approve 1, code ([A-Z2-7]{8})`).FindStringSubmatch(msgs)
	if code == nil {
		t.Fatalf("notifications = %q", msgs)
	}
	if out := call(t, m, "decide_approval", `{"id":1,"decision":"approve","confirm_code":""}`); !strings.Contains(out, "confirm code") {
		t.Fatalf("decide without code = %s", out)
	}
	if out := call(t, m, "decide_approval", `{"id":1,"decision":"approve","confirm_code":"AAAAAAAA"}`); !strings.Contains(out, "confirm code") {
		t.Fatalf("decide with wrong code = %s", out)
	}
	if out := call(t, m, "decide_approval", `{"id":1,"decision":"approve","confirm_code":"`+strings.ToLower(code[1])+`"}`); out != "ok" {
		t.Fatalf("decide = %s", out)
	}
	do(t, owner, "GET", "/audit?target=approval:1", nil, &audit)
	if audit[0].Action != "approval.decide" || audit[0].ActorType != "hermes" || audit[0].ActorID != "owner via hermes" {
		t.Fatalf("decision audit = %+v", audit[0])
	}

	step(t, w, true) // merge
	if d := taskDetail(t, owner, 2); d.Task.Status != task.Completed {
		t.Fatalf("after merge: %s (%s)", d.Task.Status, d.Task.LastError)
	}
	if msgs := notified(); !strings.Contains(msgs, "Task #2 completed") {
		t.Fatalf("notifications = %q", msgs)
	}
	var unsent int
	pool.QueryRow(ctx, `SELECT count(*) FROM notifications WHERE sent_at IS NULL`).Scan(&unsent)
	if unsent != 0 {
		t.Fatalf("%d notifications unsent", unsent)
	}
}

func rpc(t *testing.T, m *mcp.Server, method, params string) string {
	t.Helper()
	out := m.Handle(context.Background(), []byte(`{"jsonrpc":"2.0","id":1,"method":"`+method+`","params":`+params+`}`))
	return string(out)
}

// call runs an MCP tool and returns its text (tool errors included).
func call(t *testing.T, m *mcp.Server, name, arguments string) string {
	t.Helper()
	var res struct {
		Result struct {
			Content []struct{ Text string }
		}
		Error *struct{ Message string }
	}
	raw := rpc(t, m, "tools/call", `{"name":"`+name+`","arguments":`+arguments+`}`)
	if err := json.Unmarshal([]byte(raw), &res); err != nil || res.Error != nil || len(res.Result.Content) != 1 {
		t.Fatalf("%s: %s", name, raw)
	}
	return res.Result.Content[0].Text
}
