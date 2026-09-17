package docs

import (
	"fmt"
	"regexp"
	"strings"
)

const (
	relStart = "<!-- relations:start — generated from the front matter by the platform; do not edit -->"
	relEnd   = "<!-- relations:end -->"
)

var relBlockRe = regexp.MustCompile(`(?s)\n*<!-- relations:start.*?<!-- relations:end -->\n*`)

// StripRelations removes the generated relations block.
func StripRelations(body string) string {
	return relBlockRe.ReplaceAllString(body, "\n")
}

var relLabels = map[string]string{
	"satisfies": "Satisfies", "implements": "Implements", "depends_on": "Depends on",
	"affects": "Affects", "derived_from": "Derived from", "supersedes": "Supersedes",
	"contradicts": "Contradicts", "tested_by": "Tested by", "documents": "Documents",
	"references": "References",
}

// SetRelations rewrites the generated "Relations" block at the end of the body
// as Obsidian wikilinks, so the graph view shows the typed relationships (spec §20).
// The block is excluded from the content hash and from validation.
func (d *Doc) SetRelations(resolve func(ref string) *Doc) {
	body := strings.TrimRight(StripRelations(d.Body), "\n ")
	var lines []string
	for _, k := range RelKeys {
		var links []string
		for _, ref := range d.Rels[k] {
			if t := resolve(ref); t != nil {
				links = append(links, fmt.Sprintf("[[%s|%s · %s]]", t.Base(), t.ID, t.Title))
			} else {
				links = append(links, ref+" (not found)")
			}
		}
		if len(links) > 0 {
			lines = append(lines, fmt.Sprintf("- **%s:** %s", relLabels[k], strings.Join(links, ", ")))
		}
	}
	if len(lines) == 0 {
		d.Body = body + "\n"
		return
	}
	d.Body = body + "\n\n" + relStart + "\n## Relations\n\n" + strings.Join(lines, "\n") + "\n" + relEnd + "\n"
}

// Control is the document-control information shown to clients on export.
type Control struct {
	TypeTitle  string
	Project    string
	ApprovedBy string
	ApprovedAt string
	Related    []string // "PRD-001 — Title (Satisfies)"
}

var blankRuns = regexp.MustCompile(`\n{3,}`)

// ClientView renders a document for hand-over: no front matter, no guidance
// comments, no platform blocks, and a Document Control table under the title.
func (d *Doc) ClientView(ctl Control) string {
	body := commentRe.ReplaceAllString(StripRelations(d.Body), "")
	title, rest := "# "+d.Title, body
	if lines := strings.SplitN(strings.TrimLeft(body, "\n"), "\n", 2); strings.HasPrefix(lines[0], "# ") {
		title = lines[0]
		rest = ""
		if len(lines) > 1 {
			rest = lines[1]
		}
	}
	status := map[string]string{Draft: "Draft", PendingApproval: "Pending approval", Approved: "Approved",
		ChangesRequested: "Changes requested", Rejected: "Rejected", Superseded: "Superseded"}[d.Status]

	var b strings.Builder
	fmt.Fprintf(&b, "%s\n\n## Document Control\n\n| Field | Value |\n|-------|-------|\n", title)
	row := func(k, v string) {
		if v != "" {
			fmt.Fprintf(&b, "| %s | %s |\n", k, strings.ReplaceAll(v, "|", "\\|"))
		}
	}
	row("Document", d.ID+" — "+ctl.TypeTitle)
	row("Version", fmt.Sprint(d.Version))
	row("Status", status)
	row("Project", ctl.Project)
	row("Owner", d.Str("owner"))
	row("Created", d.Str("created"))
	row("Last updated", d.Str("updated"))
	if ctl.ApprovedAt != "" {
		row("Approved", ctl.ApprovedAt+" by "+ctl.ApprovedBy)
	}
	if len(ctl.Related) > 0 {
		row("Related documents", strings.Join(ctl.Related, "<br>"))
	}
	b.WriteString("\n" + strings.TrimLeft(rest, "\n"))
	return strings.TrimSpace(blankRuns.ReplaceAllString(b.String(), "\n\n")) + "\n"
}
