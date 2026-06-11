package core

import (
	"fmt"
	"strings"
)

// Cross-agent notification builders (pure).
//
// The daemon drives all messaging via `tmux send-keys`, so these helpers build
// the literal message text. They are phrased for the CLI world (`pr-agents
// cleanup`, `pr-agents peek`, …) rather than for any one harness's tools.

// FinishedItem pairs a newly-finished worker entry with its result sequence.
type FinishedItem struct {
	Entry PrEntry
	Seq   int
}

// SelectNewlyFinished finds every PR subagent (depth 1) whose ResultSeq is newer
// than the last-seen value. The seq + last-seen map dedups across ticks so each
// genuine completion notifies exactly once. Pure: no IO.
func SelectNewlyFinished(entries []PrEntry, lastSeen map[string]int) []FinishedItem {
	out := make([]FinishedItem, 0)
	for _, e := range entries {
		if e.Depth != 1 || e.ResultSeq == nil {
			continue
		}
		last, ok := lastSeen[e.ID]
		if !ok {
			last = -1
		}
		if *e.ResultSeq > last {
			out = append(out, FinishedItem{Entry: e, Seq: *e.ResultSeq})
		}
	}
	return out
}

// BuildFinishedNotification builds the orchestrator notification for one or more
// newly-finished PR subagents, combined into ONE message to reduce noise. Pure.
func BuildFinishedNotification(entries []PrEntry) string {
	blocks := make([]string, 0, len(entries))
	for _, e := range entries {
		pr := "pending"
		if e.PrNumber != nil {
			pr = fmt.Sprintf("#%d", *e.PrNumber)
		}
		result := e.LastResult
		if result == "" {
			result = "(no result captured)"
		}
		blocks = append(blocks,
			fmt.Sprintf("- id %s · PR %s · %s · %s\n  result: %s", e.ID, pr, e.PrName, e.Branch, result))
	}
	header := "A PR subagent stopped working:"
	if len(entries) != 1 {
		header = fmt.Sprintf("%d PR subagents stopped working:", len(entries))
	}
	parts := []string{header, ""}
	parts = append(parts, blocks...)
	parts = append(parts, "",
		"Review the result and decide the next step (`pr-agents peek <id>` for more, "+
			"`pr-agents send <id> <msg>` to steer, `pr-agents cleanup` if merged, or do nothing if it is "+
			"merely waiting on you). Do not take destructive actions without cause.")
	return strings.Join(parts, "\n")
}

// BuildCleanupNotification builds the orchestrator notification for one or more
// PRs that just reached a terminal state on GitHub. Multiple transitions in one
// poll tick are COALESCED into a single message (one line per PR) to reduce
// noise rather than emitting N separate notifications. Phrased for the CLI
// world: it tells the orchestrator to run `pr-agents cleanup`. Pure.
func BuildCleanupNotification(transitions []StateTransition) string {
	lines := make([]string, 0, len(transitions)+2)
	for _, t := range transitions {
		pr := "(no number)"
		if t.Entry.PrNumber != nil {
			pr = fmt.Sprintf("#%d", *t.Entry.PrNumber)
		}
		lines = append(lines,
			fmt.Sprintf("PR %s '%s' (branch %s) was %s on GitHub.", pr, t.Entry.PrName, t.Entry.Branch, t.State))
	}
	target := "its worktree, branch, and tmux window"
	if len(transitions) != 1 {
		target = "their worktrees, branches, and tmux windows"
	}
	lines = append(lines, "", fmt.Sprintf("Run `pr-agents cleanup` to remove %s.", target))
	return strings.Join(lines, "\n")
}
