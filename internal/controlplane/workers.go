package controlplane

import (
	"context"
	"errors"
	"log"
	"net/http"
	"strconv"
	"time"

	"aiempire/internal/api"
	"aiempire/internal/knowledge"
	"aiempire/internal/policy"
	"aiempire/internal/task"

	"github.com/jackc/pgx/v5"
)

func (s *Server) listWorkers(r *http.Request, a actor) (any, error) {
	return collect[api.Worker](r.Context(), s.db, `SELECT * FROM workers ORDER BY id`)
}

func (s *Server) registerWorker(r *http.Request, a actor) (any, error) {
	var in api.RegisterWorker
	if err := decode(r, &in); err != nil {
		return nil, err
	}
	if in.Name == "" {
		return nil, errf(http.StatusBadRequest, "name is required")
	}
	if in.Capabilities == nil {
		in.Capabilities = []string{}
	}
	var w api.Worker
	err := s.tx(r.Context(), func(tx pgx.Tx) error {
		var err error
		w, err = one[api.Worker](r.Context(), tx, `
			INSERT INTO workers (name, capabilities) VALUES ($1, $2)
			ON CONFLICT (name) DO UPDATE
			SET capabilities = EXCLUDED.capabilities, status = 'online', last_heartbeat_at = now()
			RETURNING *`, in.Name, in.Capabilities)
		if err != nil {
			return err
		}
		a = actor{kind: "worker", workerID: w.ID}
		if err := audit(r.Context(), tx, a, "worker.register", workerRef(w.ID), M{"worker": w}); err != nil {
			return err
		}
		// A (re)registering worker holds nothing: requeue whatever it held before it died.
		return requeueWorkerTasks(r.Context(), tx, a, w.ID, "worker restarted")
	})
	return w, err
}

// selfWorker checks the path worker id matches the caller.
func selfWorker(r *http.Request, a actor) (int64, error) {
	id, err := pathID(r)
	if err != nil {
		return 0, err
	}
	if id != a.workerID {
		return 0, errf(http.StatusForbidden, "worker %d cannot act as worker %d", a.workerID, id)
	}
	return id, nil
}

func (s *Server) heartbeat(r *http.Request, a actor) (any, error) {
	id, err := selfWorker(r, a)
	if err != nil {
		return nil, err
	}
	var in api.Heartbeat
	if err := decode(r, &in); err != nil {
		return nil, err
	}
	tag, err := s.db.Exec(r.Context(),
		`UPDATE workers SET last_heartbeat_at = now(), status = 'online' WHERE id = $1`, id)
	if err != nil {
		return nil, err
	}
	if tag.RowsAffected() == 0 {
		return nil, errf(http.StatusNotFound, "worker %d not registered", id)
	}
	reply := api.HeartbeatReply{}
	if in.TaskID != 0 {
		t, err := one[api.Task](r.Context(), s.db, `SELECT * FROM tasks WHERE id = $1`, in.TaskID)
		if err != nil {
			return nil, err
		}
		reply.Cancel = !owns(t, a)
	}
	return reply, nil
}

func (s *Server) claim(r *http.Request, a actor) (any, error) {
	id, err := selfWorker(r, a)
	if err != nil {
		return nil, err
	}
	var out *api.Claim
	err = s.tx(r.Context(), func(tx pgx.Tx) error {
		ctx := r.Context()
		t, err := one[api.Task](ctx, tx, `
			SELECT t.* FROM tasks t, workers w
			WHERE w.id = $1 AND w.status = 'online'
			  AND t.status = 'PENDING'
			  AND t.required_capabilities <@ w.capabilities
			  AND NOT EXISTS (
			      SELECT 1 FROM task_dependencies d JOIN tasks dt ON dt.id = d.depends_on_task_id
			      WHERE d.task_id = t.id AND dt.status <> 'COMPLETED')
			ORDER BY t.id
			LIMIT 1
			FOR UPDATE OF t SKIP LOCKED`, id)
		if errors.Is(err, pgx.ErrNoRows) {
			return nil
		}
		if err != nil {
			return err
		}
		if err := move(ctx, tx, a, t, task.Assigned, M{"stage": t.Stage}); err != nil {
			return err
		}
		t, err = one[api.Task](ctx, tx,
			`UPDATE tasks SET worker_id = $2, attempts = attempts + 1 WHERE id = $1 RETURNING *`, t.ID, id)
		if err != nil {
			return err
		}
		p, err := one[api.Project](ctx, tx, `SELECT * FROM projects WHERE id = $1`, t.ProjectID)
		if err != nil {
			return err
		}
		repo, err := one[api.Repository](ctx, tx, `SELECT * FROM repositories WHERE id = $1`, t.RepositoryID)
		out = &api.Claim{Task: t, Project: p, Repository: repo}
		return err
	})
	if err != nil || out == nil {
		return nil, err // nil, nil → 204 No Content
	}
	return out, nil
}

func owns(t api.Task, a actor) bool {
	return a.kind == "worker" && t.WorkerID != nil && *t.WorkerID == a.workerID && task.InFlight(t.Status)
}

// lockOwnTask locks a task the calling worker currently holds.
func lockOwnTask(ctx context.Context, tx pgx.Tx, r *http.Request, a actor) (api.Task, error) {
	id, err := pathID(r)
	if err != nil {
		return api.Task{}, err
	}
	t, err := lockTask(ctx, tx, id)
	if err != nil {
		return t, err
	}
	if !owns(t, a) {
		return t, errf(http.StatusConflict, "task %d is not held by %s (status %s)", id, a, t.Status)
	}
	return t, nil
}

// Statuses a worker may set directly. WAITING_FOR_HUMAN only comes from authorize;
// CANCELLED only from the owner.
var workerTargets = map[string]bool{
	task.Running: true, task.Testing: true, task.Reviewing: true,
	task.Completed: true, task.Failed: true, task.Pending: true,
}

func (s *Server) workerTransition(r *http.Request, a actor) (any, error) {
	var in api.Transition
	if err := decode(r, &in); err != nil {
		return nil, err
	}
	if !workerTargets[in.To] {
		return nil, errf(http.StatusForbidden, "workers cannot set status %q", in.To)
	}
	return nil, s.tx(r.Context(), func(tx pgx.Tx) error {
		ctx := r.Context()
		t, err := lockOwnTask(ctx, tx, r, a)
		if err != nil {
			return err
		}
		// Work only completes through the merge stage, which sits behind the merge gate.
		if in.To == task.Completed && t.Stage != task.StageMerge {
			return errf(http.StatusConflict, "task %d cannot complete from stage %s", t.ID, t.Stage)
		}
		if err := move(ctx, tx, a, t, in.To, M{"error": in.Error}); err != nil {
			return err
		}
		if in.To == task.Failed {
			_, err = tx.Exec(ctx, `UPDATE tasks SET last_error = $2 WHERE id = $1`, t.ID, in.Error)
		}
		return err
	})
}

// authorize asks policy whether the worker may perform an action now (spec §10, §11).
// If a human must decide, it opens an approval request and parks the task.
func (s *Server) authorize(r *http.Request, a actor) (any, error) {
	var in api.Authorize
	if err := decode(r, &in); err != nil {
		return nil, err
	}
	if !policy.Known(in.Action) {
		return nil, errf(http.StatusBadRequest, "unknown action %q", in.Action)
	}
	if in.Stage != task.StageImplement && in.Stage != task.StageMerge {
		return nil, errf(http.StatusBadRequest, "unknown stage %q", in.Stage)
	}
	var reply api.AuthorizeReply
	err := s.tx(r.Context(), func(tx pgx.Tx) error {
		ctx := r.Context()
		t, err := lockOwnTask(ctx, tx, r, a)
		if err != nil {
			return err
		}
		p, err := one[api.Project](ctx, tx, `SELECT * FROM projects WHERE id = $1`, t.ProjectID)
		if err != nil {
			return err
		}
		ref := strconv.FormatInt(t.ID, 10)
		if !policy.Requires(p.AutonomyLevel, in.Action) {
			reply.Allowed = true
			return audit(ctx, tx, a, "policy.allow", taskRef(t.ID), M{"action": in.Action, "autonomy": p.AutonomyLevel})
		}

		approved, err := one[api.ApprovalRequest](ctx, tx, `
			SELECT * FROM approval_requests
			WHERE subject_type = 'task' AND subject_ref = $1 AND gate = $2 AND status = 'APPROVED'
			ORDER BY id DESC LIMIT 1`, ref, in.Action)
		if err == nil {
			reply = api.AuthorizeReply{Allowed: true, ApprovalID: approved.ID, SubjectVersion: approved.SubjectVersion}
			return audit(ctx, tx, a, "policy.allow", taskRef(t.ID), M{"action": in.Action, "approval_id": approved.ID})
		}
		if !errors.Is(err, pgx.ErrNoRows) {
			return err
		}

		req, err := one[api.ApprovalRequest](ctx, tx, `
			INSERT INTO approval_requests (project_id, subject_type, subject_ref, subject_version, gate, summary, requested_by)
			VALUES ($1, 'task', $2, $3, $4, $5, $6) RETURNING *`,
			p.ID, ref, in.SubjectVersion, in.Action, in.Summary, a.String())
		if err != nil {
			return err
		}
		reply.ApprovalID = req.ID
		if err := audit(ctx, tx, a, "approval.request", approvalRef(req.ID), M{"request": req}); err != nil {
			return err
		}
		if err := move(ctx, tx, a, t, task.WaitingForHuman, M{"approval_id": req.ID}); err != nil {
			return err
		}
		_, err = tx.Exec(ctx, `UPDATE tasks SET stage = $2 WHERE id = $1`, t.ID, in.Stage)
		return err
	})
	return reply, err
}

func (s *Server) taskContext(r *http.Request, a actor) (any, error) {
	var scope knowledge.Scope
	err := s.tx(r.Context(), func(tx pgx.Tx) error {
		t, err := lockOwnTask(r.Context(), tx, r, a)
		if err != nil {
			return err
		}
		scope, err = scopeFor(r.Context(), tx, t.ProjectID, t.RepositoryID, t.ContextDocs)
		return err
	})
	if err != nil {
		return nil, err
	}
	b, err := knowledge.Resolve(s.cfg.KnowledgeDir, scope)
	if err != nil {
		return nil, errf(http.StatusUnprocessableEntity, "context: %v", err)
	}
	return b, nil
}

func (s *Server) startRun(r *http.Request, a actor) (any, error) {
	var in api.StartRun
	if err := decode(r, &in); err != nil {
		return nil, err
	}
	if in.ContextFiles == nil {
		in.ContextFiles = []string{}
	}
	var out api.ID
	err := s.tx(r.Context(), func(tx pgx.Tx) error {
		ctx := r.Context()
		t, err := lockOwnTask(ctx, tx, r, a)
		if err != nil {
			return err
		}
		err = tx.QueryRow(ctx, `
			INSERT INTO agent_runs (task_id, worker_id, model, context_files) VALUES ($1, $2, $3, $4)
			RETURNING id`, t.ID, a.workerID, in.Model, in.ContextFiles).Scan(&out.ID)
		if err != nil {
			return err
		}
		return audit(ctx, tx, a, "agent_run.start", taskRef(t.ID), M{"run_id": out.ID, "model": in.Model, "context_files": in.ContextFiles})
	})
	return out, err
}

func (s *Server) finishRun(r *http.Request, a actor) (any, error) {
	id, err := pathID(r)
	if err != nil {
		return nil, err
	}
	var in api.FinishRun
	if err := decode(r, &in); err != nil {
		return nil, err
	}
	return nil, s.tx(r.Context(), func(tx pgx.Tx) error {
		var taskID int64
		err := tx.QueryRow(r.Context(), `
			UPDATE agent_runs SET finished_at = now(), exit_status = $3, log_path = $4, tokens = $5, cost_usd = $6
			WHERE id = $1 AND worker_id = $2 AND finished_at IS NULL
			RETURNING task_id`, id, a.workerID, in.ExitStatus, in.LogPath, in.Tokens, in.CostUSD).Scan(&taskID)
		if errors.Is(err, pgx.ErrNoRows) {
			return errf(http.StatusConflict, "run %d is not an open run of %s", id, a)
		}
		if err != nil {
			return err
		}
		return audit(r.Context(), tx, a, "agent_run.finish", taskRef(taskID), M{"run_id": id, "result": in})
	})
}

// requeueWorkerTasks puts a worker's in-flight tasks back on the queue,
// failing any that have used up their attempts.
func requeueWorkerTasks(ctx context.Context, tx pgx.Tx, a actor, workerID int64, reason string) error {
	tasks, err := collect[api.Task](ctx, tx, `
		SELECT * FROM tasks WHERE worker_id = $1
		AND status IN ('ASSIGNED', 'RUNNING', 'TESTING', 'REVIEWING') FOR UPDATE`, workerID)
	if err != nil {
		return err
	}
	for _, t := range tasks {
		to, msg := task.Pending, reason
		if t.Attempts >= task.MaxAttempts {
			to, msg = task.Failed, reason+"; attempts exhausted"
		}
		if err := move(ctx, tx, a, t, to, M{"reason": msg}); err != nil {
			return err
		}
		if to == task.Failed {
			if _, err := tx.Exec(ctx, `UPDATE tasks SET last_error = $2 WHERE id = $1`, t.ID, msg); err != nil {
				return err
			}
		}
	}
	return nil
}

// RunReaper marks silent workers offline and requeues their tasks until ctx ends (spec §30).
func (s *Server) RunReaper(ctx context.Context) {
	tick := time.NewTicker(s.cfg.StaleAfter / 2)
	defer tick.Stop()
	for {
		if err := s.ReapOnce(ctx); err != nil && ctx.Err() == nil {
			log.Printf("reaper: %v", err)
		}
		select {
		case <-ctx.Done():
			return
		case <-tick.C:
		}
	}
}

func (s *Server) ReapOnce(ctx context.Context) error {
	sys := actor{kind: "system"}
	return s.tx(ctx, func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx, `
			UPDATE workers SET status = 'offline'
			WHERE status = 'online' AND last_heartbeat_at < now() - make_interval(secs => $1)
			RETURNING id`, s.cfg.StaleAfter.Seconds())
		if err != nil {
			return err
		}
		ids, err := pgx.CollectRows(rows, pgx.RowTo[int64])
		if err != nil {
			return err
		}
		for _, id := range ids {
			if err := audit(ctx, tx, sys, "worker.offline", workerRef(id), nil); err != nil {
				return err
			}
			if err := requeueWorkerTasks(ctx, tx, sys, id, "worker heartbeat lost"); err != nil {
				return err
			}
		}
		return nil
	})
}

func workerRef(id int64) string   { return "worker:" + strconv.FormatInt(id, 10) }
func approvalRef(id int64) string { return "approval:" + strconv.FormatInt(id, 10) }
