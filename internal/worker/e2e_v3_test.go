package worker_test

// End-to-end V3 check: workflows, documents, approvals, versioning, review,
// traceability, impact analysis and export — control plane + worker + real
// Postgres + real git, with the fake agent.

import (
	"context"
	"io/fs"
	"net/http/httptest"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"

	"aiempire/internal/api"
	"aiempire/internal/client"
	"aiempire/internal/controlplane"
	"aiempire/internal/docs"
	"aiempire/internal/task"
	"aiempire/internal/worker"
)

type v3 struct {
	t     *testing.T
	owner *client.Client
	w     *worker.Worker
	kdir  string
	tmp   string
}

func setupV3(t *testing.T) *v3 { return setupV3With(t, "../../workflows") }

func setupV3With(t *testing.T, workflows string) *v3 {
	dbURL := os.Getenv("EMPIRE_TEST_DATABASE_URL")
	if dbURL == "" {
		t.Skip("EMPIRE_TEST_DATABASE_URL not set")
	}
	ctx := context.Background()
	pool := freshDB(t, ctx, dbURL)
	tmp := t.TempDir()

	// The real contracts, templates, roles and global rules.
	kdir := filepath.Join(tmp, "knowledge")
	for _, sub := range []string{"contracts", "templates", "roles", "global"} {
		copyDir(t, filepath.Join("../../knowledge", sub), filepath.Join(kdir, sub))
	}
	writeFile(t, kdir, "stacks/go/go.md", "GO-STACK")
	writeFile(t, kdir, "stacks/node/node.md", "NODE-STACK")

	srv, err := controlplane.New(pool, controlplane.Config{
		OwnerToken: "owner-t", WorkerToken: "worker-t", KnowledgeDir: kdir,
		WorkflowsDir: workflows, StaleAfter: time.Minute,
	})
	if err != nil {
		t.Fatal(err)
	}
	hs := httptest.NewServer(srv.Handler())
	t.Cleanup(hs.Close)
	s := &v3{t: t, owner: client.New(hs.URL, "owner-t"), kdir: kdir, tmp: tmp}

	do(t, s.owner, "POST", "/projects", api.CreateProject{Slug: "guest", Name: "Guest Management", AutonomyLevel: "high"}, nil)
	for _, r := range []struct{ name, stack string }{{"backend", "go"}, {"frontend", "node"}} {
		do(t, s.owner, "POST", "/projects/guest/repositories", api.CreateRepository{
			Name: r.name, RepoURL: newRemote(t, tmp, r.name), Stack: r.stack, TestCommand: testCmd(),
		}, nil)
	}
	s.w, err = worker.New(client.New(hs.URL, "worker-t"), worker.Config{
		Name: "w", Capabilities: []string{"go", "node"}, Workspaces: filepath.Join(tmp, "ws"), Agent: worker.Fake{},
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := s.w.Register(ctx); err != nil {
		t.Fatal(err)
	}
	return s
}

func copyDir(t *testing.T, from, to string) {
	t.Helper()
	err := filepath.WalkDir(from, func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		rel, _ := filepath.Rel(from, p)
		data, err := os.ReadFile(p)
		if err != nil {
			return err
		}
		os.MkdirAll(filepath.Join(to, filepath.Dir(rel)), 0o755)
		return os.WriteFile(filepath.Join(to, rel), data, 0o644)
	})
	if err != nil {
		t.Fatal(err)
	}
}

func (s *v3) request(id int64) api.RequestDetail {
	s.t.Helper()
	var d api.RequestDetail
	do(s.t, s.owner, "GET", "/requests/"+strconv.FormatInt(id, 10), nil, &d)
	return d
}

// pending returns the single open approval, failing otherwise.
func (s *v3) pending(wantIn ...string) api.ApprovalRequest {
	s.t.Helper()
	var ps []api.ApprovalRequest
	do(s.t, s.owner, "GET", "/approvals?status=PENDING_APPROVAL", nil, &ps)
	if len(ps) != 1 {
		s.t.Fatalf("want 1 pending approval, got %d: %+v", len(ps), ps)
	}
	for _, w := range wantIn {
		if !strings.Contains(ps[0].Summary, w) {
			s.t.Fatalf("approval summary lacks %q:\n%s", w, ps[0].Summary)
		}
	}
	return ps[0]
}

func (s *v3) decide(id int64, decision, comment string) {
	s.t.Helper()
	do(s.t, s.owner, "POST", "/approvals/"+strconv.FormatInt(id, 10)+"/"+decision, api.Decision{Comment: comment}, nil)
}

func (s *v3) doc(rel string) *docs.Doc {
	s.t.Helper()
	repo, err := docs.Load(s.kdir)
	if err != nil {
		s.t.Fatal(err)
	}
	for _, d := range repo.Docs {
		if d.Path == rel || strings.HasPrefix(d.Path, rel) {
			return d
		}
	}
	s.t.Fatalf("no document %s", rel)
	return nil
}

func (s *v3) stepExpect(want string) api.Task {
	s.t.Helper()
	step(s.t, s.w, true)
	return api.Task{Status: want}
}

func TestFeatureWorkflow(t *testing.T) {
	s := setupV3(t)
	var req api.Request
	do(t, s.owner, "POST", "/requests", api.CreateRequest{
		Project: "guest", Title: "QR ticket validation [rework]",
		Description: "Guests show a QR code at the door; staff scan it to check them in.",
	}, &req)
	if req.CurrentStep != "prd" || req.Workflow != "feature" {
		t.Fatalf("request = %+v", req)
	}
	// quick-fix needs a repository when the project has several.
	expectStatus(t, s.owner, "POST", "/requests", api.CreateRequest{Project: "guest", Workflow: "quick-fix", Title: "x"}, 400)

	// PRD: written, sent back once, rewritten, approved.
	step(t, s.w, true)
	a := s.pending("Approve Product Requirements Document PRD-001 v1", "knowledge/projects/guest/requirements/PRD-001-")
	prd := s.doc("projects/guest/requirements/PRD-001-")
	if prd.Status != docs.PendingApproval || prd.Str("project") != "guest" || prd.Version != 1 {
		t.Fatalf("PRD front matter: %s v%d %s", prd.Status, prd.Version, prd.Str("project"))
	}
	s.decide(a.ID, "request-changes", "Add offline scanning to the goals.")
	if st := s.doc(prd.Path).Status; st != docs.ChangesRequested {
		t.Errorf("PRD status after request-changes = %s", st)
	}
	step(t, s.w, true)
	a = s.pending("PRD-001 v1")
	if !strings.Contains(s.doc(prd.Path).Body, "Revised by the fake agent") {
		t.Error("rework did not start from the previous draft")
	}
	s.decide(a.ID, "approve", "")
	if st := s.doc(prd.Path).Status; st != docs.Approved {
		t.Fatalf("PRD status after approval = %s", st)
	}

	// Design: technical design + ADR, approved together.
	step(t, s.w, true)
	a = s.pending("Approve Technical Design TD-001 v1", "Also approves ADR-001 v1")
	td := s.doc("projects/guest/architecture/TD-001-")
	if !slices.Equal(td.Rels["satisfies"], []string{"PRD-001"}) || !slices.Contains(td.Rels["depends_on"], "ADR-001") {
		t.Errorf("design relationships = %v", td.Rels)
	}
	if !strings.Contains(td.Body, "[[PRD-001-") {
		t.Error("design lacks the Obsidian relations block")
	}
	d := s.request(req.ID)
	designTask := d.Tasks[len(d.Tasks)-1]
	var detail api.TaskDetail
	do(t, s.owner, "GET", "/tasks/"+strconv.FormatInt(designTask.ID, 10), nil, &detail)
	files := detail.Runs[0].ContextFiles
	for _, want := range []string{"roles/architect.md", "stacks/go/go.md", "stacks/node/node.md", prd.Path} {
		if !slices.Contains(files, want) {
			t.Errorf("design context lacks %s: %v", want, files)
		}
	}
	if detail.Runs[0].Role != "architect" {
		t.Errorf("run role = %s", detail.Runs[0].Role)
	}
	s.decide(a.ID, "approve", "")

	// Plan: approved → one code task per repository, frontend after backend.
	step(t, s.w, true)
	a = s.pending("Approve Implementation Plan PLAN-001 v1")
	plan := s.doc("projects/guest/planning/PLAN-001-")
	if !slices.Contains(plan.Rels["derived_from"], "TD-001") {
		t.Errorf("plan relationships = %v", plan.Rels)
	}
	s.decide(a.ID, "approve", "")
	d = s.request(req.ID)
	var code []api.Task
	for _, tk := range d.Tasks {
		if tk.Kind == "code" {
			code = append(code, tk)
		}
	}
	if len(code) != 2 || d.Request.CurrentStep != "implement" || !code[0].Review {
		t.Fatalf("code tasks = %+v, step %s", code, d.Request.CurrentStep)
	}
	if !slices.Contains(code[0].ContextDocs, "planning/"+filepath.Base(plan.Path)) {
		t.Errorf("code task context = %v", code[0].ContextDocs)
	}

	// Each code task: implement → AI review asks for changes once → implement → review OK → merge gate → merge.
	for _, ct := range code {
		step(t, s.w, true) // review requests changes → back to PENDING
		var td api.TaskDetail
		do(t, s.owner, "GET", "/tasks/"+strconv.FormatInt(ct.ID, 10), nil, &td)
		if td.Task.Status != task.Pending || td.Task.ReviewRounds != 1 || !strings.Contains(td.Task.Feedback, "AI code review") {
			t.Fatalf("after first review: %+v", td.Task)
		}
		step(t, s.w, true) // implement again → review approves → merge gate
		a = s.pending("AI review: APPROVE", "Agent summary:")
		s.decide(a.ID, "approve", "")
		step(t, s.w, true) // merge
		do(t, s.owner, "GET", "/tasks/"+strconv.FormatInt(ct.ID, 10), nil, &td)
		if td.Task.Status != task.Completed || len(td.Task.MergeSHA) != 40 {
			t.Fatalf("after merge: %+v", td.Task)
		}
		roles := map[string]int{}
		for _, r := range td.Runs {
			roles[r.Role]++
		}
		if roles["developer"] != 2 || roles["reviewer"] != 2 {
			t.Errorf("runs by role = %v", roles)
		}
	}
	d = s.request(req.ID)
	if d.Request.Status != "completed" {
		t.Fatalf("request = %+v", d.Request)
	}

	// Traceability: why does the backend commit exist?
	var backend api.TaskDetail
	do(t, s.owner, "GET", "/tasks/"+strconv.FormatInt(code[0].ID, 10), nil, &backend)
	var tr api.Trace
	do(t, s.owner, "GET", "/trace?ref=commit:"+backend.Task.MergeSHA[:10], nil, &tr)
	why := flatten(tr.Why)
	for _, want := range []string{"PLAN-001", "TD-001", "PRD-001", "QR ticket validation"} {
		if !strings.Contains(why, want) {
			t.Errorf("trace of the commit lacks %s:\n%s", want, why)
		}
	}
	if !slices.Equal(backend.Task.ChangedFiles, []string{"empire-fake-agent.txt"}) {
		t.Errorf("changed files = %v", backend.Task.ChangedFiles)
	}
	if eff := flatten(tr.Effects); !strings.Contains(eff, "file empire-fake-agent.txt [changed]") {
		t.Errorf("commit effects lack the changed file:\n%s", eff)
	}
	// …and what depends on the PRD?
	do(t, s.owner, "GET", "/trace?ref=PRD-001&project=guest", nil, &tr)
	if eff := flatten(tr.Effects); !strings.Contains(eff, "TD-001") || !strings.Contains(eff, "merged as") {
		t.Errorf("PRD effects:\n%s", eff)
	}

	// Impact analysis.
	var imp api.Impact
	do(t, s.owner, "GET", "/impact?doc=PRD-001&project=guest", nil, &imp)
	for _, want := range []string{"TD-001", "ADR-001", "PLAN-001", "task #", "Code files that may need to change:", "- backend: empire-fake-agent.txt", "- frontend: empire-fake-agent.txt"} {
		if !strings.Contains(imp.Report, want) {
			t.Errorf("impact report lacks %s:\n%s", want, imp.Report)
		}
	}

	// Everything the workflow produced validates, and the graph index is filled.
	var probs []api.Problem
	do(t, s.owner, "POST", "/documents/validate", map[string]string{"project": "guest"}, &probs)
	if len(probs) > 0 {
		t.Errorf("project documents should validate: %+v", probs)
	}

	// Client export: approved documents, no front matter, a document-control table.
	var files2 []api.File
	do(t, s.owner, "GET", "/documents/export?project=guest", nil, &files2)
	if len(files2) != 5 || files2[0].Name != "README.md" {
		t.Fatalf("export = %d files", len(files2))
	}
	for _, f := range files2[1:] {
		if strings.HasPrefix(f.Content, "---") || strings.Contains(f.Content, "<!--") || !strings.Contains(f.Content, "## Document Control") {
			t.Errorf("%s is not client-ready:\n%.300s", f.Name, f.Content)
		}
	}

	// A rejected PRD cancels its request.
	do(t, s.owner, "POST", "/requests", api.CreateRequest{Project: "guest", Title: "Loyalty points"}, &req)
	step(t, s.w, true)
	s.decide(s.pending("PRD-002").ID, "reject", "Not this quarter.")
	if st := s.request(req.ID).Request.Status; st != "cancelled" {
		t.Errorf("request after rejected PRD = %s", st)
	}
	if st := s.doc("projects/guest/requirements/PRD-002-").Status; st != docs.Rejected {
		t.Errorf("rejected PRD status = %s", st)
	}

	// Cancelling a request withdraws its unapproved documents.
	do(t, s.owner, "POST", "/requests", api.CreateRequest{Project: "guest", Title: "Seat maps"}, &req)
	step(t, s.w, true)
	s.pending("PRD-003")
	do(t, s.owner, "POST", "/requests/"+strconv.FormatInt(req.ID, 10)+"/cancel", nil, nil)
	if st := s.doc("projects/guest/requirements/PRD-003-").Status; st != docs.Rejected {
		t.Errorf("withdrawn PRD status = %s", st)
	}
	var open []api.ApprovalRequest
	do(t, s.owner, "GET", "/approvals?status=PENDING_APPROVAL", nil, &open)
	if len(open) != 0 {
		t.Errorf("approvals left open after cancel: %+v", open)
	}

	// Quick fix goes straight to code in the named repository.
	do(t, s.owner, "POST", "/requests", api.CreateRequest{Project: "guest", Workflow: "quick-fix", Repository: "frontend", Title: "Fix typo"}, &req)
	d = s.request(req.ID)
	if len(d.Tasks) != 1 || d.Tasks[0].Kind != "code" || d.Tasks[0].RepositoryID == nil || d.Tasks[0].RequiredCapabilities[0] != "node" {
		t.Fatalf("quick-fix tasks = %+v", d.Tasks)
	}
	do(t, s.owner, "POST", "/requests/"+strconv.FormatInt(req.ID, 10)+"/cancel", nil, nil)
	if d = s.request(req.ID); d.Request.Status != "cancelled" || d.Tasks[0].Status != task.Cancelled {
		t.Errorf("cancelled quick-fix = %+v", d)
	}
}

func TestChangeWorkflowAndOwnerDocuments(t *testing.T) {
	s := setupV3(t)
	// Approve a PRD and a design first (feature workflow up to the plan).
	var req api.Request
	do(t, s.owner, "POST", "/requests", api.CreateRequest{Project: "guest", Title: "Check-in"}, &req)
	for range 2 {
		step(t, s.w, true)
		s.decide(s.pending().ID, "approve", "")
	}
	do(t, s.owner, "POST", "/requests/"+strconv.FormatInt(req.ID, 10)+"/cancel", nil, nil)
	prd := s.doc("projects/guest/requirements/PRD-001-")
	prdFile := filepath.Join(s.kdir, filepath.FromSlash(prd.Path))
	approved, _ := os.ReadFile(prdFile)

	// Editing an approved document in place is caught.
	os.WriteFile(prdFile, append(approved, []byte("\nA silent edit.\n")...), 0o644)
	var probs []api.Problem
	do(t, s.owner, "POST", "/documents/validate", map[string]string{"project": "guest"}, &probs)
	if !strings.Contains(problemsText(probs), "approved version 1 was modified in place") {
		t.Fatalf("in-place edit not detected: %+v", probs)
	}
	expectStatus(t, s.owner, "POST", "/documents/submit", api.DocumentRef{Ref: "PRD-001", Project: "guest"}, 422)
	os.WriteFile(prdFile, approved, 0o644)

	// Change request → impact analysis at the gate → PRD v2 → approval.
	do(t, s.owner, "POST", "/requests", api.CreateRequest{
		Project: "guest", Workflow: "change", Title: "Offline scanning",
		Description: "Scanners must work without network.\naffects: PRD-001",
	}, &req)
	step(t, s.w, true)
	a := s.pending("Approve Change Request CR-001 v1", "Impact analysis for PRD-001", "TD-001")
	s.decide(a.ID, "approve", "")

	step(t, s.w, true) // revise PRD-001 → v2
	a = s.pending("PRD-001 v2")
	prd = s.doc(prd.Path)
	if prd.Version != 2 || !slices.Contains(prd.Rels["derived_from"], "CR-001") {
		t.Fatalf("revised PRD: v%d %v", prd.Version, prd.Rels)
	}
	// The revise task was given only the change request; the graph added the PRD it affects.
	var rd api.TaskDetail
	do(t, s.owner, "GET", "/tasks/"+strconv.FormatInt(*a.TaskID, 10), nil, &rd)
	if files := rd.Runs[0].ContextFiles; !slices.Contains(files, prd.Path) || !slices.ContainsFunc(files, func(f string) bool { return strings.Contains(f, "CR-001") }) {
		t.Errorf("revise context = %v", files)
	}
	s.decide(a.ID, "approve", "")
	var ds []api.DocumentInfo
	do(t, s.owner, "GET", "/documents?project=guest&type=prd", nil, &ds)
	if len(ds) != 1 || ds[0].Version != 2 || !ds[0].Approved {
		t.Errorf("PRD after revision = %+v", ds)
	}

	step(t, s.w, true) // plan, derived from the change request
	s.decide(s.pending("PLAN-001").ID, "approve", "")
	d := s.request(req.ID)
	if d.Request.CurrentStep != "implement" || len(d.Tasks) != 5 { // CR, revise, plan, 2 code
		t.Fatalf("change request after plan: step %s, %d tasks", d.Request.CurrentStep, len(d.Tasks))
	}

	// A human-written document: scaffold, fill in, validate, submit, self-approve.
	var info api.DocumentInfo
	do(t, s.owner, "POST", "/documents", api.NewDocument{Project: "guest", Type: "test-plan", Title: "Check-in acceptance", Owner: "Bia, QA"}, &info)
	if info.ID != "TP-001" || !strings.HasPrefix(info.Path, "projects/guest/testing/TP-001-check-in-acceptance") {
		t.Fatalf("new document = %+v", info)
	}
	expectStatus(t, s.owner, "POST", "/documents/submit", api.DocumentRef{Ref: info.Path}, 422) // placeholders left
	tpFile := filepath.Join(s.kdir, filepath.FromSlash(info.Path))
	raw, _ := os.ReadFile(tpFile)
	filled, err := docs.Example(info.Path, raw)
	if err != nil {
		t.Fatal(err)
	}
	filled.Set("derived_from", []string{"PRD-001"})
	os.WriteFile(tpFile, filled.Bytes(), 0o644)
	var ap api.ApprovalRequest
	do(t, s.owner, "POST", "/documents/submit", api.DocumentRef{Ref: "knowledge/" + info.Path}, &ap)
	if ap.RequestedBy != "owner" || ap.TaskID != nil {
		t.Fatalf("owner submission = %+v", ap)
	}
	s.decide(ap.ID, "approve", "self-approved by the owner")
	if st := s.doc(info.Path).Status; st != docs.Approved {
		t.Errorf("owner document status = %s", st)
	}
	expectStatus(t, s.owner, "POST", "/documents/submit", api.DocumentRef{Ref: "TP-001", Project: "guest"}, 409) // already approved

	// Edits between submission and decision are refused.
	do(t, s.owner, "POST", "/documents", api.NewDocument{Project: "guest", Type: "runbook", Title: "Ops", Owner: "Bia"}, &info)
	rbFile := filepath.Join(s.kdir, filepath.FromSlash(info.Path))
	raw, _ = os.ReadFile(rbFile)
	filled, _ = docs.Example(info.Path, raw)
	os.WriteFile(rbFile, filled.Bytes(), 0o644)
	do(t, s.owner, "POST", "/documents/submit", api.DocumentRef{Ref: info.Path}, &ap)
	raw, _ = os.ReadFile(rbFile)
	os.WriteFile(rbFile, append(raw, []byte("\nlate edit\n")...), 0o644)
	expectStatus(t, s.owner, "POST", "/approvals/"+strconv.FormatInt(ap.ID, 10)+"/approve", api.Decision{}, 409)
	// ...and resubmitting replaces the stale approval, so the new content can be decided.
	stale := ap.ID
	do(t, s.owner, "POST", "/documents/submit", api.DocumentRef{Ref: info.Path}, &ap)
	if ap.ID == stale {
		t.Fatal("resubmission opened no new approval")
	}
	var old api.ApprovalRequest
	do(t, s.owner, "GET", "/approvals/"+strconv.FormatInt(stale, 10), nil, &old)
	if old.Status != "CHANGES_REQUESTED" {
		t.Errorf("stale approval status = %s", old.Status)
	}
	s.decide(ap.ID, "approve", "approved after resubmission")
	// Resubmitting unchanged content while it is pending is still a conflict.
	do(t, s.owner, "POST", "/documents", api.NewDocument{Project: "guest", Type: "runbook", Title: "Ops two", Owner: "Bia"}, &info)
	rbFile = filepath.Join(s.kdir, filepath.FromSlash(info.Path))
	raw, _ = os.ReadFile(rbFile)
	filled, _ = docs.Example(info.Path, raw)
	os.WriteFile(rbFile, filled.Bytes(), 0o644)
	do(t, s.owner, "POST", "/documents/submit", api.DocumentRef{Ref: info.Path}, &ap)
	expectStatus(t, s.owner, "POST", "/documents/submit", api.DocumentRef{Ref: info.Path}, 409)

	// Audit trail covers document lifecycles.
	var entries []api.AuditEntry
	do(t, s.owner, "GET", "/audit?"+url.Values{"target": {"document:projects/guest/PRD-001"}}.Encode(), nil, &entries)
	if len(entries) < 2 {
		t.Errorf("PRD audit entries = %d", len(entries))
	}
	_ = exec.Command
}

// The custom workflow shown in docs/workflows.md must actually run.
func TestDocumentedCustomWorkflow(t *testing.T) {
	guide, err := os.ReadFile("../../docs/workflows.md")
	if err != nil {
		t.Fatal(err)
	}
	_, rest, _ := strings.Cut(string(guide), "```yaml\n")
	example, _, _ := strings.Cut(rest, "```")
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "api-change.yaml"), []byte(example), 0o644)

	s := setupV3With(t, dir)
	var req api.Request
	do(t, s.owner, "POST", "/requests", api.CreateRequest{Project: "guest", Workflow: "api-change", Title: "Ticket lookup API"}, &req)
	for _, want := range []string{"PRD-001", "TD-001", "API-001", "PLAN-001"} {
		step(t, s.w, true)
		s.decide(s.pending(want).ID, "approve", "")
	}
	api1 := s.doc("projects/guest/api/API-001-")
	if !slices.Contains(api1.Rels["implements"], "TD-001") {
		t.Errorf("API spec relationships = %v", api1.Rels)
	}
	d := s.request(req.ID)
	if d.Request.CurrentStep != "implement" || len(d.Tasks) != 6 {
		t.Fatalf("after plan: step %s, %d tasks", d.Request.CurrentStep, len(d.Tasks))
	}
}

func flatten(ns []api.TraceNode) string {
	var b strings.Builder
	var walk func([]api.TraceNode)
	walk = func(ns []api.TraceNode) {
		for _, n := range ns {
			b.WriteString(n.Kind + " " + n.Title + " [" + n.Via + "]\n")
			walk(n.Children)
		}
	}
	walk(ns)
	return b.String()
}

func problemsText(ps []api.Problem) string {
	var b strings.Builder
	for _, p := range ps {
		b.WriteString(p.Path + ": " + p.Message + "\n")
	}
	return b.String()
}
