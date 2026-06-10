package core

import (
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

// gitTimeout bounds every git invocation so a hung git never blocks the CLI.
const gitTimeout = 30 * time.Second

// git runs `git args...` in cwd and returns trimmed stdout, or an error.
func git(cwd string, args ...string) (string, error) {
	cmd := exec.Command("git", args...)
	cmd.Dir = cwd
	out, err := runWithTimeout(cmd, gitTimeout)
	return strings.TrimSpace(string(out)), err
}

// tryGit runs git and returns trimmed stdout, or "" with ok=false on any error.
func tryGit(cwd string, args ...string) (string, bool) {
	out, err := git(cwd, args...)
	if err != nil {
		return "", false
	}
	return out, true
}

// runWithTimeout runs cmd, capturing combined stdout, killing it after d.
func runWithTimeout(cmd *exec.Cmd, d time.Duration) ([]byte, error) {
	var out strings.Builder
	cmd.Stdout = &out
	if err := cmd.Start(); err != nil {
		return nil, err
	}
	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()
	select {
	case <-time.After(d):
		_ = cmd.Process.Kill()
		<-done
		return nil, errTimeout
	case err := <-done:
		return []byte(out.String()), err
	}
}

type timeoutError struct{}

func (timeoutError) Error() string { return "git command timed out" }

var errTimeout = timeoutError{}

// RepoRoot returns the absolute path of the working tree's top level.
func RepoRoot(cwd string) (string, error) {
	return git(cwd, "rev-parse", "--show-toplevel")
}

// GitCommonDir returns the absolute path of the repo's common git directory
// (shared by the main checkout and every worktree).
func GitCommonDir(cwd string) (string, error) {
	d, err := git(cwd, "rev-parse", "--git-common-dir")
	if err != nil {
		return "", err
	}
	if filepath.IsAbs(d) {
		return d, nil
	}
	return filepath.Join(cwd, d), nil
}

// DefaultBranch resolves the repo's default branch: origin/HEAD if known,
// else "main" or "master" if they exist, else the current branch.
func DefaultBranch(cwd string) (string, error) {
	if head, ok := tryGit(cwd, "symbolic-ref", "--quiet", "--short", "refs/remotes/origin/HEAD"); ok {
		return strings.TrimPrefix(head, "origin/"), nil
	}
	for _, b := range []string{"main", "master"} {
		if _, ok := tryGit(cwd, "rev-parse", "--verify", b); ok {
			return b, nil
		}
	}
	return git(cwd, "rev-parse", "--abbrev-ref", "HEAD")
}

// WorktreesDirFrom returns the directory under which dispatched worktrees live:
// <root>/.worktrees. Nesting them inside the repo root keeps every write within
// the repo (so writes can be sandboxed to it). Pure: no git/fs.
func WorktreesDirFrom(root string) string {
	return filepath.Join(root, ".worktrees")
}

// BranchExists is a predicate: does the named branch already exist?
type BranchExists func(branch string) bool

// UniqueBranch returns desired, or desired-2, desired-3, ... — the first name
// for which exists returns false. The exists predicate is injectable so the
// pure logic can be unit-tested without touching real git.
func UniqueBranch(desired string, exists BranchExists) string {
	branch := desired
	for i := 2; exists(branch); i++ {
		branch = desired + "-" + strconv.Itoa(i)
	}
	return branch
}

// GitBranchExists returns a BranchExists predicate backed by real git in cwd.
func GitBranchExists(cwd string) BranchExists {
	return func(branch string) bool {
		_, ok := tryGit(cwd, "rev-parse", "--verify", "--quiet", branch)
		return ok
	}
}

// EnsureWorktreesIgnored appends ".worktrees/" to the repo's local
// <git-common-dir>/info/exclude (not .gitignore, so it works in any repo and
// stays uncommitted). Idempotent and best-effort: any error is returned but
// callers may safely ignore it since keeping git status clean is non-critical.
func EnsureWorktreesIgnored(root string) error {
	commonDir, err := GitCommonDir(root)
	if err != nil {
		return err
	}
	excludePath := filepath.Join(commonDir, "info", "exclude")
	current, _ := os.ReadFile(excludePath)
	for _, line := range strings.Split(string(current), "\n") {
		if strings.TrimSpace(line) == ".worktrees/" {
			return nil
		}
	}
	if err := os.MkdirAll(filepath.Dir(excludePath), 0o755); err != nil {
		return err
	}
	prefix := ""
	if len(current) > 0 && !strings.HasSuffix(string(current), "\n") {
		prefix = "\n"
	}
	f, err := os.OpenFile(excludePath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	defer f.Close()
	_, err = f.WriteString(prefix + ".worktrees/\n")
	return err
}
