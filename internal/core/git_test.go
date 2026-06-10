package core

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestWorktreesDirFrom(t *testing.T) {
	root := filepath.Join("/a", "b", "repo")
	want := filepath.Join(root, ".worktrees")
	if got := WorktreesDirFrom(root); got != want {
		t.Errorf("WorktreesDirFrom = %q, want %q", got, want)
	}
}

func TestUniqueBranch(t *testing.T) {
	tests := []struct {
		name    string
		desired string
		taken   map[string]bool
		want    string
	}{
		{"free name returned as-is", "pi/feat", nil, "pi/feat"},
		{"first collision bumps to -2", "pi/feat", map[string]bool{"pi/feat": true}, "pi/feat-2"},
		{"walks until free", "pi/feat", map[string]bool{"pi/feat": true, "pi/feat-2": true, "pi/feat-3": true}, "pi/feat-4"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			exists := func(b string) bool { return tt.taken[b] }
			if got := UniqueBranch(tt.desired, exists); got != tt.want {
				t.Errorf("UniqueBranch = %q, want %q", got, tt.want)
			}
		})
	}
}

// initRepo creates a hermetic temp git repo and returns its path. It scrubs any
// inherited GIT_* env so git resolves against the temp dir, restoring them after.
func initRepo(t *testing.T) string {
	t.Helper()
	for _, k := range []string{"GIT_DIR", "GIT_COMMON_DIR", "GIT_WORK_TREE", "GIT_INDEX_FILE"} {
		t.Setenv(k, "")
		os.Unsetenv(k)
	}
	dir := t.TempDir()
	for _, args := range [][]string{
		{"init", "-q", "-b", "main"},
		{"config", "user.email", "t@example.com"},
		{"config", "user.name", "tester"},
	} {
		cmd := exec.Command("git", args...)
		cmd.Dir = dir
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
	// macOS /var symlinks to /private/var; resolve so RepoRoot comparisons match.
	resolved, err := filepath.EvalSymlinks(dir)
	if err != nil {
		t.Fatal(err)
	}
	return resolved
}

func TestRepoRootAndCommonDir(t *testing.T) {
	dir := initRepo(t)
	root, err := RepoRoot(dir)
	if err != nil {
		t.Fatalf("RepoRoot: %v", err)
	}
	if root != dir {
		t.Errorf("RepoRoot = %q, want %q", root, dir)
	}
	common, err := GitCommonDir(dir)
	if err != nil {
		t.Fatalf("GitCommonDir: %v", err)
	}
	if !strings.HasSuffix(common, ".git") {
		t.Errorf("GitCommonDir = %q, want suffix .git", common)
	}
	if !filepath.IsAbs(common) {
		t.Errorf("GitCommonDir = %q, want absolute path", common)
	}
}

func TestGitBranchExistsAndUniqueBranchIntegration(t *testing.T) {
	dir := initRepo(t)
	// An empty repo has no commits, so create one so HEAD/branches resolve.
	if err := os.WriteFile(filepath.Join(dir, "f.txt"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	for _, args := range [][]string{{"add", "."}, {"commit", "-q", "-m", "init"}} {
		cmd := exec.Command("git", args...)
		cmd.Dir = dir
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
	exists := GitBranchExists(dir)
	if !exists("main") {
		t.Error("expected main to exist")
	}
	if exists("nope") {
		t.Error("expected nope to not exist")
	}
	if got := UniqueBranch("main", exists); got != "main-2" {
		t.Errorf("UniqueBranch(main) = %q, want main-2", got)
	}
}

func TestEnsureWorktreesIgnored(t *testing.T) {
	dir := initRepo(t)
	common, err := GitCommonDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	excludePath := filepath.Join(common, "info", "exclude")

	if err := EnsureWorktreesIgnored(dir); err != nil {
		t.Fatalf("first call: %v", err)
	}
	data, err := os.ReadFile(excludePath)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Count(string(data), ".worktrees/") != 1 {
		t.Errorf("expected exactly one .worktrees/ entry, got: %q", data)
	}

	// Idempotent: a second call adds nothing.
	if err := EnsureWorktreesIgnored(dir); err != nil {
		t.Fatalf("second call: %v", err)
	}
	data2, _ := os.ReadFile(excludePath)
	if strings.Count(string(data2), ".worktrees/") != 1 {
		t.Errorf("not idempotent, got: %q", data2)
	}
}

func TestDefaultBranch(t *testing.T) {
	dir := initRepo(t)
	if err := os.WriteFile(filepath.Join(dir, "f.txt"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	for _, args := range [][]string{{"add", "."}, {"commit", "-q", "-m", "init"}} {
		cmd := exec.Command("git", args...)
		cmd.Dir = dir
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
	got, err := DefaultBranch(dir)
	if err != nil {
		t.Fatalf("DefaultBranch: %v", err)
	}
	if got != "main" {
		t.Errorf("DefaultBranch = %q, want main", got)
	}
}
