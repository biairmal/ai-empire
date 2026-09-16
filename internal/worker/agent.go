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
)

// Agent is a coding agent working inside a task worktree (spec §32).
type Agent interface {
	Name() string
	Run(ctx context.Context, dir, prompt string, log io.Writer) (Result, error)
}

type Result struct {
	ExitStatus int
	Tokens     int64
	CostUSD    float64
}

// ClaudeCode runs Claude Code headless in the worktree. acceptEdits lets it edit
// files but not run shell commands; the worker runs tests and git itself.
type ClaudeCode struct {
	Model string // optional --model
}

func (c ClaudeCode) Name() string {
	if c.Model == "" {
		return "claude-code"
	}
	return "claude-code:" + c.Model
}

func (c ClaudeCode) Run(ctx context.Context, dir, prompt string, log io.Writer) (Result, error) {
	args := []string{"-p", "--output-format", "json", "--permission-mode", "acceptEdits"}
	if c.Model != "" {
		args = append(args, "--model", c.Model)
	}
	cmd := exec.CommandContext(ctx, "claude", args...)
	cmd.Dir = dir
	cmd.Env = agentEnv()
	cmd.Stdin = strings.NewReader(prompt)
	var out bytes.Buffer
	cmd.Stdout = io.MultiWriter(&out, log)
	cmd.Stderr = log
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

// Fake is a stand-in agent for tests and dry runs: it appends one line to a file.
type Fake struct{}

func (Fake) Name() string { return "fake" }

func (Fake) Run(ctx context.Context, dir, prompt string, log io.Writer) (Result, error) {
	first, _, _ := strings.Cut(prompt, "\n")
	line := fmt.Sprintf("%s %s\n", time.Now().UTC().Format(time.RFC3339Nano), first)
	f, err := os.OpenFile(filepath.Join(dir, "empire-fake-agent.txt"), os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return Result{ExitStatus: 1}, err
	}
	defer f.Close()
	if _, err := f.WriteString(line); err != nil {
		return Result{ExitStatus: 1}, err
	}
	fmt.Fprint(log, "fake agent wrote: ", line)
	return Result{}, nil
}
