// Command empire is the owner's CLI for the control plane.
package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"text/tabwriter"

	"aiempire/internal/api"
	"aiempire/internal/client"
	"aiempire/internal/docs"
)

const usage = `usage: empire <command> [flags] [args]

  client create -slug S -name N [-autonomy conservative|medium|high]
  client list
  project create -slug S -name N [-client C] [-autonomy conservative|medium|high]
  project list
  project set PROJECT [-name N] [-autonomy conservative|medium|high]
  project move PROJECT -client C | -none
  repo add -project P -name N -repo URL -stack go [-branch main] [-test CMD]
  repo set -project P -name N [-repo URL] [-branch B] [-stack S] [-test CMD]   (-test "" clears it)
  repo list [-project P]
  task create -project P [-repo NAME] [-desc TEXT] [-doc PATH]... [-cap CAP]... [-after ID]... TITLE...
  task list [-status S] [-project P]
  task get ID
  task cancel ID
  task retry ID
  workflows
  request create -project P [-workflow feature|quick-fix|change] [-repo NAME] [-desc TEXT] TITLE...
  request list [-project P] [-status active|completed|cancelled]
  request get ID
  request cancel ID
  docs new -project P -type TYPE -title TITLE -owner "Name, role"
  docs list [-project P] [-type TYPE]
  docs validate [-project P] [-local [-dir knowledge]]   (-local: offline, for CI and git hooks)
  docs submit PATH|KEY|ID [-project P]
  docs reindex
  docs export -project P [-out DIR] [-all]
  trace REF [-project P]     REF: task:N | request:N | commit:SHA | DOC-ID | knowledge path | a source file
  impact DOC [-project P]
  approvals [-status PENDING_APPROVAL]
  approve ID [-m COMMENT]
  request-changes ID -m COMMENT
  reject ID [-m COMMENT]
  workers
  audit [-target task:ID]

env: EMPIRE_CP_URL (default http://localhost:8787), EMPIRE_OWNER_TOKEN
`

type list []string

func (l *list) String() string     { return strings.Join(*l, ",") }
func (l *list) Set(v string) error { *l = append(*l, v); return nil }

func main() {
	if len(os.Args) < 2 {
		fmt.Fprint(os.Stderr, usage)
		os.Exit(2)
	}
	cp := os.Getenv("EMPIRE_CP_URL")
	if cp == "" {
		cp = "http://localhost:8787"
	}
	c := client.New(cp, os.Getenv("EMPIRE_OWNER_TOKEN"))
	if err := dispatch(context.Background(), c, os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}

func dispatch(ctx context.Context, c *client.Client, args []string) error {
	cmd := args[0]
	if slices.Contains([]string{"client", "project", "repo", "task", "request", "docs"}, cmd) && len(args) > 1 {
		cmd, args = cmd+" "+args[1], args[1:]
	}
	fs := flag.NewFlagSet(cmd, flag.ExitOnError)
	fs.Usage = func() { fmt.Fprint(os.Stderr, usage) }
	get := func(path string, out any) error { return c.Do(ctx, "GET", path, nil, out) }
	post := func(path string, in, out any) error { return c.Do(ctx, "POST", path, in, out) }

	switch cmd {
	case "client create":
		var in api.Client
		fs.StringVar(&in.Slug, "slug", "", "")
		fs.StringVar(&in.Name, "name", "", "")
		fs.StringVar(&in.DefaultAutonomyLevel, "autonomy", "", "")
		fs.Parse(args[1:])
		var out api.Client
		if err := post("/clients", in, &out); err != nil {
			return err
		}
		return show(out)

	case "client list":
		var cs []api.Client
		if err := get("/clients", &cs); err != nil {
			return err
		}
		return table("ID\tSLUG\tNAME\tDEFAULT AUTONOMY", cs, func(c api.Client) string {
			return fmt.Sprintf("%d\t%s\t%s\t%s", c.ID, c.Slug, c.Name, c.DefaultAutonomyLevel)
		})

	case "project create":
		var in api.CreateProject
		fs.StringVar(&in.Slug, "slug", "", "")
		fs.StringVar(&in.Name, "name", "", "")
		fs.StringVar(&in.Client, "client", "", "")
		fs.StringVar(&in.AutonomyLevel, "autonomy", "", "")
		fs.Parse(args[1:])
		var out api.Project
		if err := post("/projects", in, &out); err != nil {
			return err
		}
		fmt.Printf("created project %s (id %d, autonomy %s). Next: empire repo add -project %s ...\n",
			out.Slug, out.ID, out.AutonomyLevel, out.Slug)
		return nil

	case "project list":
		var ps []api.Project
		var cs []api.Client
		var rs []api.Repository
		if err := errors.Join(get("/projects", &ps), get("/clients", &cs), get("/repositories", &rs)); err != nil {
			return err
		}
		clients := map[int64]string{}
		for _, c := range cs {
			clients[c.ID] = c.Slug
		}
		repos := map[int64][]string{}
		for _, r := range rs {
			repos[r.ProjectID] = append(repos[r.ProjectID], r.Name+"("+r.Stack+")")
		}
		return table("ID\tSLUG\tCLIENT\tAUTONOMY\tREPOSITORIES", ps, func(p api.Project) string {
			client := "-"
			if p.ClientID != nil {
				client = clients[*p.ClientID]
			}
			return fmt.Sprintf("%d\t%s\t%s\t%s\t%s", p.ID, p.Slug, client, p.AutonomyLevel, strings.Join(repos[p.ID], ", "))
		})

	case "project set":
		if len(args) < 2 {
			return errors.New("usage: empire project set PROJECT [-name N] [-autonomy LEVEL]")
		}
		name := fs.String("name", "", "")
		autonomy := fs.String("autonomy", "", "")
		fs.Parse(args[2:])
		given := setFlags(fs)
		in := api.UpdateProject{Name: given.ptr("name", *name), AutonomyLevel: given.ptr("autonomy", *autonomy)}
		if len(given) == 0 {
			return errors.New("nothing to change: pass -name and/or -autonomy")
		}
		var out api.Project
		if err := c.Do(ctx, "PATCH", "/projects/"+url.PathEscape(args[1]), in, &out); err != nil {
			return err
		}
		fmt.Printf("project %s: name %q, autonomy %s\n", out.Slug, out.Name, out.AutonomyLevel)
		return nil

	case "project move":
		if len(args) < 2 {
			return errors.New("usage: empire project move PROJECT -client C | -none")
		}
		var in api.MoveProject
		fs.StringVar(&in.Client, "client", "", "")
		none := fs.Bool("none", false, "")
		fs.Parse(args[2:])
		if (in.Client == "") == !*none {
			return errors.New("give exactly one of -client C or -none")
		}
		var out api.Project
		if err := post("/projects/"+url.PathEscape(args[1])+"/client", in, &out); err != nil {
			return err
		}
		to := in.Client
		if to == "" {
			to = "(no client)"
		}
		fmt.Printf("moved project %s to %s (autonomy unchanged: %s)\n", out.Slug, to, out.AutonomyLevel)
		return nil

	case "repo add":
		var in api.CreateRepository
		project := fs.String("project", "", "")
		fs.StringVar(&in.Name, "name", "", "")
		fs.StringVar(&in.RepoURL, "repo", "", "")
		fs.StringVar(&in.Stack, "stack", "", "")
		fs.StringVar(&in.DefaultBranch, "branch", "", "")
		fs.StringVar(&in.TestCommand, "test", "", "")
		fs.Parse(args[1:])
		if *project == "" {
			return errors.New("-project is required")
		}
		var out api.Repository
		if err := post("/projects/"+url.PathEscape(*project)+"/repositories", in, &out); err != nil {
			return err
		}
		return show(out)

	case "repo set":
		project := fs.String("project", "", "")
		name := fs.String("name", "", "")
		repoURL := fs.String("repo", "", "")
		branch := fs.String("branch", "", "")
		stack := fs.String("stack", "", "")
		test := fs.String("test", "", "")
		fs.Parse(args[1:])
		if *project == "" || *name == "" {
			return errors.New("-project and -name are required")
		}
		given := setFlags(fs)
		in := api.UpdateRepository{
			RepoURL:       given.ptr("repo", *repoURL),
			DefaultBranch: given.ptr("branch", *branch),
			Stack:         given.ptr("stack", *stack),
			TestCommand:   given.ptr("test", *test),
		}
		if in == (api.UpdateRepository{}) {
			return errors.New("nothing to change: pass -repo, -branch, -stack and/or -test")
		}
		var out api.Repository
		if err := c.Do(ctx, "PATCH", "/projects/"+url.PathEscape(*project)+"/repositories/"+url.PathEscape(*name), in, &out); err != nil {
			return err
		}
		return show(out)

	case "repo list":
		project := fs.String("project", "", "")
		fs.Parse(args[1:])
		path := "/repositories"
		if *project != "" {
			path = "/projects/" + url.PathEscape(*project) + "/repositories"
		}
		var rs []api.Repository
		if err := get(path, &rs); err != nil {
			return err
		}
		return table("ID\tPROJECT\tNAME\tSTACK\tBRANCH\tTEST\tURL", rs, func(r api.Repository) string {
			return fmt.Sprintf("%d\t%d\t%s\t%s\t%s\t%s\t%s", r.ID, r.ProjectID, r.Name, r.Stack, r.DefaultBranch, r.TestCommand, r.RepoURL)
		})

	case "task create":
		var in api.CreateTask
		var docs, caps list
		var after []int64
		fs.StringVar(&in.Project, "project", "", "")
		fs.StringVar(&in.Repository, "repo", "", "")
		fs.StringVar(&in.Description, "desc", "", "")
		fs.Var(&docs, "doc", "")
		fs.Var(&caps, "cap", "")
		fs.Func("after", "", func(v string) error {
			var id int64
			_, err := fmt.Sscan(v, &id)
			after = append(after, id)
			return err
		})
		fs.Parse(args[1:])
		in.Title = strings.Join(fs.Args(), " ")
		in.ContextDocs, in.RequiredCapabilities, in.DependsOn = docs, caps, after
		var out api.Task
		if err := post("/tasks", in, &out); err != nil {
			return err
		}
		fmt.Printf("created task %d (%s)\n", out.ID, out.Status)
		return nil

	case "task list":
		status := fs.String("status", "", "")
		project := fs.String("project", "", "slug or id")
		fs.Parse(args[1:])
		q := url.Values{"status": {*status}}
		if *project != "" {
			var p api.Project
			if err := get("/projects/"+url.PathEscape(*project), &p); err != nil {
				return err
			}
			q.Set("project_id", fmt.Sprint(p.ID))
		}
		var ts []api.Task
		var ps []api.Project
		var rs []api.Repository
		if err := errors.Join(get("/tasks?"+q.Encode(), &ts), get("/projects", &ps), get("/repositories", &rs)); err != nil {
			return err
		}
		projects := map[int64]string{}
		for _, p := range ps {
			projects[p.ID] = p.Slug
		}
		repos := map[int64]string{}
		for _, r := range rs {
			repos[r.ID] = r.Name
		}
		return table("ID\tPROJECT/REPO\tKIND\tROLE\tSTATUS\tSTAGE\tTRIES\tTITLE", ts, func(t api.Task) string {
			return fmt.Sprintf("%d\t%s\t%s\t%s\t%s\t%s\t%d\t%s", t.ID, where(projects, repos, t), t.Kind, t.Role, t.Status, t.Stage, t.Attempts, t.Title)
		})

	case "task get":
		var d api.TaskDetail
		if err := get("/tasks/"+arg(args), &d); err != nil {
			return err
		}
		return show(d)

	case "task cancel", "task retry":
		return post("/tasks/"+arg(args)+"/"+strings.TrimPrefix(cmd, "task "), nil, nil)

	case "approvals":
		status := fs.String("status", "PENDING_APPROVAL", "")
		fs.Parse(args[1:])
		var rs []api.ApprovalRequest
		if err := get("/approvals?status="+url.QueryEscape(*status), &rs); err != nil {
			return err
		}
		if len(rs) == 0 {
			fmt.Println("nothing to review")
		}
		for _, r := range rs {
			fmt.Printf("#%d  %s  gate=%s  %s %s  by %s\n%s\n\n",
				r.ID, r.Status, r.Gate, r.SubjectType, r.SubjectRef, r.RequestedBy, indent(r.Summary))
		}
		return nil

	case "approve", "request-changes", "reject":
		m := fs.String("m", "", "comment")
		if len(args) < 2 {
			return fmt.Errorf("usage: empire %s ID [-m COMMENT]", cmd)
		}
		fs.Parse(args[2:])
		return post("/approvals/"+args[1]+"/"+cmd, api.Decision{Comment: *m}, nil)

	case "workflows":
		var wfs []struct {
			Name, Description string
			Steps             []string
		}
		if err := get("/workflows", &wfs); err != nil {
			return err
		}
		for _, w := range wfs {
			fmt.Printf("%s\n  %s\n  steps: %s\n\n", w.Name, w.Description, strings.Join(w.Steps, " → "))
		}
		return nil

	case "request create":
		var in api.CreateRequest
		fs.StringVar(&in.Project, "project", "", "")
		fs.StringVar(&in.Workflow, "workflow", "feature", "")
		fs.StringVar(&in.Repository, "repo", "", "")
		fs.StringVar(&in.Description, "desc", "", "")
		fs.Parse(args[1:])
		in.Title = strings.Join(fs.Args(), " ")
		var out api.Request
		if err := post("/requests", in, &out); err != nil {
			return err
		}
		fmt.Printf("created request %d (%s workflow), now at step %q. Follow it with: empire request get %d\n", out.ID, out.Workflow, out.CurrentStep, out.ID)
		return nil

	case "request list":
		project := fs.String("project", "", "")
		status := fs.String("status", "", "")
		fs.Parse(args[1:])
		var rs []api.Request
		if err := get("/requests?"+url.Values{"project": {*project}, "status": {*status}}.Encode(), &rs); err != nil {
			return err
		}
		return table("ID\tWORKFLOW\tSTATUS\tSTEP\tTITLE", rs, func(r api.Request) string {
			return fmt.Sprintf("%d\t%s\t%s\t%s\t%s", r.ID, r.Workflow, r.Status, r.CurrentStep, r.Title)
		})

	case "request get":
		var d api.RequestDetail
		if err := get("/requests/"+arg(args), &d); err != nil {
			return err
		}
		return printRequest(d)

	case "request cancel":
		return post("/requests/"+arg(args)+"/cancel", nil, nil)

	case "docs new":
		var in api.NewDocument
		fs.StringVar(&in.Project, "project", "", "")
		fs.StringVar(&in.Type, "type", "", "")
		fs.StringVar(&in.Title, "title", "", "")
		fs.StringVar(&in.Owner, "owner", "", "")
		fs.Parse(args[1:])
		var out api.DocumentInfo
		if err := post("/documents", in, &out); err != nil {
			return err
		}
		fmt.Printf("created %s: knowledge/%s\nFill in every {{ … }} placeholder, then: empire docs validate -project %s && empire docs submit %s -project %s\n",
			out.ID, out.Path, in.Project, out.ID, in.Project)
		return nil

	case "docs list":
		project := fs.String("project", "", "")
		typ := fs.String("type", "", "")
		fs.Parse(args[1:])
		var ds []api.DocumentInfo
		if err := get("/documents?"+url.Values{"project": {*project}, "type": {*typ}}.Encode(), &ds); err != nil {
			return err
		}
		return table("SCOPE\tID\tTYPE\tVER\tSTATUS\tAPPROVED\tTITLE", ds, func(d api.DocumentInfo) string {
			return fmt.Sprintf("%s\t%s\t%s\t%d\t%s\t%s\t%s", d.Scope, d.ID, d.Type, d.Version, d.Status, yesNo(d.Approved), d.Title)
		})

	case "docs validate":
		project := fs.String("project", "", "")
		local := fs.Bool("local", false, "validate files on disk without the control plane (CI, git hooks)")
		dir := fs.String("dir", "knowledge", "knowledge folder for -local")
		fs.Parse(args[1:])
		var ps []api.Problem
		if *local {
			// Offline: no approval records and no project→client map, so those checks are skipped.
			repo, err := docs.Load(*dir)
			if err != nil {
				return err
			}
			for _, p := range repo.Validate(docs.Env{}) {
				if *project == "" || strings.HasPrefix(p.Path, "projects/"+*project+"/") {
					ps = append(ps, api.Problem{Path: filepath.ToSlash(filepath.Join(*dir, p.Path)), Message: p.Msg})
				}
			}
		} else if err := post("/documents/validate", map[string]string{"project": *project}, &ps); err != nil {
			return err
		}
		if len(ps) == 0 {
			fmt.Println("all documents are valid")
			return nil
		}
		for _, p := range ps {
			fmt.Printf("%s: %s\n", p.Path, p.Message)
		}
		return fmt.Errorf("%d problem(s)", len(ps))

	case "docs submit":
		if len(args) < 2 {
			return errors.New("usage: empire docs submit PATH|KEY|ID [-project P]")
		}
		project := fs.String("project", "", "")
		fs.Parse(args[2:])
		var out api.ApprovalRequest
		if err := post("/documents/submit", api.DocumentRef{Ref: args[1], Project: *project}, &out); err != nil {
			return err
		}
		fmt.Printf("approval #%d opened for %s. Decide with: empire approve %d\n", out.ID, out.SubjectRef, out.ID)
		return nil

	case "docs reindex":
		var out map[string]int
		if err := post("/documents/reindex", nil, &out); err != nil {
			return err
		}
		fmt.Printf("indexed %d documents; Obsidian relation links refreshed\n", out["documents"])
		return nil

	case "docs export":
		project := fs.String("project", "", "")
		dir := fs.String("out", "", "")
		all := fs.Bool("all", false, "include documents that are not approved")
		fs.Parse(args[1:])
		if *dir == "" {
			*dir = filepath.Join("exports", *project)
		}
		var files []api.File
		if err := get("/documents/export?"+url.Values{"project": {*project}, "all": {fmt.Sprint(*all)}}.Encode(), &files); err != nil {
			return err
		}
		if err := os.MkdirAll(*dir, 0o755); err != nil {
			return err
		}
		for _, f := range files {
			if err := os.WriteFile(filepath.Join(*dir, filepath.Base(f.Name)), []byte(f.Content), 0o644); err != nil {
				return err
			}
		}
		fmt.Printf("exported %d document(s) to %s\n", len(files)-1, *dir)
		return nil

	case "trace":
		if len(args) < 2 {
			return errors.New("usage: empire trace REF [-project P]")
		}
		project := fs.String("project", "", "")
		fs.Parse(args[2:])
		refs := []string{args[1]}
		if tasks := taskTrailers(args[1]); tasks != nil { // a source file: follow its commits
			if len(tasks) == 0 {
				return fmt.Errorf("no commit touching %s has a \"Task:\" trailer", args[1])
			}
			refs = tasks
		}
		for _, ref := range refs {
			var tr api.Trace
			if err := get("/trace?"+url.Values{"ref": {ref}, "project": {*project}}.Encode(), &tr); err != nil {
				return err
			}
			printTrace(tr)
		}
		return nil

	case "impact":
		if len(args) < 2 {
			return errors.New("usage: empire impact DOC [-project P]")
		}
		project := fs.String("project", "", "")
		fs.Parse(args[2:])
		var imp api.Impact
		if err := get("/impact?"+url.Values{"doc": {args[1]}, "project": {*project}}.Encode(), &imp); err != nil {
			return err
		}
		fmt.Print(imp.Report)
		return nil

	case "workers":
		var ws []api.Worker
		if err := get("/workers", &ws); err != nil {
			return err
		}
		return table("ID\tNAME\tSTATUS\tLAST HEARTBEAT\tCAPABILITIES", ws, func(w api.Worker) string {
			return fmt.Sprintf("%d\t%s\t%s\t%s\t%s", w.ID, w.Name, w.Status, w.LastHeartbeatAt.Local().Format("15:04:05"), strings.Join(w.Capabilities, ","))
		})

	case "audit":
		target := fs.String("target", "", "")
		fs.Parse(args[1:])
		var es []api.AuditEntry
		if err := get("/audit?target="+url.QueryEscape(*target), &es); err != nil {
			return err
		}
		return table("ID\tWHEN\tACTOR\tACTION\tTARGET\tPAYLOAD", es, func(e api.AuditEntry) string {
			p, _ := json.Marshal(e.Payload)
			return fmt.Sprintf("%d\t%s\t%s:%s\t%s\t%s\t%.100s", e.ID, e.CreatedAt.Local().Format("01-02 15:04:05"), e.ActorType, e.ActorID, e.Action, e.Target, p)
		})
	}
	fs.Usage()
	os.Exit(2)
	return nil
}

// flagSet records which flags were given explicitly, so "-test ”" can clear a value
// while an omitted flag leaves it unchanged.
type flagSet map[string]bool

func setFlags(fs *flag.FlagSet) flagSet {
	given := flagSet{}
	fs.Visit(func(f *flag.Flag) { given[f.Name] = true })
	return given
}

func (g flagSet) ptr(name, value string) *string {
	if !g[name] {
		return nil
	}
	return &value
}

func where(projects, repos map[int64]string, t api.Task) string {
	if t.RepositoryID == nil {
		return projects[t.ProjectID]
	}
	return projects[t.ProjectID] + "/" + repos[*t.RepositoryID]
}

func yesNo(b bool) string {
	if b {
		return "yes"
	}
	return "no"
}

func printRequest(d api.RequestDetail) error {
	r := d.Request
	fmt.Printf("Request #%d: %s\nWorkflow: %s   Status: %s   Current step: %s\n", r.ID, r.Title, r.Workflow, r.Status, r.CurrentStep)
	if r.Description != "" {
		fmt.Printf("\n%s\n", r.Description)
	}
	fmt.Println("\nSteps:")
	for _, s := range d.Steps {
		fmt.Printf("  %-18s %-9s %-16s %s\n", s.Name, s.Kind, s.Role, s.Status)
	}
	if len(d.Tasks) > 0 {
		fmt.Println("\nTasks:")
		table("  ID\tSTEP\tKIND\tSTATUS\tTITLE", d.Tasks, func(t api.Task) string {
			return fmt.Sprintf("  %d\t%s\t%s\t%s\t%s", t.ID, t.Step, t.Kind, t.Status, t.Title)
		})
	}
	if len(d.Documents) > 0 {
		fmt.Println("\nDocuments:")
		table("  ID\tVER\tSTATUS\tFILE", d.Documents, func(x api.DocumentInfo) string {
			return fmt.Sprintf("  %s\t%d\t%s\tknowledge/%s", x.ID, x.Version, x.Status, x.Path)
		})
	}
	var open []string
	for _, a := range d.Approvals {
		if a.Status == "PENDING_APPROVAL" {
			open = append(open, fmt.Sprintf("#%d", a.ID))
		}
	}
	if len(open) > 0 {
		fmt.Printf("\nWaiting for you: approval %s (see: empire approvals)\n", strings.Join(open, ", "))
	}
	return nil
}

func printTrace(tr api.Trace) {
	var walk func(ns []api.TraceNode, indent string)
	walk = func(ns []api.TraceNode, indent string) {
		for _, n := range ns {
			via := ""
			if n.Via != "" {
				via = "  [" + n.Via + "]"
			}
			status := ""
			if n.Status != "" {
				status = " (" + n.Status + ")"
			}
			fmt.Printf("%s- %s %s%s%s\n", indent, n.Kind, n.Title, status, via)
			walk(n.Children, indent+"    ")
		}
	}
	fmt.Printf("%s %s: %s\n", tr.Subject.Kind, tr.Subject.Ref, tr.Subject.Title)
	walk(tr.Subject.Children, "  ")
	fmt.Println("\nWhy it exists:")
	if len(tr.Why) == 0 {
		fmt.Println("  (nothing upstream)")
	}
	walk(tr.Why, "  ")
	fmt.Println("\nWhat depends on it:")
	if len(tr.Effects) == 0 {
		fmt.Println("  (nothing downstream)")
	}
	walk(tr.Effects, "  ")
	fmt.Println()
}

var trailerRe = regexp.MustCompile(`(?m)^Task:\s*(\d+)\s*$`)

// taskTrailers returns "task:N" refs from the git history of a local source
// file, or nil if ref is not a file inside a git repository.
func taskTrailers(ref string) []string {
	info, err := os.Stat(ref)
	if err != nil || info.IsDir() || strings.HasSuffix(ref, ".md") && strings.Contains(filepath.ToSlash(ref), "knowledge/") {
		return nil
	}
	abs, _ := filepath.Abs(ref)
	cmd := exec.Command("git", "log", "--format=%B%x00", "--", filepath.Base(abs))
	cmd.Dir = filepath.Dir(abs)
	out, err := cmd.Output()
	if err != nil {
		return nil
	}
	refs := []string{}
	for _, m := range trailerRe.FindAllStringSubmatch(string(out), -1) {
		if r := "task:" + m[1]; !slices.Contains(refs, r) {
			refs = append(refs, r)
		}
	}
	return refs
}

func arg(args []string) string {
	if len(args) < 2 {
		fmt.Fprint(os.Stderr, usage)
		os.Exit(2)
	}
	return url.PathEscape(args[1])
}

func show(v any) error {
	e := json.NewEncoder(os.Stdout)
	e.SetIndent("", "  ")
	e.SetEscapeHTML(false)
	return e.Encode(v)
}

func table[T any](header string, rows []T, line func(T) string) error {
	tw := tabwriter.NewWriter(os.Stdout, 0, 4, 2, ' ', 0)
	fmt.Fprintln(tw, header)
	for _, r := range rows {
		fmt.Fprintln(tw, line(r))
	}
	return tw.Flush()
}

func indent(s string) string {
	if s == "" {
		return ""
	}
	return "    " + strings.ReplaceAll(strings.TrimRight(s, " \t\r\n"), "\n", "\n    ")
}
