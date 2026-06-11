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
