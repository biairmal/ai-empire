package controlplane

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"strconv"
	"strings"
	"time"

	"aiempire/internal/api"
	"aiempire/internal/docs"
	"aiempire/internal/task"

	"github.com/jackc/pgx/v5"
)

const documentGate = "document_approval"

func (s *Server) listDocuments(r *http.Request, a actor) (any, error) {
	repo, err := s.loadRepo()
	if err != nil {
		return nil, err
	}
	env, err := docEnv(r.Context(), s.db)
	if err != nil {
		return nil, err
	}
	q := r.URL.Query()
	out := []api.DocumentInfo{}
	for _, d := range repo.Docs {
		if p := q.Get("project"); p != "" && d.Scope != "projects/"+p {
			continue
		}
		if t := q.Get("type"); t != "" && d.Type != t {
			continue
		}
		out = append(out, docInfo(d, env))
	}
	return out, nil
}

// newDocument creates a document from its template with the next free id.
func (s *Server) newDocument(r *http.Request, a actor) (any, error) {
	var in api.NewDocument
	if err := decode(r, &in); err != nil {
		return nil, err
	}
	if in.Title == "" || in.Owner == "" {
		return nil, errf(http.StatusBadRequest, "title and owner are required")
	}
	s.kmu.Lock()
	defer s.kmu.Unlock()
	repo, err := s.loadRepo()
	if err != nil {
		return nil, err
	}
	c, ok := repo.Contracts[in.Type]
	if !ok || !slices.Contains(c.Scopes, "project") {
		return nil, errf(http.StatusBadRequest, "unknown project document type %q", in.Type)
	}
	p, err := projectByRef(r.Context(), s.db, in.Project)
	if err != nil {
		return nil, err
	}
	tpl, err := os.ReadFile(filepath.Join(s.cfg.KnowledgeDir, "templates", in.Type+".md"))
	if err != nil {
		return nil, errf(http.StatusInternalServerError, "no template for %s", in.Type)
	}
	id := repo.NextID("projects/"+p.Slug, c.IDPrefix)
	rel := c.DocPath(p.Slug, id, in.Title)
	d, err := docs.Parse(rel, tpl)
	if err != nil {
		return nil, err
	}
	today := time.Now().Format("2006-01-02")
	for k, v := range map[string]any{"id": id, "title": in.Title, "project": p.Slug, "owner": in.Owner, "created": today, "updated": today} {
		d.Set(k, v)
	}
	// Relationship placeholders become empty lists; the author fills them in.
	for _, k := range docs.RelKeys {
		if slices.ContainsFunc(d.Rels[k], func(ref string) bool { return strings.Contains(ref, "{{") }) {
			d.Set(k, []string{})
		}
	}
	d.Body = setHeading(d.Body, in.Title)
	var info api.DocumentInfo
	err = s.tx(r.Context(), func(tx pgx.Tx) error {
		if err := s.writeKnowledge(rel, d.Bytes()); err != nil {
			return err
		}
		info = docInfo(d, docs.Env{})
		return audit(r.Context(), tx, a, "document.create", "document:"+d.Key().String(), M{"path": rel, "type": in.Type})
	})
	return info, err
}

// setHeading replaces the template's placeholder H1 with the title.
func setHeading(body, title string) string {
	lines := strings.Split(body, "\n")
	for i, l := range lines {
		if strings.HasPrefix(l, "# ") {
			if strings.Contains(l, "{{") {
				prefix := ""
				if strings.HasPrefix(l, "# CR: ") {
					prefix = "CR: "
				}
				lines[i] = "# " + prefix + title
			}
			break
		}
	}
	return strings.Join(lines, "\n")
}

func (s *Server) validateDocuments(r *http.Request, a actor) (any, error) {
	var in struct {
		Project string `json:"project"`
	}
	if err := decode(r, &in); err != nil {
		return nil, err
	}
	repo, err := s.loadRepo()
	if err != nil {
		return nil, err
	}
	env, err := docEnv(r.Context(), s.db)
	if err != nil {
		return nil, err
	}
	out := []api.Problem{}
	for _, p := range repo.Validate(env) {
		if in.Project == "" || strings.HasPrefix(p.Path, "projects/"+in.Project+"/") {
			out = append(out, api.Problem{Path: "knowledge/" + p.Path, Message: p.Msg})
		}
	}
	return out, nil
}

func (s *Server) reindexDocuments(r *http.Request, a actor) (any, error) {
	s.kmu.Lock()
	defer s.kmu.Unlock()
	var n int
	err := s.tx(r.Context(), func(tx pgx.Tx) error {
		repo, env, err := s.repoEnv(r.Context(), tx)
		if err != nil {
			return err
		}
		n = len(repo.Docs)
		return s.reindex(r.Context(), tx, repo, env)
	})
	return M{"documents": n}, err
}

func (s *Server) repoEnv(ctx context.Context, q querier) (*docs.Repo, docs.Env, error) {
	repo, err := s.loadRepo()
	if err != nil {
		return nil, docs.Env{}, err
	}
	env, err := docEnv(ctx, q)
	return repo, env, err
}

// submitDocument sends a human-written document for approval.
func (s *Server) submitDocument(r *http.Request, a actor) (any, error) {
	var in api.DocumentRef
	if err := decode(r, &in); err != nil {
		return nil, err
	}
	s.kmu.Lock()
	defer s.kmu.Unlock()
	var out api.ApprovalRequest
	st := s.stage()
	err := s.tx(r.Context(), func(tx pgx.Tx) error {
		ctx := r.Context()
		repo, env, err := s.repoEnv(ctx, tx)
		if err != nil {
			return err
		}
		d, err := findDoc(repo, in.Ref, in.Project)
		if err != nil {
			return err
		}
		if env.IsApproved(d) {
			return errf(http.StatusConflict, "%s v%d is already approved", d.ID, d.Version)
		}
		d.Set("status", docs.PendingApproval)
		if ps := repo.ValidateDoc(d, env); len(ps) > 0 {
			return problemsError(ps)
		}
		var projectID *int64
		if slug, ok := strings.CutPrefix(d.Scope, "projects/"); ok {
			p, err := projectByRef(ctx, tx, slug)
			if err != nil {
				return err
			}
			projectID = &p.ID
		}
		// An owner submission edited after it was submitted can no longer be decided;
		// resubmitting closes that stale request. Unchanged content stays a 409 below.
		var staleID int64
		err = tx.QueryRow(ctx, `
			UPDATE approval_requests SET status = 'CHANGES_REQUESTED'
			WHERE subject_type = 'document' AND subject_ref = $1 AND gate = $2
			  AND status = 'PENDING_APPROVAL' AND task_id IS NULL AND subject_version <> $3
			RETURNING id`,
			d.Key().String(), documentGate, fmt.Sprintf("v%d@%s", d.Version, d.Hash())).Scan(&staleID)
		switch {
		case err == nil:
			if err := audit(ctx, tx, a, "approval.superseded", approvalRef(staleID), M{"reason": "document resubmitted after an edit"}); err != nil {
				return err
			}
		case !errors.Is(err, pgx.ErrNoRows):
			return err
		}
		out, err = s.openDocumentApproval(ctx, tx, a, repo, env, []*docs.Doc{d}, projectID, nil, "")
		if err != nil {
			return err
		}
		if err := st.write(d.Path, d.Bytes()); err != nil {
			return err
		}
		return s.reindexFresh(ctx, tx)
	})
	if err != nil {
		st.rollback()
	}
	return out, err
}

// reindexFresh reloads the repository from disk and reindexes it.
func (s *Server) reindexFresh(ctx context.Context, tx pgx.Tx) error {
	repo, env, err := s.repoEnv(ctx, tx)
	if err != nil {
		return err
	}
	return s.reindex(ctx, tx, repo, env)
}

// openDocumentApproval opens one approval request covering the given documents
// (the first is the primary). The caller has already validated them.
func (s *Server) openDocumentApproval(ctx context.Context, tx pgx.Tx, a actor, repo *docs.Repo, env docs.Env,
	ds []*docs.Doc, projectID, taskID *int64, agentSummary string) (api.ApprovalRequest, error) {
	primary := ds[0]
	c := repo.Contracts[primary.Type]
	var covers []string
	var b strings.Builder
	fmt.Fprintf(&b, "Approve %s %s v%d: %s\nFile: knowledge/%s\n", c.Title, primary.ID, primary.Version, primary.Title, primary.Path)
	for _, d := range ds {
		covers = append(covers, fmt.Sprintf("%s@v%d@%s", d.Key(), d.Version, d.Hash()))
		if d != primary {
			fmt.Fprintf(&b, "Also approves %s v%d: %s (knowledge/%s)\n", d.ID, d.Version, d.Title, d.Path)
		}
	}
	b.WriteString("Validation: passed\n")
	if agentSummary != "" {
		fmt.Fprintf(&b, "\nAgent summary:\n%s\n", agentSummary)
	}
	if primary.Type == "change-request" {
		for _, ref := range primary.Rels["affects"] {
			if t, err := repo.Resolve(primary, clientOf(env, primary), ref); err == nil {
				imp, err := s.impactOf(ctx, tx, repo, env, t)
				if err == nil {
					b.WriteString("\n" + imp.Report)
				}
			}
		}
	}
	req, err := one[api.ApprovalRequest](ctx, tx, `
		INSERT INTO approval_requests (project_id, task_id, subject_type, subject_ref, subject_version, gate, summary, requested_by, covers)
		VALUES ($1, $2, 'document', $3, $4, $5, $6, $7, $8) RETURNING *`,
		projectID, taskID, primary.Key().String(), fmt.Sprintf("v%d@%s", primary.Version, primary.Hash()),
		documentGate, b.String(), a.String(), covers)
	if err != nil {
		return req, err
	}
	return req, audit(ctx, tx, a, "approval.request", approvalRef(req.ID), M{"request": req})
}

type covered struct {
	key     docs.Key
	version int
	hash    string
}

func parseCovers(covers []string) ([]covered, error) {
	var out []covered
	for _, c := range covers {
		parts := strings.Split(c, "@")
		if len(parts) != 3 {
			return nil, fmt.Errorf("bad cover %q", c)
		}
		k, ok := docs.ParseKey(parts[0])
		v, err := strconv.Atoi(strings.TrimPrefix(parts[1], "v"))
		if !ok || err != nil {
			return nil, fmt.Errorf("bad cover %q", c)
		}
		out = append(out, covered{k, v, parts[2]})
	}
	return out, nil
}

// decideDocuments applies a human decision to the documents an approval covers (spec §8, §12).
func (s *Server) decideDocuments(ctx context.Context, tx pgx.Tx, a actor, req api.ApprovalRequest, status, comment string, st *staged) error {
	repo, env, err := s.repoEnv(ctx, tx)
	if err != nil {
		return err
	}
	covers, err := parseCovers(req.Covers)
	if err != nil {
		return err
	}
	var ds []*docs.Doc
	for _, c := range covers {
		d := repo.Get(c.key)
		if d == nil || d.Version != c.version || d.Hash() != c.hash {
			return errf(http.StatusConflict, "%s changed after it was submitted; review the new content and submit it again", c.key)
		}
		ds = append(ds, d)
	}

	var t *api.Task
	if req.TaskID != nil {
		tk, err := lockTask(ctx, tx, *req.TaskID)
		if err != nil {
			return err
		}
		t = &tk
	}

	fileStatus := map[string]string{"APPROVED": docs.Approved, "CHANGES_REQUESTED": docs.ChangesRequested, "REJECTED": docs.Rejected}[status]
	if status == "APPROVED" {
		for _, d := range ds {
			if _, err := tx.Exec(ctx, `
				INSERT INTO document_approvals (scope, doc_id, version, hash, approval_request_id, approved_by)
				VALUES ($1, $2, $3, $4, $5, $6)`, d.Scope, d.ID, d.Version, d.Hash(), req.ID, a.String()); err != nil {
				return err
			}
			if env.Approvals[d.Key()] == nil {
				env.Approvals[d.Key()] = map[int]string{}
			}
			env.Approvals[d.Key()][d.Version] = d.Hash()
		}
		for _, d := range ds {
			d.Set("status", docs.Approved)
		}
		for _, d := range ds {
			if ps := repo.ValidateDoc(d, env); len(ps) > 0 {
				return problemsError(ps)
			}
			for _, ref := range d.Rels["supersedes"] {
				if old, err := repo.Resolve(d, clientOf(env, d), ref); err == nil && old.Status != docs.Superseded {
					old.Set("status", docs.Superseded)
					if err := st.write(old.Path, old.Bytes()); err != nil {
						return err
					}
				}
			}
		}
	} else {
		for _, d := range ds {
			d.Set("status", fileStatus)
		}
	}
	for _, d := range ds {
		if err := audit(ctx, tx, a, "document."+fileStatus, "document:"+d.Key().String(),
			M{"version": d.Version, "hash": d.Hash(), "approval_id": req.ID, "comment": comment}); err != nil {
			return err
		}
	}

	if t != nil {
		note := M{"approval_id": req.ID}
		switch status {
		case "APPROVED":
			if err := move(ctx, tx, a, *t, task.Completed, note); err != nil {
				return err
			}
			if t.Kind == "plan" {
				if err := s.createPlanTasks(ctx, tx, a, *t, ds[0]); err != nil {
					return err
				}
			}
		case "CHANGES_REQUESTED":
			if err := move(ctx, tx, a, *t, task.Pending, note); err != nil {
				return err
			}
			if _, err := tx.Exec(ctx, `UPDATE tasks SET attempts = 0, feedback = $2 WHERE id = $1`, t.ID, comment); err != nil {
				return err
			}
		case "REJECTED":
			if err := move(ctx, tx, a, *t, task.Cancelled, note); err != nil {
				return err
			}
		}
		if t.RequestID != nil {
			if err := s.advance(ctx, tx, a, *t.RequestID); err != nil {
				return err
			}
		}
	}

	for _, d := range ds {
		if err := st.write(d.Path, d.Bytes()); err != nil {
			return err
		}
	}
	return s.reindexFresh(ctx, tx)
}

// exportDocuments renders a project's documents for hand-over to a client.
func (s *Server) exportDocuments(r *http.Request, a actor) (any, error) {
	ctx := r.Context()
	q := r.URL.Query()
	p, err := projectByRef(ctx, s.db, q.Get("project"))
	if err != nil {
		return nil, err
	}
	includeDrafts := q.Get("all") == "true"
	repo, env, err := s.repoEnv(ctx, s.db)
	if err != nil {
		return nil, err
	}
	type approval struct{ by, at string }
	approvals := map[string]approval{}
	rows, err := s.db.Query(ctx, `
		SELECT scope || '/' || doc_id || '@' || version, approved_by, to_char(approved_at, 'YYYY-MM-DD')
		FROM document_approvals WHERE scope = $1`, "projects/"+p.Slug)
	if err != nil {
		return nil, err
	}
	for rows.Next() {
		var k string
		var ap approval
		if err := rows.Scan(&k, &ap.by, &ap.at); err != nil {
			return nil, err
		}
		approvals[k] = ap
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	var files []api.File
	var index []string
	for _, d := range repo.Docs {
		if d.Scope != "projects/"+p.Slug || (!includeDrafts && !env.IsApproved(d)) {
			continue
		}
		c := repo.Contracts[d.Type]
		ctl := docs.Control{TypeTitle: c.Title, Project: p.Name}
		if ap, ok := approvals[fmt.Sprintf("%s@%d", d.Key(), d.Version)]; ok && env.IsApproved(d) {
			ctl.ApprovedBy, ctl.ApprovedAt = ap.by, ap.at
		}
		for _, rel := range docs.RelKeys {
			for _, ref := range d.Rels[rel] {
				if t, err := repo.Resolve(d, clientOf(env, d), ref); err == nil {
					ctl.Related = append(ctl.Related, fmt.Sprintf("%s — %s (%s)", t.ID, t.Title, strings.ReplaceAll(rel, "_", " ")))
				}
			}
		}
		name := filepath.Base(d.Path)
		files = append(files, api.File{Name: name, Content: d.ClientView(ctl)})
		index = append(index, fmt.Sprintf("| [%s](%s) | %s | %s | %d | %s |", d.ID, name, d.Title, c.Title, d.Version, map[bool]string{true: "Approved", false: "Draft"}[env.IsApproved(d)]))
	}
	sort.Strings(index)
	readme := fmt.Sprintf("# %s — Project Documentation\n\nExported %s.\n\n| Document | Title | Type | Version | Status |\n|----------|-------|------|---------|--------|\n%s\n",
		p.Name, time.Now().Format("2006-01-02"), strings.Join(index, "\n"))
	return append([]api.File{{Name: "README.md", Content: readme}}, files...), nil
}
