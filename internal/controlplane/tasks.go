package controlplane

import (
	"context"
	"maps"
	"net/http"
	"strconv"

	"aiempire/internal/api"
	"aiempire/internal/docs"
	"aiempire/internal/knowledge"
	"aiempire/internal/task"

	"github.com/jackc/pgx/v5"
)

func (s *Server) createTask(r *http.Request, a actor) (any, error) {
	var in api.CreateTask
	if err := decode(r, &in); err != nil {
		return nil, err
	}
	if in.Title == "" {
		return nil, errf(http.StatusBadRequest, "title is required")
	}
	ctxDocs := in.ContextDocs
	if ctxDocs == nil {
		ctxDocs = []string{}
	}

	var out api.Task
	err := s.tx(r.Context(), func(tx pgx.Tx) error {
		ctx := r.Context()
		p, err := projectByRef(ctx, tx, in.Project)
		if err != nil {
			return errf(http.StatusBadRequest, "unknown project %q", in.Project)
		}
		repo, err := pickRepository(ctx, tx, p, in.Repository)
		if err != nil {
			return err
		}
		// Fail early on docs the worker won't be able to load.
		scope, err := scopeFor(ctx, tx, p.ID, &repo.ID, "developer", ctxDocs)
		if err != nil {
			return err
		}
		if _, err := knowledge.Resolve(s.cfg.KnowledgeDir, scope); err != nil {
			return errf(http.StatusBadRequest, "context: %v", err)
		}
		caps := in.RequiredCapabilities
		if len(caps) == 0 {
			caps = []string{repo.Stack}
		}

		out, err = one[api.Task](ctx, tx, `
			INSERT INTO tasks (project_id, repository_id, title, description, required_capabilities, context_docs)
			VALUES ($1, $2, $3, $4, $5, $6) RETURNING *`,
			p.ID, repo.ID, in.Title, in.Description, caps, ctxDocs)
		if err != nil {
			return err
		}
		for _, dep := range in.DependsOn {
			// Any project, but only within the same client (or both client-less):
			// a dependency orders work, it must never link two clients (spec §11A).
			tag, err := tx.Exec(ctx, `
				INSERT INTO task_dependencies (task_id, depends_on_task_id)
				SELECT $1, t.id FROM tasks t JOIN projects dp ON dp.id = t.project_id
				WHERE t.id = $2 AND dp.client_id IS NOT DISTINCT FROM $3`, out.ID, dep, p.ClientID)
			if err != nil {
				return err
			}
			if tag.RowsAffected() == 0 {
				return errf(http.StatusBadRequest, "dependency %d not found, or it belongs to a different client", dep)
			}
		}
		return audit(ctx, tx, a, "task.create", taskRef(out.ID),
			M{"task": out, "repository": repo.Name, "depends_on": in.DependsOn})
	})
	return out, err
}

// pickRepository resolves the task's repository by name; it may be omitted
// only when the project has exactly one.
func pickRepository(ctx context.Context, q querier, p api.Project, name string) (api.Repository, error) {
	repos, err := collect[api.Repository](ctx, q, `SELECT * FROM repositories WHERE project_id = $1 ORDER BY id`, p.ID)
	if err != nil {
		return api.Repository{}, err
	}
	var names []string
	for _, r := range repos {
		if r.Name == name || (name == "" && len(repos) == 1) {
			return r, nil
		}
		names = append(names, r.Name)
	}
	switch {
	case len(repos) == 0:
		return api.Repository{}, errf(http.StatusBadRequest, "project %s has no repositories; add one first", p.Slug)
	case name == "":
		return api.Repository{}, errf(http.StatusBadRequest, "project %s has several repositories, pick one of %v", p.Slug, names)
	}
	return api.Repository{}, errf(http.StatusBadRequest, "project %s has no repository %q (have %v)", p.Slug, name, names)
}

func (s *Server) listTasks(r *http.Request, a actor) (any, error) {
	q := r.URL.Query()
	pid, _ := strconv.ParseInt(q.Get("project_id"), 10, 64)
	return collect[api.Task](r.Context(), s.db, `
		SELECT * FROM tasks
		WHERE ($1 = '' OR status::text = $1) AND ($2 = 0 OR project_id = $2)
		ORDER BY id DESC LIMIT 500`, q.Get("status"), pid)
}

func (s *Server) getTask(r *http.Request, a actor) (any, error) {
	id, err := pathID(r)
	if err != nil {
		return nil, err
	}
	ctx := r.Context()
	var d api.TaskDetail
	if d.Task, err = one[api.Task](ctx, s.db, `SELECT * FROM tasks WHERE id = $1`, id); err != nil {
		return nil, err
	}
	if d.Runs, err = collect[api.AgentRun](ctx, s.db, `SELECT * FROM agent_runs WHERE task_id = $1 ORDER BY id`, id); err != nil {
		return nil, err
	}
	d.Approvals, err = collect[api.ApprovalRequest](ctx, s.db,
		`SELECT * FROM approval_requests WHERE task_id = $1 OR (subject_type = 'task' AND subject_ref = $2) ORDER BY id`, id, strconv.FormatInt(id, 10))
	return d, err
}

func (s *Server) cancelTask(r *http.Request, a actor) (any, error) {
	id, err := pathID(r)
	if err != nil {
		return nil, err
	}
	s.kmu.Lock() // cancelling withdraws the task's unapproved documents
	defer s.kmu.Unlock()
	return nil, s.tx(r.Context(), func(tx pgx.Tx) error {
		t, err := lockTask(r.Context(), tx, id)
		if err != nil {
			return err
		}
		if err := s.cancelTaskTx(r.Context(), tx, a, t); err != nil {
			return err
		}
		if t.RequestID != nil {
			return s.advance(r.Context(), tx, a, *t.RequestID) // a cancelled step cancels its request
		}
		return nil
	})
}

// cancelTaskTx cancels a task, closes its open gates so they don't linger in the
// approval queue, and marks the documents it produced but never got approved as rejected.
// Callers hold s.kmu when the task may have produced documents.
func (s *Server) cancelTaskTx(ctx context.Context, tx pgx.Tx, a actor, t api.Task) error {
	if err := move(ctx, tx, a, t, task.Cancelled, nil); err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, `
		UPDATE approval_requests SET status = 'REJECTED'
		WHERE status = 'PENDING_APPROVAL'
		  AND (task_id = $1 OR (subject_type = 'task' AND subject_ref = $2))`, t.ID, strconv.FormatInt(t.ID, 10)); err != nil {
		return err
	}
	if len(t.OutputDocs) == 0 {
		return nil
	}
	repo, env, err := s.repoEnv(ctx, tx)
	if err != nil {
		return err
	}
	for _, key := range t.OutputDocs {
		k, _ := docs.ParseKey(key)
		d := repo.Get(k)
		if d == nil || d.Status == docs.Approved || d.Status == docs.Superseded || env.IsApproved(d) {
			continue
		}
		d.Set("status", docs.Rejected)
		if err := s.writeKnowledge(d.Path, d.Bytes()); err != nil {
			return err
		}
		if err := audit(ctx, tx, a, "document.withdrawn", "document:"+key, M{"task_id": t.ID}); err != nil {
			return err
		}
	}
	return nil
}

func (s *Server) retryTask(r *http.Request, a actor) (any, error) {
	id, err := pathID(r)
	if err != nil {
		return nil, err
	}
	return nil, s.tx(r.Context(), func(tx pgx.Tx) error {
		t, err := lockTask(r.Context(), tx, id)
		if err != nil {
			return err
		}
		if t.Status != task.Failed {
			return errf(http.StatusConflict, "only FAILED tasks can be retried (task is %s)", t.Status)
		}
		if err := move(r.Context(), tx, a, t, task.Pending, nil); err != nil {
			return err
		}
		_, err = tx.Exec(r.Context(), `UPDATE tasks SET attempts = 0, last_error = '' WHERE id = $1`, id)
		return err
	})
}

func lockTask(ctx context.Context, tx pgx.Tx, id int64) (api.Task, error) {
	return one[api.Task](ctx, tx, `SELECT * FROM tasks WHERE id = $1 FOR UPDATE`, id)
}

// move is the single place task status changes. It enforces the state machine,
// releases the worker when the task leaves an in-flight state, and audits.
func move(ctx context.Context, tx pgx.Tx, a actor, t api.Task, to string, note M) error {
	if !task.CanTransition(t.Status, to) {
		return errf(http.StatusConflict, "task %d: illegal transition %s -> %s", t.ID, t.Status, to)
	}
	_, err := tx.Exec(ctx, `
		UPDATE tasks SET status = $2, updated_at = now(),
		       worker_id = CASE WHEN $3 THEN worker_id END
		WHERE id = $1`, t.ID, to, task.InFlight(to))
	if err != nil {
		return err
	}
	payload := M{"from": t.Status, "to": to}
	maps.Copy(payload, note)
	return audit(ctx, tx, a, "task.status", taskRef(t.ID), payload)
}

func taskRef(id int64) string    { return "task:" + strconv.FormatInt(id, 10) }
func projectRef(id int64) string { return "project:" + strconv.FormatInt(id, 10) }
func clientRef(id int64) string  { return "client:" + strconv.FormatInt(id, 10) }
