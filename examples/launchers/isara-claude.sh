#!/usr/bin/env bash
# Example launcher: run the claude harness under the isara OS-level sandbox.
#
# pr-agents NEVER calls a sandbox itself. Instead it treats the launcher as an
# opaque command PREFIX and appends the harness's own args (task + flags) after
# it. To sandbox a fleet, you override ONLY that prefix — every orchestrator and
# every dispatched worker pane is then launched through this same wrapper, so a
# fresh sandbox is re-imposed per pane.
#
# Wire it in:
#
#   pr-agents start --harness claude \
#     --launcher "$(pwd)/examples/launchers/isara-claude.sh"
#
# pr-agents appends the claude adapter's BuildArgs after this command, e.g.:
#
#   isara-claude.sh <task> --append-system-prompt <instructions> \
#     --dangerously-skip-permissions
#
# so this script must forward "$@" unchanged to claude under the sandbox.
#
# Identity note: the sandbox boundary FILTERS environment variables, so workers
# do NOT rely on env to learn who they are. A worker recovers its full identity
# (id, branch, base, mode, depth, session) from cwd -> registry via
# `pr-agents tool context`. That keeps this wrapper trivial: it only has to run
# the harness in the sandbox; it does not have to thread any PRA_* state through.
exec isara claude run -- "$@"
