package cli

import (
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"syscall"

	"github.com/anonx3247/pr-agents/internal/core"
	"github.com/anonx3247/pr-agents/internal/harness"
	"github.com/anonx3247/pr-agents/internal/tmux"
)

// execProcess replaces the current process image. A package var so tests can
// stub it without actually exec'ing.
var execProcess = syscall.Exec

// tmuxEnvFlags renders an env map into sorted `-e KEY=VAL` tmux flags (used by
// new-session so the session carries the PRA_* contract). Pure.
func tmuxEnvFlags(env map[string]string) []string {
	keys := make([]string, 0, len(env))
	for k := range env {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	out := make([]string, 0, len(keys)*2)
	for _, k := range keys {
		out = append(out, "-e", k+"="+env[k])
	}
	return out
}

// buildTmuxSessionArgs builds the `tmux new-session` argv that re-execs command
// inside a fresh, uniquely-named session carrying env. Pure.
func buildTmuxSessionArgs(sessionName string, env map[string]string, command []string) []string {
	args := []string{"new-session", "-s", sessionName}
	args = append(args, tmuxEnvFlags(env)...)
	return append(args, command...)
}

// buildOrchestratorArgv assembles the argv to exec the orchestrator harness in
// the current pane: launcher tokens + adapter args + any extra passthrough
// args. Pure.
func buildOrchestratorArgv(launcher string, adapterArgs, extra []string) []string {
	argv := strings.Fields(launcher)
	argv = append(argv, adapterArgs...)
	return append(argv, extra...)
}

// stripFreshFlag removes any --fresh/-fresh token (with or without an =value)
// from args. It is used when re-execing `start` inside tmux: the OUTER process
// already minted the fresh scope id and carries it via PRA_SESSION, so the inner
// invocation must NOT mint a SECOND new id (which would mismatch the tmux
// session name). Pure.
func stripFreshFlag(args []string) []string {
	out := make([]string, 0, len(args))
	for _, a := range args {
		if a == "-fresh" || a == "--fresh" ||
			strings.HasPrefix(a, "-fresh=") || strings.HasPrefix(a, "--fresh=") {
			continue
		}
		out = append(out, a)
	}
	return out
}

// splitDoubleDash splits args at the first "--": before goes to flag parsing,
// after is passthrough to the harness. When no "--" is present, all args are
// flag args.
func splitDoubleDash(args []string) (flags, extra []string) {
	for i, a := range args {
		if a == "--" {
			return args[:i], args[i+1:]
		}
	}
	return args, nil
}

func runStart(args []string, stdout, stderr io.Writer) int {
	flagArgs, extra := splitDoubleDash(args)
	fs := flag.NewFlagSet("start", flag.ContinueOnError)
	fs.SetOutput(stderr)
	harnessKind := fs.String("harness", os.Getenv(core.EnvHarness), "Harness adapter: pi|claude|codex")
	launcher := fs.String("launcher", os.Getenv(core.EnvLauncher), "Launch-command prefix before the harness args")
	fresh := fs.Bool("fresh", false, "Mint a brand-new scope id instead of reusing the carried-over/derived one (start a clean scope that adopts no prior registry entries)")
	if err := fs.Parse(flagArgs); err != nil {
		return 2
	}
	if *harnessKind == "" {
		*harnessKind = "pi"
	}
	adapter, err := harness.Get(*harnessKind)
	if err != nil {
		fmt.Fprintf(stderr, "pr-agents start: %v\n", err)
		return 1
	}
	if *launcher == "" {
		*launcher = adapter.DefaultLauncher()
	}

	// Stable scope id across resume: prefer the orchestrator harness's own
	// resumable session ref for cwd (so resuming the same session re-scopes to
	// the same registry entries), then the PRA_SESSION env carried across the
	// tmux re-exec, then a random mint for a first-ever start. cwd is best-effort
	// here; an error leaves the resolver to fall through to env/fallback. With
	// --fresh the ref/env derivation is bypassed entirely so a brand-new id is
	// minted, starting a clean scope that re-adopts no prior entries.
	cwd, _ := os.Getwd()
	session := core.ResolveScopeID(scopeRefResolver(*harnessKind, cwd), os.Getenv(core.EnvSession), genID, *fresh)

	if !tmux.InsideTmux() {
		return startOutsideTmux(stderr, session, *harnessKind, *launcher)
	}
	return startInsideTmux(stdout, stderr, adapter, session, *harnessKind, *launcher, extra)
}

// startOutsideTmux creates a uniquely-named tmux session that re-execs this same
// `pr-agents start` invocation inside it, carrying the PRA_* contract via -e.
func startOutsideTmux(stderr io.Writer, session, harnessKind, launcher string) int {
	tmuxBin, err := exec.LookPath("tmux")
	if err != nil {
		fmt.Fprintf(stderr, "pr-agents start: tmux not found on PATH: %v\n", err)
		return 1
	}
	self, err := os.Executable()
	if err != nil {
		fmt.Fprintf(stderr, "pr-agents start: %v\n", err)
		return 1
	}
	// Re-exec the same invocation inside the session, minus --fresh: the fresh id
	// was already minted here and is carried via PRA_SESSION below.
	inner := append([]string{self}, stripFreshFlag(os.Args[1:])...)
	env := map[string]string{
		core.EnvSession:  session,
		core.EnvHarness:  harnessKind,
		core.EnvLauncher: launcher,
	}
	argv := append([]string{"tmux"}, buildTmuxSessionArgs("pra-"+session, env, inner)...)
	if err := execProcess(tmuxBin, argv, os.Environ()); err != nil {
		fmt.Fprintf(stderr, "pr-agents start: failed to launch tmux: %v\n", err)
		return 1
	}
	return 0
}

// startInsideTmux configures the session, installs the orchestrator
// instructions, best-effort starts the daemon, and execs the orchestrator
// harness in the current pane.
func startInsideTmux(stdout, stderr io.Writer, adapter harness.Adapter, session, harnessKind, launcher string, extra []string) int {
	tmux.TmuxSetup()

	cwd, err := os.Getwd()
	if err != nil {
		fmt.Fprintf(stderr, "pr-agents start: %v\n", err)
		return 1
	}

	instructions, err := harness.Instructions(harness.RoleOrchestrator, harness.InstructionData{})
	if err != nil {
		fmt.Fprintf(stderr, "pr-agents start: %v\n", err)
		return 1
	}

	spec := harness.LaunchSpec{Worktree: cwd}
	instructionsPath := ""
	if adapter.InstructionMode() == harness.InstructionFile {
		instructionsPath = filepath.Join(cwd, adapter.InstructionFileName())
		if err := os.WriteFile(instructionsPath, []byte(instructions), 0o644); err != nil {
			fmt.Fprintf(stderr, "pr-agents start: writing instructions: %v\n", err)
			return 1
		}
		// Keep the auto-loaded instruction file (e.g. AGENTS.md) untracked so the
		// orchestrator never commits it. Best-effort.
		core.EnsureExcluded(cwd, adapter.InstructionFileName())
	} else {
		spec.InstructionsText = instructions
	}

	// Carry the PRA_* contract to the orchestrator process so dispatch reads it.
	os.Setenv(core.EnvSession, session)
	os.Setenv(core.EnvHarness, harnessKind)
	os.Setenv(core.EnvLauncher, launcher)

	// Best-effort: start the per-session daemon detached. Failure or a no-op
	// (e.g. not inside tmux) must never block the orchestrator.
	startDaemon(session, harnessKind, orchestratorPane())

	// Auto-revive on a resume-start: when this scope already owns registry
	// entries, re-dock a live agent and relaunch any dead PR panes BEFORE handing
	// off to the harness. Guarded; a fresh scope (no entries) is a no-op.
	if all, err := core.LoadRegistry(cwd); err == nil {
		if len(core.EntriesForSession(all, session)) > 0 {
			reviveScope(all, cwd, session, harnessKind, launcher, orchestratorPane())
		}
	}

	argv := buildOrchestratorArgv(launcher, adapter.BuildArgs(spec, instructionsPath), extra)
	if len(argv) == 0 {
		fmt.Fprintln(stderr, "pr-agents start: empty launch command")
		return 1
	}
	bin, err := exec.LookPath(argv[0])
	if err != nil {
		fmt.Fprintf(stderr, "pr-agents start: launcher %q not found: %v\n", argv[0], err)
		return 1
	}
	if err := execProcess(bin, argv, os.Environ()); err != nil {
		fmt.Fprintf(stderr, "pr-agents start: failed to launch orchestrator: %v\n", err)
		return 1
	}
	return 0
}

// orchestratorPane resolves the orchestrator's OWN tmux pane id so the daemon
// can target it for cleanup/finished notifications. Prefers $TMUX_PANE (set by
// tmux for every pane), falling back to a display-message query. Returns ""
// when it can't be resolved (the daemon then skips orchestrator notifications).
func orchestratorPane() string {
	if p := os.Getenv("TMUX_PANE"); p != "" {
		return p
	}
	if p, ok := tmux.TryTmux("display-message", "-p", "#{pane_id}"); ok {
		return p
	}
	return ""
}

// startDaemon spawns `pr-agents daemon` detached, carrying the session id, the
// harness KIND (a selector for the session-capture store), and the
// orchestrator's pane id. It deliberately does NOT forward the launcher prefix:
// the daemon never spawns agents, so it has no use for a launch command and must
// not become a shell-injection / privilege-escalation surface. Best-effort: any
// error is swallowed and the daemon exits cleanly when there is nothing to do.
func startDaemon(session, harnessKind, orchPane string) {
	self, err := os.Executable()
	if err != nil {
		return
	}
	args := []string{"daemon", "--session", session, "--harness", harnessKind}
	if orchPane != "" {
		args = append(args, "--orchestrator-pane", orchPane)
	}
	cmd := exec.Command(self, args...)
	cmd.Stdout = nil
	cmd.Stderr = nil
	_ = cmd.Start()
	if cmd.Process != nil {
		_ = cmd.Process.Release()
	}
}
