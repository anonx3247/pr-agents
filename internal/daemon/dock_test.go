package daemon

import (
	"testing"

	"github.com/anonx3247/pr-agents/internal/core"
)

func agent(id, pane, createdAt string) core.PrEntry {
	return core.PrEntry{ID: id, SessionID: "s", Depth: 1, PaneID: pane, CreatedAt: createdAt, PrName: id}
}

func TestMaintainDockDocksNewest(t *testing.T) {
	st := &fakeStore{entries: []core.PrEntry{
		agent("a", "%a", "2024-01-01"),
		agent("b", "%b", "2024-01-02"),
	}}
	tm := &fakeTmux{alive: map[string]bool{"orch": true, "%a": true, "%b": true}}
	d := newDaemon(Config{OrchestratorPane: "orch"}, &fakeGH{}, tm, st)

	d.maintainDock()
	if d.dockedPane != "%b" {
		t.Fatalf("docked = %q, want %%b (newest)", d.dockedPane)
	}
	if len(tm.joins) != 1 || tm.joins[0] != [2]string{"%b", "orch"} {
		t.Errorf("joins = %#v", tm.joins)
	}
}

func TestMaintainDockAutoFlip(t *testing.T) {
	st := &fakeStore{entries: []core.PrEntry{agent("a", "%a", "2024-01-01")}}
	tm := &fakeTmux{alive: map[string]bool{"orch": true, "%a": true}}
	d := newDaemon(Config{OrchestratorPane: "orch"}, &fakeGH{}, tm, st)
	d.maintainDock()
	if d.dockedPane != "%a" {
		t.Fatalf("docked = %q, want %%a", d.dockedPane)
	}
	// A newer agent appears; it should be docked and the old one broken out.
	st.entries = append(st.entries, agent("b", "%b", "2024-01-03"))
	tm.alive["%b"] = true
	d.maintainDock()
	if d.dockedPane != "%b" {
		t.Fatalf("docked = %q, want %%b after flip", d.dockedPane)
	}
	if len(tm.breaks) != 1 || tm.breaks[0][0] != "%a" {
		t.Errorf("expected %%a broken out, breaks = %#v", tm.breaks)
	}
}

func TestMaintainDockCollapsesWhenDockedDies(t *testing.T) {
	st := &fakeStore{entries: []core.PrEntry{agent("a", "%a", "2024-01-01")}}
	tm := &fakeTmux{alive: map[string]bool{"orch": true, "%a": true}}
	d := newDaemon(Config{OrchestratorPane: "orch"}, &fakeGH{}, tm, st)
	d.maintainDock()
	// Docked agent's pane dies and no other live agents remain.
	tm.alive["%a"] = false
	d.maintainDock()
	if d.dockedPane != "" {
		t.Errorf("expected collapse, docked = %q", d.dockedPane)
	}
}

func TestMaintainDockNoDock(t *testing.T) {
	st := &fakeStore{entries: []core.PrEntry{agent("a", "%a", "2024-01-01")}}
	tm := &fakeTmux{alive: map[string]bool{"orch": true, "%a": true}}
	d := newDaemon(Config{OrchestratorPane: "orch", NoDock: true}, &fakeGH{}, tm, st)
	d.maintainDock()
	if len(tm.joins) != 0 || d.dockedPane != "" {
		t.Errorf("--no-dock should not dock anything; joins=%#v docked=%q", tm.joins, d.dockedPane)
	}
}

// finished builds an idle (finished, not stopped) depth-1 agent: it reported a
// result at lastResultAt and has not been re-activated since.
func finished(id, pane, createdAt, lastResultAt string) core.PrEntry {
	e := agent(id, pane, createdAt)
	e.Status = core.StatusOpen
	e.ResultSeq = intp(1)
	e.LastResultAt = lastResultAt
	return e
}

func TestMaintainDockFollowsWorkingAgent(t *testing.T) {
	// a is newer but finished/idle; b is older and still working (no result).
	idle := finished("a", "%a", "2024-03-01T00:00:00Z", "2024-03-02T00:00:00Z")
	working := agent("b", "%b", "2024-01-01T00:00:00Z") // no result => working
	st := &fakeStore{entries: []core.PrEntry{idle, working}}
	tm := &fakeTmux{alive: map[string]bool{"orch": true, "%a": true, "%b": true}}
	d := newDaemon(Config{OrchestratorPane: "orch"}, &fakeGH{}, tm, st)

	d.maintainDock()
	if d.dockedPane != "%b" {
		t.Fatalf("docked = %q, want %%b (the working agent, not the newer idle one)", d.dockedPane)
	}
}

func TestMaintainDockFlipsWhenIdleAgentReactivated(t *testing.T) {
	// Both agents are finished/idle; the newest (a) is docked initially.
	a := finished("a", "%a", "2024-03-01T00:00:00Z", "2024-03-02T00:00:00Z")
	b := finished("b", "%b", "2024-01-01T00:00:00Z", "2024-01-02T00:00:00Z")
	st := &fakeStore{entries: []core.PrEntry{a, b}}
	tm := &fakeTmux{alive: map[string]bool{"orch": true, "%a": true, "%b": true}}
	d := newDaemon(Config{OrchestratorPane: "orch"}, &fakeGH{}, tm, st)
	d.maintainDock()
	if d.dockedPane != "%a" {
		t.Fatalf("initial docked = %q, want %%a (newest while all idle)", d.dockedPane)
	}
	// b is handed a fresh review task => it starts working and should be docked.
	d.now = func() string { return "2024-09-01T00:00:00Z" }
	d.markActive("b")
	d.maintainDock()
	if d.dockedPane != "%b" {
		t.Fatalf("docked = %q, want %%b after re-activation", d.dockedPane)
	}
}

func TestMaintainDockNeverTouchesOrchestrator(t *testing.T) {
	// An (impossible) entry whose pane IS the orchestrator must never be docked.
	st := &fakeStore{entries: []core.PrEntry{agent("a", "orch", "2024-01-01")}}
	tm := &fakeTmux{alive: map[string]bool{"orch": true}}
	d := newDaemon(Config{OrchestratorPane: "orch"}, &fakeGH{}, tm, st)
	d.maintainDock()
	if len(tm.joins) != 0 {
		t.Errorf("orchestrator pane must never be joined, joins=%#v", tm.joins)
	}
}
