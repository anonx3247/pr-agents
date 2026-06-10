package daemon

import "github.com/anonx3247/pr-agents/internal/core"

// maintainDock keeps the most-recently-created LIVE depth-1 PR agent docked to
// the RIGHT of the orchestrator pane. It is GUARDED so the orchestrator pane is
// never broken or killed, and is a no-op when --no-dock is set or no
// orchestrator pane is known.
//
// Each tick it:
//   - drops a dead docked pane from tracking (it broke itself by dying);
//   - picks the newest live agent via PickRedockAgent;
//   - re-docks when that differs from the currently-docked pane (auto-flip to a
//     freshly-dispatched agent), or collapses to the orchestrator alone when no
//     live agent remains.
func (d *Daemon) maintainDock() {
	if d.cfg.NoDock || d.cfg.OrchestratorPane == "" {
		return
	}
	// A docked pane that died is no longer docked.
	if d.dockedPane != "" && !d.tm.PaneAlive(d.dockedPane) {
		d.dockedPane = ""
	}
	entries := d.entries()
	next := core.PickRedockAgent(entries, d.tm.PaneAlive)
	if next == nil {
		return // nothing live to dock; leave the orchestrator full-screen
	}
	if next.PaneID == d.dockedPane {
		return // already docked the newest agent
	}
	d.dock(entries, next.PaneID)
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
