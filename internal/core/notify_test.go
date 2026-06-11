package core

import (
	"strings"
	"testing"
)

func TestSelectNewlyFinished(t *testing.T) {
	entries := []PrEntry{
		{ID: "a", Depth: 1, ResultSeq: intp(0)},
		{ID: "b", Depth: 1, ResultSeq: intp(2)},
		{ID: "c", Depth: 1},                     // no result yet
		{ID: "d", Depth: 2, ResultSeq: intp(5)}, // helper, ignored
	}
	lastSeen := map[string]int{"b": 1} // b advanced 1->2; a is new (-1->0)
	got := SelectNewlyFinished(entries, lastSeen)
	if len(got) != 2 {
		t.Fatalf("got %d finished, want 2: %#v", len(got), got)
	}
	ids := map[string]int{}
	for _, f := range got {
		ids[f.Entry.ID] = f.Seq
	}
	if ids["a"] != 0 || ids["b"] != 2 {
		t.Errorf("finished = %#v", ids)
	}
}

func TestBuildFinishedNotification(t *testing.T) {
	single := BuildFinishedNotification([]PrEntry{
		{ID: "x", PrName: "feat", Branch: "br", PrNumber: intp(3), LastResult: "done"},
	})
	if !strings.Contains(single, "A PR subagent stopped working:") {
		t.Errorf("missing header:\n%s", single)
	}
	if !strings.Contains(single, "PR #3") || !strings.Contains(single, "result: done") {
		t.Errorf("missing detail:\n%s", single)
	}

	multi := BuildFinishedNotification([]PrEntry{
		{ID: "x", PrName: "a", Branch: "b1"},
		{ID: "y", PrName: "c", Branch: "b2"},
	})
	if !strings.Contains(multi, "2 PR subagents stopped working:") {
		t.Errorf("missing multi header:\n%s", multi)
	}
	if !strings.Contains(multi, "result: (no result captured)") {
		t.Errorf("missing no-result fallback:\n%s", multi)
	}
}

func TestBuildCleanupNotification(t *testing.T) {
	msg := BuildCleanupNotification([]StateTransition{
		{Entry: PrEntry{PrName: "feat", Branch: "br", PrNumber: intp(9)}, State: PrStateMerged},
	})
	for _, want := range []string{"PR #9", "was merged on GitHub", "pr-agents cleanup", "its worktree, branch, and tmux window"} {
		if !strings.Contains(msg, want) {
			t.Errorf("cleanup notice missing %q:\n%s", want, msg)
		}
	}

	// Multiple same-tick transitions COALESCE into ONE message, one line per PR,
	// with pluralized cleanup phrasing.
	multi := BuildCleanupNotification([]StateTransition{
		{Entry: PrEntry{PrName: "a", Branch: "b1", PrNumber: intp(1)}, State: PrStateMerged},
		{Entry: PrEntry{PrName: "c", Branch: "b2", PrNumber: intp(2)}, State: PrStateClosed},
	})
	for _, want := range []string{"PR #1", "was merged", "PR #2", "was closed", "their worktrees, branches, and tmux windows"} {
		if !strings.Contains(multi, want) {
			t.Errorf("coalesced notice missing %q:\n%s", want, multi)
		}
	}
	if strings.Count(multi, "pr-agents cleanup") != 1 {
		t.Errorf("want exactly one cleanup instruction (coalesced):\n%s", multi)
	}
}
