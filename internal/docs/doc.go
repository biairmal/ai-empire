// Package docs parses, validates, and renders knowledge documents (spec §13–§19).
// A document is Markdown with YAML front matter; the front matter carries identity,
// lifecycle, and typed relationships.
package docs

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"path"
	"regexp"
	"slices"
	"strings"

	"go.yaml.in/yaml/v3"
)

// Lifecycle statuses (knowledge/contracts/frontmatter.md).
const (
	Draft            = "draft"
	PendingApproval  = "pending_approval"
	Approved         = "approved"
	ChangesRequested = "changes_requested"
	Rejected         = "rejected"
	Superseded       = "superseded"
)

var statuses = map[string]bool{Draft: true, PendingApproval: true, Approved: true,
	ChangesRequested: true, Rejected: true, Superseded: true}

// RelKeys are the only relationship keys allowed in front matter (spec §15).
var RelKeys = []string{"satisfies", "implements", "depends_on", "affects", "derived_from",
	"supersedes", "contradicts", "tested_by", "documents", "references"}

func isRelKey(k string) bool { return slices.Contains(RelKeys, k) }

// NewID is the placeholder id for a document the platform has not numbered yet.
const NewID = "NEW"

// Key identifies a document: ids are unique within a scope.
type Key struct {
	Scope string // "global", "roles", "stacks/go", "clients/acme", "projects/guest"
	ID    string
}

func (k Key) String() string { return k.Scope + "/" + k.ID }

// ParseKey reverses Key.String.
func ParseKey(s string) (Key, bool) {
	i := strings.LastIndex(s, "/")
	if i <= 0 {
		return Key{}, false
	}
	return Key{Scope: s[:i], ID: s[i+1:]}, true
}

type Doc struct {
	Path    string // knowledge-relative, forward slashes
	Scope   string
	Type    string
	ID      string
	Title   string
	Status  string
	Version int
	Rels    map[string][]string // relationship key → raw references
	Body    string              // Markdown after the front matter
	fm      *yaml.Node          // front matter mapping node, edited in place
	fmErr   []string            // front matter shape problems found while parsing
}

func (d *Doc) Key() Key { return Key{d.Scope, d.ID} }

// ScopeOf returns the scope for a knowledge-relative path, or "" if the path is not a document location.
func ScopeOf(rel string) string {
	parts := strings.Split(rel, "/")
	switch {
	case len(parts) >= 2 && (parts[0] == "global" || parts[0] == "roles"):
		return parts[0]
	case len(parts) >= 3 && (parts[0] == "stacks" || parts[0] == "clients" || parts[0] == "projects"):
		return parts[0] + "/" + parts[1]
	}
	return ""
}

var fmSep = regexp.MustCompile(`(?m)^---[ \t]*$`)

// Parse reads a document. rel is its knowledge-relative path.
func Parse(rel string, data []byte) (*Doc, error) {
	text := strings.ReplaceAll(string(data), "\r\n", "\n")
	if !strings.HasPrefix(text, "---\n") {
		return nil, errors.New("missing YAML front matter (the file must start with ---)")
	}
	loc := fmSep.FindStringIndex(text[4:])
	if loc == nil {
		return nil, errors.New("front matter is not closed with ---")
	}
	fmText, body := text[4:4+loc[0]], strings.TrimPrefix(text[4+loc[1]:], "\n")

	var root yaml.Node
	if err := yaml.Unmarshal([]byte(fmText), &root); err != nil {
		return nil, fmt.Errorf("front matter is not valid YAML: %v", err)
	}
	if len(root.Content) == 0 || root.Content[0].Kind != yaml.MappingNode {
		return nil, errors.New("front matter must be a YAML mapping")
	}
	d := &Doc{Path: rel, Scope: ScopeOf(rel), Body: body, fm: root.Content[0], Rels: map[string][]string{}}
	d.Type = d.Str("type")
	d.ID = d.Str("id")
	d.Title = d.Str("title")
	d.Status = d.Str("status")
	if v := d.node("version"); v != nil {
		if err := v.Decode(&d.Version); err != nil {
			d.fmErr = append(d.fmErr, "version must be a whole number")
		}
	}
	for _, k := range RelKeys {
		v := d.node(k)
		if v == nil {
			continue
		}
		var refs []string
		if err := v.Decode(&refs); err != nil {
			d.fmErr = append(d.fmErr, fmt.Sprintf("%s must be a list of document ids", k))
			continue
		}
		if len(refs) > 0 {
			d.Rels[k] = refs
		}
	}
	return d, nil
}

func (d *Doc) node(key string) *yaml.Node {
	for i := 0; i+1 < len(d.fm.Content); i += 2 {
		if d.fm.Content[i].Value == key {
			return d.fm.Content[i+1]
		}
	}
	return nil
}

// Keys lists the front matter keys in order.
func (d *Doc) Keys() []string {
	var ks []string
	for i := 0; i+1 < len(d.fm.Content); i += 2 {
		ks = append(ks, d.fm.Content[i].Value)
	}
	return ks
}

// Str returns a scalar front matter value, or "".
func (d *Doc) Str(key string) string {
	if n := d.node(key); n != nil && n.Kind == yaml.ScalarNode {
		return strings.TrimSpace(n.Value)
	}
	return ""
}

// Has reports whether key is present with a non-empty value.
func (d *Doc) Has(key string) bool {
	n := d.node(key)
	if n == nil {
		return false
	}
	switch n.Kind {
	case yaml.ScalarNode:
		return strings.TrimSpace(n.Value) != "" && n.Tag != "!!null"
	case yaml.SequenceNode, yaml.MappingNode:
		return len(n.Content) > 0
	}
	return true
}

// Set writes a front matter value, keeping key order and comments.
func (d *Doc) Set(key string, value any) {
	var v yaml.Node
	if err := v.Encode(value); err != nil {
		panic(err) // only called with plain strings, ints and string slices
	}
	if v.Kind == yaml.SequenceNode {
		v.Style = yaml.FlowStyle
	}
	if n := d.node(key); n != nil {
		v.HeadComment, v.LineComment = n.HeadComment, n.LineComment
		*n = v
	} else {
		d.fm.Content = append(d.fm.Content, &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: key}, &v)
	}
	switch key {
	case "status":
		d.Status, _ = value.(string)
	case "id":
		d.ID, _ = value.(string)
	case "version":
		d.Version, _ = value.(int)
	case "title":
		d.Title, _ = value.(string)
	}
	if isRelKey(key) {
		refs, _ := value.([]string)
		if len(refs) == 0 {
			delete(d.Rels, key)
		} else {
			d.Rels[key] = refs
		}
	}
}

// AddRel appends references to a relationship key, skipping duplicates.
func (d *Doc) AddRel(key string, refs ...string) {
	cur := append([]string(nil), d.Rels[key]...)
	for _, r := range refs {
		dup := false
		for _, c := range cur {
			dup = dup || c == r
		}
		if !dup {
			cur = append(cur, r)
		}
	}
	d.Set(key, cur)
}

func marshal(n *yaml.Node) string {
	var buf bytes.Buffer
	enc := yaml.NewEncoder(&buf)
	enc.SetIndent(2)
	if err := enc.Encode(n); err != nil {
		panic(err)
	}
	enc.Close()
	return buf.String()
}

// Bytes renders the document back to Markdown with front matter.
func (d *Doc) Bytes() []byte {
	return []byte("---\n" + marshal(d.fm) + "---\n\n" + strings.TrimLeft(d.Body, "\n"))
}

// Hash identifies the approved content of a document. It ignores the `status`
// field (which changes on approval) and the generated relations block, so
// lifecycle bookkeeping never invalidates an approval (spec §12).
func (d *Doc) Hash() string {
	fm := *d.fm
	fm.Content = nil
	for i := 0; i+1 < len(d.fm.Content); i += 2 {
		if d.fm.Content[i].Value != "status" {
			fm.Content = append(fm.Content, d.fm.Content[i], d.fm.Content[i+1])
		}
	}
	body := strings.TrimSpace(StripRelations(d.Body))
	sum := sha256.Sum256([]byte(marshal(&fm) + "\n---\n" + body))
	return hex.EncodeToString(sum[:])
}

// FileName is the conventional file name for a document: "<ID>-<title-slug>.md".
func FileName(id, title string) string {
	slug := strings.Trim(nonSlug.ReplaceAllString(strings.ToLower(title), "-"), "-")
	if len(slug) > 60 {
		slug = strings.TrimRight(slug[:60], "-")
	}
	if slug == "" {
		return id + ".md"
	}
	return id + "-" + slug + ".md"
}

var nonSlug = regexp.MustCompile(`[^a-z0-9]+`)

// Base is the file name without extension, which is what Obsidian wikilinks use.
func (d *Doc) Base() string { return strings.TrimSuffix(path.Base(d.Path), ".md") }
