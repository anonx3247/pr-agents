#!/usr/bin/env bash
# Example launcher: run the codex harness under the agent-sandbox (asb).
#
# Same model as examples/launchers/asb-claude.sh: pr-agents only ever sees a
# launcher PREFIX and appends the harness args after it. Override the prefix to
# slot a sandbox in; pr-agents itself never invokes asb.
#
#   pr-agents start --harness codex \
#     --launcher "$(pwd)/examples/launchers/asb-codex.sh"
#
# pr-agents appends the codex adapter's BuildArgs after this command, e.g.:
#
#   asb-codex.sh <task> --dangerously-bypass-approvals-and-sandbox
#
# Two codex-specific wrinkles this wrapper must respect:
#
#   1. Instruction injection is FILE-based for codex. pr-agents writes an
#      AGENTS.md into the worktree (and git-excludes it) and codex auto-loads it
#      from its working dir. So the sandbox profile MUST keep the worktree
#      readable/writable (the project root normally is) for instructions and
#      commits to work. No argv flag carries the instructions.
#
#   2. --dangerously-bypass-approvals-and-sandbox disables codex's OWN in-process
#      sandbox. That is intentional: the REAL trust boundary is this asb wrapper,
#      not codex's approval prompts, so the worker can make progress unattended
#      inside an already-isolated worktree.
#
# `asb --` runs the rest of the argv inside the sandbox. Pick a profile that
# still allows network (codex needs the model API) and project writes, e.g.
# `--profile git`.
exec asb --profile git -- codex "$@"
