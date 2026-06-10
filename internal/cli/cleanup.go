package cli

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/anonx3247/pr-agents/internal/core"
	"github.com/anonx3247/pr-agents/internal/tmux"
)

// ghStateTimeout bounds the gh PR-state lookup during cleanup.
const ghStateTimeout = 15 * time.Second

// prStateClass resolves a PR's GitHub state via `gh pr view --json state,mergedAt`,
// returning PrStateUnknown when gh is unavailable or the call fails.
func prStateClass(cwd string, number int) core.PrStateClass {
	ctx, cancel := context.WithTimeout(context.Background(), ghStateTimeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, "gh", "pr", "view", fmt.Sprintf("%d", number), "--json", "state,mergedAt")
	cmd.Dir = cwd
	out, err := cmd.Output()
	if err != nil {
		return core.PrStateUnknown
	}
	var j core.PrStateJSON
	if err := json.Unmarshal(out, &j); err != nil {
		return core.PrStateUnknown
	}
	return core.ClassifyPrState(&j)
}

func runCleanup(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("cleanup", flag.ContinueOnError)
	fs.SetOutput(stderr)
	dry := fs.Bool("dry", false, "Preview removals without changing anything")
	if err := fs.Parse(args); err != nil {
		return 2
	}

	cwd, err := os.Getwd()
	if err != nil {
		fmt.Fprintf(stderr, "pr-agents cleanup: %v\n", err)
		return 1
	}
	root, err := core.RepoRoot(cwd)
	if err != nil {
		fmt.Fprintf(stderr, "pr-agents cleanup: %v\n", err)
		return 1
	}
	base, err := core.DefaultBranch(root)
	if err != nil {
		fmt.Fprintf(stderr, "pr-agents cleanup: %v\n", err)
		return 1
	}

	all, err := core.LoadRegistry(cwd)
	if err != nil {
		fmt.Fprintf(stderr, "pr-agents cleanup: %v\n", err)
		return 1
	}
	session := resolveSession(all, cwd)
	sessionEntries := core.EntriesForSession(all, session)
	otherSessions := make([]core.PrEntry, 0)
	for _, e := range all {
		if e.SessionID != session {
			otherSessions = append(otherSessions, e)
		}
	}

	// Build the PR-state map for THIS session's depth-1 entries (one gh call each).
	stateByID := make(map[string]core.PrStateClass)
	for _, e := range sessionEntries {
		if e.Depth <= 1 && e.PrNumber != nil {
			stateByID[e.ID] = prStateClass(root, *e.PrNumber)
		}
	}

	branchMerged := func(e core.PrEntry) bool {
		return core.BranchMerged(root, e.Branch, base)
	}
	remove, keep := core.SelectCleanupTargets(sessionEntries, stateByID, branchMerged)

	var lines []string
	removedParents := make(map[string]bool)
	for _, t := range remove {
		e := t.Entry
		removedParents[e.ID] = true
		verb := "removing"
		if *dry {
			verb = "would remove"
		}
		lines = append(lines, fmt.Sprintf("%s %s (%s) — %s", verb, e.PrName, e.Branch, t.Reason))
		if !*dry {
			if tmux.PaneAlive(e.PaneID) {
				tmux.TryTmux("kill-pane", "-t", e.PaneID)
			}
			core.RemoveWorktree(root, e.Worktree)
			core.DeleteBranch(root, e.Branch)
		}
	}

	// Helper entries (depth > 1): drop when their pane is gone or their parent
	// PR was reaped. Surviving helpers stay in the registry.
	survivors := make([]core.PrEntry, 0, len(keep))
	for _, e := range keep {
		if e.Depth > 1 {
			if removedParents[e.ParentID] || !tmux.PaneAlive(e.PaneID) {
				if !*dry && tmux.PaneAlive(e.PaneID) {
					tmux.TryTmux("kill-pane", "-t", e.PaneID)
				}
				continue
			}
		}
		survivors = append(survivors, e)
	}

	// Orphaned worktrees the registry no longer tracks (guarded against the FULL
	// registry so a concurrent session's worktree is never pruned).
	trackedAll := make(map[string]bool)
	for _, e := range all {
		if e.Worktree != "" {
			trackedAll[e.Worktree] = true
		}
	}
	survivorWt := make(map[string]bool)
	for _, e := range survivors {
		if e.Worktree != "" {
			survivorWt[e.Worktree] = true
		}
	}
	for _, wt := range core.ParseWorktreePaths(core.WorktreeListPorcelain(root)) {
		if wt == root || survivorWt[wt] || trackedAll[wt] {
			continue
		}
		if !strings.Contains(wt, ".worktrees"+string(os.PathSeparator)) && !strings.Contains(wt, ".worktrees/") {
			continue
		}
		if _, err := os.Stat(wt); err != nil {
			continue
		}
		verb := "pruning"
		if *dry {
			verb = "would prune"
		}
		lines = append(lines, fmt.Sprintf("%s orphan worktree %s", verb, wt))
		if !*dry {
			core.RemoveWorktree(root, wt)
		}
	}

	if !*dry {
		core.PruneWorktrees(root)
		next := append(otherSessions, survivors...)
		if err := core.SaveRegistry(cwd, next); err != nil {
			fmt.Fprintf(stderr, "pr-agents cleanup: %v\n", err)
			return 1
		}
	}

	if len(lines) == 0 {
		fmt.Fprintln(stdout, "Nothing to clean up.")
	} else {
		if *dry {
			fmt.Fprintln(stdout, "[dry run]")
		}
		fmt.Fprintln(stdout, strings.Join(lines, "\n"))
	}
	kept := make([]string, 0)
	for _, e := range keep {
		if e.Depth <= 1 {
			kept = append(kept, fmt.Sprintf("%s (%s)", e.PrName, e.Branch))
		}
	}
	if len(kept) > 0 {
		fmt.Fprintf(stdout, "\nStill active:\n  %s\n", strings.Join(kept, "\n  "))
	}
	return 0
}
