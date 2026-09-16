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
	"strings"
	"text/tabwriter"

	"aiempire/internal/api"
	"aiempire/internal/client"
)

const usage = `usage: empire <command> [flags] [args]

  client create -slug S -name N [-autonomy conservative|medium|high]
  client list
  project create -slug S -name N [-client C] [-autonomy conservative|medium|high]
  project list
  project move PROJECT -client C | -none
  repo add -project P -name N -repo URL -stack go [-branch main] [-test CMD]
  repo list [-project P]
  task create -project P [-repo NAME] [-desc TEXT] [-doc PATH]... [-cap CAP]... [-after ID]... TITLE...
  task list [-status S] [-project P]
  task get ID
  task cancel ID
  task retry ID
  approvals [-status PENDING_APPROVAL]
  approve ID [-m COMMENT]
  request-changes ID -m COMMENT
  reject ID [-m COMMENT]
  workers
  audit [-target task:ID]

env: EMPIRE_CP_URL (default http://localhost:8080), EMPIRE_OWNER_TOKEN
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
		cp = "http://localhost:8080"
	}
	c := client.New(cp, os.Getenv("EMPIRE_OWNER_TOKEN"))
	if err := dispatch(context.Background(), c, os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}

func dispatch(ctx context.Context, c *client.Client, args []string) error {
	cmd := args[0]
	if (cmd == "client" || cmd == "project" || cmd == "repo" || cmd == "task") && len(args) > 1 {
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
		return table("ID\tPROJECT/REPO\tSTATUS\tSTAGE\tTRIES\tTITLE", ts, func(t api.Task) string {
			return fmt.Sprintf("%d\t%s/%s\t%s\t%s\t%d\t%s", t.ID, projects[t.ProjectID], repos[t.RepositoryID], t.Status, t.Stage, t.Attempts, t.Title)
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
