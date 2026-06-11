package cli

import (
	"flag"
	"fmt"
	"io"
	"os"

	"github.com/anonx3247/pr-agents/internal/core"
	"github.com/anonx3247/pr-agents/internal/harness"
	"github.com/anonx3247/pr-agents/internal/tmux"
)

// resumeDeps injects the IO a resume performs (pane liveness, worktree/session
// checks, command build, pane spawn, registry update, dock) so reviveAndRedock
// stays unit-testable with fakes — no real "~"/tmux in tests.
type resumeDeps struct {
	paneAlive         func(paneID string) bool
	worktreeExists    func(path string) bool
	sessionResolvable func(e core.PrEntry) bool
	buildCommand      func(e core.PrEntry) (string, error)
	spawnPane         func(e core.PrEntry, command string) (paneID string, err error)
	updatePaneID      func(id, paneID string)
	dock              func(paneID string)
}

// reviveAndRedock is the pure-wiring core of `pr-agents resume`: it re-docks a
// live agent if one exists, then relaunches a fresh background pane for each
// revivable dead agent, updating its PaneID and re-docking it. It is fully
// guarded — one bad entry never aborts the rest. Returns the count revived.
func reviveAndRedock(entries []core.PrEntry, session string, deps resumeDeps) int {
	scoped := core.EntriesForSession(entries, session)

	// (a) Re-dock a live agent so the resumed orchestrator regains its layout.
	if live := core.PickRedockAgent(scoped, deps.paneAlive); live != nil {
		deps.dock(live.PaneID)
	}

	// (b) Relaunch each dead-but-revivable agent's own session in its worktree.
	revivable := core.SelectRevivableAgents(scoped, core.RevivableChecks{
		PaneAlive:         deps.paneAlive,
		WorktreeExists:    deps.worktreeExists,
		SessionResolvable: deps.sessionResolvable,
	})
	revived := 0
	for _, e := range revivable {
		if reviveOne(e, deps) {
			revived++
		}
	}
	return revived
}

// reviveOne relaunches a single revivable agent, guarded so a failure (or panic)
// reviving one never blocks the others. Returns true when the pane was spawned
// and its PaneID recorded.
func reviveOne(e core.PrEntry, deps resumeDeps) (ok bool) {
	defer func() {
		if recover() != nil {
			ok = false
		}
	}()
	command, err := deps.buildCommand(e)
	if err != nil {
		return false
	}
	paneID, err := deps.spawnPane(e, command)
	if err != nil || paneID == "" {
		return false
	}
	deps.updatePaneID(e.ID, paneID)
	deps.dock(paneID)
	return true
}

// resumeKindLauncher resolves the harness kind and launcher prefix to use when
// reviving an entry: the entry's OWN harness (EntryHarness) and the
// orchestrator launcher, falling back to the adapter's default launcher when no
// launcher override is set.
func resumeKindLauncher(e core.PrEntry, orchHarness, launcher string) (kind, resolvedLauncher string, a harness.Adapter, err error) {
	kind = core.EntryHarness(e, orchHarness)
	a, err = harness.Get(kind)
	if err != nil {
		return kind, "", nil, err
	}
	resolvedLauncher = launcher
	if resolvedLauncher == "" {
		resolvedLauncher = a.DefaultLauncher()
	}
	return kind, resolvedLauncher, a, nil
}

// buildResumeCommandFor builds the pane shell command that RESUMES an entry's
// own harness session in its worktree, via the entry's harness adapter
// BuildResumeArgs. It mirrors dispatch's buildLaunchCommand (launcher tokens +
// shell-quoted args + a trailing `; exec <shell>` so the pane stays alive after
// the harness exits). Pure given the entry; no IO beyond the adapter lookup.
func buildResumeCommandFor(e core.PrEntry, orchHarness, launcher, shell string) (string, error) {
	_, l, a, err := resumeKindLauncher(e, orchHarness, launcher)
	if err != nil {
		return "", err
	}
	spec := harness.LaunchSpec{PrName: e.PrName, Worktree: e.Worktree}
	args := a.BuildResumeArgs(spec, e.WorkerSessionRef)
	return buildLaunchCommand(l, args, shell), nil
}

// dockToOrchestrator joins paneID to the RIGHT of the orchestrator pane and
// re-tiles main-vertical, mirroring the daemon's dock. Guarded: a no-op when
// there is no orchestrator pane, an empty pane, or the orchestrator's own pane
// (which must never be joined into itself). Best-effort; the daemon's ongoing
// dock-maintenance reconciles the final layout.
func dockToOrchestrator(orchPane, paneID string) {
	if orchPane == "" || paneID == "" || paneID == orchPane {
		return
	}
	if !tmux.JoinPane(paneID, orchPane) {
		return
	}
	tmux.SelectLayoutMainVertical(orchPane)
	tmux.SetMainPaneWidth(orchPane, "60%")
}

// dirExists reports whether path is an existing directory. Used as the
// WorktreeExists check so a reaped worktree is not revived.
func dirExists(path string) bool {
	if path == "" {
		return false
	}
	info, err := os.Stat(path)
	return err == nil && info.IsDir()
}

// sessionResolvable reports whether an entry carries a usable resumable session
// ref (captured by the daemon). A revive needs it to relaunch the session.
func sessionResolvable(e core.PrEntry) bool {
	return e.WorkerSessionRef != ""
}

// reviveScope wires reviveAndRedock to the real tmux/registry IO for the given
// scope and returns the number of agents revived. Best-effort throughout: a bad
// registry/harness/tmux state degrades to fewer revivals, never a panic.
func reviveScope(all []core.PrEntry, cwd, session, orchHarness, launcher, orchPane string) int {
	shell := os.Getenv("SHELL")
	deps := resumeDeps{
		paneAlive:         paneAlive,
		worktreeExists:    dirExists,
		sessionResolvable: sessionResolvable,
		buildCommand: func(e core.PrEntry) (string, error) {
			return buildResumeCommandFor(e, orchHarness, launcher, shell)
		},
		spawnPane: func(e core.PrEntry, command string) (string, error) {
			kind, l, _, err := resumeKindLauncher(e, orchHarness, launcher)
			if err != nil {
				return "", err
			}
			env := dispatchPaneEnv(session, kind, l)
			titleArgs := core.PaneTitleArgs{PrNumber: e.PrNumber, PrName: e.PrName, Branch: e.Branch}
			return tmux.OpenWindow(e.Worktree, command, core.PaneTitle(titleArgs), core.WindowName(titleArgs), env)
		},
		updatePaneID: func(id, paneID string) {
			_, _, _ = core.UpdateEntry(cwd, id, func(p *core.PrEntry) { p.PaneID = paneID })
		},
		dock: func(paneID string) { dockToOrchestrator(orchPane, paneID) },
	}
	return reviveAndRedock(all, session, deps)
}

// runResume implements `pr-agents resume`: the top-level, human+orchestrator
// facing entry point the orchestrator runs on startup/resume to re-dock a live
// agent and revive any dead PR panes for its scope. It is fully guarded and a
// no-op when there is nothing to revive.
func runResume(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("resume", flag.ContinueOnError)
	fs.SetOutput(stderr)
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if !tmux.InsideTmux() {
		fmt.Fprintln(stderr, "pr-agents resume: not inside tmux; run `pr-agents start` first")
		return 1
	}
	cwd, err := os.Getwd()
	if err != nil {
		fmt.Fprintf(stderr, "pr-agents resume: %v\n", err)
		return 1
	}
	all, err := core.LoadRegistry(cwd)
	if err != nil {
		fmt.Fprintf(stderr, "pr-agents resume: %v\n", err)
		return 1
	}
	session := resolveSession(all, cwd)
	// Recover harness+launcher with the same precedence as dispatch (env >
	// persisted session record) so a resume under a sandbox launcher — where the
	// PRA_* env vars were stripped — still relaunches dead panes WITH the sandbox
	// prefix instead of escaping it via the bare default launcher. Each entry's
	// own PrEntry.Harness still wins per-pane; these are the legacy/launcher
	// fallbacks.
	var rec core.SessionRecord
	if r, ok, _ := core.LoadSessionRecord(cwd, session); ok {
		rec = r
	}
	orchHarness := core.ResolveFromSources("", os.Getenv(core.EnvHarness), rec.Harness)
	launcher := core.ResolveFromSources("", os.Getenv(core.EnvLauncher), rec.Launcher)
	revived := reviveScope(all, cwd, session, orchHarness, launcher, orchestratorPane())
	fmt.Fprintf(stdout, "pr-agents resume: scope %s, revived %d dead agent(s).\n", session, revived)
	return 0
}
