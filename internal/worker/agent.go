package worker

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"aiempire/internal/docs"
)

// Agent is an AI agent working inside a task directory (spec §32).
type Agent interface {
	Name() string
	Run(ctx context.Context, job Job) (Result, error)
}

// Job is one agent run.
type Job struct {
	Dir      string    // working directory; the agent must stay inside it
	Prompt   string    // sent on stdin
	Log      io.Writer // full agent output
	ReadOnly bool      // reviewer runs: may read, never edit
}

type Result struct {
	ExitStatus int
	Tokens     int64
	CostUSD    float64
	Summary    string // the agent's own account of what it did
}

// ClaudeCode runs Claude Code headless. acceptEdits lets it edit files but not
// run shell commands; plan mode is read-only. The worker runs tests and git itself.
type ClaudeCode struct {
	Model string // optional --model
}

func (c ClaudeCode) Name() string {
	if c.Model == "" {
		return "claude-code"
	}
	return "claude-code:" + c.Model
}

func (c ClaudeCode) Run(ctx context.Context, job Job) (Result, error) {
	mode := "acceptEdits"
	if job.ReadOnly {
		mode = "plan"
	}
	args := []string{"-p", "--output-format", "json", "--permission-mode", mode}
	if c.Model != "" {
		args = append(args, "--model", c.Model)
	}
	cmd := exec.CommandContext(ctx, "claude", args...)
	cmd.Dir = job.Dir
	cmd.Env = agentEnv()
	cmd.Stdin = strings.NewReader(job.Prompt)
	var out bytes.Buffer
	cmd.Stdout = io.MultiWriter(&out, job.Log)
	cmd.Stderr = job.Log
	err := cmd.Run()

	res := Result{ExitStatus: -1}
	if cmd.ProcessState != nil {
		res.ExitStatus = cmd.ProcessState.ExitCode()
	}
	var reply struct {
		IsError      bool    `json:"is_error"`
		Result       string  `json:"result"`
		TotalCostUSD float64 `json:"total_cost_usd"`
		Usage        struct {
			Input       int64 `json:"input_tokens"`
			Output      int64 `json:"output_tokens"`
			CacheCreate int64 `json:"cache_creation_input_tokens"`
			CacheRead   int64 `json:"cache_read_input_tokens"`
		} `json:"usage"`
	}
	if jerr := json.Unmarshal(out.Bytes(), &reply); jerr == nil {
		u := reply.Usage
		res.Tokens = u.Input + u.Output + u.CacheCreate + u.CacheRead
		res.CostUSD = reply.TotalCostUSD
		res.Summary = strings.TrimSpace(reply.Result)
		if reply.IsError && err == nil {
			err = fmt.Errorf("agent reported error: %s", reply.Result)
		}
	}
	return res, err
}

// agentEnv is the worker's environment minus platform credentials: the agent
// must not be able to call the control plane or the database as the worker.
func agentEnv() []string {
	var env []string
	for _, kv := range os.Environ() {
		k, _, _ := strings.Cut(kv, "=")
		if strings.HasPrefix(strings.ToUpper(k), "EMPIRE_") || strings.EqualFold(k, "DATABASE_URL") {
			continue
		}
		env = append(env, kv)
	}
	return env
}

// Fake is a stand-in agent for tests and free dry runs. It writes predictable,
// valid output for every task kind, driven by .empire/job.json:
//   - code: appends a line to empire-fake-agent.txt
//   - review: approves, or requests changes once if the task title contains "[rework]"
//   - documents: fills the template with example content (a plan gets one work item per
//     repository; a change request affects the ids listed after "affects:" in the request)
type Fake struct{}

func (Fake) Name() string { return "fake" }

func (Fake) Run(ctx context.Context, job Job) (Result, error) {
	var j jobFile
	if data, err := os.ReadFile(filepath.Join(job.Dir, ".empire", "job.json")); err == nil {
		if err := json.Unmarshal(data, &j); err != nil {
			return Result{ExitStatus: 1}, err
		}
	}
	var summary string
	var err error
	switch {
	case j.Role == "reviewer":
		summary = "No blocking issues.\nVERDICT: APPROVE"
		if strings.Contains(j.Title, "[rework]") && j.ReviewRounds == 0 {
			summary = "Blocking: the fake reviewer always asks for one more pass on [rework] tasks.\nVERDICT: REQUEST_CHANGES"
		}
	case j.Kind == "" || j.Kind == "code":
		first, _, _ := strings.Cut(job.Prompt, "\n")
		line := fmt.Sprintf("%s %s\n", time.Now().UTC().Format(time.RFC3339Nano), first)
		err = appendFile(filepath.Join(job.Dir, "empire-fake-agent.txt"), line)
		summary = "fake agent wrote: " + strings.TrimSpace(line)
		if repos, _ := filepath.Glob(filepath.Join(job.Dir, ".empire", "repos", "*", ".git")); len(repos) > 0 {
			summary += fmt.Sprintf(" (sees %d sibling repo(s))", len(repos))
		}
	default:
		summary, err = fakeDocuments(job.Dir, j)
	}
	if err != nil {
		return Result{ExitStatus: 1}, err
	}
	fmt.Fprintln(job.Log, summary)
	return Result{Summary: summary}, nil
}

func appendFile(path, line string) error {
	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	defer f.Close()
	_, err = f.WriteString(line)
	return err
}

func fakeDocuments(dir string, j jobFile) (string, error) {
	out := filepath.Join(dir, "output")
	drafts, _ := filepath.Glob(filepath.Join(out, "*.md"))
	if len(drafts) > 0 { // rework or revision: touch the existing documents
		for _, p := range drafts {
			if err := appendFile(p, fmt.Sprintf("\nRevised by the fake agent (%s).\n", time.Now().UTC().Format(time.RFC3339Nano))); err != nil {
				return "", err
			}
		}
		return fmt.Sprintf("fake agent revised %d document(s)", len(drafts)), nil
	}
	write := func(typ string) (*docs.Doc, error) {
		tpl, err := os.ReadFile(filepath.Join(dir, ".empire", "templates", typ+".md"))
		if err != nil {
			return nil, err
		}
		d, err := docs.Example("projects/x/"+typ+".md", tpl)
		if err != nil {
			return nil, err
		}
		d.Set("id", docs.NewID)
		d.Set("title", j.Title)
		return d, nil
	}
	d, err := write(j.DocType)
	if err != nil {
		return "", err
	}
	switch j.DocType {
	case "implementation-plan":
		var items []docs.PlanTask
		for i, r := range j.Repositories {
			it := docs.PlanTask{Key: r, Repository: r, Title: "Implement " + j.Request + " in " + r, Description: "Fake work item."}
			if i > 0 {
				it.DependsOn = []string{j.Repositories[i-1]}
			}
			items = append(items, it)
		}
		d.SetWorkBreakdown(items)
	case "change-request":
		_, list, _ := strings.Cut(j.Description, "affects:")
		list, _, _ = strings.Cut(list, "\n")
		var ids []string
		for _, id := range strings.Split(list, ",") {
			if id = strings.TrimSpace(id); id != "" {
				ids = append(ids, id)
			}
		}
		d.Set("affects", ids)
	}
	if err := os.WriteFile(filepath.Join(out, j.DocType+".md"), d.Bytes(), 0o644); err != nil {
		return "", err
	}
	n := 1
	for _, extra := range j.ExtraTypes {
		x, err := write(extra)
		if err != nil {
			return "", err
		}
		x.Set("title", j.Title+" decision")
		if err := os.WriteFile(filepath.Join(out, extra+".md"), x.Bytes(), 0o644); err != nil {
			return "", err
		}
		n++
	}
	return fmt.Sprintf("fake agent wrote %d document(s) from the templates", n), nil
}
