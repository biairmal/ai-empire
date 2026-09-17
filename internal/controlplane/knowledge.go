package controlplane

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"slices"
	"strings"

	"aiempire/internal/api"
	"aiempire/internal/docs"

	"github.com/jackc/pgx/v5"
)

// The knowledge repository is Markdown on disk (the source of truth for
// content); the database holds who approved which version and a graph index.

func (s *Server) loadRepo() (*docs.Repo, error) {
	// ponytail: full re-scan per call; fine for hundreds of documents, add caching if it shows up in profiles.
	return docs.Load(s.cfg.KnowledgeDir)
}

// docEnv is what the validator needs from the database.
func docEnv(ctx context.Context, q querier) (docs.Env, error) {
	env := docs.Env{ProjectClient: map[string]string{}, Approvals: map[docs.Key]map[int]string{}}
	rows, err := q.Query(ctx, `SELECT p.slug, coalesce(c.slug, '') FROM projects p LEFT JOIN clients c ON c.id = p.client_id`)
	if err != nil {
		return env, err
	}
	for rows.Next() {
		var p, c string
		if err := rows.Scan(&p, &c); err != nil {
			return env, err
		}
		env.ProjectClient[p] = c
	}
	if err := rows.Err(); err != nil {
		return env, err
	}
	rows, err = q.Query(ctx, `SELECT scope, doc_id, version, hash FROM document_approvals`)
	if err != nil {
		return env, err
	}
	for rows.Next() {
		var k docs.Key
		var v int
		var h string
		if err := rows.Scan(&k.Scope, &k.ID, &v, &h); err != nil {
			return env, err
		}
		if env.Approvals[k] == nil {
			env.Approvals[k] = map[int]string{}
		}
		env.Approvals[k][v] = h
	}
	return env, rows.Err()
}

func clientOf(env docs.Env, d *docs.Doc) string {
	return env.ProjectClient[strings.TrimPrefix(d.Scope, "projects/")]
}

// writeKnowledge writes a file inside the knowledge root only.
func (s *Server) writeKnowledge(rel string, data []byte) error {
	root, err := os.OpenRoot(s.cfg.KnowledgeDir)
	if err != nil {
		return err
	}
	defer root.Close()
	if err := root.MkdirAll(path.Dir(rel), 0o755); err != nil {
		return err
	}
	return root.WriteFile(rel, data, 0o644)
}

func (s *Server) removeKnowledge(rel string) error {
	root, err := os.OpenRoot(s.cfg.KnowledgeDir)
	if err != nil {
		return err
	}
	defer root.Close()
	return root.Remove(rel)
}

func (s *Server) readKnowledge(rel string) (string, error) {
	root, err := os.OpenRoot(s.cfg.KnowledgeDir)
	if err != nil {
		return "", err
	}
	defer root.Close()
	b, err := root.ReadFile(rel)
	return string(b), err
}

// staged writes files and can undo them if the surrounding operation fails.
type staged struct {
	s      *Server
	backup map[string][]byte // nil value = file did not exist
}

func (s *Server) stage() *staged { return &staged{s: s, backup: map[string][]byte{}} }

func (st *staged) write(rel string, data []byte) error {
	if _, seen := st.backup[rel]; !seen {
		old, err := st.s.readKnowledge(rel)
		if err == nil {
			st.backup[rel] = []byte(old)
		} else {
			st.backup[rel] = nil
		}
	}
	return st.s.writeKnowledge(rel, data)
}

func (st *staged) rollback() {
	for rel, old := range st.backup {
		if old == nil {
			st.s.removeKnowledge(rel)
		} else {
			st.s.writeKnowledge(rel, old)
		}
	}
}

// reindex regenerates the Obsidian relations blocks and rebuilds the graph index (spec §14, §20).
func (s *Server) reindex(ctx context.Context, tx pgx.Tx, repo *docs.Repo, env docs.Env) error {
	for _, d := range repo.Docs {
		if d.ID == "" || d.ID == docs.NewID {
			continue
		}
		before := d.Body
		d.SetRelations(func(ref string) *docs.Doc {
			t, _ := repo.Resolve(d, clientOf(env, d), ref)
			return t
		})
		if d.Body != before {
			if err := s.writeKnowledge(d.Path, d.Bytes()); err != nil {
				return err
			}
		}
	}
	if _, err := tx.Exec(ctx, `DELETE FROM doc_edges; DELETE FROM documents`); err != nil {
		return err
	}
	for _, d := range repo.Docs {
		if d.ID == "" || d.ID == docs.NewID || repo.Get(d.Key()) != d {
			continue
		}
		if _, err := tx.Exec(ctx, `
			INSERT INTO documents (scope, doc_id, type, title, path, status, version, hash)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8)`,
			d.Scope, d.ID, d.Type, d.Title, d.Path, d.Status, d.Version, d.Hash()); err != nil {
			return err
		}
		for rel, refs := range d.Rels {
			for _, ref := range refs {
				t, err := repo.Resolve(d, clientOf(env, d), ref)
				if err != nil {
					continue // the validator reports it
				}
				if _, err := tx.Exec(ctx, `
					INSERT INTO doc_edges (from_scope, from_id, rel, to_scope, to_id)
					VALUES ($1, $2, $3, $4, $5) ON CONFLICT DO NOTHING`,
					d.Scope, d.ID, rel, t.Scope, t.ID); err != nil {
					return err
				}
			}
		}
	}
	return nil
}

func docInfo(d *docs.Doc, env docs.Env) api.DocumentInfo {
	return api.DocumentInfo{
		Key: d.Key().String(), Scope: d.Scope, ID: d.ID, Type: d.Type, Title: d.Title,
		Path: d.Path, Status: d.Status, Version: d.Version, Approved: env.IsApproved(d),
	}
}

// findDoc accepts a knowledge-relative path ("projects/x/requirements/PRD-001-a.md",
// optionally prefixed with "knowledge/"), a key ("projects/x/PRD-001"), or a bare
// id together with a project.
func findDoc(repo *docs.Repo, ref, project string) (*docs.Doc, error) {
	ref = strings.TrimPrefix(filepath.ToSlash(strings.TrimSpace(ref)), "knowledge/")
	if strings.HasSuffix(ref, ".md") {
		for _, d := range repo.Docs {
			if d.Path == ref {
				return d, nil
			}
		}
		return nil, errf(http.StatusNotFound, "no document at knowledge/%s", ref)
	}
	if k, ok := docs.ParseKey(ref); ok {
		if d := repo.Get(k); d != nil {
			return d, nil
		}
		return nil, errf(http.StatusNotFound, "no document %s", ref)
	}
	if project == "" {
		return nil, errf(http.StatusBadRequest, "give a path, a key like projects/<slug>/%s, or a project", ref)
	}
	if d := repo.Get(docs.Key{Scope: "projects/" + project, ID: ref}); d != nil {
		return d, nil
	}
	return nil, errf(http.StatusNotFound, "no document %s in project %s", ref, project)
}

// projectRel turns a project document path into the project-relative form used in task context_docs.
func projectRel(d *docs.Doc) string {
	return strings.TrimPrefix(d.Path, d.Scope+"/")
}

func problemStrings(ps []docs.Problem) []string {
	out := make([]string, len(ps))
	for i, p := range ps {
		out[i] = fmt.Sprintf("knowledge/%s: %s", p.Path, p.Msg)
	}
	return out
}

func problemsError(ps []docs.Problem) error {
	return errf(http.StatusUnprocessableEntity, "document is not valid:\n  %s", strings.Join(problemStrings(ps), "\n  "))
}

func docsKey(s string) (docs.Key, bool) { return docs.ParseKey(s) }

func pathBase(p string) string { return path.Base(p) }

// expandDocs follows the knowledge graph from a task's listed project documents
// (spec §22): the requirements they satisfy, decisions they depend on, specs that
// implement them, and test plans. Paths are project-relative.
func expandDocs(repo *docs.Repo, env docs.Env, project, client string, paths []string) []string {
	var start []*docs.Doc
	for _, p := range paths {
		for _, d := range repo.Docs {
			if d.Scope == "projects/"+project && projectRel(d) == p {
				start = append(start, d)
			}
		}
	}
	out := slices.Clone(paths)
	for _, d := range repo.ExpandContext(start, client, 3) {
		if p := projectRel(d); !slices.Contains(out, p) {
			out = append(out, p)
		}
	}
	return out
}

// Reindex rebuilds the graph index and Obsidian links from the files on disk.
// Run at startup so edits made while the control plane was down are picked up.
func (s *Server) Reindex(ctx context.Context) error {
	s.kmu.Lock()
	defer s.kmu.Unlock()
	return s.tx(ctx, func(tx pgx.Tx) error { return s.reindexFresh(ctx, tx) })
}
