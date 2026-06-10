package cli

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/anonx3247/pr-agents/internal/core"
)

// initWorkerRepo creates a hermetic git repo, seeds a single registry entry
// whose worktree is the repo dir, chdirs into it, and returns the repo path and
// entry id. The entry has no pane so relabelPane is a no-op (no tmux needed).
func initWorkerRepo(t *testing.T) (dir, id string) {
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
	id = "abc123"
	entry := core.PrEntry{
		ID:        id,
		SessionID: "sess",
		PrName:    "My PR",
		Branch:    "pi/pr-my-pr",
		Base:      "main",
		Mode:      core.ModeIndependent,
		Worktree:  dir,
		Depth:     1,
		Status:    core.StatusWorking,
	}
	if err := core.SaveRegistry(dir, []core.PrEntry{entry}); err != nil {
		t.Fatal(err)
	}
	t.Chdir(dir)
	return dir, id
}

// stubNow pins nowRFC3339 to a fixed timestamp and returns a restore func.
func stubNow(t *testing.T) func() {
	t.Helper()
	prev := nowRFC3339
	nowRFC3339 = func() string { return "2026-01-01T00:00:00Z" }
	return func() { nowRFC3339 = prev }
}

func TestRunSetPrNumber(t *testing.T) {
	dir, id := initWorkerRepo(t)
	var out, errOut bytes.Buffer
	if code := runSetPrNumber([]string{"42", "--url", "http://x/42"}, &out, &errOut); code != 0 {
		t.Fatalf("exit = %d, stderr=%s", code, errOut.String())
	}
	entries, _ := core.LoadRegistry(dir)
	e := core.FindEntry(entries, id)
	if e.PrNumber == nil || *e.PrNumber != 42 {
		t.Errorf("PrNumber = %v, want 42", e.PrNumber)
	}
	if e.PrURL != "http://x/42" {
		t.Errorf("PrURL = %q", e.PrURL)
	}
	if e.Status != core.StatusOpen {
		t.Errorf("Status = %q, want open", e.Status)
	}
}

func TestRunSetPrNumberInvalid(t *testing.T) {
	initWorkerRepo(t)
	var out, errOut bytes.Buffer
	if code := runSetPrNumber([]string{"notanum"}, &out, &errOut); code != 2 {
		t.Errorf("exit = %d, want 2", code)
	}
}

func TestRunMarkPushed(t *testing.T) {
	dir, id := initWorkerRepo(t)
	defer stubNow(t)()
	var out, errOut bytes.Buffer
	if code := runMarkPushed([]string{"--number", "7"}, &out, &errOut); code != 0 {
		t.Fatalf("exit = %d, stderr=%s", code, errOut.String())
	}
	entries, _ := core.LoadRegistry(dir)
	e := core.FindEntry(entries, id)
	if !e.Pushed {
		t.Error("Pushed should be true")
	}
	if e.PushedAt != "2026-01-01T00:00:00Z" {
		t.Errorf("PushedAt = %q", e.PushedAt)
	}
	if e.PrNumber == nil || *e.PrNumber != 7 {
		t.Errorf("PrNumber = %v, want 7", e.PrNumber)
	}
}

func TestRunReportResult(t *testing.T) {
	dir, id := initWorkerRepo(t)
	defer stubNow(t)()
	var out, errOut bytes.Buffer
	if code := runReportResult([]string{"done", "and", "dusted"}, &out, &errOut); code != 0 {
		t.Fatalf("exit = %d, stderr=%s", code, errOut.String())
	}
	entries, _ := core.LoadRegistry(dir)
	e := core.FindEntry(entries, id)
	if e.LastResult != "done and dusted" {
		t.Errorf("LastResult = %q", e.LastResult)
	}
	if e.ResultSeq == nil || *e.ResultSeq != 0 {
		t.Errorf("ResultSeq = %v, want 0", e.ResultSeq)
	}
	// A second report bumps the sequence.
	if code := runReportResult([]string{"again"}, &out, &errOut); code != 0 {
		t.Fatalf("exit = %d", code)
	}
	entries, _ = core.LoadRegistry(dir)
	e = core.FindEntry(entries, id)
	if e.ResultSeq == nil || *e.ResultSeq != 1 {
		t.Errorf("ResultSeq = %v, want 1", e.ResultSeq)
	}
}
