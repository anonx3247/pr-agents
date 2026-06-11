package core

import (
	"os"
	"path/filepath"
	"testing"
)

func TestSessionRecordRoundTrip(t *testing.T) {
	dir := initRepo(t)
	rec := SessionRecord{Harness: "claude", Launcher: "isara claude run"}
	if err := SaveSessionRecord(dir, "sess1", rec); err != nil {
		t.Fatalf("SaveSessionRecord: %v", err)
	}
	got, ok, err := LoadSessionRecord(dir, "sess1")
	if err != nil || !ok {
		t.Fatalf("LoadSessionRecord ok=%v err=%v", ok, err)
	}
	if got != rec {
		t.Errorf("round-trip mismatch: got %+v want %+v", got, rec)
	}
}

func TestSessionRecordMultipleSessions(t *testing.T) {
	dir := initRepo(t)
	if err := SaveSessionRecord(dir, "a", SessionRecord{Harness: "pi"}); err != nil {
		t.Fatal(err)
	}
	if err := SaveSessionRecord(dir, "b", SessionRecord{Harness: "codex", Launcher: "asb -- codex"}); err != nil {
		t.Fatal(err)
	}
	// Upserting a does not clobber b.
	if err := SaveSessionRecord(dir, "a", SessionRecord{Harness: "claude"}); err != nil {
		t.Fatal(err)
	}
	a, ok, _ := LoadSessionRecord(dir, "a")
	if !ok || a.Harness != "claude" {
		t.Errorf("session a: got %+v ok=%v", a, ok)
	}
	b, ok, _ := LoadSessionRecord(dir, "b")
	if !ok || b.Harness != "codex" || b.Launcher != "asb -- codex" {
		t.Errorf("session b: got %+v ok=%v", b, ok)
	}
}

func TestLoadSessionRecordMissing(t *testing.T) {
	dir := initRepo(t)
	_, ok, err := LoadSessionRecord(dir, "nope")
	if err != nil {
		t.Fatalf("LoadSessionRecord: %v", err)
	}
	if ok {
		t.Errorf("expected ok=false for missing record")
	}
}

func TestLoadSessionRecordEmptyID(t *testing.T) {
	dir := initRepo(t)
	if err := SaveSessionRecord(dir, "x", SessionRecord{Harness: "pi"}); err != nil {
		t.Fatal(err)
	}
	_, ok, err := LoadSessionRecord(dir, "")
	if err != nil || ok {
		t.Errorf("empty id should return ok=false err=nil, got ok=%v err=%v", ok, err)
	}
}

func TestSaveSessionRecordEmptyIDNoOp(t *testing.T) {
	dir := initRepo(t)
	if err := SaveSessionRecord(dir, "", SessionRecord{Harness: "pi"}); err != nil {
		t.Fatalf("SaveSessionRecord empty id: %v", err)
	}
	// No file should have been created with a junk key.
	path, err := SessionsPath(dir)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(path); err == nil {
		t.Errorf("expected no sessions file written for empty id")
	}
}

func TestLoadSessionRecordGarbage(t *testing.T) {
	dir := initRepo(t)
	path, err := SessionsPath(dir)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("{not json"), 0o644); err != nil {
		t.Fatal(err)
	}
	_, ok, err := LoadSessionRecord(dir, "anything")
	if err != nil {
		t.Fatalf("expected tolerant read, got err %v", err)
	}
	if ok {
		t.Errorf("garbage file should yield ok=false")
	}
	// And a save after garbage should overwrite cleanly.
	if err := SaveSessionRecord(dir, "anything", SessionRecord{Harness: "pi"}); err != nil {
		t.Fatalf("SaveSessionRecord after garbage: %v", err)
	}
	got, ok, _ := LoadSessionRecord(dir, "anything")
	if !ok || got.Harness != "pi" {
		t.Errorf("post-garbage save: got %+v ok=%v", got, ok)
	}
}

func TestCurrentSessionRoundTrip(t *testing.T) {
	dir := initRepo(t)
	if got := LoadCurrentSession(dir); got != "" {
		t.Errorf("missing marker should be empty, got %q", got)
	}
	if err := SaveCurrentSession(dir, "S_real"); err != nil {
		t.Fatalf("SaveCurrentSession: %v", err)
	}
	if got := LoadCurrentSession(dir); got != "S_real" {
		t.Errorf("LoadCurrentSession = %q, want S_real", got)
	}
	// Last writer wins.
	if err := SaveCurrentSession(dir, "S_two"); err != nil {
		t.Fatal(err)
	}
	if got := LoadCurrentSession(dir); got != "S_two" {
		t.Errorf("LoadCurrentSession = %q, want S_two", got)
	}
}

func TestSaveCurrentSessionEmptyNoOp(t *testing.T) {
	dir := initRepo(t)
	if err := SaveCurrentSession(dir, ""); err != nil {
		t.Fatalf("SaveCurrentSession empty: %v", err)
	}
	path, err := CurrentSessionPath(dir)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(path); err == nil {
		t.Errorf("expected no marker file for empty id")
	}
}

func TestResolveFromSources(t *testing.T) {
	cases := []struct {
		name              string
		flag, env, record string
		want              string
	}{
		{"flag wins over all", "claude", "codex", "pi", "claude"},
		{"env wins when no flag", "", "codex", "pi", "codex"},
		{"record used when flag+env empty", "", "", "claude", "claude"},
		{"all empty falls through to caller", "", "", "", ""},
		{"flag wins even when others empty", "pi", "", "", "pi"},
		{"record skipped when env set", "", "codex", "claude", "codex"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := ResolveFromSources(c.flag, c.env, c.record); got != c.want {
				t.Errorf("ResolveFromSources(%q,%q,%q) = %q, want %q",
					c.flag, c.env, c.record, got, c.want)
			}
		})
	}
}

func TestSessionsPathLocation(t *testing.T) {
	dir := initRepo(t)
	path, err := SessionsPath(dir)
	if err != nil {
		t.Fatal(err)
	}
	want := filepath.Join(".pr-agents", "sessions.json")
	if filepath.Base(filepath.Dir(path)) != ".pr-agents" || filepath.Base(path) != "sessions.json" {
		t.Errorf("SessionsPath = %q, want it to end with %q", path, want)
	}
}
