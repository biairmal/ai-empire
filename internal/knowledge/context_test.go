package knowledge

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

func TestResolveIsolation(t *testing.T) {
	root := t.TempDir()
	write := func(rel, body string) {
		p := filepath.Join(root, rel)
		os.MkdirAll(filepath.Dir(p), 0o755)
		if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write("global/principles.md", "GLOBAL")
	write("global/README.md", "NAV")
	write("stacks/go/testing.md", "GO-STACK")
	write("stacks/dotnet/efcore.md", "DOTNET-STACK")
	write("projects/alpha/requirements/prd.md", "ALPHA-PRD")
	write("projects/alpha/api/unlisted.md", "ALPHA-UNLISTED")
	write("projects/beta/secret.md", "BETA-SECRET")

	b, err := Resolve(root, "go", "alpha", []string{"requirements/prd.md"})
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"global/principles.md", "stacks/go/testing.md", "projects/alpha/requirements/prd.md"}
	if !slices.Equal(b.Files, want) {
		t.Errorf("files = %v, want %v", b.Files, want)
	}
	for _, s := range []string{"GLOBAL", "GO-STACK", "ALPHA-PRD"} {
		if !strings.Contains(b.Content, s) {
			t.Errorf("content missing %s", s)
		}
	}
	for _, s := range []string{"NAV", "DOTNET-STACK", "ALPHA-UNLISTED", "BETA-SECRET"} {
		if strings.Contains(b.Content, s) {
			t.Errorf("content leaked %s", s)
		}
	}

	for _, doc := range []string{"../beta/secret.md", filepath.Join(root, "projects/beta/secret.md"), "requirements/missing.md", "notes.txt"} {
		if _, err := Resolve(root, "go", "alpha", []string{doc}); err == nil {
			t.Errorf("doc %q: expected error", doc)
		}
	}
	for _, bad := range [][2]string{{"../x", "alpha"}, {"go", "../beta"}} {
		if _, err := Resolve(root, bad[0], bad[1], nil); err == nil {
			t.Errorf("stack/project %v: expected error", bad)
		}
	}
}

func TestResolveMissingStackIsFine(t *testing.T) {
	b, err := Resolve(t.TempDir(), "rust", "alpha", nil)
	if err != nil || len(b.Files) != 0 {
		t.Fatalf("got %v, %v", b, err)
	}
}
