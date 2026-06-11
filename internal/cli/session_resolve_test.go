package cli

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/anonx3247/pr-agents/internal/core"
)

// initBareRepo creates a hermetic git repo with NO registry entries (an
// orchestrator at the repo root) and chdirs into it. Returns the repo path.
func initBareRepo(t *testing.T) string {
	t.Helper()
	for _, k := range []string{"GIT_DIR", "GIT_COMMON_DIR", "GIT_WORK_TREE", "GIT_INDEX_FILE"} {
		t.Setenv(k, "")
		os.Unsetenv(k)
	}
	raw := t.TempDir()
	for _, args := range [][]string{
		{"init", "-q", "-b", "main"},
		{"config", "user.email", "t@example.com"},
		{"config", "user.name", "tester"},
	} {
		cmd := exec.Command("git", args...)
		cmd.Dir = raw
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
	dir, err := filepath.EvalSymlinks(raw)
	if err != nil {
		t.Fatal(err)
	}
	t.Chdir(dir)
	return dir
}

// TestResolveSessionPrefersEnv proves PRA_SESSION wins when present.
func TestResolveSessionPrefersEnv(t *testing.T) {
	dir := initBareRepo(t)
	if err := core.SaveCurrentSession(dir, "S_marker"); err != nil {
		t.Fatal(err)
	}
	t.Setenv(core.EnvSession, "S_env")
	if got := resolveSession(nil, dir); got != "S_env" {
		t.Errorf("resolveSession = %q, want S_env (env wins)", got)
	}
}

// TestResolveSessionFallsBackToMarker proves that with PRA_SESSION stripped (as a
// sandbox would), an orchestrator at the repo root recovers the REAL session id
// from the on-disk current-session marker rather than a harness-ref re-derivation.
func TestResolveSessionFallsBackToMarker(t *testing.T) {
	dir := initBareRepo(t)
	t.Setenv(core.EnvSession, "")
	t.Setenv(core.EnvHarness, "")
	if err := core.SaveCurrentSession(dir, "S_real"); err != nil {
		t.Fatal(err)
	}
	if got := resolveSession(nil, dir); got != "S_real" {
		t.Errorf("resolveSession = %q, want S_real (marker fallback)", got)
	}
}
