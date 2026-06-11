// Package daemon implements the per-session background daemon that replaces
// pi's in-process timers with a harness-agnostic, long-lived loop. It polls
// GitHub PR state, CI checks, and review activity, and drives ALL cross-agent
// messaging through `tmux send-keys` (no harness-specific message APIs). One
// daemon runs per orchestrator session, started best-effort by `pr-agents
// start`.
//
// All gh/git/tmux access is behind small interfaces (GH, Tmuxer, Store) so the
// tick decision logic is unit-tested with fakes. Every tick is wrapped so a
// gh/git/tmux failure never kills the loop.
package daemon

import (
	"io"
	"time"

	"github.com/anonx3247/pr-agents/internal/core"
)

// Default poll intervals. gh state polls are network-bound so they run slower;
// review/CI polls run on their own slightly slower cadence.
const (
	DefaultGhInterval     = 15 * time.Second
	DefaultReviewInterval = 30 * time.Second
	// dockInterval drives the cheap, tmux-only dock maintenance / finished-result
	// poll; it is fast because it makes no network calls.
	dockInterval = 2 * time.Second
)

// Config holds the daemon's runtime configuration, supplied by the CLI verb.
//
// The daemon only READS gh/git state and sends tmux messages to EXISTING panes
// — it never spawns agents. It therefore carries NO launcher (the spawn-command
// prefix), which would otherwise be a shell-injection / privilege-escalation
// surface; pane/process creation stays solely in `start`/`dispatch`.
type Config struct {
	Session          string
	OrchestratorPane string
	// Harness is the fallback harness KIND ("pi"/"claude"/"codex"), used by the
	// upstack session-capture step to pick which on-disk session store to scan.
	// It is just a selector string, never an executed command — not a launch or
	// privilege vector.
	Harness        string
	GhInterval     time.Duration
	ReviewInterval time.Duration
	NoDock         bool
	// Strategy is the project's default stacking strategy. When it is
	// StrategyGraphite, the gh-state poller reads the whole stack's PR/merge
	// state from Graphite's `.graphite_pr_info` cache in one shot, falling back to
	// per-PR gh reads when gt/graphite is absent.
	Strategy core.StackStrategy
}

// GH abstracts the gh/git reads the daemon performs, so the tick logic can be
// driven by fakes in tests. Every method degrades to ok=false on any failure.
type GH interface {
	// PrState reads `gh pr view <n> --json state,mergedAt,closedAt,url` in
	// worktree and returns the parsed JSON subset.
	PrState(worktree string, number int) (*core.PrStateJSON, bool)
	// CiChecks reads the PR head sha and CI check states in cwd.
	CiChecks(cwd string, number int) (headSha string, checks []core.CiCheck, ok bool)
	// ReviewActivity reads inline comments + reviews + issue comments for a PR.
	ReviewActivity(owner, repo string, number int, cwd string) (core.FetchedReviewActivity, bool)
	// OwnerRepo resolves the current repo's owner/name in cwd.
	OwnerRepo(cwd string) (owner, repo string, ok bool)
	// GraphitePrStates refreshes and reads the whole Graphite stack's PR/merge
	// state from `.graphite_pr_info` in ONE shot (used only for the graphite
	// strategy). It returns nil when gt/graphite is absent or the cache is
	// unreadable, so callers degrade to per-PR gh reads.
	GraphitePrStates(cwd string) []core.GraphitePrInfo
}

// Tmuxer abstracts the tmux operations the daemon performs (messaging + dock
// primitives), so the dock/notification logic is testable with a fake.
type Tmuxer interface {
	SendToPane(paneID, message string) bool
	PaneAlive(paneID string) bool
	JoinPane(srcPane, targetPane string) bool
	BreakPane(srcPane, windowName string) bool
	SelectLayoutMainVertical(targetPane string)
	SetMainPaneWidth(targetPane, width string)
}

// Store abstracts the registry reads/writes the daemon performs, so the tick
// logic is testable without a real git repo.
type Store interface {
	Load() []core.PrEntry
	Update(id string, patch func(*core.PrEntry))
}

// SessionResolver resolves the resumable session reference for a harness session
// of the given kind running in cwd, considering only sessions created at or
// after since. It returns ok=false when none has appeared yet (so the daemon
// retries on a later tick). The daemon injects this so tests use a fake instead
// of scanning the real "~" session stores.
type SessionResolver func(kind, cwd string, since time.Time) (ref string, ok bool)

// Daemon is the per-session poller. It holds the injected IO interfaces plus the
// in-memory dedup state seeded from the registry on the first tick.
type Daemon struct {
	cfg   Config
	gh    GH
	tm    Tmuxer
	store Store
	cwd   string

	// resolveSession resolves a worker's resumable session ref (injected so tests
	// avoid the real session stores).
	resolveSession SessionResolver

	// In-memory dedup state across ticks.
	lastState      map[string]core.PrStateClass
	lastSeenResult map[string]int
	ownerRepo      *ownerRepo
	dockedPane     string
	// lastActivated maps entry id → RFC3339 time the daemon last handed that
	// worker a task (review/CI). It distinguishes a worker that is busy on fresh
	// work from one that has gone idle after reporting a result, so the dock can
	// follow whichever agent is actually working.
	lastActivated map[string]string
	// now is the activity clock (RFC3339), injectable so dock tests are
	// deterministic.
	now func() string
}

type ownerRepo struct {
	owner string
	repo  string
}

// New constructs a Daemon with the given config, IO interfaces, and cwd.
func New(cfg Config, gh GH, tm Tmuxer, store Store, cwd string) *Daemon {
	if cfg.GhInterval <= 0 {
		cfg.GhInterval = DefaultGhInterval
	}
	if cfg.ReviewInterval <= 0 {
		cfg.ReviewInterval = DefaultReviewInterval
	}
	return &Daemon{
		cfg:            cfg,
		gh:             gh,
		tm:             tm,
		store:          store,
		cwd:            cwd,
		resolveSession: realSessionResolver,
		lastState:      map[string]core.PrStateClass{},
		lastSeenResult: map[string]int{},
		lastActivated:  map[string]string{},
		now:            func() string { return time.Now().UTC().Format(time.RFC3339) },
	}
}

// entries returns this session's registry entries.
func (d *Daemon) entries() []core.PrEntry {
	return core.EntriesForSession(d.store.Load(), d.cfg.Session)
}

// seed primes the dedup maps from the current registry so existing results and
// PR states don't replay as notifications the moment the daemon starts. Pollable
// entries start as "open" and are re-classified on the first tick, so a PR that
// merged while the daemon was down still surfaces as a fresh transition.
func (d *Daemon) seed() {
	for _, e := range d.entries() {
		if core.IsPollable(e) {
			d.lastState[e.ID] = core.PrStateOpen
		}
		if e.Depth == 1 && e.ResultSeq != nil {
			d.lastSeenResult[e.ID] = *e.ResultSeq
		}
	}
}

// safe runs fn, swallowing any panic so one bad tick never kills the loop.
func safe(fn func()) {
	defer func() { _ = recover() }()
	fn()
}

// Run starts the daemon loop until ctx-equivalent stop channel is closed. It is
// resilient: every tick is wrapped so a gh/git/tmux failure never kills the
// loop. The loop drives three cadences: a fast tmux-only tick (dock + finished),
// a gh-state tick, and a review/CI tick.
func (d *Daemon) Run(stop <-chan struct{}, logw io.Writer) {
	d.seed()

	// Fire each cadence once up front so a freshly-started daemon reacts without
	// waiting a full interval.
	safe(d.tickFast)
	safe(d.pollPrState)
	safe(d.pollWorkers)

	fast := time.NewTicker(dockInterval)
	gh := time.NewTicker(d.cfg.GhInterval)
	review := time.NewTicker(d.cfg.ReviewInterval)
	defer fast.Stop()
	defer gh.Stop()
	defer review.Stop()

	for {
		select {
		case <-stop:
			return
		case <-fast.C:
			safe(d.tickFast)
		case <-gh.C:
			safe(d.pollPrState)
		case <-review.C:
			safe(d.pollWorkers)
		}
	}
}

// tickFast runs the cheap, local maintenance: dock auto-flip, the
// registry-driven worker-finished notification, and resumable-session capture
// (all tmux/registry/disk only, no network).
func (d *Daemon) tickFast() {
	d.maintainDock()
	d.pollFinished()
	d.captureSessions()
}

// captureSessions records each live worker's resumable session reference the
// first time its harness session file appears on disk. It is idempotent (an
// entry that already carries a ref is skipped by SelectSessionCaptureTargets),
// bounded (a resolver miss simply retries on a later tick), and degrades
// silently: an absent session store yields ok=false and the entry is left
// untouched. The harness kind comes from the ENTRY's own dispatched harness
// (falling back to the daemon's config for legacy entries) and is persisted
// alongside the ref so a later revive knows how to resume it.
func (d *Daemon) captureSessions() {
	for _, e := range core.SelectSessionCaptureTargets(d.entries(), d.tm.PaneAlive) {
		kind := core.EntryHarness(e, d.cfg.Harness)
		ref, ok := d.resolveSession(kind, e.Worktree, parseCreatedAt(e.CreatedAt))
		if !ok {
			continue
		}
		d.store.Update(e.ID, func(p *core.PrEntry) {
			p.WorkerSessionRef = ref
			p.WorkerSessionHarness = kind
		})
	}
}

// parseCreatedAt parses an entry's RFC3339 CreatedAt into a since cutoff,
// falling back to the zero time (accept any session file) on a parse error.
func parseCreatedAt(s string) time.Time {
	t, err := time.Parse(time.RFC3339, s)
	if err != nil {
		return time.Time{}
	}
	return t
}
