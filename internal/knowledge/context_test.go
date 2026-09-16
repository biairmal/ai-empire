package knowledge

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

func fixture(t *testing.T) string {
	root := t.TempDir()
	for rel, body := range map[string]string{
		"global/principles.md":               "GLOBAL",
		"global/README.md":                   "NAV",
		"stacks/go/testing.md":               "GO-STACK",
		"stacks/dotnet/efcore.md":            "DOTNET-STACK",
		"clients/acme/conventions.md":        "ACME-CLIENT",
		"clients/globex/conventions.md":      "GLOBEX-CLIENT",
		"projects/alpha/requirements/prd.md": "ALPHA-PRD",
		"projects/alpha/api/unlisted.md":     "ALPHA-UNLISTED",
		"projects/beta/secret.md":            "BETA-SECRET",
	} {
		p := filepath.Join(root, rel)
		os.MkdirAll(filepath.Dir(p), 0o755)
		if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return root
}

func check(t *testing.T, b Bundle, wantFiles, mustHave, mustNot []string) {
	t.Helper()
	if !slices.Equal(b.Files, wantFiles) {
		t.Errorf("files = %v, want %v", b.Files, wantFiles)
	}
	for _, s := range mustHave {
		if !strings.Contains(b.Content, s) {
			t.Errorf("content missing %s", s)
		}
	}
	for _, s := range mustNot {
		if strings.Contains(b.Content, s) {
			t.Errorf("content leaked %s", s)
		}
	}
}

func TestResolveIsolation(t *testing.T) {
	root := fixture(t)

	b, err := Resolve(root, Scope{Stack: "go", Client: "acme", Project: "alpha", Docs: []string{"requirements/prd.md"}})
	if err != nil {
		t.Fatal(err)
	}
	check(t, b,
		[]string{"global/principles.md", "stacks/go/testing.md", "clients/acme/conventions.md", "projects/alpha/requirements/prd.md"},
		[]string{"GLOBAL", "GO-STACK", "ACME-CLIENT", "ALPHA-PRD"},
		[]string{"NAV", "DOTNET-STACK", "GLOBEX-CLIENT", "ALPHA-UNLISTED", "BETA-SECRET"})

	// A client-less project gets no client knowledge at all.
	b, err = Resolve(root, Scope{Stack: "dotnet", Project: "alpha"})
	if err != nil {
		t.Fatal(err)
	}
	check(t, b,
		[]string{"global/principles.md", "stacks/dotnet/efcore.md"},
		[]string{"DOTNET-STACK"},
		[]string{"GO-STACK", "ACME-CLIENT", "GLOBEX-CLIENT", "ALPHA-PRD"})

	for _, doc := range []string{"../beta/secret.md", filepath.Join(root, "projects/beta/secret.md"), "requirements/missing.md", "notes.txt", "../../clients/globex/conventions.md"} {
		if _, err := Resolve(root, Scope{Stack: "go", Project: "alpha", Docs: []string{doc}}); err == nil {
			t.Errorf("doc %q: expected error", doc)
		}
	}
	for _, bad := range []Scope{
		{Stack: "../x", Project: "alpha"},
		{Stack: "go", Project: "../beta"},
		{Stack: "go", Project: "alpha", Client: "../globex"},
	} {
		if _, err := Resolve(root, bad); err == nil {
			t.Errorf("scope %+v: expected error", bad)
		}
	}
}

func TestResolveMissingFoldersAreFine(t *testing.T) {
	b, err := Resolve(t.TempDir(), Scope{Stack: "rust", Client: "newco", Project: "alpha"})
	if err != nil || len(b.Files) != 0 {
		t.Fatalf("got %v, %v", b, err)
	}
}
