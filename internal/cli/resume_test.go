package cli

import (
	"reflect"
	"sort"
	"testing"

	"github.com/anonx3247/pr-agents/internal/core"
)

// TestBuildResumeCommandFor asserts the resume pane command resolves the
// adapter via the ENTRY's own harness and renders the right BuildResumeArgs argv
// with a `; exec <shell>` tail so the pane survives the harness exiting.
func TestBuildResumeCommandFor(t *testing.T) {
	e := core.PrEntry{Harness: "pi", PrName: "My PR", Worktree: "/wt", WorkerSessionRef: "/path/sess.jsonl"}
	got, err := buildResumeCommandFor(e, "claude", "", "bash")
	if err != nil {
		t.Fatalf("buildResumeCommandFor err = %v", err)
	}
	// pi adapter wins via PrEntry.Harness (not the "claude" orchestrator fallback);
	// launcher "" falls back to the adapter's default launcher "pi".
	want := "pi '--session' '/path/sess.jsonl' '-a' '--name' 'PR: My PR'; exec bash"
	if got != want {
		t.Errorf("command = %q, want %q", got, want)
	}

	// WorkerSessionHarness (captured) wins over the dispatched Harness.
	e2 := core.PrEntry{WorkerSessionHarness: "pi", Harness: "codex", PrName: "x", WorkerSessionRef: "ref"}
	got2, err := buildResumeCommandFor(e2, "claude", "pi", "bash")
	if err != nil {
		t.Fatalf("buildResumeCommandFor err = %v", err)
	}
	if want2 := "pi '--session' 'ref' '-a' '--name' 'PR: x'; exec bash"; got2 != want2 {
		t.Errorf("command = %q, want %q", got2, want2)
	}

	// Unknown harness degrades to an error (never a panic).
	if _, err := buildResumeCommandFor(core.PrEntry{Harness: "bogus"}, "", "", "bash"); err == nil {
		t.Error("expected error for unknown harness, got nil")
	}
}

// TestReviveAndRedock drives the resume wiring with fakes: dead depth-1 entries
// with worktree+ref are relaunched (PaneID updated, pane docked); live,
// missing-worktree, no-ref and depth-2 entries are skipped; a live agent is
// re-docked; a different scope is ignored.
func TestReviveAndRedock(t *testing.T) {
	entries := []core.PrEntry{
		{ID: "dead", SessionID: "s", Depth: 1, Status: core.StatusWorking, PaneID: "%dead", Worktree: "/wt/dead", WorkerSessionRef: "ref-dead", Harness: "pi"},
		{ID: "live", SessionID: "s", Depth: 1, Status: core.StatusWorking, PaneID: "%live", Worktree: "/wt/live", WorkerSessionRef: "ref-live", CreatedAt: "2024-01-01T00:00:00Z"},
		{ID: "noref", SessionID: "s", Depth: 1, Status: core.StatusWorking, PaneID: "%dead2", Worktree: "/wt/noref", WorkerSessionRef: ""},
		{ID: "gone", SessionID: "s", Depth: 1, Status: core.StatusWorking, PaneID: "%dead3", Worktree: "/gone", WorkerSessionRef: "ref"},
		{ID: "helper", SessionID: "s", Depth: 2, Status: core.StatusWorking, PaneID: "%dead4", Worktree: "/wt/h", WorkerSessionRef: "ref"},
		{ID: "other", SessionID: "other", Depth: 1, Status: core.StatusWorking, PaneID: "%dead5", Worktree: "/wt/o", WorkerSessionRef: "ref"},
	}

	type spawn struct{ id, command string }
	var spawned []spawn
	var docked []string
	updated := map[string]string{}

	deps := resumeDeps{
		paneAlive:         func(p string) bool { return p == "%live" },
		worktreeExists:    func(p string) bool { return p != "/gone" },
		sessionResolvable: func(e core.PrEntry) bool { return e.WorkerSessionRef != "" },
		buildCommand:      func(e core.PrEntry) (string, error) { return "CMD:" + e.ID, nil },
		spawnPane: func(e core.PrEntry, command string) (string, error) {
			spawned = append(spawned, spawn{e.ID, command})
			return "%new-" + e.ID, nil
		},
		updatePaneID: func(id, paneID string) { updated[id] = paneID },
		dock:         func(p string) { docked = append(docked, p) },
	}

	revived := reviveAndRedock(entries, "s", deps)
	if revived != 1 {
		t.Errorf("revived = %d, want 1", revived)
	}
	// Only the dead, in-scope, worktree+ref entry is relaunched.
	if !reflect.DeepEqual(spawned, []spawn{{"dead", "CMD:dead"}}) {
		t.Errorf("spawned = %+v, want [{dead CMD:dead}]", spawned)
	}
	if updated["dead"] != "%new-dead" || len(updated) != 1 {
		t.Errorf("updated = %v, want {dead: %%new-dead}", updated)
	}
	// The live agent is re-docked AND the revived pane is docked.
	sort.Strings(docked)
	if !reflect.DeepEqual(docked, []string{"%live", "%new-dead"}) {
		t.Errorf("docked = %v, want [%%live %%new-dead]", docked)
	}
}

// TestReviveOneGuarded asserts a failing/panicking entry never aborts revival
// and is not counted/recorded.
func TestReviveOneGuarded(t *testing.T) {
	e := core.PrEntry{ID: "x", Worktree: "/wt", WorkerSessionRef: "ref"}

	// build error → skipped.
	deps := resumeDeps{
		buildCommand: func(core.PrEntry) (string, error) { return "", errBuild },
		spawnPane:    func(core.PrEntry, string) (string, error) { t.Fatal("spawn should not run"); return "", nil },
		updatePaneID: func(string, string) { t.Fatal("update should not run") },
		dock:         func(string) { t.Fatal("dock should not run") },
	}
	if reviveOne(e, deps) {
		t.Error("reviveOne = true on build error, want false")
	}

	// spawn panic → recovered, returns false.
	deps2 := resumeDeps{
		buildCommand: func(core.PrEntry) (string, error) { return "cmd", nil },
		spawnPane:    func(core.PrEntry, string) (string, error) { panic("boom") },
		updatePaneID: func(string, string) {},
		dock:         func(string) {},
	}
	if reviveOne(e, deps2) {
		t.Error("reviveOne = true on spawn panic, want false")
	}
}

var errBuild = &buildErr{}

type buildErr struct{}

func (*buildErr) Error() string { return "build failed" }
