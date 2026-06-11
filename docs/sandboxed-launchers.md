# Running pr-agents under a sandbox

pr-agents is **sandbox-agnostic by design**. It never invokes `asb`, `sx`,
`isara`, or any other OS-level sandbox itself. Instead, the command used to spawn
each harness process is a configurable PREFIX — the **launcher** — and a sandbox
is layered on by overriding ONLY that prefix. Everything else (worktree
creation, the registry, tmux panes, the daemon) is unchanged whether or not a
sandbox is in play.

This doc explains the launcher model, why worker identity does not depend on
environment variables, how each harness receives its instructions, and gives
two copy-paste examples.

## The launcher-prefix model

When pr-agents launches a harness — the orchestrator in `pr-agents start`, or a
worker in `pr-agents dispatch` — it builds the argv as:

```
<launcher tokens>  <harness BuildArgs: task positional + harness flags>
```

The launcher is split on whitespace into command words, and the harness
adapter's `BuildArgs` are appended transparently after it. The default launcher
is just the bare harness binary (`pi`, `claude`, or `codex`), so out of the box
nothing is sandboxed. Set the launcher to a sandbox wrapper and every spawn goes
through it instead:

```bash
# bare (default): pi <task> -a --append-system-prompt <instructions> ...
pr-agents start --harness pi

# sandboxed: isara pi run -- <task> -a --append-system-prompt <instructions> ...
pr-agents start --harness pi --launcher "isara pi run --"
```

You configure the launcher in one of two equivalent ways:

- Per command with the `--launcher` flag on `start` / `dispatch`.
- Once via the `PRA_LAUNCHER` environment variable (read by `start` and
  forwarded to `dispatch`/the daemon as the orchestrator-side default). Pair it
  with `PRA_HARNESS` to pick the adapter.

```bash
export PRA_HARNESS=claude
export PRA_LAUNCHER="isara claude run --"
pr-agents start
```

### Every pane is re-wrapped

A fleet is not one sandboxed process with sandboxed children. The orchestrator
runs through the launcher, and then **each dispatched worker is its own separate
launch through that same prefix** (`pr-agents dispatch` re-applies the launcher
per pane). So a fresh sandbox is re-imposed on every worker and helper rather
than inherited from the parent — the launcher prefix, not process ancestry, is
the trust boundary.

The one thing that is deliberately NOT wrapped is the per-session daemon:
`pr-agents start` spawns `pr-agents daemon` directly, without the launcher,
because the daemon never spawns agents — it only polls GitHub/Graphite and
drives tmux. Keeping the launcher away from it avoids turning a launch-command
string into a privilege-escalation surface.

## Why identity comes from cwd, not env

OS-level sandboxes **filter environment variables across the launch boundary**.
A variable you set outside the sandbox may simply not be present inside it. If a
worker had to learn its branch, base, mode, or depth from an env var, sandboxing
would silently break it.

pr-agents sidesteps this entirely: a worker recovers its FULL identity from
`cwd → registry`. Each dispatched PR is recorded in the shared registry
(`<git-common-dir>/.pr-agents/registry.json`) keyed by its worktree path, and a
worker resolves its own entry by matching its current directory against that
registry (`core.ResolveContextFromCwd`). It is surfaced by the verb:

```bash
pr-agents tool context        # human-readable identity for the cwd
pr-agents tool context --json # machine-readable
```

Depth is derived the same way (`core.DepthFromCwd`): 0 = orchestrator (the main
repo, outside any worktree), 1 = PR subagent, 2 = helper. This is exactly why
the worker-targeted `PRA_*` identity vars were dropped — they would not survive
a sandbox boundary anyway.

The only env vars pr-agents carries across harness processes are the
**orchestrator-side** contract — `PRA_SESSION`, `PRA_HARNESS`, `PRA_LAUNCHER` —
set by `start` and read by `dispatch`/`daemon`. These live on the orchestrator
side of the boundary and are not required for a worker to function; a worker that
sees none of them still resolves its identity from the cwd.

The practical consequence for a launcher wrapper: it can be trivial. It does not
need to thread any pr-agents state through the sandbox. It only has to run the
harness inside the sandbox with the args pr-agents appended.

## Instruction injection per harness

Role instructions (orchestrator / worker / helper) reach the harness in one of
two ways, depending on the adapter. This matters when choosing a sandbox profile
because the FILE mode needs the worktree to stay writable.

| Harness  | Mode | How instructions arrive                                                                 |
| -------- | ---- | --------------------------------------------------------------------------------------- |
| `pi`     | flag | `--append-system-prompt <text>` appended to argv (inline; nothing written to disk).     |
| `claude` | flag | `--append-system-prompt <text>` appended to argv (inline; nothing written to disk).     |
| `codex`  | file | An `AGENTS.md` is written into the worktree (and git-excluded) and codex auto-loads it.  |

For the two flag-based harnesses the instructions travel inside the argv, so they
cross the launch boundary as ordinary arguments — no filesystem access required.
For codex, pr-agents writes `AGENTS.md` into the worktree before launching and
relies on codex picking it up from its working directory, so **the sandbox
profile must keep the worktree readable and writable** (a project-scoped profile
normally does). The same applies to commits and edits in any harness: the
worktree must be writable.

Note also that the worker adapters run the harness in its autonomous mode
(`pi -a`, `claude --dangerously-skip-permissions`,
`codex --dangerously-bypass-approvals-and-sandbox`). These only disable the
harness's OWN in-pane approval prompts / in-process sandbox inside an already
isolated worktree. The real isolation is the launcher's OS-level sandbox, not the
harness's approval flow — which is why bypassing the latter is safe here.

## Example: claude under isara

```bash
pr-agents start --harness claude --launcher "isara claude run --"
```

This expands per spawn to:

```
isara claude run -- <task> --append-system-prompt <instructions> --dangerously-skip-permissions
```

A small wrapper script is often cleaner than an inline prefix, and lets you pin
flags/profiles in one place. See
[`examples/launchers/isara-claude.sh`](../examples/launchers/isara-claude.sh):

```bash
pr-agents start --harness claude \
  --launcher "$(pwd)/examples/launchers/isara-claude.sh"
```

## Example: codex under asb

```bash
pr-agents start --harness codex --launcher "asb --profile git -- codex"
```

This expands per spawn to:

```
asb --profile git -- codex <task> --dangerously-bypass-approvals-and-sandbox
```

Pick a profile that still allows the model API network and project writes
(`asb --profile git` does both); a `sealed`/`locked` profile would block the
model call and codex's `AGENTS.md`/commit writes. See
[`examples/launchers/asb-codex.sh`](../examples/launchers/asb-codex.sh):

```bash
pr-agents start --harness codex \
  --launcher "$(pwd)/examples/launchers/asb-codex.sh"
```

For secrets that the sandbox walls off (e.g. a token in `.env`), inject them
into a single command with `sx` rather than reading the file — pr-agents stays
out of this too; it is a property of your launcher/sandbox, not of pr-agents.
