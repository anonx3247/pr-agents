package harness

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

// useFakeStore points the SessionRef scanners at root for the duration of the
// test and restores the prior override afterwards.
func useFakeStore(t *testing.T, root string) {
	t.Helper()
	orig := sessionHomeOverride
	sessionHomeOverride = root
	t.Cleanup(func() { sessionHomeOverride = orig })
}

// writeFile writes data to path (creating parent dirs) and stamps its mtime.
func writeFile(t *testing.T, path string, data string, mod time.Time) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(data), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Chtimes(path, mod, mod); err != nil {
		t.Fatal(err)
	}
}

func TestPiSessionRef(t *testing.T) {
	home := t.TempDir()
	useFakeStore(t, home)
	a, _ := Get("pi")

	cwd := "/Users/anas/dev/x/.worktrees/y"
	dir := filepath.Join(home, ".pi", "agent", "sessions", encodePiSessionDir(cwd))
	base := time.Now().Add(-time.Hour)
	old := filepath.Join(dir, "old.jsonl")
	newest := filepath.Join(dir, "newest.jsonl")
	writeFile(t, old, "{}", base)
	writeFile(t, newest, "{}", base.Add(30*time.Minute))

	ref, ok := a.SessionRef(cwd, base.Add(-time.Minute))
	if !ok {
		t.Fatal("SessionRef ok=false, want true")
	}
	if ref != newest {
		t.Errorf("ref = %q, want absolute path %q (newest-wins)", ref, newest)
	}

	// since-filtering: nothing on/after a future cutoff.
	if _, ok := a.SessionRef(cwd, time.Now().Add(time.Hour)); ok {
		t.Error("SessionRef ok=true for future since, want false")
	}
	// non-matching cwd → missing dir → ok=false, no panic.
	if _, ok := a.SessionRef("/no/such/cwd", base.Add(-time.Minute)); ok {
		t.Error("SessionRef ok=true for unknown cwd, want false")
	}
}

func TestClaudeSessionRef(t *testing.T) {
	home := t.TempDir()
	useFakeStore(t, home)
	a, _ := Get("claude")

	cwd := "/Users/anas/dev/x/.worktrees/y"
	dir := filepath.Join(home, ".claude", "projects", encodeClaudeProjectDir(cwd))
	base := time.Now().Add(-time.Hour)
	writeFile(t, filepath.Join(dir, "11111111-1111-1111-1111-111111111111.jsonl"), "{}", base)
	newUUID := "22222222-2222-2222-2222-222222222222"
	writeFile(t, filepath.Join(dir, newUUID+".jsonl"), "{}", base.Add(30*time.Minute))
	// a non-jsonl file must be ignored even though it is newer.
	writeFile(t, filepath.Join(dir, "ignore.txt"), "x", base.Add(time.Hour))

	ref, ok := a.SessionRef(cwd, base.Add(-time.Minute))
	if !ok {
		t.Fatal("SessionRef ok=false, want true")
	}
	if ref != newUUID {
		t.Errorf("ref = %q, want uuid %q (base name without .jsonl, newest-wins)", ref, newUUID)
	}

	if _, ok := a.SessionRef(cwd, time.Now().Add(time.Hour)); ok {
		t.Error("SessionRef ok=true for future since, want false")
	}
	if _, ok := a.SessionRef("/other/cwd", base.Add(-time.Minute)); ok {
		t.Error("SessionRef ok=true for unknown cwd, want false")
	}
}

func TestCodexSessionRef(t *testing.T) {
	home := t.TempDir()
	useFakeStore(t, home)
	a, _ := Get("codex")

	cwd := "/Users/anas/dev/x/.worktrees/y"
	root := filepath.Join(home, ".codex", "sessions", "2026", "06", "10")
	base := time.Now().Add(-time.Hour)

	meta := func(id, c string) string {
		return `{"type":"session_meta","payload":{"id":"` + id + `","cwd":"` + c + `"}}` + "\n" +
			`{"type":"event","payload":{}}` + "\n"
	}
	// older matching session
	writeFile(t, filepath.Join(root, "rollout-2026-06-10T01-00-00-aaaa.jsonl"), meta("old-id", cwd), base)
	// newer matching session (should win, newest-first)
	writeFile(t, filepath.Join(root, "rollout-2026-06-10T02-00-00-bbbb.jsonl"), meta("new-id", cwd), base.Add(30*time.Minute))
	// newest but different cwd (must be skipped)
	writeFile(t, filepath.Join(root, "rollout-2026-06-10T03-00-00-cccc.jsonl"), meta("other-id", "/different/cwd"), base.Add(45*time.Minute))

	ref, ok := a.SessionRef(cwd, base.Add(-time.Minute))
	if !ok {
		t.Fatal("SessionRef ok=false, want true")
	}
	if ref != "new-id" {
		t.Errorf("ref = %q, want payload.id new-id (newest matching cwd)", ref)
	}

	if _, ok := a.SessionRef("/never/matches", base.Add(-time.Minute)); ok {
		t.Error("SessionRef ok=true for unknown cwd, want false")
	}
	if _, ok := a.SessionRef(cwd, time.Now().Add(time.Hour)); ok {
		t.Error("SessionRef ok=true for future since, want false")
	}
}

func TestSessionRefMissingStoreNoPanic(t *testing.T) {
	useFakeStore(t, t.TempDir()) // empty: no harness dirs exist
	for _, kind := range []string{"pi", "claude", "codex"} {
		a, _ := Get(kind)
		if _, ok := a.SessionRef("/Users/anas/dev/x", time.Time{}); ok {
			t.Errorf("%s SessionRef ok=true on empty store, want false", kind)
		}
	}
}
