// Package worker claims tasks and executes them in isolated git worktrees (spec §27).
package worker

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"aiempire/internal/api"
	"aiempire/internal/client"
	"aiempire/internal/policy"
	"aiempire/internal/task"
)

type Config struct {
	Name         string
	Capabilities []string
	Workspaces   string // root for clones, worktrees and run logs
	Agent        Agent
	Poll         time.Duration
	Heartbeat    time.Duration
}

type Worker struct {
	c   *client.Client
	cfg Config

	mu      sync.Mutex
	current int64 // task being executed, 0 if idle
	cancel  context.CancelFunc
}

// errParked means the task left this worker on purpose (e.g. waiting for a human).
var errParked = errors.New("task parked")

func New(c *client.Client, cfg Config) (*Worker, error) {
	abs, err := filepath.Abs(cfg.Workspaces)
	if err != nil {
		return nil, err
	}
	cfg.Workspaces = abs
	if cfg.Poll <= 0 {
		cfg.Poll = 5 * time.Second
	}
	if cfg.Heartbeat <= 0 {
		cfg.Heartbeat = 10 * time.Second
	}
	return &Worker{c: c, cfg: cfg}, nil
}

func (w *Worker) Register(ctx context.Context) error {
	var reg api.Worker
	w.c.WorkerID = 0
	err := w.c.Do(ctx, "POST", "/workers/register",
		api.RegisterWorker{Name: w.cfg.Name, Capabilities: w.cfg.Capabilities}, &reg)
	if err != nil {
		return err
	}
	w.c.WorkerID = reg.ID
	log.Printf("registered as worker %d (%s) caps=%v", reg.ID, reg.Name, reg.Capabilities)
	return nil
}

// Run registers, then heartbeats and processes tasks until ctx ends.
func (w *Worker) Run(ctx context.Context) error {
	if err := w.Register(ctx); err != nil {
		return fmt.Errorf("register: %w", err)
	}
	go w.heartbeatLoop(ctx)
	for ctx.Err() == nil {
		worked, err := w.Step(ctx)
		if err != nil {
			log.Printf("step: %v", err)
		}
		if !worked {
			select {
			case <-ctx.Done():
			case <-time.After(w.cfg.Poll):
			}
		}
	}
	return nil
}

func (w *Worker) heartbeatLoop(ctx context.Context) {
	t := time.NewTicker(w.cfg.Heartbeat)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
		}
		w.mu.Lock()
		taskID := w.current
		w.mu.Unlock()
		var reply api.HeartbeatReply
		err := w.c.Do(ctx, "POST", w.selfPath("heartbeat"), api.Heartbeat{TaskID: taskID}, &reply)
		var ce *client.Error
		if errors.As(err, &ce) && ce.Status == http.StatusNotFound {
			// Control plane forgot us (e.g. DB reset): register again.
			err = w.Register(ctx)
		}
		if err != nil {
			log.Printf("heartbeat: %v", err)
			continue
		}
		if reply.Cancel {
			w.mu.Lock()
			if w.current == taskID && w.cancel != nil {
				log.Printf("task %d: no longer ours, stopping", taskID)
				w.cancel()
			}
			w.mu.Unlock()
		}
	}
}

func (w *Worker) selfPath(action string) string {
	return fmt.Sprintf("/workers/%d/%s", w.c.WorkerID, action)
}

// Step claims and executes at most one task. It reports whether a task was claimed.
func (w *Worker) Step(ctx context.Context) (bool, error) {
	var cl *api.Claim
	if err := w.c.Do(ctx, "POST", w.selfPath("claim"), nil, &cl); err != nil {
		return false, err
	}
	if cl == nil {
		return false, nil
	}
	t := cl.Task
	log.Printf("task %d: claimed (%s %s, %s, stage %s, attempt %d)", t.ID, t.Kind, t.Role, t.Title, t.Stage, t.Attempts)

	tctx, cancel := context.WithCancel(ctx)
	defer cancel()
	w.mu.Lock()
	w.current, w.cancel = t.ID, cancel
	w.mu.Unlock()
	defer func() {
		w.mu.Lock()
		w.current, w.cancel = 0, nil
		w.mu.Unlock()
	}()

	err := w.execute(tctx, *cl)
	switch {
	case err == nil:
		log.Printf("task %d: done", t.ID)
	case errors.Is(err, errParked):
		log.Printf("task %d: waiting for human approval", t.ID)
	case errors.Is(err, errRework):
		log.Printf("task %d: AI review requested changes; back to the developer", t.ID)
	default:
		log.Printf("task %d: failed: %v", t.ID, err)
		// Parent ctx: report even if the task ctx was cancelled. If the task was
		// cancelled by the owner this is rejected, which is fine.
		if terr := w.transition(ctx, t.ID, task.Failed, tail(err.Error(), 4000)); terr != nil {
			log.Printf("task %d: report failure: %v", t.ID, terr)
		}
	}
	return true, nil
}

// errRework means the AI reviewer sent the task back to the developer.
var errRework = errors.New("sent back for rework")

func (w *Worker) execute(ctx context.Context, cl api.Claim) error {
	t, repo := cl.Task, cl.Repository
	if err := w.transition(ctx, t.ID, task.Running, ""); err != nil {
		return err
	}
	if t.Kind != "code" {
		return w.document(ctx, cl)
	}
	// One clone per repository; worktrees per task (spec §11A, §29).
	repoDir := filepath.Join(w.cfg.Workspaces, cl.Project.Slug, repo.Name)
	base := filepath.Join(repoDir, "_base")
	if err := ensureBase(ctx, base, repo.RepoURL); err != nil {
		return err
	}
	dir := filepath.Join(repoDir, fmt.Sprintf("task-%d", t.ID))
	defer removeWorktree(context.WithoutCancel(ctx), base, dir)

	switch t.Stage {
	case task.StageImplement:
		return w.implement(ctx, cl, base, dir)
	case task.StageMerge:
		return w.merge(ctx, cl, base, dir)
	}
	return fmt.Errorf("unknown stage %q", t.Stage)
}

func branchName(t api.Task) string { return fmt.Sprintf("ai/task-%d", t.ID) }

func (w *Worker) implement(ctx context.Context, cl api.Claim, base, dir string) error {
	t, repo := cl.Task, cl.Repository
	branch := branchName(t)
	// Rework after "request changes" continues from the previously pushed branch.
	start := "origin/" + repo.DefaultBranch
	if refExists(ctx, base, "refs/remotes/origin/"+branch) {
		start = "origin/" + branch
	}
	if err := addWorktree(ctx, base, dir, start, "-B", branch); err != nil {
		return err
	}
	startSHA, err := git(ctx, dir, "rev-parse", "HEAD")
	if err != nil {
		return err
	}

	tc, err := w.context(ctx, t.ID)
	if err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(dir, contextFile), []byte(tc.Content), 0o644); err != nil {
		return err
	}

	agentSummary, err := w.runAgent(ctx, cl, Job{Dir: dir, Prompt: developerPrompt(cl)}, t.Role, tc.Files)
	if err != nil {
		return err
	}

	if err := w.transition(ctx, t.ID, task.Testing, ""); err != nil {
		return err
	}
	if err := w.test(ctx, t, repo, dir); err != nil {
		return err
	}

	if _, err := git(ctx, dir, "add", "-A"); err != nil {
		return err
	}
	if _, err := git(ctx, dir, "diff", "--cached", "--quiet"); err != nil {
		msg := fmt.Sprintf("%s\n\nTask: %d", t.Title, t.ID)
		if _, err := git(ctx, dir, "-c", "user.name=empire-worker", "-c", "user.email=worker@empire.local",
			"commit", "-m", msg); err != nil {
			return err
		}
	}
	head, err := git(ctx, dir, "rev-parse", "HEAD")
	if err != nil {
		return err
	}
	if head == startSHA {
		return errors.New("agent made no changes")
	}

	if ok, _, err := w.authorize(ctx, t, policy.PushBranch, task.StageImplement, "", head); err != nil || !ok {
		return orParked(err)
	}
	if _, err := git(ctx, dir, "push", "origin", "HEAD:refs/heads/"+branch); err != nil {
		return err
	}

	diffRange := "origin/" + repo.DefaultBranch + "..." + head
	var review string
	if t.Review {
		if review, err = w.review(ctx, cl, dir, diffRange, tc.Files); err != nil {
			return err
		}
	}

	diffstat, _ := git(ctx, dir, "diff", "--stat", diffRange)
	if agentSummary == "" {
		agentSummary = "(the agent gave no summary)"
	}
	summary := fmt.Sprintf("Merge %s (%s) into %s:%s for task #%d: %s\n\nAgent summary:\n%s\n\n%s%s",
		branch, head[:min(12, len(head))], repo.Name, repo.DefaultBranch, t.ID, t.Title,
		truncate(agentSummary, 3000), review, diffstat)
	ok, _, err := w.authorize(ctx, t, policy.MergeProtected, task.StageMerge, summary, head)
	if err != nil {
		return err
	}
	if ok {
		return errors.New("policy allowed an unreviewed merge to a protected branch; refusing")
	}
	return errParked
}

// review runs an independent, read-only reviewer agent on the change (spec §7 "AI Review", §26).
// It returns the text for the approval request, or errRework if the task went back to the developer.
func (w *Worker) review(ctx context.Context, cl api.Claim, dir, diffRange string, files []string) (string, error) {
	t := cl.Task
	if err := w.transition(ctx, t.ID, task.Reviewing, ""); err != nil {
		return "", err
	}
	diff, err := git(ctx, dir, "diff", diffRange)
	if err != nil {
		return "", err
	}
	if err := writeJob(dir, cl, jobFile{Role: "reviewer"}); err != nil {
		return "", err
	}
	if err := os.WriteFile(filepath.Join(dir, ".empire", "review.diff"), []byte(diff), 0o644); err != nil {
		return "", err
	}
	out, err := w.runAgent(ctx, cl, Job{Dir: dir, Prompt: reviewerPrompt(cl), ReadOnly: true}, "reviewer", files)
	if err != nil {
		return "", err
	}
	verdict := parseVerdict(out)
	var reply api.ReviewReply
	if err := w.c.Do(ctx, "POST", fmt.Sprintf("/tasks/%d/review", t.ID),
		api.ReviewReport{Verdict: verdict, Summary: truncate(out, 20000)}, &reply); err != nil {
		return "", err
	}
	if reply.Rework {
		return "", errRework
	}
	note := ""
	if verdict == "REQUEST_CHANGES" {
		note = " (still requesting changes after the maximum review rounds; please look closely)"
	}
	return fmt.Sprintf("AI review: %s%s\n%s\n\n", verdict, note, truncate(out, 3000)), nil
}

func parseVerdict(s string) string {
	lines := strings.Split(strings.TrimSpace(s), "\n")
	for i := len(lines) - 1; i >= 0 && i >= len(lines)-5; i-- {
		l := strings.ToUpper(strings.Trim(lines[i], " *`_#"))
		switch {
		case strings.HasPrefix(l, "VERDICT: APPROVE"):
			return "APPROVE"
		case strings.HasPrefix(l, "VERDICT: REQUEST_CHANGES"):
			return "REQUEST_CHANGES"
		}
	}
	return "NONE"
}

func (w *Worker) merge(ctx context.Context, cl api.Claim, base, dir string) error {
	t, repo := cl.Task, cl.Repository
	ok, sha, err := w.authorize(ctx, t, policy.MergeProtected, task.StageMerge, "", "")
	if err != nil || !ok {
		return orParked(err)
	}
	if sha == "" {
		return errors.New("approval has no commit to merge")
	}
	if err := addWorktree(ctx, base, dir, "origin/"+repo.DefaultBranch, "--detach"); err != nil {
		return err
	}

	// Merge exactly the commit the human approved, not whatever the branch holds now.
	msg := fmt.Sprintf("Merge task #%d: %s\n\nTask: %d", t.ID, t.Title, t.ID)
	if _, err := git(ctx, dir, "-c", "user.name=empire-worker", "-c", "user.email=worker@empire.local",
		"merge", "--no-ff", "-m", msg, sha); err != nil {
		git(ctx, dir, "merge", "--abort")
		return fmt.Errorf("merge conflict, rework needed: %w", err)
	}
	if err := w.transition(ctx, t.ID, task.Testing, ""); err != nil {
		return err
	}
	if err := w.test(ctx, t, repo, dir); err != nil {
		return fmt.Errorf("tests failed after merge: %w", err)
	}
	if _, err := git(ctx, dir, "push", "origin", "HEAD:refs/heads/"+repo.DefaultBranch); err != nil {
		return err
	}
	merged, err := git(ctx, dir, "rev-parse", "HEAD")
	if err != nil {
		return err
	}
	// Files the merge brought into the default branch (for impact analysis, spec §37).
	var files []string
	if out, err := git(ctx, dir, "diff", "--name-only", "HEAD^1", "HEAD"); err == nil && out != "" {
		for _, f := range strings.Split(out, "\n") {
			if f = strings.TrimSpace(f); f != "" {
				files = append(files, f)
			}
		}
	}
	return w.c.Do(ctx, "POST", fmt.Sprintf("/tasks/%d/transition", t.ID),
		api.Transition{To: task.Completed, MergeSHA: merged, ChangedFiles: files}, nil)
}

func (w *Worker) context(ctx context.Context, id int64) (api.TaskContext, error) {
	var tc api.TaskContext
	if err := w.c.Do(ctx, "GET", fmt.Sprintf("/tasks/%d/context", id), nil, &tc); err != nil {
		return tc, fmt.Errorf("load context: %w", err)
	}
	return tc, nil
}

// runAgent runs the agent once, records the run, and returns the agent's summary.
func (w *Worker) runAgent(ctx context.Context, cl api.Claim, job Job, role string, files []string) (string, error) {
	t := cl.Task
	var run api.ID
	err := w.c.Do(ctx, "POST", fmt.Sprintf("/tasks/%d/runs", t.ID),
		api.StartRun{Model: w.cfg.Agent.Name(), ContextFiles: files, Role: role}, &run)
	if err != nil {
		return "", err
	}
	logDir := filepath.Join(w.cfg.Workspaces, "logs")
	os.MkdirAll(logDir, 0o755)
	logPath := filepath.Join(logDir, fmt.Sprintf("task-%d-run-%d.log", t.ID, run.ID))
	f, err := os.Create(logPath)
	if err != nil {
		return "", err
	}
	job.Log = f
	res, runErr := w.cfg.Agent.Run(ctx, job)
	f.Close()

	logData, _ := os.ReadFile(logPath)
	finish := api.FinishRun{ExitStatus: res.ExitStatus, LogPath: logPath, Tokens: res.Tokens, CostUSD: res.CostUSD,
		Summary: truncate(res.Summary, 10000), LogTail: strings.ToValidUTF8(tail(string(logData), 8000), "")}
	if err := w.c.Do(context.WithoutCancel(ctx), "POST", fmt.Sprintf("/runs/%d/finish", run.ID), finish, nil); err != nil {
		log.Printf("task %d: record run finish: %v", t.ID, err)
	}
	if runErr != nil {
		return "", fmt.Errorf("agent failed (log %s): %w", logPath, runErr)
	}
	return res.Summary, nil
}

// truncate keeps the first n bytes of s, cut on a rune boundary (Postgres rejects invalid UTF-8).
func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	for n > 0 && !utf8.RuneStart(s[n]) {
		n--
	}
	return s[:n] + "…"
}

func (w *Worker) test(ctx context.Context, t api.Task, repo api.Repository, dir string) error {
	if repo.TestCommand == "" {
		return nil
	}
	out, err := shell(ctx, dir, repo.TestCommand)
	if err != nil {
		return fmt.Errorf("tests failed: %w", err)
	}
	log.Printf("task %d: tests passed: %s", t.ID, tail(out, 200))
	return nil
}

func (w *Worker) transition(ctx context.Context, id int64, to, errMsg string) error {
	return w.c.Do(ctx, "POST", fmt.Sprintf("/tasks/%d/transition", id), api.Transition{To: to, Error: errMsg}, nil)
}

// authorize asks the control plane's policy. !ok means the task is now waiting for a human.
func (w *Worker) authorize(ctx context.Context, t api.Task, action, stage, summary, version string) (bool, string, error) {
	var reply api.AuthorizeReply
	err := w.c.Do(ctx, "POST", fmt.Sprintf("/tasks/%d/authorize", t.ID), api.Authorize{
		Action: action, Stage: stage, Summary: summary, SubjectVersion: version,
	}, &reply)
	return reply.Allowed, reply.SubjectVersion, err
}

func orParked(err error) error {
	if err != nil {
		return err
	}
	return errParked
}
