package core

import (
	"path/filepath"
	"strings"
)

// ResolveContextFromCwd returns the entry whose Worktree is cwd or an ancestor
// of cwd, with the LONGEST matching worktree winning (so a nested worktree
// resolves to itself, not the repo root). Returns nil when nothing matches.
//
// This lets a worker derive its own PR identity from its worktree path WITHOUT
// relying on PRA_* env crossing a sandbox boundary: the registry plus the cwd
// are enough to recover its entry.
func ResolveContextFromCwd(entries []PrEntry, cwd string) *PrEntry {
	cwd = normalizePath(cwd)
	var best *PrEntry
	bestLen := -1
	for i := range entries {
		wt := normalizePath(entries[i].Worktree)
		if wt == "" {
			continue
		}
		if cwd == wt || isSubpath(cwd, wt) {
			if len(wt) > bestLen {
				best = &entries[i]
				bestLen = len(wt)
			}
		}
	}
	return best
}

// DepthFromCwd derives the caller's nesting depth purely from cwd→registry,
// with NO reliance on an env contract: depth 0 when cwd is not inside any
// registered worktree (the orchestrator in the main repo), else the resolved
// entry's Depth (1 = PR subagent, 2 = helper).
func DepthFromCwd(entries []PrEntry, cwd string) int {
	if e := ResolveContextFromCwd(entries, cwd); e != nil {
		return e.Depth
	}
	return 0
}

// normalizePath cleans p and strips a single trailing separator so comparisons
// are stable. Empty input stays empty.
func normalizePath(p string) string {
	if p == "" {
		return ""
	}
	return filepath.Clean(p)
}

// isSubpath reports whether child is strictly nested under parent (parent is a
// proper ancestor directory of child).
func isSubpath(child, parent string) bool {
	if parent == "" || child == "" {
		return false
	}
	prefix := parent
	if !strings.HasSuffix(prefix, string(filepath.Separator)) {
		prefix += string(filepath.Separator)
	}
	return strings.HasPrefix(child, prefix)
}

// ScopeRefResolver resolves the orchestrator's OWN resumable session reference
// for the main repo cwd, returning ok=false when none can be located. It is
// injected so ResolveScopeID stays pure and unit-testable without scanning the
// real "~" session stores.
type ScopeRefResolver func() (ref string, ok bool)

// ResolveScopeID derives the STABLE scope id that owns the orchestrator's
// registry entries (a port of pi's resolveOrchestratorSessionId, made
// harness-agnostic). Resolution order:
//
//  1. the orchestrator's real harness session ref (resolveRef): because the
//     harness reopens the SAME session on resume, this yields the same ref and
//     thus the same scope, so the orchestrator re-scopes to its existing
//     entries; a genuinely fresh session yields a new ref and a new scope.
//  2. the PRA_SESSION env var (carried across a tmux re-exec / nested process).
//  3. a random fallback for a first-ever start with no session on disk yet.
func ResolveScopeID(resolveRef ScopeRefResolver, env string, fallback func() string) string {
	if resolveRef != nil {
		if ref, ok := resolveRef(); ok {
			if r := strings.TrimSpace(ref); r != "" {
				return r
			}
		}
	}
	if e := strings.TrimSpace(env); e != "" {
		return e
	}
	return fallback()
}

// PickRedockAgent chooses which agent to (re)dock: the most-recently-created
// LIVE depth-1 PR agent. Liveness is supplied via the isAlive predicate so the
// logic stays pure and unit-testable. Returns nil when there are no live agents.
func PickRedockAgent(entries []PrEntry, isAlive func(paneID string) bool) *PrEntry {
	var best *PrEntry
	for i := range entries {
		e := &entries[i]
		if e.Depth != 1 || e.PaneID == "" || !isAlive(e.PaneID) {
			continue
		}
		if best == nil || e.CreatedAt > best.CreatedAt {
			best = e
		}
	}
	return best
}

// PickWorkingAgent chooses the most-recently-ACTIVE live depth-1 agent that is
// currently WORKING (handed a task it has not reported finishing), or nil when
// none is working. Working-ness and liveness are injected predicates so the
// logic stays pure; recency uses activatedAt (a sortable RFC3339 activity
// timestamp) falling back to CreatedAt, so a freshly-dispatched agent that has
// never been re-activated still ranks by when it was created.
//
// The daemon prefers this over PickRedockAgent so the docked pane follows the
// agent that is actually doing work: when the docked agent goes idle/stopped and
// another agent starts (e.g. handling fresh review comments), the dock flips to
// the working one.
func PickWorkingAgent(
	entries []PrEntry,
	isAlive func(paneID string) bool,
	isWorking func(e PrEntry) bool,
	activatedAt func(e PrEntry) string,
) *PrEntry {
	var best *PrEntry
	var bestRank string
	for i := range entries {
		e := &entries[i]
		if e.Depth != 1 || e.PaneID == "" || !isAlive(e.PaneID) || !isWorking(*e) {
			continue
		}
		rank := activatedAt(*e)
		if rank < e.CreatedAt {
			rank = e.CreatedAt
		}
		if best == nil || rank > bestRank {
			best = e
			bestRank = rank
		}
	}
	return best
}

// SelectSessionCaptureTargets returns the LIVE depth-1 worker entries whose
// resumable WorkerSessionRef has not yet been captured, so the daemon can try to
// resolve and persist it. Liveness is supplied via isAlive so the selection
// stays pure and unit-testable. Entries that already carry a WorkerSessionRef,
// pane-less or dead-pane entries, and helpers (depth != 1) are all skipped, so
// the capture is idempotent and bounded to genuine workers still running.
func SelectSessionCaptureTargets(entries []PrEntry, isAlive func(paneID string) bool) []PrEntry {
	out := make([]PrEntry, 0)
	for _, e := range entries {
		if e.Depth != 1 || e.WorkerSessionRef != "" || e.PaneID == "" || !isAlive(e.PaneID) {
			continue
		}
		out = append(out, e)
	}
	return out
}

// RevivableChecks injects the liveness/filesystem/session facts that
// SelectRevivableAgents needs, keeping the selection pure and unit-testable
// without touching tmux, the filesystem, or the real "~" session stores.
type RevivableChecks struct {
	// PaneAlive reports whether a pane id is still a live tmux pane.
	PaneAlive func(paneID string) bool
	// WorktreeExists reports whether the entry's worktree dir still exists.
	WorktreeExists func(path string) bool
	// SessionResolvable reports whether the entry carries a usable
	// WorkerSessionRef that its harness can resume.
	SessionResolvable func(e PrEntry) bool
}

// SelectRevivableAgents returns the entries that should be REVIVED on
// orchestrator resume (a port of pi's selectRevivableAgents). An entry is
// revivable when it is a depth-1 PR worker, non-terminal, its tmux pane is DEAD,
// its worktree still EXISTS on disk, and it has a resumable WorkerSessionRef.
//
// Depth-2 helpers are intentionally EXCLUDED: a revived worker re-spawns its own
// helpers if it needs them, so reviving helpers here would double them. Live
// panes (still running), terminal/finished entries (merged/closed/stopped), and
// entries whose worktree was reaped are all skipped too. Pure: every fact comes
// from checks, so it is table-testable with fakes.
func SelectRevivableAgents(entries []PrEntry, checks RevivableChecks) []PrEntry {
	out := make([]PrEntry, 0)
	for _, e := range entries {
		if e.Depth != 1 {
			continue // depth-2 helpers are re-spawned by their revived parent
		}
		if e.Status == StatusMerged || e.Status == StatusClosed || e.Status == StatusStopped {
			continue // terminal/finished: nothing to revive
		}
		if e.PaneID != "" && checks.PaneAlive(e.PaneID) {
			continue // still running
		}
		if e.Worktree == "" || !checks.WorktreeExists(e.Worktree) {
			continue // worktree reaped; cannot resume in place
		}
		if !checks.SessionResolvable(e) {
			continue // no usable session ref to resume
		}
		out = append(out, e)
	}
	return out
}

// CleanupTarget pairs an entry selected for removal with the human-readable
// reason it is being reaped.
type CleanupTarget struct {
	Entry  PrEntry
	Reason string
}

// SelectCleanupTargets is the PURE core of cleanup: given this session's depth-1
// entries, a map of entry-id → classified PR state, and a predicate reporting
// whether an entry's branch is merged into its base, it returns the entries that
// should be reaped (merged/closed PR, or branch merged) and the entries kept.
//
// Helper entries (depth > 1) and IO (worktree/branch/pane removal) are handled
// by the caller; this function only decides WHICH entries are removable.
func SelectCleanupTargets(
	entries []PrEntry,
	stateByID map[string]PrStateClass,
	branchMerged func(e PrEntry) bool,
) (remove []CleanupTarget, keep []PrEntry) {
	remove = make([]CleanupTarget, 0)
	keep = make([]PrEntry, 0)
	for _, e := range entries {
		if e.Depth > 1 {
			keep = append(keep, e)
			continue
		}
		reason := ""
		switch stateByID[e.ID] {
		case PrStateMerged:
			reason = "PR merged"
		case PrStateClosed:
			reason = "PR closed"
		}
		if reason == "" && branchMerged != nil && branchMerged(e) {
			reason = "branch merged into " + e.Base
		}
		if reason == "" {
			keep = append(keep, e)
			continue
		}
		remove = append(remove, CleanupTarget{Entry: e, Reason: reason})
	}
	return remove, keep
}
