# pr-agents

Harness-agnostic PR orchestration. A standalone Go CLI + per-session daemon that
splits work into small pull requests and hands **each PR to its own dedicated
subagent** running in an isolated git worktree, laid out as a labelled **tmux
pane**. Works with multiple agent harnesses (pi, claude, codex) and integrates
with `asb`/`sx` for sandboxing and secrets, polling GitHub / Graphite for PR
state, CI, and reviews.

Status: early scaffolding. See PRs for incremental build-out.
