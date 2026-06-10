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
