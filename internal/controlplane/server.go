// Package controlplane is the authoritative AI Dev OS API (spec §6).
// All state lives in PostgreSQL; every mutation is audited in the same transaction.
package controlplane

import (
	"context"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"maps"
	"net/http"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"

	"aiempire/internal/api"
	"aiempire/internal/docs"
	"aiempire/internal/workflow"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Config struct {
	OwnerToken   string
	WorkerToken  string // shared by workers without their own token; empty = per-worker tokens only
	HermesToken  string // the Hermes operator (V2); empty = disabled
	NotifyURL    string // ntfy topic URL for notifications (V2); empty = disabled
	NotifyToken  string // optional ntfy access token
	KnowledgeDir string
	WorkflowsDir string
	StaleAfter   time.Duration // worker considered dead after this long without a heartbeat
}

type Server struct {
	db        *pgxpool.Pool
	cfg       Config
	workflows map[string]workflow.Workflow

	// kmu serialises writes to the knowledge repository (load → change → write).
	// ponytail: one process-wide lock; per-project locks if document traffic grows.
	kmu sync.Mutex
}

func New(db *pgxpool.Pool, cfg Config) (*Server, error) {
	if cfg.OwnerToken == "" {
		return nil, errors.New("owner token must be set")
	}
	if cfg.OwnerToken == cfg.WorkerToken || cfg.OwnerToken == cfg.HermesToken ||
		(cfg.HermesToken != "" && cfg.HermesToken == cfg.WorkerToken) {
		return nil, errors.New("owner, worker and hermes tokens must be different")
	}
	if cfg.StaleAfter <= 0 {
		cfg.StaleAfter = time.Minute
	}
	repo, err := docs.Load(cfg.KnowledgeDir)
	if err != nil {
		return nil, fmt.Errorf("knowledge: %w", err)
	}
	wfs := map[string]workflow.Workflow{}
	if cfg.WorkflowsDir != "" {
		if wfs, err = workflow.Load(cfg.WorkflowsDir, slices.Collect(maps.Keys(repo.Contracts))); err != nil {
			return nil, fmt.Errorf("workflows: %w", err)
		}
	}
	return &Server{db: db, cfg: cfg, workflows: wfs}, nil
}

type M = map[string]any

// actor is who is calling: the owner (human), Hermes (the AI operator) or a worker.
type actor struct {
	kind       string // "owner" | "hermes" | "worker" | "system"
	via        string // "hermes" when Hermes relays an owner decision (it had the confirm code)
	workerID   int64
	workerName string // set when the worker authenticated with its own token
}

func (a actor) String() string {
	switch {
	case a.kind == "worker":
		return fmt.Sprintf("worker:%d", a.workerID)
	case a.via != "":
		return a.kind + " via " + a.via
	}
	return a.kind
}

type role int

const (
	ownerOnly  role = iota + 1
	operator        // owner or Hermes
	workerOnly      // workers never read the API: least privilege for remote machines (spec §34)
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
	h("GET /clients", operator, s.listClients)
	h("GET /clients/{ref}", operator, s.getClient)

	h("POST /projects", ownerOnly, s.createProject)
	h("GET /projects", operator, s.listProjects)
	h("GET /projects/{ref}", operator, s.getProject)
	h("PATCH /projects/{ref}", ownerOnly, s.updateProject)
	h("PATCH /projects/{ref}/repositories/{name}", ownerOnly, s.updateRepository)
	h("POST /projects/{ref}/client", ownerOnly, s.moveProject)
	h("POST /projects/{ref}/repositories", ownerOnly, s.createRepository)
	h("GET /projects/{ref}/repositories", operator, s.listProjectRepositories)
	h("GET /repositories", operator, s.listRepositories)

	h("POST /tasks", operator, s.createTask)
	h("GET /tasks", operator, s.listTasks)
	h("GET /tasks/{id}", operator, s.getTask)
	h("POST /tasks/{id}/cancel", operator, s.cancelTask)
	h("POST /tasks/{id}/retry", operator, s.retryTask)

	h("GET /approvals", operator, s.listApprovals)
	h("GET /approvals/{id}", operator, s.getApproval)
	h("POST /approvals/{id}/{decision}", operator, s.decide)

	h("GET /audit", operator, s.listAudit)
	h("GET /workers", operator, s.listWorkers)
	h("POST /workers", ownerOnly, s.createWorker)

	// V3: workflows (spec §7)
	h("GET /workflows", operator, s.listWorkflows)
	h("POST /requests", operator, s.createRequest)
	h("GET /requests", operator, s.listRequests)
	h("GET /requests/{id}", operator, s.getRequest)
	h("POST /requests/{id}/cancel", operator, s.cancelRequest)

	// V3: documents and the knowledge graph (spec §13–§19, §37, §38)
	h("GET /documents", operator, s.listDocuments)
	h("POST /documents", ownerOnly, s.newDocument)
	h("POST /documents/validate", operator, s.validateDocuments)
	h("POST /documents/submit", ownerOnly, s.submitDocument)
	h("POST /documents/reindex", ownerOnly, s.reindexDocuments)
	h("GET /documents/export", ownerOnly, s.exportDocuments)
	h("GET /trace", operator, s.trace)
	h("GET /impact", operator, s.impact)

	// Worker protocol
	h("POST /workers/register", workerOnly, s.registerWorker)
	h("POST /workers/{id}/heartbeat", workerOnly, s.heartbeat)
	h("POST /workers/{id}/claim", workerOnly, s.claim)
	h("POST /tasks/{id}/transition", workerOnly, s.workerTransition)
	h("POST /tasks/{id}/authorize", workerOnly, s.authorize)
	h("GET /tasks/{id}/context", workerOnly, s.taskContext)
	h("POST /tasks/{id}/runs", workerOnly, s.startRun)
	h("POST /runs/{id}/finish", workerOnly, s.finishRun)
	h("POST /tasks/{id}/output", workerOnly, s.taskOutput)
	h("POST /tasks/{id}/review", workerOnly, s.taskReview)

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
	eq := func(want string) bool {
		return want != "" && subtle.ConstantTimeCompare([]byte(tok), []byte(want)) == 1
	}
	var a actor
	switch {
	case eq(s.cfg.OwnerToken):
		a.kind = "owner"
	case eq(s.cfg.HermesToken):
		a.kind = "hermes"
	case eq(s.cfg.WorkerToken):
		// The shared token's worker identity is self-declared, so it may not act as a worker with its own token.
		a.kind = "worker"
		a.workerID, _ = strconv.ParseInt(r.Header.Get("X-Worker-ID"), 10, 64)
		var own bool
		err := s.db.QueryRow(r.Context(), `SELECT token_hash IS NOT NULL FROM workers WHERE id = $1`, a.workerID).Scan(&own)
		if err != nil && !errors.Is(err, pgx.ErrNoRows) {
			return a, err
		}
		if own {
			return a, errf(http.StatusForbidden, "worker %d has its own token", a.workerID)
		}
	case tok != "":
		err := s.db.QueryRow(r.Context(), `SELECT id, name FROM workers WHERE token_hash = $1`, hashToken(tok)).
			Scan(&a.workerID, &a.workerName)
		if errors.Is(err, pgx.ErrNoRows) {
			return a, errf(http.StatusUnauthorized, "unauthorized")
		}
		if err != nil {
			return a, err
		}
		a.kind = "worker"
	default:
		return a, errf(http.StatusUnauthorized, "unauthorized")
	}
	allowed := map[role]bool{ownerOnly: a.kind == "owner", operator: a.kind != "worker", workerOnly: a.kind == "worker"}
	if !allowed[rl] {
		return actor{}, errf(http.StatusForbidden, "forbidden for this role")
	}
	return a, nil
}

func hashToken(tok string) string {
	sum := sha256.Sum256([]byte(tok))
	return hex.EncodeToString(sum[:])
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
	dec := json.NewDecoder(http.MaxBytesReader(nil, r.Body, 8<<20)) // documents can be large
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
	typ, id := a.kind, a.kind
	switch {
	case a.kind == "worker":
		id = strconv.FormatInt(a.workerID, 10)
	case a.via != "":
		typ, id = a.via, a.String()
	}
	q := `INSERT INTO audit_log (actor_type, actor_id, action, target, payload) VALUES ($1, $2, $3, $4, $5)`
	if notifies(action, payload) {
		// Queue the notification in the event's own transaction (outbox, spec §39).
		q = `WITH a AS (` + q + ` RETURNING id) INSERT INTO notifications (audit_id) SELECT id FROM a`
	}
	_, err := tx.Exec(ctx, q, typ, id, action, target, payload)
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
