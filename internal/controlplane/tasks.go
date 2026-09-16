package controlplane

import (
	"context"
	"maps"
	"net/http"
	"strconv"

	"aiempire/internal/api"
	"aiempire/internal/knowledge"
	"aiempire/internal/task"

	"github.com/jackc/pgx/v5"
)

func (s *Server) createProject(r *http.Request, a actor) (any, error) {
	var p api.Project
	if err := decode(r, &p); err != nil {
		return nil, err
	}
	if p.Slug == "" || p.Name == "" || p.RepoURL == "" || p.Stack == "" {
		return nil, errf(http.StatusBadRequest, "slug, name, repo_url and stack are required")
	}
	if p.DefaultBranch == "" {
		p.DefaultBranch = "main"
	}
	if p.AutonomyLevel == "" {
		p.AutonomyLevel = "conservative"
	}
	var out api.Project
	err := s.tx(r.Context(), func(tx pgx.Tx) error {
		var err error
		out, err = one[api.Project](r.Context(), tx, `
			INSERT INTO projects (slug, name, repo_url, default_branch, stack, autonomy_level, test_command)
			VALUES ($1, $2, $3, $4, $5, $6, $7) RETURNING *`,
			p.Slug, p.Name, p.RepoURL, p.DefaultBranch, p.Stack, p.AutonomyLevel, p.TestCommand)
		if err != nil {
			return err
		}
		return audit(r.Context(), tx, a, "project.create", projectRef(out.ID), M{"project": out})
	})
	return out, err
}

func (s *Server) listProjects(r *http.Request, a actor) (any, error) {
	return collect[api.Project](r.Context(), s.db, `SELECT * FROM projects ORDER BY id`)
}

func (s *Server) getProject(r *http.Request, a actor) (any, error) {
	id, err := pathID(r)
	if err != nil {
		return nil, err
	}
	return one[api.Project](r.Context(), s.db, `SELECT * FROM projects WHERE id = $1`, id)
}

func (s *Server) createTask(r *http.Request, a actor) (any, error) {
	var in api.CreateTask
	if err := decode(r, &in); err != nil {
		return nil, err
	}
	if in.Title == "" {
		return nil, errf(http.StatusBadRequest, "title is required")
	}
	p, err := one[api.Project](r.Context(), s.db, `SELECT * FROM projects WHERE slug = $1`, in.Project)
	if err != nil {
		return nil, errf(http.StatusBadRequest, "unknown project %q", in.Project)
	}
	// Fail early on docs the worker won't be able to load.
	if _, err := knowledge.Resolve(s.cfg.KnowledgeDir, p.Stack, p.Slug, in.ContextDocs); err != nil {
		return nil, errf(http.StatusBadRequest, "context: %v", err)
	}
	caps := in.RequiredCapabilities
	if len(caps) == 0 {
		caps = []string{p.Stack}
	}
	docs := in.ContextDocs
	if docs == nil {
		docs = []string{}
	}

	var out api.Task
	err = s.tx(r.Context(), func(tx pgx.Tx) error {
		var err error
		out, err = one[api.Task](r.Context(), tx, `
			INSERT INTO tasks (project_id, title, description, required_capabilities, context_docs)
			VALUES ($1, $2, $3, $4, $5) RETURNING *`,
			p.ID, in.Title, in.Description, caps, docs)
		if err != nil {
			return err
		}
		for _, dep := range in.DependsOn {
			// Same-project only: dependencies must not bridge isolated projects.
			tag, err := tx.Exec(r.Context(), `
				INSERT INTO task_dependencies (task_id, depends_on_task_id)
				SELECT $1, id FROM tasks WHERE id = $2 AND project_id = $3`, out.ID, dep, p.ID)
			if err != nil {
				return err
			}
			if tag.RowsAffected() == 0 {
				return errf(http.StatusBadRequest, "dependency %d not found in project %s", dep, p.Slug)
			}
		}
		return audit(r.Context(), tx, a, "task.create", taskRef(out.ID), M{"task": out, "depends_on": in.DependsOn})
	})
	return out, err
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
		`SELECT * FROM approval_requests WHERE subject_type = 'task' AND subject_ref = $1 ORDER BY id`, strconv.FormatInt(id, 10))
	return d, err
}

func (s *Server) cancelTask(r *http.Request, a actor) (any, error) {
	id, err := pathID(r)
	if err != nil {
		return nil, err
	}
	return nil, s.tx(r.Context(), func(tx pgx.Tx) error {
		t, err := lockTask(r.Context(), tx, id)
		if err != nil {
			return err
		}
		if err := move(r.Context(), tx, a, t, task.Cancelled, nil); err != nil {
			return err
		}
		// Close open gates so they don't linger in the approval queue.
		_, err = tx.Exec(r.Context(), `
			UPDATE approval_requests SET status = 'REJECTED'
			WHERE subject_type = 'task' AND subject_ref = $1 AND status = 'PENDING_APPROVAL'`, strconv.FormatInt(id, 10))
		return err
	})
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
