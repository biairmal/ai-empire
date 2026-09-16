// Package controlplane is the authoritative AI Dev OS API (spec §6).
// All state lives in PostgreSQL; every mutation is audited in the same transaction.
package controlplane

import (
	"context"
	"crypto/subtle"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"strconv"
	"strings"
	"time"

	"aiempire/internal/api"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Config struct {
	OwnerToken   string
	WorkerToken  string
	KnowledgeDir string
	StaleAfter   time.Duration // worker considered dead after this long without a heartbeat
}

type Server struct {
	db  *pgxpool.Pool
	cfg Config
}

func New(db *pgxpool.Pool, cfg Config) (*Server, error) {
	if cfg.OwnerToken == "" || cfg.WorkerToken == "" || cfg.OwnerToken == cfg.WorkerToken {
		return nil, errors.New("owner and worker tokens must be set and different")
	}
	if cfg.StaleAfter <= 0 {
		cfg.StaleAfter = time.Minute
	}
	return &Server{db: db, cfg: cfg}, nil
}

type M = map[string]any

// actor is who is calling: the owner (human) or a worker.
type actor struct {
	kind     string // "owner" | "worker" | "system"
	workerID int64
}

func (a actor) String() string {
	if a.kind == "worker" {
		return fmt.Sprintf("worker:%d", a.workerID)
	}
	return a.kind
}

type role int

const (
	ownerOnly role = iota + 1
	workerOnly
	anyone
)

type apiFunc func(r *http.Request, a actor) (any, error)

type apiError struct {
	code int
	msg  string
}

func (e *apiError) Error() string { return e.msg }

func errf(code int, format string, args ...any) error {
	return &apiError{code, fmt.Sprintf(format, args...)}
}

func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, r *http.Request) { w.Write([]byte("ok")) })

	h := func(pattern string, rl role, fn apiFunc) { mux.Handle(pattern, s.wrap(rl, fn)) }

	// {ref} = slug or numeric id
	h("POST /clients", ownerOnly, s.createClient)
	h("GET /clients", anyone, s.listClients)
	h("GET /clients/{ref}", anyone, s.getClient)

	h("POST /projects", ownerOnly, s.createProject)
	h("GET /projects", anyone, s.listProjects)
	h("GET /projects/{ref}", anyone, s.getProject)
	h("PATCH /projects/{ref}", ownerOnly, s.updateProject)
	h("PATCH /projects/{ref}/repositories/{name}", ownerOnly, s.updateRepository)
	h("POST /projects/{ref}/client", ownerOnly, s.moveProject)
	h("POST /projects/{ref}/repositories", ownerOnly, s.createRepository)
	h("GET /projects/{ref}/repositories", anyone, s.listProjectRepositories)
	h("GET /repositories", anyone, s.listRepositories)

	h("POST /tasks", ownerOnly, s.createTask)
	h("GET /tasks", anyone, s.listTasks)
	h("GET /tasks/{id}", anyone, s.getTask)
	h("POST /tasks/{id}/cancel", ownerOnly, s.cancelTask)
	h("POST /tasks/{id}/retry", ownerOnly, s.retryTask)

	h("GET /approvals", anyone, s.listApprovals)
	h("GET /approvals/{id}", anyone, s.getApproval)
	h("POST /approvals/{id}/{decision}", ownerOnly, s.decide)

	h("GET /audit", ownerOnly, s.listAudit)
	h("GET /workers", anyone, s.listWorkers)

	// Worker protocol
	h("POST /workers/register", workerOnly, s.registerWorker)
	h("POST /workers/{id}/heartbeat", workerOnly, s.heartbeat)
	h("POST /workers/{id}/claim", workerOnly, s.claim)
	h("POST /tasks/{id}/transition", workerOnly, s.workerTransition)
	h("POST /tasks/{id}/authorize", workerOnly, s.authorize)
	h("GET /tasks/{id}/context", workerOnly, s.taskContext)
	h("POST /tasks/{id}/runs", workerOnly, s.startRun)
	h("POST /runs/{id}/finish", workerOnly, s.finishRun)

	return mux
}

func (s *Server) wrap(rl role, fn apiFunc) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		a, err := s.authenticate(r, rl)
		if err == nil {
			var out any
			if out, err = fn(r, a); err == nil {
				if out == nil {
					w.WriteHeader(http.StatusNoContent)
					return
				}
				writeJSON(w, http.StatusOK, out)
				return
			}
		}
		code, msg := toHTTP(err)
		if code == http.StatusInternalServerError {
			log.Printf("%s %s: %v", r.Method, r.URL.Path, err)
		}
		writeJSON(w, code, M{"error": msg})
	})
}

func (s *Server) authenticate(r *http.Request, rl role) (actor, error) {
	tok, _ := strings.CutPrefix(r.Header.Get("Authorization"), "Bearer ")
	eq := func(want string) bool { return subtle.ConstantTimeCompare([]byte(tok), []byte(want)) == 1 }
	switch {
	case eq(s.cfg.OwnerToken) && rl != workerOnly:
		return actor{kind: "owner"}, nil
	case eq(s.cfg.WorkerToken) && rl != ownerOnly:
		// ponytail: shared worker token, worker identity is self-declared; per-worker tokens in M2.4.
		id, _ := strconv.ParseInt(r.Header.Get("X-Worker-ID"), 10, 64)
		return actor{kind: "worker", workerID: id}, nil
	case eq(s.cfg.OwnerToken) || eq(s.cfg.WorkerToken):
		return actor{}, errf(http.StatusForbidden, "forbidden for this role")
	}
	return actor{}, errf(http.StatusUnauthorized, "unauthorized")
}

func toHTTP(err error) (int, string) {
	var ae *apiError
	var pe *pgconn.PgError
	switch {
	case errors.As(err, &ae):
		return ae.code, ae.msg
	case errors.Is(err, pgx.ErrNoRows):
		return http.StatusNotFound, "not found"
	case errors.As(err, &pe):
		switch pe.Code {
		case "23505":
			return http.StatusConflict, "already exists: " + pe.ConstraintName
		case "23503", "23514", "22P02", "23502":
			return http.StatusBadRequest, "invalid input: " + pe.Message
		}
	}
	return http.StatusInternalServerError, "internal error"
}

func writeJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	json.NewEncoder(w).Encode(v)
}

func decode(r *http.Request, v any) error {
	dec := json.NewDecoder(http.MaxBytesReader(nil, r.Body, 1<<20))
	dec.DisallowUnknownFields()
	if err := dec.Decode(v); err != nil {
		return errf(http.StatusBadRequest, "bad json: %v", err)
	}
	return nil
}

func pathID(r *http.Request) (int64, error) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		return 0, errf(http.StatusBadRequest, "bad id")
	}
	return id, nil
}

func (s *Server) tx(ctx context.Context, fn func(pgx.Tx) error) error {
	return pgx.BeginFunc(ctx, s.db, fn)
}

func audit(ctx context.Context, tx pgx.Tx, a actor, action, target string, payload M) error {
	if payload == nil {
		payload = M{}
	}
	id := a.kind
	if a.kind == "worker" {
		id = strconv.FormatInt(a.workerID, 10)
	}
	_, err := tx.Exec(ctx,
		`INSERT INTO audit_log (actor_type, actor_id, action, target, payload) VALUES ($1, $2, $3, $4, $5)`,
		a.kind, id, action, target, payload)
	return err
}

// querier is satisfied by both the pool and a transaction.
type querier interface {
	Query(context.Context, string, ...any) (pgx.Rows, error)
	QueryRow(context.Context, string, ...any) pgx.Row
}

func collect[T any](ctx context.Context, q querier, sql string, args ...any) ([]T, error) {
	rows, err := q.Query(ctx, sql, args...)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, pgx.RowToStructByName[T])
}

func one[T any](ctx context.Context, q querier, sql string, args ...any) (T, error) {
	rows, err := q.Query(ctx, sql, args...)
	if err != nil {
		var zero T
		return zero, err
	}
	return pgx.CollectExactlyOneRow(rows, pgx.RowToStructByName[T])
}

func (s *Server) listAudit(r *http.Request, a actor) (any, error) {
	return collect[api.AuditEntry](r.Context(), s.db,
		`SELECT * FROM audit_log WHERE ($1 = '' OR target = $1) ORDER BY id DESC LIMIT 200`,
		r.URL.Query().Get("target"))
}
