package cli

import (
	"testing"

	"github.com/anonx3247/pr-agents/internal/core"
)

func intp(n int) *int { return &n }

func TestBuildListRows(t *testing.T) {
	alive := func(id string) bool { return id == "%live" }
	entries := []core.PrEntry{
		{ID: "a", PrNumber: intp(7), PrName: "Feat", Branch: "br-a", Mode: core.ModeGraphite, Status: core.StatusOpen, PaneID: "%live"},
		{ID: "b", PrName: "Fix", Branch: "br-b", Mode: core.ModeIndependent, Status: core.StatusWorking, PaneID: "%dead"},
		{ID: "c", PrName: "Nopane", Branch: "br-c", Mode: core.ModeStack, Status: core.StatusWorking, PaneID: ""},
	}
	rows := buildListRows(entries, alive)
	if len(rows) != 3 {
		t.Fatalf("rows = %d, want 3", len(rows))
	}
	if !rows[0].Live {
		t.Error("row a should be live")
	}
	if rows[0].PrNumber == nil || *rows[0].PrNumber != 7 {
		t.Error("row a PrNumber should be 7")
	}
	if rows[1].Live {
		t.Error("row b should be dead")
	}
	if rows[2].Live {
		t.Error("row c (no pane) should be dead")
	}
}

func TestResolveSession(t *testing.T) {
	entries := []core.PrEntry{
		{ID: "x", SessionID: "sess-cwd", Worktree: "/repo/.worktrees/pr-1"},
	}
	t.Run("env wins", func(t *testing.T) {
		t.Setenv(core.EnvSession, "sess-env")
		if got := resolveSession(entries, "/repo/.worktrees/pr-1"); got != "sess-env" {
			t.Errorf("got %q, want sess-env", got)
		}
	})
	t.Run("falls back to cwd entry", func(t *testing.T) {
		t.Setenv(core.EnvSession, "")
		if got := resolveSession(entries, "/repo/.worktrees/pr-1/src"); got != "sess-cwd" {
			t.Errorf("got %q, want sess-cwd", got)
		}
	})
	t.Run("no match yields empty", func(t *testing.T) {
		t.Setenv(core.EnvSession, "")
		if got := resolveSession(entries, "/elsewhere"); got != "" {
			t.Errorf("got %q, want empty", got)
		}
	})
}
