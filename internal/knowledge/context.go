// Package knowledge reads the Markdown knowledge repository.
package knowledge

import (
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
)

// Bundle is the minimal context handed to an agent (spec §22).
type Bundle struct {
	Files   []string `json:"files"` // knowledge-relative paths, in load order
	Content string   `json:"content"`
}

var slugRe = regexp.MustCompile(`^[a-z0-9][a-z0-9-]*$`)

// Scope says which knowledge a task may see.
type Scope struct {
	Stacks  []string // the task repository's stack; all project stacks for document tasks
	Client  string   // project's client slug; "" = no client knowledge at all
	Project string   // project slug
	Role    string   // agent role; loads roles/<role>.md ("" = none)
	Docs    []string // project-relative docs listed on the task
}

// Resolve builds a context bundle:
// global → roles/<role> → stacks/<stack>… → clients/<client> → projects/<project>/guidelines → listed project docs.
// Other stacks, other clients and other projects are never read (spec §11B, §21).
func Resolve(root string, s Scope) (Bundle, error) {
	valid := slugRe.MatchString(s.Project) && (s.Client == "" || slugRe.MatchString(s.Client)) &&
		(s.Role == "" || slugRe.MatchString(s.Role))
	for _, st := range s.Stacks {
		valid = valid && slugRe.MatchString(st)
	}
	if !valid {
		return Bundle{}, fmt.Errorf("invalid scope stacks=%q client=%q project=%q role=%q", s.Stacks, s.Client, s.Project, s.Role)
	}
	project, docs := s.Project, s.Docs
	var b Bundle
	var sb strings.Builder
	add := func(rel string, data []byte) {
		if slices.Contains(b.Files, rel) { // a listed doc may also be a project guideline
			return
		}
		b.Files = append(b.Files, rel)
		fmt.Fprintf(&sb, "## File: %s\n\n%s\n\n", rel, strings.TrimSpace(string(data)))
	}

	dirs := []string{"global"}
	if s.Role != "" {
		dirs = append(dirs, "roles/"+s.Role+".md")
	}
	for _, st := range s.Stacks {
		dirs = append(dirs, "stacks/"+st)
	}
	if s.Client != "" {
		dirs = append(dirs, "clients/"+s.Client)
	}
	// Project guidelines (tooling, conventions) apply to every task of the project.
	dirs = append(dirs, "projects/"+project+"/guidelines")
	for _, dir := range dirs {
		err := filepath.WalkDir(filepath.Join(root, dir), func(p string, d fs.DirEntry, err error) error {
			if err != nil {
				return err
			}
			// READMEs are folder navigation, not knowledge.
			if d.IsDir() || filepath.Ext(p) != ".md" || d.Name() == "README.md" {
				return nil
			}
			data, err := os.ReadFile(p)
			if err != nil {
				return err
			}
			rel, _ := filepath.Rel(root, p)
			add(filepath.ToSlash(rel), data)
			return nil
		})
		if err != nil && !errors.Is(err, fs.ErrNotExist) {
			return Bundle{}, err
		}
	}

	projectDir := filepath.Join(root, "projects", project)
	for _, doc := range docs {
		if path.Ext(doc) != ".md" {
			return Bundle{}, fmt.Errorf("context doc %q: not a .md file", doc)
		}
		// OpenInRoot rejects "..", absolute paths and symlinks escaping the project.
		f, err := os.OpenInRoot(projectDir, doc)
		if err != nil {
			return Bundle{}, fmt.Errorf("context doc %q: %w", doc, err)
		}
		data, err := io.ReadAll(f)
		f.Close()
		if err != nil {
			return Bundle{}, err
		}
		add("projects/"+project+"/"+path.Clean(doc), data)
	}

	b.Content = sb.String()
	return b, nil
}
