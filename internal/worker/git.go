package worker

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
)

const contextFile = ".empire-context.md"

// run executes a command in dir and returns trimmed combined output.
func run(ctx context.Context, dir, name string, args ...string) (string, error) {
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Dir = dir
	cmd.Env = append(agentEnv(), "GIT_TERMINAL_PROMPT=0")
	var out bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &out
	err := cmd.Run()
	// TrimRight, not TrimSpace: leading spaces matter (e.g. diffstat alignment).
	s := strings.TrimRight(out.String(), " \t\r\n")
	if err != nil {
		return s, fmt.Errorf("%s %s: %w\n%s", name, strings.Join(args, " "), err, tail(s, 2000))
	}
	return s, nil
}

func git(ctx context.Context, dir string, args ...string) (string, error) {
	return run(ctx, dir, "git", args...)
}

// shell runs a project-configured command (e.g. the test command).
func shell(ctx context.Context, dir, command string) (string, error) {
	if runtime.GOOS == "windows" {
		return run(ctx, dir, "cmd", "/C", command)
	}
	return run(ctx, dir, "sh", "-c", command)
}

// ensureBase keeps one fetched clone per project; tasks get worktrees off it (spec §29).
// ponytail: no per-project lock, fine for one sequential worker per machine; add one when a machine runs tasks in parallel.
func ensureBase(ctx context.Context, base, repoURL string) error {
	if _, err := os.Stat(filepath.Join(base, ".git")); os.IsNotExist(err) {
		if err := os.MkdirAll(filepath.Dir(base), 0o755); err != nil {
			return err
		}
		if _, err := git(ctx, "", "clone", "--no-checkout", repoURL, base); err != nil {
			return err
		}
	}
	// Keep platform files out of every task commit.
	excl := filepath.Join(base, ".git", "info", "exclude")
	os.MkdirAll(filepath.Dir(excl), 0o755)
	current, _ := os.ReadFile(excl)
	for _, pattern := range []string{"/" + contextFile, "/.empire/"} {
		if !strings.Contains(string(current), "\n"+pattern+"\n") {
			current = append(current, []byte("\n"+pattern+"\n")...)
		}
	}
	if err := os.WriteFile(excl, current, 0o644); err != nil {
		return err
	}
	_, err := git(ctx, base, "fetch", "--prune", "origin")
	return err
}

func refExists(ctx context.Context, dir, ref string) bool {
	_, err := git(ctx, dir, "rev-parse", "--verify", "--quiet", ref)
	return err == nil
}

// addWorktree creates a fresh worktree at dir, clearing any leftover from a crashed run.
// flag is "-B <branch>" or "--detach"; ref is the start commit.
func addWorktree(ctx context.Context, base, dir, ref string, flag ...string) error {
	removeWorktree(ctx, base, dir)
	args := append([]string{"worktree", "add"}, flag...)
	_, err := git(ctx, base, append(args, dir, ref)...)
	return err
}

func removeWorktree(ctx context.Context, base, dir string) {
	git(ctx, base, "worktree", "remove", "--force", dir)
	os.RemoveAll(dir)
	git(ctx, base, "worktree", "prune")
}

func tail(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return "…" + s[len(s)-n:]
}
