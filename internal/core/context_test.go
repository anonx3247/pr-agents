package core

import "testing"

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
