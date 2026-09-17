package docs

import (
	"os"
	"path"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

// realKnowledge is the repository's own knowledge folder: tests run against the
// real contracts and templates, so template/contract drift fails the build.
const realKnowledge = "../../knowledge"

func newRepo(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	files, _ := filepath.Glob(filepath.Join(realKnowledge, "contracts", "*.yaml"))
	if len(files) == 0 {
		t.Fatal("no contracts found")
	}
	os.MkdirAll(filepath.Join(root, "contracts"), 0o755)
	for _, f := range files {
		data, _ := os.ReadFile(f)
		os.WriteFile(filepath.Join(root, "contracts", filepath.Base(f)), data, 0o644)
	}
	return root
}

// exampleDoc writes a valid example of typ into project `proj` and returns its path.
func exampleDoc(t *testing.T, root, proj, typ, id string, edit func(*Doc)) string {
	t.Helper()
	tpl, err := os.ReadFile(filepath.Join(realKnowledge, "templates", typ+".md"))
	if err != nil {
		t.Fatal(err)
	}
	repo, err := Load(root)
	if err != nil {
		t.Fatal(err)
	}
	c := repo.Contracts[typ]
	rel := c.DocPath(proj, id, "Example "+typ)
	d, err := Example(rel, tpl)
	if err != nil {
		t.Fatal(err)
	}
	d.Set("id", id)
	d.Set("title", "Example "+typ)
	d.Set("project", proj)
	if typ == "implementation-plan" {
		d.SetWorkBreakdown([]PlanTask{{Key: "api", Repository: "backend", Title: "Add endpoint", Description: "Do it."}})
	}
	if edit != nil {
		edit(d)
	}
	write(t, root, rel, d.Bytes())
	return rel
}

func write(t *testing.T, root, rel string, data []byte) {
	t.Helper()
	p := filepath.Join(root, filepath.FromSlash(rel))
	os.MkdirAll(filepath.Dir(p), 0o755)
	if err := os.WriteFile(p, data, 0o644); err != nil {
		t.Fatal(err)
	}
}

func load(t *testing.T, root string) *Repo {
	t.Helper()
	r, err := Load(root)
	if err != nil {
		t.Fatal(err)
	}
	return r
}

func messages(ps []Problem) string {
	var s []string
	for _, p := range ps {
		s = append(s, p.String())
	}
	return strings.Join(s, "\n")
}

func expectProblem(t *testing.T, ps []Problem, want string) {
	t.Helper()
	if !strings.Contains(messages(ps), want) {
		t.Errorf("expected a problem containing %q, got:\n%s", want, messages(ps))
	}
}

func TestEveryTemplateSatisfiesItsContract(t *testing.T) {
	root := newRepo(t)
	contracts := load(t, root).Contracts
	for typ, c := range contracts {
		if _, err := os.Stat(filepath.Join(realKnowledge, "templates", typ+".md")); err != nil {
			if slices.Contains(c.Scopes, "project") {
				t.Errorf("contract %s has no template", typ)
			}
			continue
		}
		raw, _ := os.ReadFile(filepath.Join(realKnowledge, "templates", typ+".md"))
		tpl, err := Parse("templates/"+typ+".md", raw)
		if err != nil {
			t.Fatalf("template %s: %v", typ, err)
		}
		for _, k := range append([]string{"type", "id", "title", "status", "version"}, c.RequiredMetadata...) {
			if !slices.Contains(tpl.Keys(), k) {
				t.Errorf("template %s lacks front matter key %s", typ, k)
			}
		}
		secs := Sections(tpl.Body)
		for _, s := range c.RequiredSections {
			if _, ok := secs[normSection(s)]; !ok {
				t.Errorf("template %s lacks section %q", typ, s)
			}
		}
		if !placeholderRe.MatchString(string(raw)) {
			t.Errorf("template %s has no {{ }} placeholders to guide the author", typ)
		}
		exampleDoc(t, root, "shop", typ, c.IDPrefix+"-001", nil)
	}
	if ps := load(t, root).Validate(Env{}); len(ps) > 0 {
		t.Errorf("filled templates should validate:\n%s", messages(ps))
	}
}

func TestValidatorCatchesProblems(t *testing.T) {
	root := newRepo(t)
	exampleDoc(t, root, "shop", "prd", "PRD-001", nil)
	exampleDoc(t, root, "other", "prd", "PRD-001", nil)
	write(t, root, "clients/globex/decisions/ADR-001-x.md", []byte("---\ntype: adr\nid: ADR-001\ntitle: X\nclient: globex\nstatus: draft\nversion: 1\nowner: me\n---\n"))
	exampleDoc(t, root, "shop", "technical-design", "td-1", func(d *Doc) {
		d.Set("satisfies", []string{"PRD-009", "projects/other/PRD-001", "clients/globex/ADR-001"})
		d.Set("affects", []string{"PRD-001"})
		d.Set("contradicts", []string{"PRD-001"})
		d.Set("owner", "")
		d.Set("created", "NEW")
		d.Body = strings.Replace(d.Body, "## Observability", "## Observability\n\n<!-- only a comment -->\n\n## Something", 1)
		d.Body = strings.Replace(d.Body, "## Failure Scenarios", "## Failure Modes", 1)
		d.Body += "\nSee {{ nothing }} and [spec](missing.md) and [[NOPE-1]].\n"
	})
	write(t, root, "projects/shop/requirements/bad.md", []byte("no front matter"))
	write(t, root, "projects/shop/requirements/wrong-folder.md", []byte("---\ntype: adr\nid: ADR-001\ntitle: T\nproject: shop\nstatus: done\nversion: 0\nowner: me\n---\n"))

	ps := load(t, root).Validate(Env{ProjectClient: map[string]string{"shop": "acme"}})
	for _, want := range []string{
		`id "td-1" must look like TD-001`,
		"missing required metadata `owner`",
		`"PRD-009" does not exist`,
		`"projects/other/PRD-001" is outside the scopes`,
		`"clients/globex/ADR-001" is outside the scopes`,
		"`affects: PRD-001` must point to",
		"relationship `contradicts` is not allowed",
		`missing section "## Failure Scenarios"`,
		`section "Observability" is empty`,
		"unfilled template placeholder {{ nothing }}",
		`broken link "missing.md"`,
		"broken wikilink [[NOPE-1]]",
		"missing YAML front matter",
		"belongs in projects/shop/decisions/",
		`status "done" must be one of`,
		"`version` must be 1 or higher",
	} {
		expectProblem(t, ps, want)
	}
}

func TestVersioningAndUpstream(t *testing.T) {
	root := newRepo(t)
	prdPath := exampleDoc(t, root, "shop", "prd", "PRD-001", func(d *Doc) { d.Set("status", Approved) })
	tdPath := exampleDoc(t, root, "shop", "technical-design", "TD-001", func(d *Doc) {
		d.Set("status", PendingApproval)
		d.Set("satisfies", []string{"PRD-001"})
	})
	r := load(t, root)
	prd := r.Get(Key{"projects/shop", "PRD-001"})
	env := Env{Approvals: map[Key]map[int]string{}}

	// Approved in front matter but no record; the design's upstream is not approved either.
	ps := r.Validate(env)
	expectProblem(t, ps, "has no approval record")
	expectProblem(t, ps, "needs at least 1 approved prd in `satisfies`")

	env.Approvals[prd.Key()] = map[int]string{1: prd.Hash()}
	if ps := r.Validate(env); len(ps) > 0 {
		t.Fatalf("approved PRD + pending design should validate:\n%s", messages(ps))
	}

	// Status changes and relation blocks do not invalidate the approval.
	prd.Set("status", Superseded)
	prd.SetRelations(func(string) *Doc { return nil })
	if h := prd.Hash(); h != env.Approvals[prd.Key()][1] {
		t.Error("hash must ignore status and the relations block")
	}

	// Editing the approved version in place is refused.
	prd.Set("status", Approved)
	prd.Body += "\nA silent change.\n"
	write(t, root, prdPath, prd.Bytes())
	expectProblem(t, load(t, root).Validate(env), "approved version 1 was modified in place")

	// A new version needs an approved change request that affects it.
	prd.Set("version", 2)
	prd.Set("status", Draft)
	write(t, root, prdPath, prd.Bytes())
	expectProblem(t, load(t, root).Validate(env), "add `derived_from: [CR-…]`")

	crPath := exampleDoc(t, root, "shop", "change-request", "CR-001", func(d *Doc) {
		d.Set("status", Approved)
		d.Set("affects", []string{"PRD-001"})
	})
	prd.Set("derived_from", []string{"CR-001"})
	write(t, root, prdPath, prd.Bytes())
	r = load(t, root)
	cr := r.Get(Key{"projects/shop", "CR-001"})
	env.Approvals[cr.Key()] = map[int]string{1: cr.Hash()}
	ps = r.Validate(env)
	if strings.Contains(messages(ps), "PRD-001") && strings.Contains(messages(ps), prdPath+":") {
		t.Errorf("v2 authorised by an approved CR should validate:\n%s", messages(ps))
	}
	// …but the design now points at a PRD whose current version is not approved.
	expectProblem(t, ps, tdPath+": needs at least 1 approved prd")
	_ = crPath
}

func TestExpandContext(t *testing.T) {
	root := newRepo(t)
	exampleDoc(t, root, "shop", "prd", "PRD-001", func(d *Doc) { d.Set("tested_by", []string{"TP-001"}) })
	exampleDoc(t, root, "shop", "prd", "PRD-002", func(d *Doc) { d.Set("status", Superseded) })
	exampleDoc(t, root, "shop", "test-plan", "TP-001", nil)
	exampleDoc(t, root, "shop", "adr", "ADR-001", nil)
	exampleDoc(t, root, "shop", "technical-design", "TD-001", func(d *Doc) {
		d.Set("satisfies", []string{"PRD-001", "PRD-002"})
		d.Set("depends_on", []string{"ADR-001"})
	})
	exampleDoc(t, root, "shop", "api-spec", "API-001", func(d *Doc) { d.Set("implements", []string{"TD-001"}) })
	exampleDoc(t, root, "shop", "implementation-plan", "PLAN-001", func(d *Doc) { d.Set("derived_from", []string{"TD-001"}) })
	exampleDoc(t, root, "shop", "runbook", "RB-001", nil) // unrelated
	exampleDoc(t, root, "other", "prd", "PRD-001", nil)   // other project
	r := load(t, root)

	got := r.ExpandContext([]*Doc{r.Get(Key{"projects/shop", "PLAN-001"})}, "", 3)
	var ids []string
	for _, d := range got {
		ids = append(ids, d.ID)
	}
	for _, want := range []string{"PLAN-001", "TD-001", "PRD-001", "ADR-001", "API-001", "TP-001"} {
		if !slices.Contains(ids, want) {
			t.Errorf("expanded context lacks %s: %v", want, ids)
		}
	}
	for _, bad := range []string{"PRD-002", "RB-001"} {
		if slices.Contains(ids, bad) {
			t.Errorf("expanded context must not contain %s: %v", bad, ids)
		}
	}
	for _, d := range got {
		if d.Scope != "projects/shop" {
			t.Errorf("expanded into another scope: %s", d.Key())
		}
	}
	if n := len(r.ExpandContext([]*Doc{r.Get(Key{"projects/shop", "PLAN-001"})}, "", 1)); n != 2 {
		t.Errorf("depth 1 should reach only the design: %d docs", n)
	}
}

func TestParsePlan(t *testing.T) {
	d := &Doc{Body: "## Work Breakdown\n"}
	d.SetWorkBreakdown([]PlanTask{
		{Key: "fe", Repository: "frontend", Title: "UI", Description: "d", DependsOn: []string{"be"}},
		{Key: "be", Repository: "backend", Title: "API", Description: "d", DependsOn: []string{"sdk"}},
		{Key: "sdk", Repository: "backend", Title: "Lib", Description: "d"},
	})
	tasks, err := ParsePlan(d.Body, []string{"backend", "frontend"})
	if err != nil {
		t.Fatal(err)
	}
	var order []string
	for _, tk := range tasks {
		order = append(order, tk.Key)
	}
	if !slices.Equal(order, []string{"sdk", "be", "fe"}) {
		t.Errorf("order = %v", order)
	}
	if _, err := ParsePlan(d.Body, []string{"backend"}); err == nil || !strings.Contains(err.Error(), `unknown repository "frontend"`) {
		t.Errorf("unknown repo: %v", err)
	}

	d.SetWorkBreakdown([]PlanTask{
		{Key: "a", Repository: "r", Title: "A", Description: "d", DependsOn: []string{"b"}},
		{Key: "b", Repository: "r", Title: "B", Description: "d", DependsOn: []string{"a"}},
	})
	if _, err := ParsePlan(d.Body, nil); err == nil || !strings.Contains(err.Error(), "cycle") {
		t.Errorf("cycle: %v", err)
	}
	d.SetWorkBreakdown([]PlanTask{{Key: "Bad Key", Title: strings.Repeat("x", 80)}, {Key: "Bad Key"}})
	_, err = ParsePlan(d.Body, nil)
	for _, want := range []string{"key must be lowercase", "duplicate key", "at most 72", "description is required", "repository is required"} {
		if err == nil || !strings.Contains(err.Error(), want) {
			t.Errorf("want %q in %v", want, err)
		}
	}
	if _, err := ParsePlan("## Work Breakdown\nno yaml", nil); err == nil {
		t.Error("missing yaml block must fail")
	}
}

func TestResolveVisibility(t *testing.T) {
	root := newRepo(t)
	for _, rel := range []string{"clients/acme/x.md", "clients/globex/x.md", "stacks/go/x.md", "stacks/node/x.md", "global/x.md"} {
		scope := ScopeOf(rel)
		key := ""
		if k, v, ok := strings.Cut(scope, "/"); ok {
			key = strings.TrimSuffix(k, "s") + ": " + v + "\n"
		}
		write(t, root, rel, []byte("---\ntype: adr\nid: ADR-001\ntitle: T\n"+key+"status: draft\nversion: 1\n---\n"))
	}
	exampleDoc(t, root, "shop", "prd", "PRD-001", nil)
	r := load(t, root)
	from := r.Get(Key{"projects/shop", "PRD-001"})

	if d, err := r.Resolve(from, "acme", "ADR-001"); err != nil || d.Scope != "clients/acme" {
		t.Errorf("client scope should win over stacks and global: %v %v", d, err)
	}
	if d, err := r.Resolve(from, "", "global/ADR-001"); err != nil || d.Scope != "global" {
		t.Errorf("qualified global: %v %v", d, err)
	}
	if _, err := r.Resolve(from, "", "ADR-001"); err == nil || !strings.Contains(err.Error(), "several stacks") {
		t.Errorf("ambiguous stacks should ask to qualify: %v", err)
	}
	if _, err := r.Resolve(from, "acme", "clients/globex/ADR-001"); err == nil {
		t.Error("another client's documents must be invisible")
	}
	if _, err := r.Resolve(from, "", "clients/acme/ADR-001"); err == nil {
		t.Error("a client-less project must not see client documents")
	}
}

func TestClientViewAndIDs(t *testing.T) {
	root := newRepo(t)
	rel := exampleDoc(t, root, "shop", "prd", "PRD-001", func(d *Doc) {
		d.Body = "<!-- guidance -->\n" + d.Body
		d.Set("owner", "Jane | PM")
		d.SetRelations(func(string) *Doc { return nil })
	})
	exampleDoc(t, root, "shop", "prd", "PRD-007", nil)
	r := load(t, root)
	d := r.Get(Key{"projects/shop", "PRD-001"})
	out := d.ClientView(Control{TypeTitle: "Product Requirements Document", Project: "Shop", ApprovedBy: "owner", ApprovedAt: "2026-01-01"})
	for _, bad := range []string{"---\ntype:", "<!--", "relations:start"} {
		if strings.Contains(out, bad) {
			t.Errorf("client view contains %q", bad)
		}
	}
	for _, want := range []string{"# Example\n\n## Document Control", "| Document | PRD-001 — Product Requirements Document |", "| Owner | Jane \\| PM |", "| Approved | 2026-01-01 by owner |"} {
		if !strings.Contains(out, want) {
			t.Errorf("client view lacks %q:\n%s", want, out)
		}
	}
	if got := r.NextID("projects/shop", "PRD"); got != "PRD-008" {
		t.Errorf("NextID = %s", got)
	}
	if got := r.NextID("projects/other", "PRD"); got != "PRD-001" {
		t.Errorf("NextID in empty scope = %s", got)
	}
	if path.Base(rel) != "PRD-001-example-prd.md" {
		t.Errorf("file name = %s", path.Base(rel))
	}
}
