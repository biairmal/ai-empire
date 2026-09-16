package controlplane

import (
	"net/http"
	"strconv"

	"aiempire/internal/api"
	"aiempire/internal/policy"
	"aiempire/internal/task"

	"github.com/jackc/pgx/v5"
)

func (s *Server) listApprovals(r *http.Request, a actor) (any, error) {
	return collect[api.ApprovalRequest](r.Context(), s.db, `
		SELECT * FROM approval_requests WHERE ($1 = '' OR status::text = $1)
		ORDER BY id DESC LIMIT 200`, r.URL.Query().Get("status"))
}

func (s *Server) getApproval(r *http.Request, a actor) (any, error) {
	id, err := pathID(r)
	if err != nil {
		return nil, err
	}
	return one[api.ApprovalRequest](r.Context(), s.db, `SELECT * FROM approval_requests WHERE id = $1`, id)
}

var decisions = map[string]string{
	"approve":         "APPROVED",
	"request-changes": "CHANGES_REQUESTED",
	"reject":          "REJECTED",
}

// decide records a human decision and resumes, reworks or cancels the gated task (spec §8).
func (s *Server) decide(r *http.Request, a actor) (any, error) {
	id, err := pathID(r)
	if err != nil {
		return nil, err
	}
	status, ok := decisions[r.PathValue("decision")]
	if !ok {
		return nil, errf(http.StatusNotFound, "unknown decision %q", r.PathValue("decision"))
	}
	var in api.Decision
	if err := decode(r, &in); err != nil {
		return nil, err
	}
	if status == "CHANGES_REQUESTED" && in.Comment == "" {
		return nil, errf(http.StatusBadRequest, "request-changes needs a comment")
	}

	return nil, s.tx(r.Context(), func(tx pgx.Tx) error {
		ctx := r.Context()
		req, err := one[api.ApprovalRequest](ctx, tx, `SELECT * FROM approval_requests WHERE id = $1 FOR UPDATE`, id)
		if err != nil {
			return err
		}
		if req.Status != "PENDING_APPROVAL" {
			return errf(http.StatusConflict, "approval %d is already %s", id, req.Status)
		}
		if !policy.CanDecide(req.RequestedBy, a.String()) {
			return errf(http.StatusForbidden, "%s may not decide a request made by %s", a, req.RequestedBy)
		}
		if _, err := tx.Exec(ctx, `
			INSERT INTO approval_decisions (approval_request_id, decision, comment, decided_by) VALUES ($1, $2, $3, $4)`,
			id, status, in.Comment, a.String()); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `UPDATE approval_requests SET status = $2 WHERE id = $1`, id, status); err != nil {
			return err
		}
		if err := audit(ctx, tx, a, "approval.decide", approvalRef(id), M{"decision": status, "comment": in.Comment}); err != nil {
			return err
		}
		if req.SubjectType != "task" {
			return nil
		}

		taskID, err := strconv.ParseInt(req.SubjectRef, 10, 64)
		if err != nil {
			return err
		}
		t, err := lockTask(ctx, tx, taskID)
		if err != nil {
			return err
		}
		note := M{"approval_id": id}
		switch status {
		case "APPROVED":
			if err := move(ctx, tx, a, t, task.Pending, note); err != nil {
				return err
			}
			_, err = tx.Exec(ctx, `UPDATE tasks SET attempts = 0 WHERE id = $1`, t.ID)
		case "CHANGES_REQUESTED":
			if err := move(ctx, tx, a, t, task.Pending, note); err != nil {
				return err
			}
			_, err = tx.Exec(ctx, `UPDATE tasks SET attempts = 0, stage = $2, feedback = $3 WHERE id = $1`,
				t.ID, task.StageImplement, in.Comment)
		case "REJECTED":
			err = move(ctx, tx, a, t, task.Cancelled, note)
		}
		return err
	})
}
