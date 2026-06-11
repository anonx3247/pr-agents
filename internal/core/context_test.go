package core

import (
	"reflect"
	"testing"
)

func ptr(n int) *int { return &n }

func TestResolveContextFromCwd(t *testing.T) {
	entries := []PrEntry{
		{ID: "root", Worktree: "/repo"},
		{ID: "wt1", Worktree: "/repo/.worktrees/pr-1"},
		{ID: "wt2", Worktree: "/repo/.worktrees/pr-1/nested"},
		{ID: "nowt", Worktree: ""},
	}
	tests := []struct {
		name string
		cwd  string
		want string // entry id, "" for nil
	}{
		{"exact worktree", "/repo/.worktrees/pr-1", "wt1"},
		{"descendant of worktree", "/repo/.worktrees/pr-1/src/pkg", "wt1"},
		{"longest match wins", "/repo/.worktrees/pr-1/nested/deep", "wt2"},
		{"repo root", "/repo", "root"},
		{"descendant of root only", "/repo/cmd", "root"},
		{"trailing slash normalized", "/repo/.worktrees/pr-1/", "wt1"},
		{"no match", "/elsewhere", ""},
		{"sibling not a submatch", "/repo/.worktrees/pr-12", "root"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ResolveContextFromCwd(entries, tt.cwd)
			if tt.want == "" {
				if got != nil {
					t.Fatalf("got %q, want nil", got.ID)
				}
				return
			}
			if got == nil {
				t.Fatalf("got nil, want %q", tt.want)
			}
			if got.ID != tt.want {
				t.Errorf("got %q, want %q", got.ID, tt.want)
			}
		})
	}
}

func TestDepthFromCwd(t *testing.T) {
	entries := []PrEntry{
		{ID: "wt1", Worktree: "/repo/.worktrees/pr-1", Depth: 1},
		{ID: "wt2", Worktree: "/repo/.worktrees/pr-1/nested", Depth: 2},
	}
	tests := []struct {
		name string
		cwd  string
		want int
	}{
		{"orchestrator in main repo", "/repo", 0},
		{"unregistered path", "/elsewhere", 0},
		{"depth-1 worktree", "/repo/.worktrees/pr-1", 1},
		{"descendant of depth-1 worktree", "/repo/.worktrees/pr-1/src", 1},
		{"nested depth-2 worktree", "/repo/.worktrees/pr-1/nested", 2},
		{"descendant of nested worktree", "/repo/.worktrees/pr-1/nested/deep", 2},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := DepthFromCwd(entries, tt.cwd); got != tt.want {
				t.Errorf("DepthFromCwd(%q) = %d, want %d", tt.cwd, got, tt.want)
			}
		})
	}
}

func TestPickRedockAgent(t *testing.T) {
	alive := func(id string) bool { return id != "%dead" }
	entries := []PrEntry{
		{ID: "a", Depth: 1, PaneID: "%1", CreatedAt: "2024-01-01T00:00:00Z"},
		{ID: "b", Depth: 1, PaneID: "%2", CreatedAt: "2024-03-01T00:00:00Z"},
		{ID: "c", Depth: 1, PaneID: "%dead", CreatedAt: "2024-09-01T00:00:00Z"}, // dead, ignored
		{ID: "helper", Depth: 2, PaneID: "%3", CreatedAt: "2024-12-01T00:00:00Z"},
		{ID: "nopane", Depth: 1, PaneID: "", CreatedAt: "2024-12-01T00:00:00Z"},
	}
	got := PickRedockAgent(entries, alive)
	if got == nil || got.ID != "b" {
		t.Fatalf("got %v, want b", got)
	}

	if got := PickRedockAgent(nil, alive); got != nil {
		t.Errorf("empty: got %v, want nil", got)
	}
	allDead := []PrEntry{{ID: "x", Depth: 1, PaneID: "%dead", CreatedAt: "2024-01-01T00:00:00Z"}}
	if got := PickRedockAgent(allDead, alive); got != nil {
		t.Errorf("all dead: got %v, want nil", got)
	}
}

func TestPickWorkingAgent(t *testing.T) {
	alive := func(string) bool { return true }
	entries := []PrEntry{
		{ID: "old", Depth: 1, PaneID: "%1", CreatedAt: "2024-01-01T00:00:00Z"},
		{ID: "new", Depth: 1, PaneID: "%2", CreatedAt: "2024-03-01T00:00:00Z"},
	}
	working := func(e PrEntry) bool { return true }
	noActivity := func(PrEntry) string { return "" }

	// No activity recorded: ranks by CreatedAt, newest wins.
	if got := PickWorkingAgent(entries, alive, working, noActivity); got == nil || got.ID != "new" {
		t.Fatalf("got %v, want new", got)
	}

	// The older agent was just re-activated (handed a task) → it now ranks above
	// the newer-but-idle agent.
	activity := func(e PrEntry) string {
		if e.ID == "old" {
			return "2024-09-01T00:00:00Z"
		}
		return ""
	}
	if got := PickWorkingAgent(entries, alive, working, activity); got == nil || got.ID != "old" {
		t.Fatalf("got %v, want old (most recently active)", got)
	}

	// Only the idle/finished agent is excluded; when none works, nil.
	noneWorking := func(PrEntry) bool { return false }
	if got := PickWorkingAgent(entries, alive, noneWorking, noActivity); got != nil {
		t.Errorf("none working: got %v, want nil", got)
	}
}

func TestSelectSessionCaptureTargets(t *testing.T) {
	alive := func(id string) bool { return id != "%dead" }
	entries := []PrEntry{
		{ID: "eligible", Depth: 1, PaneID: "%1"},
		{ID: "hasref", Depth: 1, PaneID: "%2", WorkerSessionRef: "already"}, // skip: captured
		{ID: "dead", Depth: 1, PaneID: "%dead"},                             // skip: dead pane
		{ID: "helper", Depth: 2, PaneID: "%3"},                              // skip: depth-2
		{ID: "nopane", Depth: 1, PaneID: ""},                                // skip: no pane
	}
	got := SelectSessionCaptureTargets(entries, alive)
	if ids := idsOf(got); !reflect.DeepEqual(ids, []string{"eligible"}) {
		t.Errorf("capture targets = %v, want [eligible]", ids)
	}
	if got := SelectSessionCaptureTargets(nil, alive); len(got) != 0 {
		t.Errorf("empty: got %v, want none", got)
	}
}

func TestResolveScopeID(t *testing.T) {
	ref := func(r string, ok bool) ScopeRefResolver {
		return func() (string, bool) { return r, ok }
	}
	fallback := func() string { return "RANDOM" }
	tests := []struct {
		name     string
		resolver ScopeRefResolver
		env      string
		fresh    bool
		want     string
	}{
		{"harness ref wins over env", ref("REF", true), "ENV", false, "REF"},
		{"ref trimmed", ref("  REF ", true), "ENV", false, "REF"},
		{"blank ref falls through to env", ref("   ", true), "ENV", false, "ENV"},
		{"ref ok=false falls through to env", ref("REF", false), "ENV", false, "ENV"},
		{"nil resolver uses env", nil, "ENV", false, "ENV"},
		{"env trimmed", ref("", false), "  ENV  ", false, "ENV"},
		{"no ref no env uses fallback", ref("", false), "", false, "RANDOM"},
		{"nil resolver empty env uses fallback", nil, "", false, "RANDOM"},
		// fresh=true bypasses ref + env entirely and always mints via fallback.
		{"fresh ignores winning ref", ref("REF", true), "ENV", true, "RANDOM"},
		{"fresh ignores env", ref("", false), "ENV", true, "RANDOM"},
		{"fresh with nil resolver", nil, "ENV", true, "RANDOM"},
		{"fresh with no ref no env", ref("", false), "", true, "RANDOM"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := ResolveScopeID(tt.resolver, tt.env, fallback, tt.fresh); got != tt.want {
				t.Errorf("ResolveScopeID = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestEntryHarness(t *testing.T) {
	tests := []struct {
		name     string
		entry    PrEntry
		fallback string
		want     string
	}{
		{"captured wins", PrEntry{WorkerSessionHarness: "claude", Harness: "pi"}, "codex", "claude"},
		{"dispatched when no captured", PrEntry{Harness: "pi"}, "codex", "pi"},
		{"fallback when legacy entry", PrEntry{}, "codex", "codex"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := EntryHarness(tt.entry, tt.fallback); got != tt.want {
				t.Errorf("EntryHarness = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestSelectRevivableAgents(t *testing.T) {
	checks := RevivableChecks{
		PaneAlive:         func(p string) bool { return p == "%live" },
		WorktreeExists:    func(p string) bool { return p != "/gone" },
		SessionResolvable: func(e PrEntry) bool { return e.WorkerSessionRef != "" },
	}
	entries := []PrEntry{
		{ID: "revivable", Depth: 1, Status: StatusWorking, PaneID: "%dead", Worktree: "/wt", WorkerSessionRef: "ref"},
		{ID: "deadnopane", Depth: 1, Status: StatusOpen, PaneID: "", Worktree: "/wt", WorkerSessionRef: "ref"},             // empty pane = dead
		{ID: "live", Depth: 1, Status: StatusWorking, PaneID: "%live", Worktree: "/wt", WorkerSessionRef: "ref"},           // skip: still running
		{ID: "merged", Depth: 1, Status: StatusMerged, PaneID: "%dead", Worktree: "/wt", WorkerSessionRef: "ref"},          // skip: terminal
		{ID: "closed", Depth: 1, Status: StatusClosed, PaneID: "%dead", Worktree: "/wt", WorkerSessionRef: "ref"},          // skip: terminal
		{ID: "stopped", Depth: 1, Status: StatusStopped, PaneID: "%dead", Worktree: "/wt", WorkerSessionRef: "ref"},        // skip: terminal
		{ID: "helper", Depth: 2, Status: StatusWorking, PaneID: "%dead", Worktree: "/wt", WorkerSessionRef: "ref"},         // skip: depth-2
		{ID: "noworktree", Depth: 1, Status: StatusWorking, PaneID: "%dead", Worktree: "", WorkerSessionRef: "ref"},        // skip: no worktree
		{ID: "goneworktree", Depth: 1, Status: StatusWorking, PaneID: "%dead", Worktree: "/gone", WorkerSessionRef: "ref"}, // skip: reaped
		{ID: "noref", Depth: 1, Status: StatusWorking, PaneID: "%dead", Worktree: "/wt", WorkerSessionRef: ""},             // skip: no session
	}
	got := SelectRevivableAgents(entries, checks)
	if ids := idsOf(got); !reflect.DeepEqual(ids, []string{"revivable", "deadnopane"}) {
		t.Errorf("revivable = %v, want [revivable deadnopane]", ids)
	}
	if got := SelectRevivableAgents(nil, checks); len(got) != 0 {
		t.Errorf("empty: got %v, want none", got)
	}
}

func TestSelectCleanupTargets(t *testing.T) {
	entries := []PrEntry{
		{ID: "merged", Branch: "br-m", Base: "main", Depth: 1, PrNumber: ptr(1)},
		{ID: "closed", Branch: "br-c", Base: "main", Depth: 1, PrNumber: ptr(2)},
		{ID: "open", Branch: "br-o", Base: "main", Depth: 1, PrNumber: ptr(3)},
		{ID: "bmerged", Branch: "br-b", Base: "main", Depth: 1},
		{ID: "helper", Branch: "br-h", Base: "main", Depth: 2},
	}
	stateByID := map[string]PrStateClass{
		"merged": PrStateMerged,
		"closed": PrStateClosed,
		"open":   PrStateOpen,
	}
	branchMerged := func(e PrEntry) bool { return e.ID == "bmerged" }

	remove, keep := SelectCleanupTargets(entries, stateByID, branchMerged)

	gotRemove := map[string]string{}
	for _, t := range remove {
		gotRemove[t.Entry.ID] = t.Reason
	}
	if gotRemove["merged"] != "PR merged" {
		t.Errorf("merged reason = %q", gotRemove["merged"])
	}
	if gotRemove["closed"] != "PR closed" {
		t.Errorf("closed reason = %q", gotRemove["closed"])
	}
	if gotRemove["bmerged"] != "branch merged into main" {
		t.Errorf("bmerged reason = %q", gotRemove["bmerged"])
	}
	if len(remove) != 3 {
		t.Errorf("remove count = %d, want 3", len(remove))
	}

	keptIDs := map[string]bool{}
	for _, e := range keep {
		keptIDs[e.ID] = true
	}
	if !keptIDs["open"] || !keptIDs["helper"] {
		t.Errorf("keep = %v, want open+helper kept", keptIDs)
	}
	if len(keep) != 2 {
		t.Errorf("keep count = %d, want 2", len(keep))
	}
}
