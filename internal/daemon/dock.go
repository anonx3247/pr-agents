package daemon

import "github.com/anonx3247/pr-agents/internal/core"

// maintainDock keeps a LIVE depth-1 PR agent docked to the RIGHT of the
// orchestrator pane, preferring whichever agent is actually WORKING. It is
// GUARDED so the orchestrator pane is never broken or killed, and is a no-op
// when --no-dock is set or no orchestrator pane is known.
//
// Each tick it:
//   - drops a dead docked pane from tracking (it broke itself by dying);
//   - picks the most-recently-active WORKING agent, falling back to the newest
//     live agent when none is working;
//   - re-docks when that differs from the currently-docked pane (so the dock
//     auto-flips to a freshly-dispatched agent AND follows work as an idle
//     docked agent is superseded by another agent that starts working), or
//     collapses to the orchestrator alone when no live agent remains.
func (d *Daemon) maintainDock() {
	if d.cfg.NoDock || d.cfg.OrchestratorPane == "" {
		return
	}
	// A docked pane that died is no longer docked.
	if d.dockedPane != "" && !d.tm.PaneAlive(d.dockedPane) {
		d.dockedPane = ""
	}
	entries := d.entries()
	next := d.pickDockTarget(entries)
	if next == nil {
		return // nothing live to dock; leave the orchestrator full-screen
	}
	if next.PaneID == d.dockedPane {
		return // already docked the right agent
	}
	d.dock(entries, next.PaneID)
}

// pickDockTarget chooses the agent to dock: the most-recently-active WORKING
// agent if any, else the newest live agent (so a fleet where everyone is idle
// still shows the latest PR rather than collapsing).
func (d *Daemon) pickDockTarget(entries []core.PrEntry) *core.PrEntry {
	if w := core.PickWorkingAgent(entries, d.tm.PaneAlive, d.isWorking, d.activatedAt); w != nil {
		return w
	}
	return core.PickRedockAgent(entries, d.tm.PaneAlive)
}

// isWorking reports whether a depth-1 agent is busy on work (as opposed to
// idle/stopped/terminal). An agent that has never reported a result is assumed
// working; one that HAS reported a result is idle until the daemon re-activates
// it (hands it a fresh review/CI task) after that result.
func (d *Daemon) isWorking(e core.PrEntry) bool {
	switch e.Status {
	case core.StatusStopped, core.StatusMerged, core.StatusClosed:
		return false
	}
	if e.ResultSeq == nil {
		return true
	}
	// Finished at least once: working only if re-activated AFTER the last result
	// (RFC3339 timestamps compare lexicographically in chronological order).
	return d.activatedAt(e) > e.LastResultAt
}

// activatedAt returns the RFC3339 time the daemon last handed e a task, or "".
func (d *Daemon) activatedAt(e core.PrEntry) string {
	return d.lastActivated[e.ID]
}

// dock joins paneID to the RIGHT of the orchestrator pane, undocking the current
// one first. Guarded so the orchestrator pane is never the source of a
// join/break. On a join failure the agent stays in its hidden window (no harm)
// and dockedPane is unchanged.
func (d *Daemon) dock(entries []core.PrEntry, paneID string) {
	target := d.cfg.OrchestratorPane
	if !d.dockable(paneID) {
		return
	}
	// Undock the currently-docked agent back to its own hidden window.
	if d.dockable(d.dockedPane) && d.tm.PaneAlive(d.dockedPane) {
		d.tm.BreakPane(d.dockedPane, d.windowNameFor(entries, d.dockedPane))
	}
	d.dockedPane = ""
	if !d.tm.JoinPane(paneID, target) {
		return
	}
	d.tm.SelectLayoutMainVertical(target)
	d.tm.SetMainPaneWidth(target, "60%")
	d.dockedPane = paneID
}

// dockable reports whether paneID is safe to dock/undock: non-empty and not the
// orchestrator's own pane (which must never be broken/killed).
func (d *Daemon) dockable(paneID string) bool {
	return paneID != "" && paneID != d.cfg.OrchestratorPane
}

// windowNameFor returns the tmux window name to restore a broken-out pane to,
// derived from its registry entry, falling back to "pr".
func (d *Daemon) windowNameFor(entries []core.PrEntry, paneID string) string {
	for _, e := range entries {
		if e.PaneID == paneID {
			return core.WindowName(core.PaneTitleArgs{PrNumber: e.PrNumber, PrName: e.PrName, Branch: e.Branch})
		}
	}
	return "pr"
}
