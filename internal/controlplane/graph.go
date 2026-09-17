package controlplane

import (
	"context"
	"fmt"
	"maps"
	"net/http"
	"slices"
	"strconv"
	"strings"

	"aiempire/internal/api"
	"aiempire/internal/docs"
)

// The knowledge graph (spec §14, §37, §38). Documents point at each other
// through typed relationships; tasks point at documents (context and output);
// merged tasks point at commits.

// Relationships where the SOURCE depends on the TARGET: changing the target impacts the source.
var dependsRels = []string{"satisfies", "derived_from", "depends_on", "implements", "documents", "references", "contradicts"}

// Relationships where changing the SOURCE impacts the TARGET.
var impactsRels = []string{"affects", "tested_by", "contradicts"}

// Upstream relationships answer "why does this exist?".
var whyRels = []string{"satisfies", "implements", "derived_from", "depends_on"}

type edge struct {
	from, to *docs.Doc
	rel      string
}

func edges(repo *docs.Repo, env docs.Env) []edge {
	var out []edge
	for _, d := range repo.Docs {
		for rel, refs := range d.Rels {
			for _, ref := range refs {
				if t, err := repo.Resolve(d, clientOf(env, d), ref); err == nil {
					out = append(out, edge{d, t, rel})
				}
			}
		}
	}
	return out
}

func (s *Server) impact(r *http.Request, a actor) (any, error) {
	ctx := r.Context()
	repo, env, err := s.repoEnv(ctx, s.db)
	if err != nil {
		return nil, err
	}
	d, err := findDoc(repo, r.URL.Query().Get("doc"), r.URL.Query().Get("project"))
	if err != nil {
		return nil, err
	}
	return s.impactOf(ctx, s.db, repo, env, d)
}

// impactOf lists everything that may need to change if d changes (spec §37).
func (s *Server) impactOf(ctx context.Context, q querier, repo *docs.Repo, env docs.Env, d *docs.Doc) (api.Impact, error) {
	out := api.Impact{Document: docInfo(d, env), Documents: []api.ImpactedDoc{}, Tasks: []api.Task{}}
	all := edges(repo, env)
	seen := map[docs.Key]bool{d.Key(): true}
	type item struct {
		doc   *docs.Doc
		depth int
	}
	queue := []item{{d, 0}}
	for len(queue) > 0 {
		cur := queue[0]
		queue = queue[1:]
		if cur.depth >= 6 {
			continue
		}
		for _, e := range all {
			var next *docs.Doc
			var via string
			switch {
			case e.to.Key() == cur.doc.Key() && slices.Contains(dependsRels, e.rel):
				next, via = e.from, fmt.Sprintf("%s %s %s", e.from.ID, strings.ReplaceAll(e.rel, "_", " "), cur.doc.ID)
			case e.from.Key() == cur.doc.Key() && slices.Contains(impactsRels, e.rel):
				next, via = e.to, fmt.Sprintf("%s %s %s", cur.doc.ID, strings.ReplaceAll(e.rel, "_", " "), e.to.ID)
			}
			if next == nil || seen[next.Key()] || next.Status == docs.Superseded || next.Status == docs.Rejected {
				continue
			}
			seen[next.Key()] = true
			out.Documents = append(out.Documents, api.ImpactedDoc{DocumentInfo: docInfo(next, env), Via: via, Depth: cur.depth + 1})
			queue = append(queue, item{next, cur.depth + 1})
		}
	}

	// Tasks built from, or producing, any affected document.
	var paths, keys []string
	var projectSlug string
	for k := range seen {
		doc := repo.Get(k)
		keys = append(keys, k.String())
		if slug, ok := strings.CutPrefix(k.Scope, "projects/"); ok {
			paths = append(paths, projectRel(doc))
			if k == d.Key() {
				projectSlug = slug
			}
		}
	}
	if projectSlug != "" {
		tasks, err := collect[api.Task](ctx, q, `
			SELECT t.* FROM tasks t JOIN projects p ON p.id = t.project_id
			WHERE p.slug = $1 AND t.status <> 'CANCELLED'
			  AND (t.context_docs && $2 OR t.output_docs && $3)
			ORDER BY t.id`, projectSlug, paths, keys)
		if err != nil {
			return out, err
		}
		out.Tasks = tasks
	}

	var b strings.Builder
	fmt.Fprintf(&b, "Impact analysis for %s v%d (%s):\n", d.ID, d.Version, d.Title)
	if len(out.Documents) == 0 {
		b.WriteString("- No other documents depend on it.\n")
	}
	for _, x := range out.Documents {
		state := "not approved"
		if x.Approved {
			state = "approved — needs a new version if affected"
		}
		fmt.Fprintf(&b, "- %s v%d %s [%s] — via %s\n", x.ID, x.Version, x.Title, state, x.Via)
	}
	if len(out.Tasks) > 0 {
		b.WriteString("Work built on these documents:\n")
		files := map[string][]string{} // repository → files
		for _, t := range out.Tasks {
			repo := s.repoName(ctx, q, t)
			line := fmt.Sprintf("- task #%d %s [%s]", t.ID, t.Title, t.Status)
			if repo != "" {
				line = fmt.Sprintf("- task #%d %s: %s [%s]", t.ID, repo, t.Title, t.Status)
			}
			if t.MergeSHA != "" {
				line += " merged " + short(t.MergeSHA)
			}
			b.WriteString(line + "\n")
			for _, f := range t.ChangedFiles {
				if !slices.Contains(files[repo], f) {
					files[repo] = append(files[repo], f)
				}
			}
		}
		if len(files) > 0 {
			b.WriteString("Code files that may need to change:\n")
			for _, repo := range slices.Sorted(maps.Keys(files)) {
				slices.Sort(files[repo])
				for _, f := range files[repo] {
					fmt.Fprintf(&b, "- %s: %s\n", repo, f)
				}
			}
		}
	}
	out.Report = b.String()
	return out, nil
}

// trace answers "why does this exist?" and "what depends on it?" (spec §38).
// ref: "task:N", "request:N", "commit:SHA", a document key/path, or an id with ?project=.
func (s *Server) trace(r *http.Request, a actor) (any, error) {
	ctx := r.Context()
	q := r.URL.Query()
	ref := strings.TrimSpace(q.Get("ref"))
	repo, env, err := s.repoEnv(ctx, s.db)
	if err != nil {
		return nil, err
	}
	all := edges(repo, env)
	var out api.Trace

	kind, val, _ := strings.Cut(ref, ":")
	switch kind {
	case "commit", "task":
		var t api.Task
		if kind == "commit" {
			if len(val) < 7 {
				return nil, errf(http.StatusBadRequest, "give at least 7 characters of the commit")
			}
			t, err = one[api.Task](ctx, s.db, `
				SELECT t.* FROM tasks t WHERE t.merge_sha LIKE $1 || '%'
				   OR t.id IN (SELECT subject_ref::bigint FROM approval_requests
				               WHERE subject_type = 'task' AND subject_version LIKE $1 || '%')
				ORDER BY t.id DESC LIMIT 1`, strings.ToLower(val))
			if err != nil {
				return nil, errf(http.StatusNotFound, "no task produced commit %s", val)
			}
		} else {
			id, _ := strconv.ParseInt(val, 10, 64)
			if t, err = one[api.Task](ctx, s.db, `SELECT * FROM tasks WHERE id = $1`, id); err != nil {
				return nil, err
			}
		}
		node := s.taskNode(ctx, t)
		out.Subject = node
		if kind == "commit" {
			out.Subject = api.TraceNode{Kind: "commit", Ref: val, Title: "commit " + val, Children: []api.TraceNode{node}}
		}
		out.Why = s.taskWhy(ctx, repo, all, t)
		out.Effects = commitNodes(t)
		for _, key := range t.OutputDocs {
			if k, ok := docs.ParseKey(key); ok && repo.Get(k) != nil {
				out.Effects = append(out.Effects, downstream(repo.Get(k), all, map[docs.Key]bool{}, 0)...)
			}
		}

	case "request":
		id, _ := strconv.ParseInt(val, 10, 64)
		req, err := one[api.Request](ctx, s.db, `SELECT * FROM requests WHERE id = $1`, id)
		if err != nil {
			return nil, err
		}
		out.Subject = requestNode(req)
		tasks, err := collect[api.Task](ctx, s.db, `SELECT * FROM tasks WHERE request_id = $1 ORDER BY id`, id)
		if err != nil {
			return nil, err
		}
		for _, t := range tasks {
			n := s.taskNode(ctx, t)
			n.Children = append(n.Children, commitNodes(t)...)
			out.Effects = append(out.Effects, n)
		}

	default:
		d, err := findDoc(repo, ref, q.Get("project"))
		if err != nil {
			return nil, err
		}
		out.Subject = docNode(d, "")
		out.Why = upstream(d, all, map[docs.Key]bool{d.Key(): true}, 0)
		if n, ok := s.producedBy(ctx, d); ok {
			out.Why = append(out.Why, n)
		}
		out.Effects = downstream(d, all, map[docs.Key]bool{d.Key(): true}, 0)
		imp, err := s.impactOf(ctx, s.db, repo, env, d)
		if err != nil {
			return nil, err
		}
		for _, t := range imp.Tasks {
			if slices.Contains(t.OutputDocs, d.Key().String()) {
				continue // the task that wrote it is upstream
			}
			n := s.taskNode(ctx, t)
			n.Children = commitNodes(t)
			out.Effects = append(out.Effects, n)
		}
	}
	return out, nil
}

func docNode(d *docs.Doc, via string) api.TraceNode {
	return api.TraceNode{Kind: "document", Ref: d.Key().String(), Title: fmt.Sprintf("%s v%d %s", d.ID, d.Version, d.Title), Status: d.Status, Via: via}
}

func requestNode(r api.Request) api.TraceNode {
	title := r.Title
	if r.Description != "" {
		title += " — " + truncate(strings.ReplaceAll(r.Description, "\n", " "), 160)
	}
	return api.TraceNode{Kind: "request", Ref: fmt.Sprintf("request:%d", r.ID), Title: title, Status: r.Status}
}

func (s *Server) taskNode(ctx context.Context, t api.Task) api.TraceNode {
	title := t.Title
	if repo := s.repoName(ctx, s.db, t); repo != "" {
		title = repo + ": " + title
	}
	return api.TraceNode{Kind: "task", Ref: fmt.Sprintf("task:%d", t.ID), Title: title, Status: t.Status}
}

func (s *Server) repoName(ctx context.Context, q querier, t api.Task) string {
	var name string
	if t.RepositoryID != nil {
		q.QueryRow(ctx, `SELECT name FROM repositories WHERE id = $1`, *t.RepositoryID).Scan(&name)
	}
	return name
}

func commitNodes(t api.Task) []api.TraceNode {
	if t.MergeSHA == "" {
		return nil
	}
	n := api.TraceNode{Kind: "commit", Ref: "commit:" + t.MergeSHA, Title: "merged as " + short(t.MergeSHA), Via: "merged"}
	for _, f := range t.ChangedFiles {
		n.Children = append(n.Children, api.TraceNode{Kind: "file", Ref: "file:" + f, Title: f, Via: "changed"})
	}
	return []api.TraceNode{n}
}

// upstream follows why-relationships: satisfies, implements, derived_from, depends_on.
func upstream(d *docs.Doc, all []edge, seen map[docs.Key]bool, depth int) []api.TraceNode {
	var out []api.TraceNode
	if depth > 6 {
		return nil
	}
	for _, e := range all {
		if e.from.Key() != d.Key() || !slices.Contains(whyRels, e.rel) || seen[e.to.Key()] {
			continue
		}
		seen[e.to.Key()] = true
		n := docNode(e.to, strings.ReplaceAll(e.rel, "_", " "))
		n.Children = upstream(e.to, all, seen, depth+1)
		out = append(out, n)
	}
	return out
}

// downstream follows the reverse: documents that satisfy / implement / derive from d.
func downstream(d *docs.Doc, all []edge, seen map[docs.Key]bool, depth int) []api.TraceNode {
	var out []api.TraceNode
	if depth > 6 {
		return nil
	}
	for _, e := range all {
		if e.to.Key() != d.Key() || !slices.Contains(dependsRels, e.rel) || seen[e.from.Key()] {
			continue
		}
		seen[e.from.Key()] = true
		n := docNode(e.from, strings.ReplaceAll(e.rel, "_", " ")+" "+d.ID)
		n.Children = downstream(e.from, all, seen, depth+1)
		out = append(out, n)
	}
	return out
}

// taskWhy: the request a task belongs to, and the documents it was built from.
func (s *Server) taskWhy(ctx context.Context, repo *docs.Repo, all []edge, t api.Task) []api.TraceNode {
	var out []api.TraceNode
	seen := map[docs.Key]bool{}
	for _, rel := range t.ContextDocs {
		for _, d := range repo.Docs {
			if d.Scope == "projects/"+s.projectSlug(ctx, t.ProjectID) && projectRel(d) == rel && !seen[d.Key()] {
				seen[d.Key()] = true
				n := docNode(d, "built from")
				n.Children = upstream(d, all, seen, 0)
				out = append(out, n)
			}
		}
	}
	if t.RequestID != nil {
		if req, err := one[api.Request](ctx, s.db, `SELECT * FROM requests WHERE id = $1`, *t.RequestID); err == nil {
			n := requestNode(req)
			n.Via = "requested by the owner"
			out = append(out, n)
		}
	}
	return out
}

func (s *Server) producedBy(ctx context.Context, d *docs.Doc) (api.TraceNode, bool) {
	t, err := one[api.Task](ctx, s.db, `SELECT * FROM tasks WHERE $1 = ANY(output_docs) ORDER BY id LIMIT 1`, d.Key().String())
	if err != nil || t.RequestID == nil {
		return api.TraceNode{}, false
	}
	req, err := one[api.Request](ctx, s.db, `SELECT * FROM requests WHERE id = $1`, *t.RequestID)
	if err != nil {
		return api.TraceNode{}, false
	}
	n := requestNode(req)
	n.Via = fmt.Sprintf("written by task #%d (%s)", t.ID, t.Role)
	return n, true
}

func (s *Server) projectSlug(ctx context.Context, id int64) string {
	var slug string
	s.db.QueryRow(ctx, `SELECT slug FROM projects WHERE id = $1`, id).Scan(&slug)
	return slug
}

func short(sha string) string { return sha[:min(12, len(sha))] }
