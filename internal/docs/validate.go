package docs

import (
	"fmt"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"slices"
	"sort"
	"strings"
	"time"
)

// Env is what the validator needs to know beyond the files themselves.
type Env struct {
	// ProjectClient maps project slug → client slug ("" = no client).
	ProjectClient map[string]string
	// Approvals are the authoritative approved versions from the control plane:
	// key → version → content hash. nil skips approval-record checks.
	Approvals map[Key]map[int]string
}

func (e Env) approvedHash(k Key, version int) (string, bool) {
	h, ok := e.Approvals[k][version]
	return h, ok
}

// IsApproved reports whether d's current content is an approved version.
func (e Env) IsApproved(d *Doc) bool {
	if d.Status != Approved {
		return false
	}
	if e.Approvals == nil {
		return true
	}
	h, ok := e.approvedHash(d.Key(), d.Version)
	return ok && h == d.Hash()
}

var (
	idRe          = regexp.MustCompile(`^[A-Z][A-Z0-9]*-\d{3,}$`)
	placeholderRe = regexp.MustCompile(`\{\{[^{}]*\}\}`)
	commentRe     = regexp.MustCompile(`(?s)<!--.*?-->`)
	numberingRe   = regexp.MustCompile(`^\d+(\.\d+)*\.?\s+`)
	mdLinkRe      = regexp.MustCompile(`\[[^\]]*\]\(([^)\s]+)(?:\s+"[^"]*")?\)`)
	wikiLinkRe    = regexp.MustCompile(`\[\[([^\]|#]+)`)
)

// Validate checks every document in the repository.
func (r *Repo) Validate(env Env) []Problem {
	probs := append([]Problem(nil), r.Problems...)
	seen := map[Key]string{}
	for _, d := range r.Docs {
		if d.ID != "" {
			if first, dup := seen[d.Key()]; dup {
				probs = append(probs, Problem{d.Path, fmt.Sprintf("id %s is already used by %s in the same scope", d.ID, first)})
			} else {
				seen[d.Key()] = d.Path
			}
		}
		probs = append(probs, r.ValidateDoc(d, env)...)
	}
	sort.SliceStable(probs, func(i, j int) bool { return probs[i].Path < probs[j].Path })
	return probs
}

// ValidateDoc checks one document against its contract and the rest of the repository.
func (r *Repo) ValidateDoc(d *Doc, env Env) []Problem {
	var msgs []string
	add := func(format string, args ...any) { msgs = append(msgs, fmt.Sprintf(format, args...)) }

	msgs = append(msgs, d.fmErr...)
	c, ok := r.Contracts[d.Type]
	if !ok {
		if d.Type == "" {
			add("missing `type`")
		} else {
			add("unknown type %q (see knowledge/contracts/)", d.Type)
		}
		return toProblems(d, msgs)
	}

	// Identity and lifecycle.
	switch {
	case d.ID == "" || d.ID == NewID:
		add("missing `id` (the platform assigns one on submit, or use `empire docs new`)")
	case !idRe.MatchString(d.ID) || !strings.HasPrefix(d.ID, c.IDPrefix+"-"):
		add("id %q must look like %s-001", d.ID, c.IDPrefix)
	}
	if d.Title == "" {
		add("missing `title`")
	}
	if !statuses[d.Status] {
		add("status %q must be one of draft, pending_approval, approved, changes_requested, rejected, superseded", d.Status)
	}
	if d.Version < 1 {
		add("`version` must be 1 or higher")
	}
	for _, k := range c.RequiredMetadata {
		if !d.Has(k) {
			add("missing required metadata `%s`", k)
		}
	}
	for _, k := range dateKeys {
		if v := d.Str(k); v != "" && !IsDate(v) && !placeholderRe.MatchString(v) {
			add("`%s: %s` must be a date written as YYYY-MM-DD", k, v)
		}
	}

	// Scope and location.
	kind, slug, _ := strings.Cut(d.Scope, "/")
	scopeName := map[string]string{"projects": "project", "clients": "client", "stacks": "stack", "global": "global", "roles": "roles"}[kind]
	if !slices.Contains(c.Scopes, scopeName) {
		add("a %s may not live in %s/ (allowed: %s)", c.Title, kind, strings.Join(c.Scopes, ", "))
	}
	for key, want := range map[string]string{"project": "projects", "client": "clients", "stack": "stacks"} {
		got := d.Str(key)
		switch {
		case kind == want && got != slug:
			add("`%s: %s` is required for a document in %s", key, slug, d.Scope)
		case kind != want && d.Has(key):
			add("`%s` is only allowed in %s/ documents", key, want)
		}
	}
	if kind == "projects" && c.Folder != "" {
		if want := path.Join("projects", slug, c.Folder); path.Dir(d.Path) != want {
			add("a %s belongs in %s/", c.Title, want)
		}
	}

	// Relationships.
	client := env.ProjectClient[slug]
	for _, k := range d.Keys() {
		if isRelKey(k) {
			if _, allowed := c.Relationships[k]; !allowed && len(d.Rels[k]) > 0 {
				add("relationship `%s` is not allowed for a %s", k, c.Title)
			}
		}
	}
	for rel, refs := range d.Rels {
		allowed := c.Relationships[rel]
		for _, ref := range refs {
			if ref == d.ID {
				add("`%s` refers to the document itself", rel)
				continue
			}
			t, err := r.Resolve(d, client, ref)
			if err != nil {
				add("`%s`: %v", rel, err)
				continue
			}
			if allowed != nil && !matchType(allowed, t.Type) {
				add("`%s: %s` must point to %s, not a %s", rel, ref, strings.Join(allowed, " / "), t.Type)
			}
		}
	}

	// Body.
	secs := Sections(d.Body)
	for _, want := range c.RequiredSections {
		content, found := secs[normSection(want)]
		switch {
		case !found:
			add("missing section \"## %s\"", want)
		case strings.TrimSpace(commentRe.ReplaceAllString(content, "")) == "":
			add("section \"%s\" is empty (write \"Not applicable\" and why, if it does not apply)", want)
		}
	}
	if m := placeholderRe.FindString(marshal(d.fm) + d.Body); m != "" {
		add("unfilled template placeholder %s", m)
	}
	if d.Type == "implementation-plan" {
		if _, err := ParsePlan(d.Body, nil); err != nil {
			add("work breakdown: %v", err)
		}
	}
	msgs = append(msgs, r.checkLinks(d)...)

	// Approval records and versioning (spec §12).
	if env.Approvals != nil {
		msgs = append(msgs, r.checkVersioning(d, env)...)
	}

	// Upstream documents must be approved before this one goes for approval.
	if d.Status == PendingApproval || d.Status == Approved {
		for _, u := range c.Upstream {
			n := 0
			for _, ref := range d.Rels[u.Relationship] {
				if t, err := r.Resolve(d, client, ref); err == nil && matchType(u.Types, t.Type) && env.IsApproved(t) {
					n++
				}
			}
			if n < u.Min {
				add("needs at least %d approved %s in `%s` before it can go for approval", u.Min, strings.Join(u.Types, " / "), u.Relationship)
			}
		}
	}
	return toProblems(d, msgs)
}

func (r *Repo) checkVersioning(d *Doc, env Env) []string {
	var msgs []string
	versions := env.Approvals[d.Key()]
	latest := 0
	for v := range versions {
		latest = max(latest, v)
	}
	hash, recorded := versions[d.Version]
	switch {
	case d.Status == Approved && !recorded:
		msgs = append(msgs, fmt.Sprintf("marked approved, but version %d has no approval record; submit it with `empire docs submit`", d.Version))
	case recorded && hash != d.Hash():
		msgs = append(msgs, fmt.Sprintf("approved version %d was modified in place; restore it and create version %d through an approved change request", d.Version, latest+1))
	case latest > 0 && d.Version < latest:
		msgs = append(msgs, fmt.Sprintf("version %d is older than the approved version %d", d.Version, latest))
	case latest > 0 && d.Version > latest+1:
		msgs = append(msgs, fmt.Sprintf("version %d skips versions; the next version is %d", d.Version, latest+1))
	case latest > 0 && d.Version == latest+1 && !r.authorisedByCR(d, env):
		msgs = append(msgs, fmt.Sprintf("version %d changes an approved document; add `derived_from: [CR-…]` pointing to an approved change request that lists %s in `affects`", d.Version, d.ID))
	}
	return msgs
}

// authorisedByCR reports whether an approved change request in derived_from affects d.
func (r *Repo) authorisedByCR(d *Doc, env Env) bool {
	client := env.ProjectClient[strings.TrimPrefix(d.Scope, "projects/")]
	for _, ref := range d.Rels["derived_from"] {
		cr, err := r.Resolve(d, client, ref)
		if err != nil || cr.Type != "change-request" || !env.IsApproved(cr) {
			continue
		}
		for _, a := range cr.Rels["affects"] {
			if t, err := r.Resolve(cr, client, a); err == nil && t.Key() == d.Key() {
				return true
			}
		}
	}
	return false
}

func (r *Repo) checkLinks(d *Doc) []string {
	var msgs []string
	body := commentRe.ReplaceAllString(StripRelations(d.Body), "")
	for _, m := range mdLinkRe.FindAllStringSubmatch(body, -1) {
		target := m[1]
		if strings.Contains(target, "://") || strings.HasPrefix(target, "#") || strings.HasPrefix(target, "mailto:") {
			continue
		}
		target, _, _ = strings.Cut(target, "#")
		p := filepath.Join(r.Root, filepath.FromSlash(path.Dir(d.Path)), filepath.FromSlash(target))
		if _, err := os.Stat(p); err != nil {
			msgs = append(msgs, fmt.Sprintf("broken link %q", m[1]))
		}
	}
	for _, m := range wikiLinkRe.FindAllStringSubmatch(body, -1) {
		name := strings.TrimSpace(m[1])
		if !r.hasWikiTarget(name) {
			msgs = append(msgs, fmt.Sprintf("broken wikilink [[%s]]", name))
		}
	}
	return msgs
}

func (r *Repo) hasWikiTarget(name string) bool {
	for _, d := range r.Docs {
		if d.Base() == name || d.ID == name || strings.TrimSuffix(d.Path, ".md") == name {
			return true
		}
	}
	return false
}

var dateKeys = []string{"created", "updated", "decision_date", "release_date"}

// IsDate reports whether s is a calendar date in YYYY-MM-DD form.
func IsDate(s string) bool {
	_, err := time.Parse("2006-01-02", s)
	return err == nil
}

func toProblems(d *Doc, msgs []string) []Problem {
	ps := make([]Problem, len(msgs))
	for i, m := range msgs {
		ps[i] = Problem{d.Path, m}
	}
	return ps
}

func normSection(s string) string {
	return strings.ToLower(strings.TrimSpace(numberingRe.ReplaceAllString(strings.TrimSpace(s), "")))
}

// Sections maps each level-2 heading (normalised) to its content, ignoring
// headings inside fenced code blocks.
func Sections(body string) map[string]string {
	out := map[string]string{}
	cur, inFence := "", false
	var buf strings.Builder
	flush := func() {
		if cur != "" {
			out[cur] += buf.String()
		}
		buf.Reset()
	}
	for line := range strings.SplitSeq(body, "\n") {
		if strings.HasPrefix(strings.TrimSpace(line), "```") {
			inFence = !inFence
		}
		if !inFence && strings.HasPrefix(line, "## ") {
			flush()
			cur = normSection(line[3:])
			if _, ok := out[cur]; !ok {
				out[cur] = ""
			}
			continue
		}
		buf.WriteString(line + "\n")
	}
	flush()
	return out
}
