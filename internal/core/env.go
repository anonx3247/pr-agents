// Package core holds the harness-agnostic pure logic of pr-agents: the PR
// registry, per-project config, string/format helpers, thin git/worktree
// wrappers, and the pure PR-state classification used by the pollers. None of
// it depends on a particular agent harness (pi/claude/codex) or on tmux.
package core

import (
	"encoding/json"
	"os"
	"strconv"
)

// Environment-variable contract carried across harness processes.
//
// These generalize the pi-only PI_PR_* names to harness-agnostic PRA_* names.
// The orchestrator (depth 0) sets them when launching a PR subagent (depth 1),
// which in turn sets PRA_DEPTH=2 for any helper it spawns.
const (
	// EnvDepth is the nesting depth: 0 = orchestrator, 1 = PR subagent,
	// 2 = helper. Helpers cannot dispatch further.
	EnvDepth = "PRA_DEPTH"
	// EnvSession is the id of the orchestrator session that owns an entry, so
	// concurrent orchestrators sharing one repo only see their own subagents.
	EnvSession = "PRA_SESSION"
	// EnvID is the registry id of the entry a subagent is working on.
	EnvID = "PRA_ID"
	// EnvMode is the stacking mode: independent | stack | graphite | helper.
	EnvMode = "PRA_MODE"
	// EnvBase is the base branch the PR targets.
	EnvBase = "PRA_BASE"
	// EnvBranch is the working branch for the PR.
	EnvBranch = "PRA_BRANCH"
	// EnvName is the human-readable PR name.
	EnvName = "PRA_NAME"
	// EnvSimplify is "1" when the worker should simplify its diff before
	// opening the PR, "0" otherwise.
	EnvSimplify = "PRA_SIMPLIFY"
	// EnvHarness selects the agent harness adapter (e.g. pi | claude | codex).
	EnvHarness = "PRA_HARNESS"
	// EnvLauncher is the launch-command PREFIX placed before the harness's own
	// task + flags (e.g. "pi", or a sandbox wrapper like "isara codex run").
	// pr-agents appends the adapter's BuildArgs transparently after it.
	EnvLauncher = "PRA_LAUNCHER"
	// EnvHarnessArgs carries the operator's harness passthrough flags — the args
	// after `--` on `pr-agents start` (e.g. a YOLO/--full-auto override, a model
	// choice) — JSON-encoded so they survive intact across processes. dispatch
	// replays them onto every worker pane so subagents run with the same harness
	// options the operator chose for the main agent.
	EnvHarnessArgs = "PRA_HARNESS_ARGS"
)

// EncodeHarnessArgs serializes the operator's harness passthrough args to a
// compact JSON string for transport via EnvHarnessArgs. Empty/nil encodes to ""
// so an unset env var and "no extra args" are indistinguishable. Pure.
func EncodeHarnessArgs(args []string) string {
	if len(args) == 0 {
		return ""
	}
	b, err := json.Marshal(args)
	if err != nil {
		return ""
	}
	return string(b)
}

// DecodeHarnessArgs is the inverse of EncodeHarnessArgs: it parses the JSON
// string back into a slice. Empty or malformed input yields nil so callers can
// append the result unconditionally. Pure.
func DecodeHarnessArgs(raw string) []string {
	if raw == "" {
		return nil
	}
	var args []string
	if err := json.Unmarshal([]byte(raw), &args); err != nil {
		return nil
	}
	return args
}

// Depth reads PRA_DEPTH from the environment and returns it as an int, falling
// back to 0 when it is unset or not a valid integer.
func Depth() int {
	return depthFrom(os.Getenv(EnvDepth))
}

// depthFrom parses a raw PRA_DEPTH value, returning 0 on empty or invalid input.
// Kept separate so it can be unit-tested without touching the process env.
func depthFrom(raw string) int {
	if raw == "" {
		return 0
	}
	n, err := strconv.Atoi(raw)
	if err != nil {
		return 0
	}
	return n
}
