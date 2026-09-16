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
	"strings"
)

// Bundle is the minimal context handed to an agent (spec §22).
type Bundle struct {
	Files   []string `json:"files"` // knowledge-relative paths, in load order
	Content string   `json:"content"`
}

var slugRe = regexp.MustCompile(`^[a-z0-9][a-z0-9-]*$`)

// Resolve builds a context bundle: global → stacks/<stack> → listed project docs.
// Sibling stacks and other projects are never read (spec §21).
func Resolve(root, stack, project string, docs []string) (Bundle, error) {
	if !slugRe.MatchString(stack) || !slugRe.MatchString(project) {
		return Bundle{}, fmt.Errorf("invalid stack %q or project %q", stack, project)
	}
	var b Bundle
	var sb strings.Builder
	add := func(rel string, data []byte) {
		b.Files = append(b.Files, rel)
		fmt.Fprintf(&sb, "## File: %s\n\n%s\n\n", rel, strings.TrimSpace(string(data)))
	}

	for _, dir := range []string{"global", "stacks/" + stack} {
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
