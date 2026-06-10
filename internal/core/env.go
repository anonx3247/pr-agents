// Package core holds the harness-agnostic pure logic of pr-agents: the PR
// registry, per-project config, string/format helpers, thin git/worktree
// wrappers, and the pure PR-state classification used by the pollers. None of
// it depends on a particular agent harness (pi/claude/codex) or on tmux.
package core

// Orchestrator-side environment-variable contract.
//
// These are set in the orchestrator process (the main repo, depth 0) by
// `start` and read by `dispatch`/`daemon`. They never need to cross a sandbox
// boundary: a worker recovers its full identity (id, branch, base, mode,
// simplify, depth, session) from cwd→registry via ResolveContextFromCwd, so
// there is deliberately NO worker-targeted env contract.
const (
	// EnvSession is the id of the orchestrator session that owns an entry, so
	// concurrent orchestrators sharing one repo only see their own subagents.
	EnvSession = "PRA_SESSION"
	// EnvHarness selects the agent harness adapter (e.g. pi | claude | codex).
	EnvHarness = "PRA_HARNESS"
	// EnvLauncher is the launch-command PREFIX placed before the harness's own
	// task + flags (e.g. "pi", or a sandbox wrapper like "isara codex run").
	// pr-agents appends the adapter's BuildArgs transparently after it.
	EnvLauncher = "PRA_LAUNCHER"
)
