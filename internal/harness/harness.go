// Package harness abstracts the differences between agent harnesses (pi,
// claude, codex) behind a single Adapter interface. pr-agents drives every
// harness the same way: it builds a LaunchSpec, asks the adapter for the argv
// to append after a configurable launcher prefix, and renders harness-agnostic
// instruction templates for the orchestrator/worker/helper roles.
//
// The launcher prefix is configurable so a sandbox wrapper (e.g.
// "isara codex run" or "asb -- codex") can be slotted in by overriding ONLY
// the prefix; the adapter's BuildArgs are appended transparently after it.
// pr-agents itself never calls a sandbox tool.
package harness

import (
	"fmt"
	"sort"
	"strings"
	"time"
)

// Role is the part an agent plays in the PR fleet. Instruction templates are
// selected by role.
type Role string

const (
	// RoleOrchestrator is the depth-0 main agent: it never edits code, it
	// splits work into PRs and dispatches one worker per PR.
	RoleOrchestrator Role = "orchestrator"
	// RoleWorker is a depth-1 subagent owning exactly one PR in its worktree.
	RoleWorker Role = "worker"
	// RoleHelper is a depth-2 subagent doing a focused sub-task in the parent
	// worktree; it cannot dispatch further.
	RoleHelper Role = "helper"
)

// InstructionMode reports how an adapter injects role instructions.
type InstructionMode string

const (
	// InstructionFlag passes the instructions text via a CLI flag (pi, claude).
	InstructionFlag InstructionMode = "flag"
	// InstructionFile writes the instructions to a file in the worktree that the
	// harness auto-loads (codex: AGENTS.md, claude: optional CLAUDE.md).
	InstructionFile InstructionMode = "file"
)

// LaunchSpec carries everything an adapter needs to build a launch argv. Env is
// best-effort convenience only (a sandbox wrapper may filter it); worker
// identity is recovered from cwd→registry, not from env.
type LaunchSpec struct {
	Task             string
	InstructionsText string
	PrName           string
	Worktree         string
	Env              map[string]string
}

// Adapter is the contract every harness implements. BuildArgs must be PURE
// (no IO, deterministic) so it can be table-tested by asserting exact argv.
type Adapter interface {
	// Kind is the adapter id: "pi" | "claude" | "codex".
	Kind() string
	// DefaultLauncher is the bare harness binary used when no --launcher is
	// given (e.g. "pi").
	DefaultLauncher() string
	// BuildArgs returns the args appended AFTER the launcher prefix: the task
	// positional plus harness flags. instructionsPath is "" when instructions
	// are injected via a flag rather than written to a file.
	BuildArgs(spec LaunchSpec, instructionsPath string) []string
	// InstructionMode reports whether instructions go via a flag or a file.
	InstructionMode() InstructionMode
	// InstructionFileName is the file an InstructionFile adapter expects (e.g.
	// "AGENTS.md"); "" for flag-based adapters.
	InstructionFileName() string
	// SessionRef locates the resumable session reference for a harness session
	// running in cwd, by scanning that harness's on-disk session store. since
	// lets callers ignore sessions created before the worker was dispatched
	// (e.g. the registry entry's CreatedAt). It returns ok=false when nothing
	// matches or the store dir is absent; it never panics on a missing store.
	// The exact ref shape is harness-specific (uuid for claude/codex, absolute
	// session file path for pi).
	SessionRef(cwd string, since time.Time) (ref string, ok bool)
}

// registry maps adapter kind → Adapter. Populated by each adapter's init.
var registry = map[string]Adapter{}

// register adds an adapter to the lookup table. Called from adapter files.
func register(a Adapter) {
	registry[a.Kind()] = a
}

// Get returns the adapter for kind, or an error listing the known kinds.
func Get(kind string) (Adapter, error) {
	if a, ok := registry[kind]; ok {
		return a, nil
	}
	return nil, fmt.Errorf("unknown harness %q (known: %s)", kind, KnownKinds())
}

// KnownKinds returns the registered adapter kinds, sorted, comma-separated.
func KnownKinds() string {
	kinds := make([]string, 0, len(registry))
	for k := range registry {
		kinds = append(kinds, k)
	}
	sort.Strings(kinds)
	return strings.Join(kinds, ", ")
}
