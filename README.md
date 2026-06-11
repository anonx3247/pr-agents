# pr-agents

Harness-agnostic PR orchestration. A standalone Go CLI + per-session daemon that
splits work into small pull requests and hands **each PR to its own dedicated
subagent** running in an isolated git worktree, laid out as a labelled **tmux
pane**. Works with multiple agent harnesses (pi, claude, codex) and integrates
with `asb`/`sx` for sandboxing and secrets, polling GitHub / Graphite for PR
state, CI, and reviews.

Status: early scaffolding. The Go module, core domain (registry, config,
string/git/PR-state helpers), tmux layer, CLI verbs, harness adapters, and the
per-session daemon are in place, with the pi and claude harness adapters. Later
PRs add the interactive `select` picker and the codex adapter.

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
  env.go                   PRA_* env-var contract + Depth()
  strings.go               Slugify, Shq, BuildEnv, CapTail, PaneTitle, WindowName
  git.go                   RepoRoot, GitCommonDir, DefaultBranch, UniqueBranch,
                           WorktreesDirFrom, EnsureWorktreesIgnored
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
  the worker an address-and-reply task; replies go via `pr-agents reply-review`
  (which never resolves the thread).
- **Worker finished → orchestrator** — purely registry-driven via the result
  recorded by `pr-agents report-result`.
- **Dock auto-flip** — keeps the newest live PR pane docked right of the
  orchestrator (opt out with `--no-dock`); the orchestrator pane is never
  broken or killed.

The CLI uses only the Go stdlib `flag` package plus a small hand-rolled
subcommand dispatch — no third-party CLI framework — to keep dependencies
minimal.

## State & configuration

- **Shared registry** — dispatched PRs are tracked in
  `<git-common-dir>/.pr-agents/registry.json`, shared by the main checkout and
  every worktree, so each agent sees the same set of PRs. Writes are atomic
  (temp file + rename).
- **Project config** — the default stacking strategy (`github` | `graphite`)
  used when stacking dependent PRs is stored at `<repo-root>/.pr-agents.json`
  (harness-agnostic: at the repo root, not under a harness-specific dir).
- **Depth & environment** — nesting depth and dispatch parameters are carried
  across harness processes via `PRA_*` environment variables (`PRA_DEPTH`,
  `PRA_SESSION`, `PRA_ID`, `PRA_MODE`, `PRA_BASE`, `PRA_BRANCH`, `PRA_NAME`,
  `PRA_SIMPLIFY`, `PRA_HARNESS`). Depth 0 = orchestrator, 1 = PR subagent,
  2 = helper.
