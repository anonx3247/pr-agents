# pr-agents

Harness-agnostic PR orchestration. A standalone Go CLI + per-session daemon that
splits work into small pull requests and hands **each PR to its own dedicated
subagent** running in an isolated git worktree, laid out as a labelled **tmux
pane**. Works with multiple agent harnesses (pi, claude, codex) and integrates
with `asb`/`sx` for sandboxing and secrets, polling GitHub / Graphite for PR
state, CI, and reviews.

Status: early scaffolding. This PR lays the non-UI foundation — the Go module,
core domain (registry, config, string/git/PR-state helpers) and CLI skeleton.
Later PRs add the tmux layer, real CLI verbs, the daemon, and harness adapters.

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
internal/cli/            Root command, stdlib-flag dispatch, version + stub verbs
internal/core/           Harness-agnostic pure logic:
  env.go                   PRA_* env-var contract + Depth()
  strings.go               Slugify, Shq, BuildEnv, CapTail, PaneTitle, WindowName
  git.go                   RepoRoot, GitCommonDir, DefaultBranch, UniqueBranch,
                           WorktreesDirFrom, EnsureWorktreesIgnored
  registry.go              PrEntry + shared registry (atomic load/save, UpdateEntry,
                           EntriesForSession, FindEntry)
  config.go                Per-project config (stacking strategy)
  prstate.go               Pure PR-state classification + diff parse helpers
```

The CLI uses only the Go stdlib `flag` package plus a small hand-rolled
subcommand dispatch — no third-party CLI framework — to keep dependencies
minimal.

## State & configuration

- **Shared registry** — dispatched PRs are tracked in
  `<git-common-dir>/pr-agents/registry.json`, shared by the main checkout and
  every worktree, so each agent sees the same set of PRs. Writes are atomic
  (temp file + rename).
- **Project config** — the default stacking strategy (`github` | `graphite`)
  used when stacking dependent PRs is stored at `<repo-root>/.pi/pr-agents.json`.
- **Depth & environment** — nesting depth and dispatch parameters are carried
  across harness processes via `PRA_*` environment variables (`PRA_DEPTH`,
  `PRA_SESSION`, `PRA_ID`, `PRA_MODE`, `PRA_BASE`, `PRA_BRANCH`, `PRA_NAME`,
  `PRA_SIMPLIFY`, `PRA_HARNESS`). Depth 0 = orchestrator, 1 = PR subagent,
  2 = helper.
