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
		"roles/developer.md":                 "DEV-ROLE",
		"roles/architect.md":                 "ARCH-ROLE",
		"stacks/go/testing.md":               "GO-STACK",
		"stacks/dotnet/efcore.md":            "DOTNET-STACK",
		"clients/acme/conventions.md":        "ACME-CLIENT",
		"clients/globex/conventions.md":      "GLOBEX-CLIENT",
		"projects/alpha/requirements/prd.md": "ALPHA-PRD",
		"projects/alpha/api/unlisted.md":     "ALPHA-UNLISTED",
		"projects/beta/secret.md":            "BETA-SECRET",
		"projects/alpha/guidelines/tools.md": "ALPHA-GUIDE",
		"projects/beta/guidelines/tools.md":  "BETA-GUIDE",
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

	b, err := Resolve(root, Scope{Stacks: []string{"go"}, Client: "acme", Project: "alpha", Role: "developer", Docs: []string{"requirements/prd.md"}})
	if err != nil {
		t.Fatal(err)
	}
	check(t, b,
		[]string{"global/principles.md", "roles/developer.md", "stacks/go/testing.md", "clients/acme/conventions.md", "projects/alpha/guidelines/tools.md", "projects/alpha/requirements/prd.md"},
		[]string{"GLOBAL", "DEV-ROLE", "GO-STACK", "ACME-CLIENT", "ALPHA-GUIDE", "ALPHA-PRD"},
		[]string{"NAV", "ARCH-ROLE", "DOTNET-STACK", "GLOBEX-CLIENT", "ALPHA-UNLISTED", "BETA-SECRET", "BETA-GUIDE"})

	// A client-less project gets no client knowledge at all.
	b, err = Resolve(root, Scope{Stacks: []string{"dotnet"}, Project: "alpha"})
	if err != nil {
		t.Fatal(err)
	}
	check(t, b,
		[]string{"global/principles.md", "stacks/dotnet/efcore.md", "projects/alpha/guidelines/tools.md"},
		[]string{"DOTNET-STACK"},
		[]string{"GO-STACK", "ACME-CLIENT", "GLOBEX-CLIENT", "ALPHA-PRD"})

	for _, doc := range []string{"../beta/secret.md", filepath.Join(root, "projects/beta/secret.md"), "requirements/missing.md", "notes.txt", "../../clients/globex/conventions.md"} {
		if _, err := Resolve(root, Scope{Stacks: []string{"go"}, Project: "alpha", Docs: []string{doc}}); err == nil {
			t.Errorf("doc %q: expected error", doc)
		}
	}
	for _, bad := range []Scope{
		{Stacks: []string{"../x"}, Project: "alpha"},
		{Stacks: []string{"go"}, Project: "../beta"},
		{Stacks: []string{"go"}, Project: "alpha", Client: "../globex"},
	} {
		if _, err := Resolve(root, bad); err == nil {
			t.Errorf("scope %+v: expected error", bad)
		}
	}
}

func TestResolveMissingFoldersAreFine(t *testing.T) {
	b, err := Resolve(t.TempDir(), Scope{Stacks: []string{"rust"}, Client: "newco", Project: "alpha", Role: "nobody"})
	if err != nil || len(b.Files) != 0 {
		t.Fatalf("got %v, %v", b, err)
	}
}
