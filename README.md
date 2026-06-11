# pr-agents

Harness-agnostic PR orchestration. A standalone Go CLI + per-session daemon that
splits work into small pull requests and hands **each PR to its own dedicated
subagent** running in an isolated git worktree, laid out as a labelled **tmux
pane**. Works with multiple agent harnesses (pi, claude, codex) and integrates
with `asb`/`sx` for sandboxing and secrets, polling GitHub / Graphite for PR
state, CI, and reviews.

Status: early scaffolding. The Go module, core domain (registry, config,
string/git/PR-state helpers), tmux layer, CLI verbs, harness adapters, and the
per-session daemon are in place, with the pi, claude, and codex harness
adapters. Later PRs add the interactive `select` picker.

## Build & test

Requires Go 1.26+. Common targets (see the `Makefile`):

```bash
make build       # go build -o bin/pr-agents .
make test        # go test ./...
make vet         # go vet ./...
make fmt         # gofmt -w .
make fmt-check   # fail if anything is not gofmt-clean
make ci          # fmt-check + vet + test (the gate CI runs)
```

Or directly:

```bash
go build ./...
go test ./...
go vet ./...
gofmt -l .       # lists files needing formatting (should be empty)
```

CI (`.github/workflows/ci.yml`) runs the gofmt check, `go vet ./...`,
`go build ./...`, and `go test ./...` on every push and pull request.

## Package layout

```
main.go                  Thin entrypoint → internal/cli.Run
internal/cli/            Root command, stdlib-flag dispatch, CLI verbs (incl. daemon)
internal/core/           Harness-agnostic pure logic:
  env.go                   Orchestrator-side PRA_* env contract
  context.go               ResolveContextFromCwd + DepthFromCwd (cwd→registry)
  strings.go               Slugify, Shq, BuildEnv, CapTail, PaneTitle, WindowName
  git.go                   RepoRoot, GitCommonDir, DefaultBranch, UniqueBranch,
                           WorktreesDirFrom, AppendExcludeEntry/EnsureExcluded
  registry.go              PrEntry + shared registry (atomic load/save, UpdateEntry,
                           EntriesForSession, FindEntry)
  config.go                Per-project config (stacking strategy)
  prstate.go               Pure PR-state classification + diff parse helpers
  review.go / ci.go        Pure review/CI selectors + task builders (daemon polls)
  notify.go                Pure finished/cleanup notification builders
internal/harness/        Harness adapters behind a single Adapter interface:
  harness.go               Adapter contract, LaunchSpec, kind registry (Get)
  instructions.go          Role instruction templates (orchestrator/worker/helper)
  pi.go                    pi adapter (flag-based --append-system-prompt)
  claude.go                claude (Claude Code) adapter (flag-based
                           --append-system-prompt, --dangerously-skip-permissions)
  codex.go                 codex (Codex CLI) adapter (file-based: instructions
                           injected via an auto-loaded AGENTS.md; subagents run
                           fully autonomously via
                           --dangerously-bypass-approvals-and-sandbox)
internal/daemon/         Per-session background daemon:
  daemon.go                Poll loop + GH/Tmuxer/Store interfaces (testable ticks)
  poll.go                  PR-state / CI / review / finished tick logic
  dock.go                  Dock auto-flip + layout maintenance
  adapters.go              Real gh/tmux/registry adapters (IO)
```

## The per-session daemon

`pr-agents start` best-effort spawns one long-lived daemon per orchestrator
session (`pr-agents daemon --session <id> --orchestrator-pane <paneId>`). It
replaces in-process timers and drives ALL cross-agent messaging through
`tmux send-keys`, so it is harness-agnostic. Each tick is wrapped so a
gh/git/tmux failure never kills the loop, and the daemon exits cleanly when not
inside tmux. It polls:

- **PR state → orchestrator** — `gh pr view` per pushed PR; on a merge/close
  transition it persists the status and tells the orchestrator to run
  `pr-agents cleanup`.
- **CI failures → worker** — new failing checks (deduped per head commit) hand
  the worker a fix task on its own pane.
- **Review comments → worker** — new inline comments/reviews/issue comments hand
  the worker an address-and-reply task; replies go via `pr-agents tool
  reply-review` (which never resolves the thread).
- **Worker finished → orchestrator** — purely registry-driven via the result
  recorded by `pr-agents tool report-result`.
- **Dock auto-flip** — keeps the newest live PR pane docked right of the
  orchestrator (opt out with `--no-dock`); the orchestrator pane is never
  broken or killed.
- **Session capture → registry** — for every live depth-1 worker with an empty
  `WorkerSessionRef`, the daemon resolves the harness-specific resumable session
  reference (claude uuid / pi session-file path / codex session id) via
  `harness.Get(kind).SessionRef` once the worker's session file appears on disk,
  and persists it (with its harness kind) to the entry. It is idempotent (set
  once), bounded (a miss retries next tick), and local-only (no network).

## Resume & revive

`pr-agents` is a CLI with no harness `session_start` hook, so resume is an
EXPLICIT verb the orchestrator runs on startup/resume:

```bash
pr-agents resume
```

It ports pi's `reviveDeadAgents` harness-agnostically:

- **Stable scope id** — `core.ResolveScopeID` derives the scope that owns the
  registry entries: the orchestrator's real harness session ref
  (`harness.Get(kind).SessionRef(mainRepoCwd, …)`) wins, then `PRA_SESSION`,
  then a random mint. Because the harness reopens the SAME session on resume,
  the ref — and thus the scope — is stable, so the orchestrator re-scopes to its
  existing entries; a fresh session yields a new scope. `start` and the
  registry-scoping verbs resolve the scope the same way. Pass `pr-agents start
  --fresh` to BYPASS this derivation and force a brand-new random scope id, so
  the session starts a clean scope that adopts no prior registry entries.
- **Revive selection** — `core.SelectRevivableAgents` picks the depth-1 workers
  that are non-terminal, whose tmux pane is DEAD, whose worktree still EXISTS,
  and which carry a usable `WorkerSessionRef`. It is pure (pane/worktree/session
  facts are injected). Depth-2 helpers are intentionally EXCLUDED: a revived
  worker re-spawns its own helpers if it needs them.
- **Relaunch + re-dock** — for each revivable entry, `resume` relaunches a new
  background tmux pane running `<launcher> <harness.Get(entry-harness)
  .BuildResumeArgs(spec, entry.WorkerSessionRef)>` in the worktree (reusing the
  dispatch/daemon tmux + dock primitives, ending the pane command with
  `; exec $SHELL` so it survives the harness exiting), updates the entry's
  `PaneID`, and re-docks the pane beside the orchestrator. It is fully guarded —
  one bad entry never aborts the rest.

Each worker entry records the harness it was DISPATCHED with (`PrEntry.Harness`),
so both session capture and revive pick the adapter via the ENTRY's own harness
(falling back to the daemon/orchestrator harness for legacy entries).
`pr-agents start` auto-runs the revive path when its scope already owns entries;
`pr-agents resume` is the always-safe-to-repeat explicit entry point.

The CLI uses only the Go stdlib `flag` package plus a small hand-rolled
subcommand dispatch — no third-party CLI framework — to keep dependencies
minimal. Human/orchestrator-facing verbs (`start`, `daemon`, `dispatch`, `resume`,
`list`, `peek`, `focus`, `send`, `stop`, `cleanup`, `select`, `version`) are
top-level; the agent-only worker-plumbing verbs are namespaced under a `tool`
parent command (`pr-agents tool context`, `tool set-pr-number`,
`tool mark-pushed`, `tool report-result`, `tool reply-review`). Running a moved
verb at the top level prints a hint pointing at its `pr-agents tool …` form.

## State & configuration

- **Shared registry** — dispatched PRs are tracked in
  `<git-common-dir>/.pr-agents/registry.json`, shared by the main checkout and
  every worktree, so each agent sees the same set of PRs. Writes are atomic
  (temp file + rename).
- **Project config** — the default stacking strategy (`github` | `graphite`)
  used when stacking dependent PRs is stored at `<repo-root>/.pr-agents.json`
  (harness-agnostic: at the repo root, not under a harness-specific dir).
- **Depth & identity** — a worker recovers its FULL identity (id, branch, base,
  mode, simplify, depth, session) purely from `cwd→registry` via
  `pr-agents tool context` (`core.ResolveContextFromCwd`), so nothing worker-targeted
  has to cross a sandbox boundary. Depth is likewise derived from the cwd
  (`core.DepthFromCwd`): 0 = orchestrator (the main repo, outside any
  worktree), 1 = PR subagent, 2 = helper. The only env vars carried across
  harness processes are the orchestrator-side `PRA_SESSION`, `PRA_HARNESS`, and
  `PRA_LAUNCHER`, set by `start` and read by `dispatch`/`daemon`.
