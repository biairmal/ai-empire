package worker

import (
	"fmt"
	"sort"
	"strings"

	"aiempire/internal/api"
)

// Prompts for each role. The role definition itself (responsibilities, limits,
// quality bar) is in the context bundle; prompts state the concrete job.

func header(b *strings.Builder, cl api.Claim) {
	t := cl.Task
	fmt.Fprintf(b, "Task #%d: %s\n\n", t.ID, t.Title)
	fmt.Fprintf(b, "You are the %s for project %q", roleName(t.Role), cl.Project.Name)
	if cl.Repository.Name != "" {
		fmt.Fprintf(b, ", repository %q (stack: %s)", cl.Repository.Name, cl.Repository.Stack)
	}
	b.WriteString(".\nYour role definition is in .empire-context.md under roles/. Follow it strictly.\n\n")
	if t.Description != "" {
		fmt.Fprintf(b, "## Request\n\n%s\n\n", t.Description)
	}
	if t.Feedback != "" {
		fmt.Fprintf(b, "## Feedback on the previous attempt (address all of it)\n\n%s\n\n", t.Feedback)
	}
}

func roleName(role string) string {
	words := strings.Fields(strings.ReplaceAll(role, "-", " "))
	for i, w := range words {
		words[i] = strings.ToUpper(w[:1]) + w[1:]
	}
	return strings.Join(words, " ")
}

func developerPrompt(cl api.Claim) string {
	var b strings.Builder
	header(&b, cl)
	fmt.Fprintf(&b, `## Rules

- Read %[1]s first: engineering rules, your role, and the approved requirements and design for this task.
- Also follow the repository's own guidance (README, CLAUDE.md, AGENTS.md, CONTRIBUTING.md).
- Work only inside the current directory.
- The project's other repositories (e.g. the backend and its API contract) are checked out read-only
  under .empire/repos/<name>. Read them instead of guessing interfaces; never edit them.
- Implement the task completely, with tests where it makes sense.
- Do not run git commands, commit, or push; the worker handles version control.
- Do not edit %[1]s or anything under .empire/.
- If an approved requirement or design looks wrong, say so in your summary instead of silently changing it.
- End with a short summary of what you changed and why.
`, contextFile)
	return b.String()
}

func reviewerPrompt(cl api.Claim) string {
	var b strings.Builder
	header(&b, cl)
	b.WriteString(`## Your job

Review the change in .empire/review.diff. It was written by another agent for the task above.
Read .empire-context.md for the approved requirements and design, and read repository files as needed.
You may not edit anything.

Report:
1. **Blocking issues** — correctness bugs, missing or wrong tests, security problems, breaking changes,
   work outside the task. For each: file, problem, and why it blocks.
2. **Suggestions** — optional improvements.

The very last line of your answer must be exactly one of:
VERDICT: APPROVE
VERDICT: REQUEST_CHANGES
`)
	return b.String()
}

func documentPrompt(cl api.Claim, tc api.TaskContext, extras, problems []string) string {
	t := cl.Task
	var b strings.Builder
	header(&b, cl)
	if tc.Request != nil {
		fmt.Fprintf(&b, "## Original request\n\n**%s**\n\n%s\n\n", tc.Request.Title, tc.Request.Description)
	}
	b.WriteString("## Your job\n\n")
	switch t.Kind {
	case "revise":
		fmt.Fprintf(&b, "An approved change request (in .empire-context.md) requires a new version of the %s in output/.\n"+
			"Edit that file in place: keep its `id`, keep its structure, and change only what the change request requires.\n"+
			"Do not change `version`, `status`, or `derived_from`; the platform sets them.\n", t.DocType)
	case "plan":
		fmt.Fprintf(&b, "Write one Implementation Plan to output/ following .empire/templates/%s.md.\n"+
			"The \"Work Breakdown\" section must contain exactly one ```yaml block listing the tasks.\n"+
			"Set each task's `repository:` to one of these repository names: %s.\n", t.DocType, strings.Join(repoNames(tc), ", "))
	default:
		fmt.Fprintf(&b, "Write one %s to output/ following .empire/templates/%s.md and its contract .empire/contracts/%s.yaml.\n",
			t.DocType, t.DocType, t.DocType)
		if len(extras) > 0 {
			fmt.Fprintf(&b, "You may also write documents of type %s to output/ (templates in .empire/templates/), "+
				"one file per decision, if the design contains significant decisions.\n", strings.Join(extras, ", "))
		}
	}
	if len(tc.Drafts) > 0 && t.Kind != "revise" {
		b.WriteString("Your previous draft is already in output/; improve it in place and keep its `id`.\n")
	}
	b.WriteString(`
## Writing rules

- The audience includes the client. Write clearly, professionally, and concretely; no filler.
- Keep the template's front matter keys and every "## " section of the template, in order.
- Replace every {{ … }} placeholder. Remove the <!-- guidance --> comments once a section is written.
- If a section does not apply, write "Not applicable" and one sentence explaining why.
- Never invent facts, numbers, customers, or commitments: record them as assumptions or open questions.
- New documents use ` + "`id: NEW`" + `. Do not edit ` + "`version`, `status`, `created` or `updated`" + `: the platform sets ids, versions, dates, and status.
- Relationships (satisfies, depends_on, …) must use ids of documents shown in .empire-context.md.
- Write only inside output/. Do not run commands.
`)
	if len(tc.Relations) > 0 {
		keys := make([]string, 0, len(tc.Relations))
		for k := range tc.Relations {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		b.WriteString("\nThe platform will add these relationships automatically:")
		for _, k := range keys {
			fmt.Fprintf(&b, " %s: %s;", k, strings.Join(tc.Relations[k], ", "))
		}
		b.WriteString("\n")
	}
	if len(problems) > 0 {
		b.WriteString("\n## Your previous output was rejected by validation. Fix every problem:\n\n")
		for _, p := range problems {
			b.WriteString("- " + p + "\n")
		}
	}
	b.WriteString("\nEnd with a short summary of what you wrote and any open questions for the reviewer.\n")
	return b.String()
}

func repoNames(tc api.TaskContext) []string {
	var out []string
	for _, r := range tc.Repositories {
		out = append(out, fmt.Sprintf("`%s` (stack %s)", r.Name, r.Stack))
	}
	return out
}
