package core

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// Mode is the stacking mode of a PR entry.
type Mode string

const (
	ModeIndependent Mode = "independent"
	ModeStack       Mode = "stack"
	ModeGraphite    Mode = "graphite"
	ModeHelper      Mode = "helper"
)

// Status is a PR entry's lifecycle status as tracked locally.
type Status string

const (
	StatusWorking Status = "working"
	StatusOpen    Status = "open"
	StatusMerged  Status = "merged"
	StatusClosed  Status = "closed"
	StatusStopped Status = "stopped"
)

// PrEntry is one dispatched PR (or helper) tracked in the shared registry. It
// mirrors the TypeScript PrEntry shape so a mixed fleet stays interoperable.
// Pointer/omitempty fields are optional and absent until set by a worker.
type PrEntry struct {
	ID string `json:"id"`
	// SessionID is the orchestrator (depth-0) session that owns this entry, so
	// concurrent orchestrators sharing one repo only see their own subagents.
	SessionID string `json:"sessionId,omitempty"`
	PrName    string `json:"prName"`
	Branch    string `json:"branch"`
	Base      string `json:"base"`
	Mode      Mode   `json:"mode"`
	PaneID    string `json:"paneId"`
	Worktree  string `json:"worktree"`
	Depth     int    `json:"depth"`
	ParentID  string `json:"parentId"`
	Simplify  bool   `json:"simplify,omitempty"`
	PrNumber  *int   `json:"prNumber,omitempty"`
	PrURL     string `json:"prUrl,omitempty"`
	Status    Status `json:"status"`
	CreatedAt string `json:"createdAt"`
	// Pushed is set by the worker once the branch is pushed AND the PR exists;
	// only then does the orchestrator poll this entry's GitHub state.
	Pushed   bool   `json:"pushed,omitempty"`
	PushedAt string `json:"pushedAt,omitempty"`
	// LastResult/ResultSeq capture a subagent's final turn output so the
	// orchestrator can notify itself of completions via the shared registry.
	LastResult   string `json:"lastResult,omitempty"`
	LastResultAt string `json:"lastResultAt,omitempty"`
	ResultSeq    *int   `json:"resultSeq,omitempty"`
	// SeenReviewIds dedups review/CI items already surfaced (rc:/rv:/ic:/ci: keys).
	SeenReviewIds []string `json:"seenReviewIds,omitempty"`
	// WorkerSessionRef is the harness-specific resumable session reference
	// (claude uuid / pi session-file path / codex session id) captured by the
	// daemon once the worker's session file appears on disk. It is recorded once
	// and used later to revive a dead pane by resuming its session.
	WorkerSessionRef string `json:"workerSessionRef,omitempty"`
	// WorkerSessionHarness records the harness kind (pi | claude | codex) needed
	// to resume WorkerSessionRef, captured alongside the ref. PrEntry carries no
	// other harness field, so this is the resume-time source of truth.
	WorkerSessionHarness string `json:"workerSessionHarness,omitempty"`
}

// RegistryPath returns the shared registry path,
// <git-common-dir>/.pr-agents/registry.json, creating the parent dir.
func RegistryPath(cwd string) (string, error) {
	commonDir, err := GitCommonDir(cwd)
	if err != nil {
		return "", err
	}
	dir := filepath.Join(commonDir, ".pr-agents")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	return filepath.Join(dir, "registry.json"), nil
}

// LoadRegistry reads the registry, returning an empty slice when the file is
// missing or unparseable (so callers never have to special-case a fresh repo).
func LoadRegistry(cwd string) ([]PrEntry, error) {
	path, err := RegistryPath(cwd)
	if err != nil {
		return nil, err
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return []PrEntry{}, nil
		}
		return nil, err
	}
	var entries []PrEntry
	if err := json.Unmarshal(raw, &entries); err != nil {
		return []PrEntry{}, nil
	}
	if entries == nil {
		return []PrEntry{}, nil
	}
	return entries, nil
}

// SaveRegistry writes entries atomically: marshal to a temp file in the same
// directory, then rename over the target so a reader never sees a partial file.
func SaveRegistry(cwd string, entries []PrEntry) error {
	path, err := RegistryPath(cwd)
	if err != nil {
		return err
	}
	data, err := json.MarshalIndent(entries, "", "  ")
	if err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), ".registry-*.json.tmp")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		os.Remove(tmpName)
		return err
	}
	if err := tmp.Close(); err != nil {
		os.Remove(tmpName)
		return err
	}
	if err := os.Rename(tmpName, path); err != nil {
		os.Remove(tmpName)
		return err
	}
	return nil
}

// UpdateEntry applies patch (a function mutating the matched entry) to the entry
// with the given id, saving the registry. It returns the updated entry, or nil
// with ok=false when no entry matches. A patch func keeps the merge explicit and
// type-safe (Go has no spread operator for partial structs).
func UpdateEntry(cwd, id string, patch func(*PrEntry)) (*PrEntry, bool, error) {
	entries, err := LoadRegistry(cwd)
	if err != nil {
		return nil, false, err
	}
	for i := range entries {
		if entries[i].ID == id {
			patch(&entries[i])
			if err := SaveRegistry(cwd, entries); err != nil {
				return nil, false, err
			}
			updated := entries[i]
			return &updated, true, nil
		}
	}
	return nil, false, nil
}

// EntriesForSession returns only the entries owned by sessionID. An empty
// sessionID naturally returns only legacy untagged entries.
func EntriesForSession(entries []PrEntry, sessionID string) []PrEntry {
	out := make([]PrEntry, 0)
	for _, e := range entries {
		if e.SessionID == sessionID {
			out = append(out, e)
		}
	}
	return out
}

// FindEntry resolves a human-friendly ref to an entry: exact id, id prefix,
// branch, prName, or PR number (with or without a leading '#'). Returns nil when
// nothing matches.
func FindEntry(entries []PrEntry, ref string) *PrEntry {
	numRef := strings.TrimPrefix(ref, "#")
	for i := range entries {
		e := &entries[i]
		if e.ID == ref || strings.HasPrefix(e.ID, ref) || e.Branch == ref || e.PrName == ref {
			return e
		}
		if e.PrNumber != nil && strconv.Itoa(*e.PrNumber) == numRef {
			return e
		}
	}
	return nil
}
