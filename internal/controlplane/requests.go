package controlplane

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"slices"
	"sort"
	"strconv"
	"strings"
	"time"

	"aiempire/internal/api"
	"aiempire/internal/docs"
	"aiempire/internal/task"
	"aiempire/internal/workflow"

	"github.com/jackc/pgx/v5"
)

// The workflow engine (spec §7): a request walks through its workflow's steps.
// Each step creates tasks; when all of a step's tasks are completed the next step
// starts. Human gates are ordinary approval requests on those tasks.

func (s *Server) listWorkflows(r *http.Request, a actor) (any, error) {
	names := slices.Sorted(func(yield func(string) bool) {
		for n := range s.workflows {
			if !yield(n) {
				return
			}
		}
	})
	out := make([]M, 0, len(names))
	for _, n := range names {
		wf := s.workflows[n]
		var steps []string
		for _, st := range wf.Steps {
			steps = append(steps, st.Name)
		}
		out = append(out, M{"name": n, "description": wf.Description, "steps": steps})
	}
	return out, nil
}

func (s *Server) createRequest(r *http.Request, a actor) (any, error) {
	var in api.CreateRequest
	if err := decode(r, &in); err != nil {
		return nil, err
	}
	if in.Workflow == "" {
		in.Workflow = "feature"
	}
	wf, ok := s.workflows[in.Workflow]
	if !ok {
		return nil, errf(http.StatusBadRequest, "unknown workflow %q", in.Workflow)
	}
	if strings.TrimSpace(in.Title) == "" {
		return nil, errf(http.StatusBadRequest, "title is required")
	}
	var out api.Request
	err := s.tx(r.Context(), func(tx pgx.Tx) error {
		ctx := r.Context()
		p, err := projectByRef(ctx, tx, in.Project)
		if err != nil {
			return errf(http.StatusBadRequest, "unknown project %q", in.Project)
		}
		var repoID *int64
		if in.Repository != "" || wf.StartsWithCode() {
			repo, err := pickRepository(ctx, tx, p, in.Repository)
			if err != nil {
				return err
			}
			repoID = &repo.ID
		}
		out, err = one[api.Request](ctx, tx, `
			INSERT INTO requests (project_id, repository_id, workflow, title, description)
			VALUES ($1, $2, $3, $4, $5) RETURNING *`, p.ID, repoID, wf.Name, in.Title, in.Description)
		if err != nil {
			return err
		}
		if err := audit(ctx, tx, a, "request.create", requestRef(out.ID), M{"request": out}); err != nil {
			return err
		}
		if err := s.advance(ctx, tx, a, out.ID); err != nil {
			return err
		}
		out, err = one[api.Request](ctx, tx, `SELECT * FROM requests WHERE id = $1`, out.ID)
		return err
	})
	return out, err
}

func (s *Server) listRequests(r *http.Request, a actor) (any, error) {
	q := r.URL.Query()
	var pid int64
	if ref := q.Get("project"); ref != "" {
		p, err := projectByRef(r.Context(), s.db, ref)
		if err != nil {
			return nil, err
		}
		pid = p.ID
	}
	return collect[api.Request](r.Context(), s.db, `
		SELECT * FROM requests WHERE ($1 = 0 OR project_id = $1) AND ($2 = '' OR status = $2)
		ORDER BY id DESC LIMIT 200`, pid, q.Get("status"))
}

func (s *Server) getRequest(r *http.Request, a actor) (any, error) {
	id, err := pathID(r)
	if err != nil {
		return nil, err
	}
	ctx := r.Context()
	var d api.RequestDetail
	if d.Request, err = one[api.Request](ctx, s.db, `SELECT * FROM requests WHERE id = $1`, id); err != nil {
		return nil, err
	}
	if d.Tasks, err = collect[api.Task](ctx, s.db, `SELECT * FROM tasks WHERE request_id = $1 ORDER BY id`, id); err != nil {
		return nil, err
	}
	if d.Approvals, err = collect[api.ApprovalRequest](ctx, s.db, `
		SELECT a.* FROM approval_requests a
		WHERE a.task_id IN (SELECT id FROM tasks WHERE request_id = $1)
		   OR (a.subject_type = 'task' AND a.subject_ref IN (SELECT id::text FROM tasks WHERE request_id = $1))
		ORDER BY a.id`, id); err != nil {
		return nil, err
	}
	for _, st := range s.workflows[d.Request.Workflow].Steps {
		d.Steps = append(d.Steps, api.StepStatus{Name: st.Name, Kind: st.Kind, Role: st.Role, Status: stepStatus(d.Tasks, st.Name)})
	}
	repo, env, err := s.repoEnv(ctx, s.db)
	if err != nil {
		return nil, err
	}
	d.Documents = []api.DocumentInfo{}
	for _, t := range d.Tasks {
		for _, key := range t.OutputDocs {
			if k, ok := docs.ParseKey(key); ok {
				if doc := repo.Get(k); doc != nil {
					d.Documents = append(d.Documents, docInfo(doc, env))
				}
			}
		}
	}
	return d, nil
}

func stepStatus(tasks []api.Task, step string) string {
	n, done, waiting, cancelled := 0, 0, 0, 0
	for _, t := range tasks {
		if t.Step != step {
			continue
		}
		n++
		switch t.Status {
		case task.Completed:
			done++
		case task.WaitingForHuman:
			waiting++
		case task.Cancelled:
			cancelled++
		}
	}
	switch {
	case n == 0:
		return "not_started"
	case cancelled > 0:
		return "cancelled"
	case done == n:
		return "completed"
	case waiting > 0:
		return "waiting_for_human"
	}
	return "in_progress"
}

func (s *Server) cancelRequest(r *http.Request, a actor) (any, error) {
	id, err := pathID(r)
	if err != nil {
		return nil, err
	}
	s.kmu.Lock()
	defer s.kmu.Unlock()
	return nil, s.tx(r.Context(), func(tx pgx.Tx) error {
		return s.finishRequest(r.Context(), tx, a, id, "cancelled", "cancelled by owner")
	})
}

// finishRequest closes a request; cancelling also cancels its open tasks.
func (s *Server) finishRequest(ctx context.Context, tx pgx.Tx, a actor, id int64, status, reason string) error {
	req, err := one[api.Request](ctx, tx, `SELECT * FROM requests WHERE id = $1 FOR UPDATE`, id)
	if err != nil {
		return err
	}
	if req.Status != "active" {
		return errf(http.StatusConflict, "request %d is already %s", id, req.Status)
	}
	if status == "cancelled" {
		open, err := collect[api.Task](ctx, tx, `
			SELECT * FROM tasks WHERE request_id = $1 AND status NOT IN ('COMPLETED', 'CANCELLED') FOR UPDATE`, id)
		if err != nil {
			return err
		}
		for _, t := range open {
			if t.Status == task.Failed { // FAILED can only be retried; cancel via pending
				if err := move(ctx, tx, a, t, task.Pending, M{"reason": reason}); err != nil {
					return err
				}
				t.Status = task.Pending
			}
			if err := s.cancelTaskTx(ctx, tx, a, t); err != nil {
				return err
			}
		}
	}
	if _, err := tx.Exec(ctx, `UPDATE requests SET status = $2, updated_at = now() WHERE id = $1`, id, status); err != nil {
		return err
	}
	return audit(ctx, tx, a, "request."+status, requestRef(id), M{"reason": reason})
}

// advance moves a request forward: it starts the first step that has no tasks
// yet, once every earlier step is completed (spec §7).
func (s *Server) advance(ctx context.Context, tx pgx.Tx, a actor, id int64) error {
	req, err := one[api.Request](ctx, tx, `SELECT * FROM requests WHERE id = $1 FOR UPDATE`, id)
	if err != nil || req.Status != "active" {
		return err
	}
	wf, ok := s.workflows[req.Workflow]
	if !ok {
		return fmt.Errorf("request %d uses unknown workflow %q", id, req.Workflow)
	}
	tasks, err := collect[api.Task](ctx, tx, `SELECT * FROM tasks WHERE request_id = $1 ORDER BY id`, id)
	if err != nil {
		return err
	}
	for _, step := range wf.Steps {
		switch stepStatus(tasks, step.Name) {
		case "cancelled":
			return s.finishRequest(ctx, tx, a, id, "cancelled", fmt.Sprintf("step %s was rejected or cancelled", step.Name))
		case "not_started":
			if err := s.startStep(ctx, tx, a, req, step, tasks); err != nil {
				return err
			}
			return s.setStep(ctx, tx, req, step.Name)
		case "completed":
			continue
		default:
			return s.setStep(ctx, tx, req, step.Name)
		}
	}
	return s.finishRequest(ctx, tx, a, id, "completed", "all steps completed")
}

func (s *Server) setStep(ctx context.Context, tx pgx.Tx, req api.Request, step string) error {
	if req.CurrentStep == step {
		return nil
	}
	_, err := tx.Exec(ctx, `UPDATE requests SET current_step = $2, updated_at = now() WHERE id = $1`, req.ID, step)
	return err
}

// stepDocs returns the documents produced by the named steps of a request.
func stepDocs(repo *docs.Repo, tasks []api.Task, steps ...string) []*docs.Doc {
	var out []*docs.Doc
	for _, t := range tasks {
		if !slices.Contains(steps, t.Step) || t.Status == task.Cancelled {
			continue
		}
		for _, key := range t.OutputDocs {
			if k, ok := docs.ParseKey(key); ok {
				if d := repo.Get(k); d != nil {
					out = append(out, d)
				}
			}
		}
	}
	return out
}

// primaryDocs is like stepDocs but keeps only each step's main document type
// (a design step's technical design, not its ADRs). Used for workflow relationships.
func primaryDocs(wf workflow.Workflow, repo *docs.Repo, tasks []api.Task, steps ...string) []*docs.Doc {
	var out []*docs.Doc
	for _, name := range steps {
		st, _, _ := wf.Step(name)
		for _, d := range stepDocs(repo, tasks, name) {
			if st.DocType == "" || d.Type == st.DocType {
				out = append(out, d)
			}
		}
	}
	return out
}

func docPaths(ds []*docs.Doc) []string {
	out := []string{}
	for _, d := range ds {
		if p := projectRel(d); !slices.Contains(out, p) {
			out = append(out, p)
		}
	}
	return out
}

// Revision order: requirements before the documents that satisfy them.
var reviseOrder = []string{"prd", "ux-spec", "architecture-overview", "technical-design", "adr",
	"api-spec", "database-design", "test-plan", "implementation-plan", "deployment-plan", "runbook", "release-notes"}

func (s *Server) startStep(ctx context.Context, tx pgx.Tx, a actor, req api.Request, step workflow.Step, tasks []api.Task) error {
	repo, env, err := s.repoEnv(ctx, tx)
	if err != nil {
		return err
	}
	inputs := docPaths(stepDocs(repo, tasks, step.Inputs...))
	insert := func(kind, docType, revises, title, desc string, repoID *int64, caps []string) (int64, error) {
		if caps == nil {
			caps = []string{}
		}
		var id int64
		err := tx.QueryRow(ctx, `
			INSERT INTO tasks (project_id, repository_id, kind, role, request_id, step, doc_type, revises,
			                   title, description, required_capabilities, context_docs, review)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13) RETURNING id`,
			req.ProjectID, repoID, kind, step.Role, req.ID, step.Name, docType, revises,
			title, desc, caps, inputs, step.Review).Scan(&id)
		if err != nil {
			return 0, err
		}
		return id, audit(ctx, tx, a, "task.create", taskRef(id), M{"request_id": req.ID, "step": step.Name, "kind": kind, "role": step.Role})
	}

	switch step.Kind {
	case workflow.Document, workflow.Plan:
		c := repo.Contracts[step.DocType]
		_, err := insert(step.Kind, step.DocType, "", truncate(c.Title+": "+req.Title, 200), req.Description, nil, nil)
		return err

	case workflow.Revise:
		var targets []*docs.Doc
		for _, cr := range stepDocs(repo, tasks, step.Inputs...) {
			if cr.Type != "change-request" {
				continue
			}
			for _, ref := range cr.Rels["affects"] {
				t, err := repo.Resolve(cr, clientOf(env, cr), ref)
				if err != nil {
					return fmt.Errorf("change request %s: %w", cr.ID, err)
				}
				if !slices.ContainsFunc(targets, func(d *docs.Doc) bool { return d.Key() == t.Key() }) {
					targets = append(targets, t)
				}
			}
		}
		if len(targets) == 0 {
			return errors.New("the change request affects no documents")
		}
		sort.SliceStable(targets, func(i, j int) bool {
			return slices.Index(reviseOrder, targets[i].Type) < slices.Index(reviseOrder, targets[j].Type)
		})
		var prev int64
		for _, t := range targets {
			latest := 0
			for v := range env.Approvals[t.Key()] {
				latest = max(latest, v)
			}
			title := fmt.Sprintf("Revise %s to v%d: %s", t.ID, latest+1, t.Title)
			desc := fmt.Sprintf("%s\n\nUpdate %s (%s) as required by the approved change request. Keep its id and structure; change only what the change request requires.", req.Description, t.ID, t.Title)
			id, err := insert(workflow.Revise, t.Type, t.Key().String(), truncate(title, 200), desc, nil, nil)
			if err != nil {
				return err
			}
			// A design is revised only after the requirements it satisfies are approved again.
			if prev != 0 {
				if _, err := tx.Exec(ctx, `INSERT INTO task_dependencies (task_id, depends_on_task_id) VALUES ($1, $2)`, id, prev); err != nil {
					return err
				}
			}
			prev = id
		}
		return nil

	case workflow.Code:
		// Plans create their code tasks when approved; reaching here means the workflow has no plan.
		if req.RepositoryID == nil {
			return errors.New("this workflow needs a repository on the request")
		}
		r, err := one[api.Repository](ctx, tx, `SELECT * FROM repositories WHERE id = $1`, *req.RepositoryID)
		if err != nil {
			return err
		}
		_, err = insert(workflow.Code, "", "", req.Title, req.Description, req.RepositoryID, []string{r.Stack})
		return err
	}
	return fmt.Errorf("unknown step kind %q", step.Kind)
}

// createPlanTasks turns an accepted implementation plan into code tasks (spec §7 "Create Tasks").
func (s *Server) createPlanTasks(ctx context.Context, tx pgx.Tx, a actor, planTask api.Task, plan *docs.Doc) error {
	if planTask.RequestID == nil {
		return errors.New("plan task has no request")
	}
	req, err := one[api.Request](ctx, tx, `SELECT * FROM requests WHERE id = $1`, *planTask.RequestID)
	if err != nil {
		return err
	}
	wf := s.workflows[req.Workflow]
	_, i, ok := wf.Step(planTask.Step)
	if !ok || i+1 >= len(wf.Steps) {
		return fmt.Errorf("workflow %s has no code step after %s", wf.Name, planTask.Step)
	}
	codeStep := wf.Steps[i+1]

	repos, err := collect[api.Repository](ctx, tx, `SELECT * FROM repositories WHERE project_id = $1`, req.ProjectID)
	if err != nil {
		return err
	}
	byName := map[string]api.Repository{}
	var names []string
	for _, r := range repos {
		byName[r.Name] = r
		names = append(names, r.Name)
	}
	items, err := docs.ParsePlan(plan.Body, names)
	if err != nil {
		return errf(http.StatusConflict, "%s: %v", plan.ID, err)
	}
	tasks, err := collect[api.Task](ctx, tx, `SELECT * FROM tasks WHERE request_id = $1`, req.ID)
	if err != nil {
		return err
	}
	for i := range tasks {
		if tasks[i].ID == planTask.ID {
			tasks[i].OutputDocs = []string{plan.Key().String()}
		}
	}
	repo, err := s.loadRepo()
	if err != nil {
		return err
	}
	inputs := docPaths(stepDocs(repo, tasks, codeStep.Inputs...))

	ids := map[string]int64{}
	for _, it := range items {
		r := byName[it.Repository]
		desc := fmt.Sprintf("%s\n\n(Work item `%s` of implementation plan %s.)", strings.TrimSpace(it.Description), it.Key, plan.ID)
		var id int64
		err := tx.QueryRow(ctx, `
			INSERT INTO tasks (project_id, repository_id, kind, role, request_id, step,
			                   title, description, required_capabilities, context_docs, review)
			VALUES ($1, $2, 'code', $3, $4, $5, $6, $7, $8, $9, $10) RETURNING id`,
			req.ProjectID, r.ID, codeStep.Role, req.ID, codeStep.Name,
			it.Title, desc, []string{r.Stack}, inputs, codeStep.Review).Scan(&id)
		if err != nil {
			return err
		}
		ids[it.Key] = id
		for _, dep := range it.DependsOn {
			if _, err := tx.Exec(ctx, `INSERT INTO task_dependencies (task_id, depends_on_task_id) VALUES ($1, $2)`, id, ids[dep]); err != nil {
				return err
			}
		}
		if err := audit(ctx, tx, a, "task.create", taskRef(id), M{"request_id": req.ID, "plan": plan.ID, "work_item": it.Key, "repository": r.Name}); err != nil {
			return err
		}
	}
	return nil
}

// taskOutput receives the documents an agent wrote for a document, plan, or
// revise task. Invalid output goes back to the agent with the problems (spec §18);
// valid output is stored and sent for approval.
func (s *Server) taskOutput(r *http.Request, a actor) (any, error) {
	var in api.TaskOutput
	if err := decode(r, &in); err != nil {
		return nil, err
	}
	if len(in.Files) == 0 || len(in.Files) > 10 {
		return nil, errf(http.StatusBadRequest, "send between 1 and 10 files")
	}
	for _, f := range in.Files {
		if len(f.Content) > 512<<10 {
			return nil, errf(http.StatusBadRequest, "%s is larger than 512 KB", f.Name)
		}
	}
	s.kmu.Lock()
	defer s.kmu.Unlock()

	var reply api.OutputReply
	st := s.stage()
	err := s.tx(r.Context(), func(tx pgx.Tx) error {
		ctx := r.Context()
		t, err := lockOwnTask(ctx, tx, r, a)
		if err != nil {
			return err
		}
		if t.Kind == workflow.Code || t.RequestID == nil {
			return errf(http.StatusBadRequest, "task %d does not produce documents", t.ID)
		}
		req, err := one[api.Request](ctx, tx, `SELECT * FROM requests WHERE id = $1`, *t.RequestID)
		if err != nil {
			return err
		}
		step, _, _ := s.workflows[req.Workflow].Step(t.Step)
		p, err := one[api.Project](ctx, tx, `SELECT * FROM projects WHERE id = $1`, t.ProjectID)
		if err != nil {
			return err
		}
		repo, env, err := s.repoEnv(ctx, tx)
		if err != nil {
			return err
		}
		reqTasks, err := collect[api.Task](ctx, tx, `SELECT * FROM tasks WHERE request_id = $1`, req.ID)
		if err != nil {
			return err
		}

		ds, problems := s.prepareOutput(repo, env, t, s.workflows[req.Workflow], step, p, reqTasks, in.Files)
		if len(problems) > 0 {
			reply.Problems = problems
			return nil
		}
		for _, d := range ds {
			if err := st.write(d.Path, d.Bytes()); err != nil {
				return err
			}
		}
		// Validate against the repository as it now is on disk.
		repo2, err := s.loadRepo()
		if err != nil {
			return err
		}
		var fresh []*docs.Doc
		for _, d := range ds {
			nd := repo2.Get(d.Key())
			if nd == nil {
				return fmt.Errorf("%s vanished after writing", d.Key())
			}
			fresh = append(fresh, nd)
			reply.Problems = append(reply.Problems, problemStrings(repo2.ValidateDoc(nd, env))...)
		}
		if t.Kind == workflow.Plan {
			var names []string
			rows, err := collect[api.Repository](ctx, tx, `SELECT * FROM repositories WHERE project_id = $1`, p.ID)
			if err != nil {
				return err
			}
			for _, rp := range rows {
				names = append(names, rp.Name)
			}
			if _, err := docs.ParsePlan(fresh[0].Body, names); err != nil {
				reply.Problems = append(reply.Problems, fmt.Sprintf("knowledge/%s: work breakdown: %v", fresh[0].Path, err))
			}
		}
		if len(reply.Problems) > 0 {
			st.rollback()
			return nil
		}

		var keys []string
		for _, d := range fresh {
			keys = append(keys, d.Key().String())
		}
		t.OutputDocs = keys
		if _, err := tx.Exec(ctx, `UPDATE tasks SET output_docs = $2, updated_at = now() WHERE id = $1`, t.ID, keys); err != nil {
			return err
		}
		if err := audit(ctx, tx, a, "task.output", taskRef(t.ID), M{"documents": keys}); err != nil {
			return err
		}

		if t.Kind != workflow.Plan || step.PlanGate() {
			if _, err := s.openDocumentApproval(ctx, tx, a, repo2, env, fresh, &p.ID, &t.ID, strings.TrimSpace(in.Summary)); err != nil {
				return err
			}
			if err := move(ctx, tx, a, t, task.WaitingForHuman, M{"documents": keys}); err != nil {
				return err
			}
			reply.Status = task.WaitingForHuman
		} else {
			if err := move(ctx, tx, a, t, task.Completed, M{"documents": keys}); err != nil {
				return err
			}
			if err := s.createPlanTasks(ctx, tx, a, t, fresh[0]); err != nil {
				return err
			}
			if err := s.advance(ctx, tx, a, req.ID); err != nil {
				return err
			}
			reply.Status = task.Completed
		}
		reply.Accepted, reply.Documents = true, keys
		return s.reindex(ctx, tx, repo2, env)
	})
	if err != nil || !reply.Accepted {
		st.rollback()
	}
	return reply, err
}

// prepareOutput parses the agent's files and sets everything the platform
// owns: ids, versions, project, status, dates, and workflow relationships.
func (s *Server) prepareOutput(repo *docs.Repo, env docs.Env, t api.Task, wf workflow.Workflow, step workflow.Step, p api.Project,
	reqTasks []api.Task, files []api.File) ([]*docs.Doc, []string) {
	var problems []string
	scope := "projects/" + p.Slug
	own := map[string]bool{}
	for _, k := range t.OutputDocs {
		own[k] = true
	}

	var primary *docs.Doc
	var extras []*docs.Doc
	for _, f := range files {
		d, err := docs.Parse(scope+"/_incoming/"+f.Name, []byte(f.Content))
		if err != nil {
			problems = append(problems, fmt.Sprintf("%s: %v", f.Name, err))
			continue
		}
		switch {
		case d.Type == t.DocType && primary == nil:
			primary = d
		case t.Kind != workflow.Revise && slices.Contains(step.ExtraTypes, d.Type):
			extras = append(extras, d)
		default:
			problems = append(problems, fmt.Sprintf("%s: unexpected document of type %q; this task produces one %s", f.Name, d.Type, t.DocType))
		}
	}
	if primary == nil {
		problems = append(problems, fmt.Sprintf("no document of type %s was written", t.DocType))
	}
	if len(problems) > 0 {
		return nil, problems
	}

	today := time.Now().Format("2006-01-02")
	gated := t.Kind != workflow.Plan || step.PlanGate()
	var assigned []string
	all := append([]*docs.Doc{primary}, extras...)
	for _, d := range all {
		c := repo.Contracts[d.Type]
		var existing *docs.Doc
		switch {
		case t.Kind == workflow.Revise:
			k, _ := docs.ParseKey(t.Revises)
			existing = repo.Get(k)
			if existing == nil {
				return nil, []string{fmt.Sprintf("the document being revised (%s) no longer exists", t.Revises)}
			}
			if d.ID != existing.ID {
				problems = append(problems, fmt.Sprintf("keep `id: %s` when revising it", existing.ID))
				continue
			}
		case d.ID == "" || d.ID == docs.NewID:
			d.Set("id", repo.NextID(scope, c.IDPrefix, assigned...))
		default:
			k := docs.Key{Scope: scope, ID: d.ID}
			if !own[k.String()] {
				problems = append(problems, fmt.Sprintf("id %s does not belong to this task; use `id: NEW` for new documents", d.ID))
				continue
			}
			existing = repo.Get(k)
		}
		assigned = append(assigned, d.ID)

		d.Set("project", p.Slug)
		version := 1
		if existing != nil {
			latest := 0
			for v := range env.Approvals[existing.Key()] {
				latest = max(latest, v)
			}
			version = max(existing.Version, 1)
			if env.IsApproved(existing) || (t.Kind == workflow.Revise && existing.Version <= latest) {
				version = latest + 1
			}
			if c := existing.Str("created"); c != "" {
				d.Set("created", c)
			}
		}
		d.Set("version", version)
		if !d.Has("owner") || strings.Contains(d.Str("owner"), "{{") {
			d.Set("owner", "AI Empire ("+t.Role+")")
		}
		if !docs.IsDate(d.Str("created")) {
			d.Set("created", today)
		}
		d.Set("updated", today)
		status := docs.Draft
		if gated {
			status = docs.PendingApproval
		}
		d.Set("status", status)
		if existing != nil {
			d.Path = existing.Path
		} else {
			d.Path = c.DocPath(p.Slug, d.ID, d.Title)
		}
	}
	if len(problems) > 0 {
		return nil, problems
	}

	// Relationships the workflow guarantees.
	idsOf := func(steps []string) []string {
		var ids []string
		for _, d := range primaryDocs(wf, repo, reqTasks, steps...) {
			ids = append(ids, d.ID)
		}
		return ids
	}
	for rel, steps := range step.Relations {
		if ids := idsOf(steps); len(ids) > 0 {
			primary.AddRel(rel, ids...)
		}
	}
	for _, x := range extras {
		primary.AddRel("depends_on", x.ID)
		x.AddRel("derived_from", primary.ID)
	}
	if t.Kind == workflow.Revise {
		for _, cr := range stepDocs(repo, reqTasks, step.Inputs...) {
			if cr.Type == "change-request" {
				primary.AddRel("derived_from", cr.ID)
			}
		}
	}
	return all, nil
}

// taskReview records the AI reviewer's verdict (spec §7 "AI Review"). A
// REQUEST_CHANGES verdict sends the task back to the developer, at most twice;
// after that the human decides at the merge gate.
func (s *Server) taskReview(r *http.Request, a actor) (any, error) {
	var in api.ReviewReport
	if err := decode(r, &in); err != nil {
		return nil, err
	}
	var reply api.ReviewReply
	err := s.tx(r.Context(), func(tx pgx.Tx) error {
		ctx := r.Context()
		t, err := lockOwnTask(ctx, tx, r, a)
		if err != nil {
			return err
		}
		if t.Status != task.Reviewing {
			return errf(http.StatusConflict, "task %d is %s, not REVIEWING", t.ID, t.Status)
		}
		if err := audit(ctx, tx, a, "task.review", taskRef(t.ID), M{"verdict": in.Verdict, "round": t.ReviewRounds + 1, "summary": in.Summary}); err != nil {
			return err
		}
		if in.Verdict != "REQUEST_CHANGES" || t.ReviewRounds >= maxReviewRounds {
			return nil
		}
		reply.Rework = true
		if err := move(ctx, tx, a, t, task.Pending, M{"reason": "AI review requested changes"}); err != nil {
			return err
		}
		_, err = tx.Exec(ctx, `
			UPDATE tasks SET review_rounds = review_rounds + 1, stage = $2, feedback = $3, attempts = 0 WHERE id = $1`,
			t.ID, task.StageImplement, "AI code review (round "+strconv.Itoa(t.ReviewRounds+1)+") requested changes:\n\n"+in.Summary)
		return err
	})
	return reply, err
}

const maxReviewRounds = 2

func requestRef(id int64) string { return "request:" + strconv.FormatInt(id, 10) }

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return strings.ToValidUTF8(s[:n], "") + "…"
}
