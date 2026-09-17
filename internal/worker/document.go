package worker

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"aiempire/internal/api"
	"aiempire/internal/task"
)

// jobFile (.empire/job.json) describes the task in machine-readable form for the agent.
type jobFile struct {
	TaskID       int64               `json:"task_id"`
	Kind         string              `json:"kind"`
	Role         string              `json:"role"`
	DocType      string              `json:"doc_type,omitempty"`
	ExtraTypes   []string            `json:"extra_types,omitempty"`
	Title        string              `json:"title"`
	Request      string              `json:"request,omitempty"`
	Description  string              `json:"description,omitempty"`
	Feedback     string              `json:"feedback,omitempty"`
	ReviewRounds int                 `json:"review_rounds"`
	Repositories []string            `json:"repositories,omitempty"`
	Relations    map[string][]string `json:"relations,omitempty"`
}

func writeJob(dir string, cl api.Claim, j jobFile) error {
	t := cl.Task
	j.TaskID, j.Title, j.Feedback, j.ReviewRounds = t.ID, t.Title, t.Feedback, t.ReviewRounds
	if j.Kind == "" {
		j.Kind = t.Kind
	}
	if j.Role == "" {
		j.Role = t.Role
	}
	if j.Description == "" {
		j.Description = t.Description
	}
	if err := os.MkdirAll(filepath.Join(dir, ".empire"), 0o755); err != nil {
		return err
	}
	data, _ := json.MarshalIndent(j, "", "  ")
	return os.WriteFile(filepath.Join(dir, ".empire", "job.json"), data, 0o644)
}

const maxDocAttempts = 3

// document runs a document, plan, or revise task: the agent writes Markdown into
// output/, the control plane validates it, and invalid output goes back to the
// agent with the problems (spec §18).
func (w *Worker) document(ctx context.Context, cl api.Claim) error {
	t := cl.Task
	dir := filepath.Join(w.cfg.Workspaces, cl.Project.Slug, "_docs", fmt.Sprintf("task-%d", t.ID))
	os.RemoveAll(dir)
	defer os.RemoveAll(dir)
	for _, d := range []string{"output", ".empire/templates", ".empire/contracts"} {
		if err := os.MkdirAll(filepath.Join(dir, d), 0o755); err != nil {
			return err
		}
	}

	tc, err := w.context(ctx, t.ID)
	if err != nil {
		return err
	}
	files := map[string]string{contextFile: tc.Content}
	var extras []string
	for typ, body := range tc.Templates {
		files[".empire/templates/"+typ+".md"] = body
		if typ != tc.DocType {
			extras = append(extras, typ)
		}
	}
	sort.Strings(extras)
	for typ, body := range tc.Contracts {
		files[".empire/contracts/"+typ+".yaml"] = body
	}
	for _, f := range tc.Drafts {
		files["output/"+filepath.Base(f.Name)] = f.Content
	}
	for rel, body := range files {
		if err := os.WriteFile(filepath.Join(dir, filepath.FromSlash(rel)), []byte(body), 0o644); err != nil {
			return err
		}
	}
	j := jobFile{DocType: tc.DocType, ExtraTypes: extras, Relations: tc.Relations}
	if tc.Request != nil {
		j.Request, j.Description = tc.Request.Title, tc.Request.Description
	}
	for _, r := range tc.Repositories {
		j.Repositories = append(j.Repositories, r.Name)
	}
	if err := writeJob(dir, cl, j); err != nil {
		return err
	}

	var problems []string
	for attempt := 1; attempt <= maxDocAttempts; attempt++ {
		summary, err := w.runAgent(ctx, cl, Job{Dir: dir, Prompt: documentPrompt(cl, tc, extras, problems)}, t.Role, tc.Files)
		if err != nil {
			return err
		}
		out, err := readOutput(filepath.Join(dir, "output"))
		if err != nil {
			return err
		}
		if len(out) == 0 {
			problems = []string{"No Markdown file was written to output/."}
			continue
		}
		var reply api.OutputReply
		if err := w.c.Do(ctx, "POST", fmt.Sprintf("/tasks/%d/output", t.ID), api.TaskOutput{Files: out, Summary: summary}, &reply); err != nil {
			return err
		}
		if reply.Accepted {
			log.Printf("task %d: documents accepted: %s", t.ID, strings.Join(reply.Documents, ", "))
			if reply.Status == task.Completed {
				return nil
			}
			return errParked
		}
		problems = reply.Problems
		log.Printf("task %d: attempt %d rejected by validation (%d problems)", t.ID, attempt, len(problems))
	}
	return fmt.Errorf("documents still invalid after %d attempts:\n%s", maxDocAttempts, strings.Join(problems, "\n"))
}

func readOutput(dir string) ([]api.File, error) {
	paths, err := filepath.Glob(filepath.Join(dir, "*.md"))
	if err != nil {
		return nil, err
	}
	var out []api.File
	for _, p := range paths {
		data, err := os.ReadFile(p)
		if err != nil {
			return nil, err
		}
		out = append(out, api.File{Name: filepath.Base(p), Content: string(data)})
	}
	return out, nil
}
