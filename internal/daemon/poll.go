package daemon

import "github.com/anonx3247/pr-agents/internal/core"

// pollPrState polls each pollable worker's GitHub PR state and, when a PR
// transitions to a terminal state (merged/closed), persists the status to the
// registry and notifies the orchestrator pane to run cleanup. Entries the worker
// has not yet pushed make ZERO gh calls.
//
// For the graphite strategy the whole stack's PR/merge state is read from
// Graphite's `.graphite_pr_info` cache in ONE shot; per-PR gh reads remain the
// fallback whenever gt has no data for a PR (so github/independent strategies,
// and graphite repos without a gt cache, behave exactly as before).
func (d *Daemon) pollPrState() {
	gtStates := d.graphiteStates()
	classified := make([]core.ClassifiedEntry, 0)
	for _, e := range d.entries() {
		if !core.IsPollable(e) {
			continue
		}
		state := gtStates[*e.PrNumber]
		if state == "" || state == core.PrStateUnknown {
			// No gt data for this PR — fall back to the per-entry gh read.
			j, ok := d.gh.PrState(e.Worktree, *e.PrNumber)
			if !ok {
				continue
			}
			state = core.ClassifyPrState(j)
		}
		if state == core.PrStateUnknown {
			continue // gh/gt missing/unauth/error — degrade silently
		}
		classified = append(classified, core.ClassifiedEntry{Entry: e, State: state})
	}

	transitions := core.SelectStateTransitions(classified, d.lastState)
	// Record every fresh state (incl. "open") so the map stays current.
	for _, c := range classified {
		d.lastState[c.Entry.ID] = c.State
	}

	for _, t := range transitions {
		status := core.StatusMerged
		if t.State == core.PrStateClosed {
			status = core.StatusClosed
		}
		d.store.Update(t.Entry.ID, func(p *core.PrEntry) { p.Status = status })
	}
	if len(transitions) > 0 && d.cfg.OrchestratorPane != "" {
		d.tm.SendToPane(d.cfg.OrchestratorPane, core.BuildCleanupNotification(transitions))
	}
}

// graphiteStates returns a prNumber→PrStateClass map read from Graphite's
// `.graphite_pr_info` cache in one shot, but only for the graphite strategy.
// For every other strategy (or when gt/graphite has no data) it returns nil, so
// callers transparently fall back to per-PR gh reads. Classification is done
// here via the pure core helper.
func (d *Daemon) graphiteStates() map[int]core.PrStateClass {
	if d.cfg.Strategy != core.StrategyGraphite {
		return nil
	}
	infos := d.gh.GraphitePrStates(d.cwd)
	if len(infos) == 0 {
		return nil
	}
	states := make(map[int]core.PrStateClass, len(infos))
	for _, i := range infos {
		states[i.PrNumber] = core.ClassifyGraphitePrState(i.State)
	}
	return states
}

// pollFinished notifies the orchestrator when a worker's reported result
// advances. It is purely registry-driven (no pane scraping): workers call
// `pr-agents tool report-result "<summary>"` as their final step, which bumps
// ResultSeq, and SelectNewlyFinished dedups against the in-memory last-seen map.
func (d *Daemon) pollFinished() {
	if d.cfg.OrchestratorPane == "" {
		return
	}
	fresh := core.SelectNewlyFinished(d.entries(), d.lastSeenResult)
	if len(fresh) == 0 {
		return
	}
	items := make([]core.PrEntry, 0, len(fresh))
	for _, f := range fresh {
		d.lastSeenResult[f.Entry.ID] = f.Seq
		items = append(items, f.Entry)
	}
	d.tm.SendToPane(d.cfg.OrchestratorPane, core.BuildFinishedNotification(items))
}

// pollWorkers polls each ALIVE worker's PR for new review feedback and CI
// failures, handing the worker a fresh task on its own pane when either appears.
// Dedup is via the per-entry seen-set persisted in the registry (rc:/rv:/ic:
// for reviews, ci:<sha>:<name> for CI), so nothing re-surfaces.
func (d *Daemon) pollWorkers() {
	if d.ownerRepo == nil {
		if owner, repo, ok := d.gh.OwnerRepo(d.cwd); ok {
			d.ownerRepo = &ownerRepo{owner: owner, repo: repo}
		}
	}
	for _, e := range d.entries() {
		if !core.IsPollable(e) || !d.tm.PaneAlive(e.PaneID) {
			continue
		}
		seen := seenSet(e.SeenReviewIds)

		// Review-comment loop: surface (and mark seen) only when there are NEW
		// actionable inline comments, so standalone context lingers until it
		// accompanies real work.
		if d.ownerRepo != nil {
			if fetched, ok := d.gh.ReviewActivity(d.ownerRepo.owner, d.ownerRepo.repo, *e.PrNumber, e.Worktree); ok {
				sel := core.SelectNewReviewItems(fetched, seen)
				if len(sel.Actionable) > 0 {
					d.markSeen(e.ID, sel.NewIDs)
					for _, id := range sel.NewIDs {
						seen[id] = true
					}
					d.tm.SendToPane(e.PaneID, core.BuildReviewTask(sel.Actionable, sel.ContextNotes, *e.PrNumber))
					d.markActive(e.ID)
				}
			}
		}

		// CI-failure loop: surface NEW failures for the current head commit.
		if headSha, checks, ok := d.gh.CiChecks(e.Worktree, *e.PrNumber); ok {
			sel := core.SelectNewCiFailures(checks, headSha, seen)
			if len(sel.Failures) > 0 {
				d.markSeen(e.ID, sel.NewKeys)
				d.tm.SendToPane(e.PaneID, core.BuildCiFixTask(sel.Failures, *e.PrNumber))
				d.markActive(e.ID)
			}
		}
	}
}

// markActive records that the daemon just handed a worker a fresh task, so the
// dock follows it as a now-working agent even if it had previously reported a
// result and gone idle.
func (d *Daemon) markActive(id string) {
	d.lastActivated[id] = d.now()
}

// markSeen merges newIDs into the entry's persisted seen-set (a union, never an
// overwrite) so the poller never re-surfaces the same item.
func (d *Daemon) markSeen(id string, newIDs []string) {
	if len(newIDs) == 0 {
		return
	}
	d.store.Update(id, func(p *core.PrEntry) {
		p.SeenReviewIds = core.MergeSeen(p.SeenReviewIds, newIDs)
	})
}

// seenSet builds a lookup set from a persisted seen-id slice.
func seenSet(ids []string) map[string]bool {
	set := make(map[string]bool, len(ids))
	for _, id := range ids {
		set[id] = true
	}
	return set
}
