package controlplane

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"strconv"
	"strings"
	"time"

	"aiempire/internal/api"
	"aiempire/internal/task"

	"github.com/jackc/pgx/v5"
)

// notifies reports whether an audited event is announced to the owner (spec §39).
// audit() queues it in the notifications outbox; RunNotifier sends it.
func notifies(action string, payload M) bool {
	switch action {
	case "approval.request", "worker.offline", "request.completed":
		return true
	case "task.status":
		return payload["to"] == task.Failed || payload["to"] == task.Completed
	}
	return false
}

// RunNotifier sends queued notifications until ctx ends. Without a NotifyURL it does nothing.
func (s *Server) RunNotifier(ctx context.Context) {
	if s.cfg.NotifyURL == "" {
		return
	}
	tick := time.NewTicker(2 * time.Second)
	defer tick.Stop()
	for {
		if err := s.NotifyOnce(ctx); err != nil && ctx.Err() == nil {
			log.Printf("notifier: %v", err)
		}
		select {
		case <-ctx.Done():
			return
		case <-tick.C:
		}
	}
}

type outboxRow struct {
	ID      int64  `db:"id"`
	Action  string `db:"action"`
	Target  string `db:"target"`
	Payload M      `db:"payload"`
}

// NotifyOnce sends what is due. A failed send is retried with backoff; anything a day old is dropped.
func (s *Server) NotifyOnce(ctx context.Context) error {
	return s.tx(ctx, func(tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, `
			UPDATE notifications SET sent_at = now(), last_error = 'expired'
			WHERE sent_at IS NULL AND created_at < now() - interval '1 day'`); err != nil {
			return err
		}
		rows, err := collect[outboxRow](ctx, tx, `
			SELECT n.id, a.action, a.target, a.payload
			FROM notifications n JOIN audit_log a ON a.id = n.audit_id
			WHERE n.sent_at IS NULL AND n.next_attempt_at <= now()
			ORDER BY n.id LIMIT 20
			FOR UPDATE OF n SKIP LOCKED`)
		if err != nil {
			return err
		}
		for _, n := range rows {
			title, body, err := s.render(ctx, tx, n)
			if err == nil && title != "" {
				err = s.send(ctx, title, body)
			}
			if err != nil {
				log.Printf("notification %d: %v", n.ID, err)
				_, err = tx.Exec(ctx, `
					UPDATE notifications SET attempts = attempts + 1, last_error = $2,
					       next_attempt_at = now() + make_interval(secs => least(power(2, attempts), 3600))
					WHERE id = $1`, n.ID, err.Error())
			} else {
				_, err = tx.Exec(ctx, `UPDATE notifications SET sent_at = now() WHERE id = $1`, n.ID)
			}
			if err != nil {
				return err
			}
		}
		return nil
	})
}

// render turns an audit event into a short message. An empty title means "don't send".
func (s *Server) render(ctx context.Context, tx pgx.Tx, n outboxRow) (title, body string, err error) {
	_, idStr, _ := strings.Cut(n.Target, ":")
	id, _ := strconv.ParseInt(idStr, 10, 64)
	switch n.Action {
	case "approval.request":
		req, err := one[api.ApprovalRequest](ctx, tx, `SELECT * FROM approval_requests WHERE id = $1`, id)
		if err != nil || req.Status != "PENDING_APPROVAL" {
			return "", "", err // already decided: nothing to ask
		}
		summary, _, _ := strings.Cut(req.Summary, "\n")
		return fmt.Sprintf("Approval #%d needed (%s)", id, req.Gate),
			fmt.Sprintf("%s %s: %s\n\nempire approve %d\nor tell Hermes: approve %d, code %s",
				req.SubjectType, req.SubjectRef, clip(summary, 300), id, id, s.confirmCode(id)), nil
	case "task.status":
		t, err := one[api.Task](ctx, tx, `SELECT * FROM tasks WHERE id = $1`, id)
		if err != nil {
			return "", "", err
		}
		if n.Payload["to"] == task.Completed {
			if t.RequestID != nil {
				return "", "", nil // workflow steps: the request's completion is announced instead
			}
			return fmt.Sprintf("Task #%d completed", id), t.Title, nil
		}
		var fails int
		if err := tx.QueryRow(ctx, `SELECT count(*) FROM audit_log
			WHERE target = $1 AND action = 'task.status' AND payload->>'to' = 'FAILED'`, n.Target).Scan(&fails); err != nil {
			return "", "", err
		}
		title = fmt.Sprintf("Task #%d failed", id)
		if fails > 1 {
			title = fmt.Sprintf("Task #%d failed again (%d times)", id, fails)
		}
		return title, t.Title + "\n\n" + clip(t.LastError, 500), nil
	case "worker.offline":
		var name string
		if err := tx.QueryRow(ctx, `SELECT name FROM workers WHERE id = $1`, id).Scan(&name); err != nil {
			return "", "", err
		}
		return fmt.Sprintf("Worker %s is offline", name), "Its tasks went back on the queue.", nil
	case "request.completed":
		req, err := one[api.Request](ctx, tx, `SELECT * FROM requests WHERE id = $1`, id)
		if err != nil {
			return "", "", err
		}
		return fmt.Sprintf("Request #%d completed", id), req.Title, nil
	}
	return "", "", nil
}

// send posts one message to ntfy (https://docs.ntfy.sh/publish/).
func (s *Server) send(ctx context.Context, title, body string) error {
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, "POST", s.cfg.NotifyURL, strings.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Title", title)
	if s.cfg.NotifyToken != "" {
		req.Header.Set("Authorization", "Bearer "+s.cfg.NotifyToken)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	resp.Body.Close()
	if resp.StatusCode >= 300 {
		return fmt.Errorf("ntfy: %s", resp.Status)
	}
	return nil
}

func clip(s string, n int) string {
	if r := []rune(s); len(r) > n {
		return string(r[:n]) + "…"
	}
	return s
}
