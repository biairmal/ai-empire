package docs

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"slices"
	"sort"
	"strconv"
	"strings"

	"go.yaml.in/yaml/v3"
)

// Contract defines a document type (spec §16), loaded from knowledge/contracts/<type>.yaml.
type Contract struct {
	Type             string              `yaml:"type"`
	Title            string              `yaml:"title"`
	IDPrefix         string              `yaml:"id_prefix"`
	Question         string              `yaml:"question"`
	Audience         string              `yaml:"audience"`
	Scopes           []string            `yaml:"scopes"` // project, client, stack, global, roles
	Folder           string              `yaml:"folder"` // required sub-folder inside projects/<slug>/
	Approval         string              `yaml:"approval"`
	RequiredMetadata []string            `yaml:"required_metadata"`
	RequiredSections []string            `yaml:"required_sections"`
	Relationships    map[string][]string `yaml:"relationships"`
	Upstream         []Upstream          `yaml:"upstream"`
}

// Upstream requires approved documents behind a relationship before this
// document may be submitted or approved (spec §18).
type Upstream struct {
	Relationship string   `yaml:"relationship"`
	Types        []string `yaml:"types"`
	Min          int      `yaml:"min"`
}

func (c Contract) ApprovalRequired() bool { return c.Approval == "required" }

func matchType(allowed []string, t string) bool {
	return slices.Contains(allowed, "*") || slices.Contains(allowed, t)
}

// Repo is the loaded knowledge repository.
type Repo struct {
	Root      string
	Contracts map[string]Contract
	Docs      []*Doc
	Problems  []Problem // files that could not be parsed at all
	byKey     map[Key]*Doc
}

type Problem struct {
	Path string `json:"path"`
	Msg  string `json:"message"`
}

func (p Problem) String() string { return p.Path + ": " + p.Msg }

// Load reads contracts and every document under the knowledge root.
// templates/, contracts/ and README.md files are not documents.
func Load(root string) (*Repo, error) {
	r := &Repo{Root: root, Contracts: map[string]Contract{}, byKey: map[Key]*Doc{}}
	files, err := filepath.Glob(filepath.Join(root, "contracts", "*.yaml"))
	if err != nil {
		return nil, err
	}
	for _, f := range files {
		data, err := os.ReadFile(f)
		if err != nil {
			return nil, err
		}
		var c Contract
		if err := yaml.Unmarshal(data, &c); err != nil {
			return nil, fmt.Errorf("%s: %w", f, err)
		}
		if c.Type == "" || c.IDPrefix == "" {
			return nil, fmt.Errorf("%s: type and id_prefix are required", f)
		}
		r.Contracts[c.Type] = c
	}

	err = filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(root, p)
		rel = filepath.ToSlash(rel)
		if d.IsDir() {
			if rel == "templates" || rel == "contracts" || strings.HasPrefix(d.Name(), ".") && rel != "." {
				return filepath.SkipDir
			}
			return nil
		}
		if filepath.Ext(p) != ".md" || d.Name() == "README.md" || ScopeOf(rel) == "" {
			return nil
		}
		data, err := os.ReadFile(p)
		if err != nil {
			return err
		}
		doc, err := Parse(rel, data)
		if err != nil {
			r.Problems = append(r.Problems, Problem{rel, err.Error()})
			return nil
		}
		r.Docs = append(r.Docs, doc)
		return nil
	})
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		return nil, err
	}
	sort.Slice(r.Docs, func(i, j int) bool { return r.Docs[i].Path < r.Docs[j].Path })
	for _, d := range r.Docs {
		if _, dup := r.byKey[d.Key()]; !dup && d.ID != "" {
			r.byKey[d.Key()] = d
		}
	}
	return r, nil
}

// Get returns the document with the given key.
func (r *Repo) Get(k Key) *Doc { return r.byKey[k] }

// Visible lists the scopes a document in scope `from` may reference (spec §11B, §21).
// client is the client slug of the project (only used for project scopes).
func Visible(from, client string) []string {
	switch {
	case strings.HasPrefix(from, "projects/"):
		v := []string{from}
		if client != "" {
			v = append(v, "clients/"+client)
		}
		return append(v, "stacks/*", "global", "roles")
	case strings.HasPrefix(from, "clients/"):
		return []string{from, "stacks/*", "global"}
	case strings.HasPrefix(from, "stacks/"):
		return []string{from, "global"}
	case from == "roles":
		return []string{"roles", "global"}
	}
	return []string{"global"}
}

func scopeAllowed(visible []string, scope string) bool {
	for _, v := range visible {
		if v == scope || (v == "stacks/*" && strings.HasPrefix(scope, "stacks/")) {
			return true
		}
	}
	return false
}

// Resolve finds the document a reference points to, as seen from `from`.
// A reference is either an id ("PRD-001"), searched in the visible scopes in
// order, or a qualified key ("global/ADR-001", "stacks/go/ADR-002").
func (r *Repo) Resolve(from *Doc, client, ref string) (*Doc, error) {
	visible := Visible(from.Scope, client)
	if k, ok := ParseKey(ref); ok {
		if !scopeAllowed(visible, k.Scope) {
			return nil, fmt.Errorf("%q is outside the scopes this document may reference", ref)
		}
		if d := r.byKey[k]; d != nil {
			return d, nil
		}
		return nil, fmt.Errorf("%q does not exist", ref)
	}
	// Most specific scope first: own → client → stacks → global.
	for _, s := range visible {
		if s != "stacks/*" {
			if d := r.byKey[Key{s, ref}]; d != nil {
				return d, nil
			}
			continue
		}
		var hits []*Doc
		for k, d := range r.byKey {
			if k.ID == ref && strings.HasPrefix(k.Scope, "stacks/") {
				hits = append(hits, d)
			}
		}
		switch len(hits) {
		case 1:
			return hits[0], nil
		case 0:
		default:
			sort.Slice(hits, func(i, j int) bool { return hits[i].Scope < hits[j].Scope })
			return nil, fmt.Errorf("%q exists in several stacks; qualify it, e.g. %q", ref, hits[0].Key().String())
		}
	}
	return nil, fmt.Errorf("%q does not exist in any scope this document may reference", ref)
}

// NextID returns the next free id for a type in a scope, e.g. "PRD-004".
func (r *Repo) NextID(scope, prefix string, reserved ...string) string {
	re := regexp.MustCompile("^" + regexp.QuoteMeta(prefix) + `-(\d+)$`)
	max := 0
	consider := func(id string) {
		if m := re.FindStringSubmatch(id); m != nil {
			if n, _ := strconv.Atoi(m[1]); n > max {
				max = n
			}
		}
	}
	for k := range r.byKey {
		if k.Scope == scope {
			consider(k.ID)
		}
	}
	for _, id := range reserved {
		consider(id)
	}
	return fmt.Sprintf("%s-%03d", prefix, max+1)
}

// DocPath is where a new project document of a contract type is stored.
func (c Contract) DocPath(project, id, title string) string {
	return path.Join("projects", project, c.Folder, FileName(id, title))
}
