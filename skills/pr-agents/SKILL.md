---
name: pr-agents
description: Orchestrate work as a series of small pull requests by shelling out to the harness-agnostic `pr-agents` CLI. Use this whenever you are asked to implement, build, refactor, or ship something and want each PR handled by its own dedicated worker subagent in an isolated git worktree + tmux pane. Covers starting a session, dispatching workers, monitoring/steering them (list/peek/send/stop/focus), resuming, stacking, and cleanup — all via real `pr-agents <verb>` invocations.
---

# pr-agents (CLI-driven PR orchestration)

`pr-agents` is a standalone Go CLI (plus a per-session background daemon) that
splits work into small pull requests and hands **each PR to its own dedicated
subagent** running in an isolated git worktree, laid out as a labelled **tmux
pane**. It is **harness-agnostic** — it drives `pi`, `claude`, or `codex` and
talks to every agent purely through `tmux send-keys`, so you orchestrate it by
shelling out to `pr-agents <verb>`. There is **no MCP server**; the CLI verbs
*are* the interface.

This skill is for the **orchestrator** (the top-level agent splitting work into
PRs). The worker side — how each dispatched subagent implements its single PR —
lives in the companion file [worker.md](worker.md).

## Prerequisites

- A `pr-agents` binary on `PATH` (`pr-agents version`), `git`, `tmux`, and `gh`
  (Graphite stacks also need `gt`).
- A repo with a clean working tree and a remote for PRs.
- You orchestrate from **inside a tmux session** so PR panes can open beside you.
  `pr-agents start` creates that session for you when run outside tmux.

## Identity model (read this first)

`pr-agents` keeps no per-agent env contract across the sandbox boundary. Instead:

- **Registry** — dispatched PRs are tracked in
  `<git-common-dir>/.pr-agents/registry.json`, shared by the main checkout and
  every worktree, so every agent sees the same set of PRs.
- **cwd → identity** — a worker recovers its FULL identity (id, branch, base,
  mode, simplify, depth, session) purely from its **current working directory**
  resolved against the registry, via `pr-agents tool context`. Nothing
  worker-targeted has to cross a sandbox boundary.
- **Session scope** — registry views are scoped to a stable session id derived
  from the orchestrator harness's own resumable session ref, so a resume
  re-scopes to the same entries. The only env vars carried across harness
  processes are the orchestrator-side `PRA_SESSION`, `PRA_HARNESS`, and
  `PRA_LAUNCHER` (set by `start`, read by `dispatch`/`daemon`).

## Step 0 — Start (or resume) a session

```bash
pr-agents start                       # default harness: pi
pr-agents start --harness claude      # or claude / codex
pr-agents start --launcher "asb run --profile dev --"   # sandboxed launch prefix
```

`start` launches the orchestrator harness in the current tmux pane (creating a
fresh `pra-<session>` tmux session first if you are outside tmux), best-effort
spawns the per-session **daemon**, and — if this scope already owns registry
entries — auto-revives any dead PR panes before handing off.

- `--harness pi|claude|codex` picks the adapter (default `pi`).
- `--launcher "<prefix>"` is the launch-command prefix prepended before the
  harness args for **every** pane it spawns. Use it for sandboxed/secret setups
  (e.g. `asb run … --`, `sx run --env .env --`). It is inherited by `dispatch`
  and revive, so the whole session launches the same way.
- Anything after a literal `--` is passed straight through to the harness.

On a later resume, re-dock the live agent and relaunch dead PR panes explicitly:

```bash
pr-agents resume
```

`resume` is always safe to repeat and is a no-op when there is nothing to revive.

## Step 1 — Decompose into PRs

Break the work into the smallest set of independently reviewable PRs (prefer many
small PRs over one big one). For each PR decide its **mode**:

- `independent` — branches off the base branch, reviewable on its own (default).
- `stack` — a manual stacked GitHub PR whose base is the previous PR's branch.
- `graphite` — a Graphite (`gt`) stack.

See the companion [pr-stacks.md](pr-stacks.md) for how to build and merge stacks.

## Step 2 — Dispatch one worker per PR

```bash
pr-agents dispatch \
  --name "Add token-bucket rate limiter" \
  --task "Self-contained instructions: what to build, constraints, files/areas, how to verify (tests/build/lint)." \
  --mode independent \
  --simplify
```

Key flags (see `pr-agents dispatch -h`):

| Flag | Meaning |
|---|---|
| `--name` | Short PR title (**required**). |
| `--task` | Full, self-contained worker instructions (**required**, or `--task-file`). |
| `--task-file <path>` | Read the task from a file instead of `--task` (good for long briefs). |
| `--mode independent\|stack\|graphite` | Stacking mode (default `independent`). |
| `--base <branch>` | Base branch (defaults to the stack ref or repo default). |
| `--stack-on <id\|branch>` | The prior PR to stack on (for `stack`/`graphite`). |
| `--branch <name>` | Override the auto-generated working branch. |
| `--simplify` | Ask the worker to simplify its diff before opening the PR. |
| `--harness`, `--launcher` | Override the inherited harness/launcher for this worker. |

Write the `--task` as if briefing a competent engineer who cannot see this
conversation: goal, constraints, files/areas involved, and **how to verify**.
`dispatch` creates the worktree + branch, opens a labelled tmux pane running the
worker, and records the entry in the registry so `pr-agents tool context`
resolves immediately inside the worker. For a stack, dispatch **in order** and
set `--stack-on` to the previous PR (or omit it to stack on the most recent
depth-1 entry).

> `dispatch` only works **inside tmux** and refuses to nest beyond depth 2: a
> worker (depth 1) may dispatch helpers, but a helper (depth 2) cannot dispatch
> further.

## Step 3 — Monitor and steer

```bash
pr-agents list                 # table of every PR: number, name, branch, mode, status, pane
pr-agents list --json          # machine-readable (id, prNumber, name, branch, mode, status, live)
pr-agents peek <id> --lines 80 # read a worker's recent pane output without interrupting it
pr-agents send <id> "message"  # type a follow-up / answer into a running worker
pr-agents focus <id>           # move your tmux focus to a worker's pane
pr-agents stop <id>            # interrupt (Escape) a worker going the wrong way
pr-agents stop <id> --kill     # kill its pane (worktree + branch kept for inspection)
```

`<id>` accepts the registry id, the PR number, or a branch/name ref (resolved
against the current session scope). When a worker asks a question in its pane,
answer it with `pr-agents send`. To redirect a worker: `pr-agents stop <id>`
(interrupt), then `pr-agents send <id> "<new direction>"`.

The **daemon** drives all the background messaging automatically: PR
merge/close → tells you to run cleanup; CI failure → hands the worker a fix task;
new review comments → hands the worker an address-and-reply task; worker finished
→ notifies you. You don't poll any of this yourself.

## Step 4 — Clean up as PRs land

```bash
pr-agents cleanup --dry         # preview removals
pr-agents cleanup               # remove merged/closed worktrees, branches, panes
```

Run cleanup from the **main repo** (as the orchestrator), not inside a worktree.
It uses `gh pr view` for PR state (falling back to "branch merged into default"
when `gh` is unavailable), drops finished entries from the registry, and prunes
orphaned `<repo>/.worktrees/` directories. See [cleanup.md](cleanup.md).

## Rules

- Delegate all code changes via `dispatch` / `send` — keep each PR small and
  single-purpose.
- Dispatch and merge stacks **bottom-up**; never stack on an un-dispatched
  branch.
- You may run for a very long time as the single persistent orchestrator;
  worktrees keep each PR's work isolated.
