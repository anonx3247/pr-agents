package harness

import (
	"path/filepath"
	"time"
)

// piAdapter drives the pi harness. Instructions are injected via the
// --append-system-prompt flag; the bare launcher is "pi".
type piAdapter struct{}

func init() { register(piAdapter{}) }

func (piAdapter) Kind() string { return "pi" }

func (piAdapter) DefaultLauncher() string { return "pi" }

func (piAdapter) InstructionMode() InstructionMode { return InstructionFlag }

func (piAdapter) InstructionFileName() string { return "" }

// BuildArgs mirrors the pi worker command:
//
//	<task> -a --append-system-prompt <instructionsText> --name "PR: <prName>"
//
// "-a" trusts project-local files for this run (a dispatched worktree of a repo
// the user already chose to work in). The instructions are passed inline via
// the flag, so instructionsPath is ignored. An empty Task is omitted so the
// orchestrator launches interactively (no seed message). Pure: no IO.
func (piAdapter) BuildArgs(spec LaunchSpec, _ string) []string {
	args := []string{}
	if spec.Task != "" {
		args = append(args, spec.Task)
	}
	args = append(args, "-a")
	if spec.InstructionsText != "" {
		args = append(args, "--append-system-prompt", spec.InstructionsText)
	}
	if spec.PrName != "" {
		args = append(args, "--name", "PR: "+spec.PrName)
	}
	return args
}

// SessionRef locates the newest pi session file for cwd. pi stores sessions at
// ~/.pi/agent/sessions/<ENC>/<file>, where <ENC> = encodePiSessionDir(cwd). The
// returned ref is the ABSOLUTE session file path (pi's --session accepts a
// path). Picks the newest file with mtime >= since; ok=false when the dir is
// absent or empty.
func (piAdapter) SessionRef(cwd string, since time.Time) (string, bool) {
	home, err := sessionStoreHome()
	if err != nil {
		return "", false
	}
	dir := filepath.Join(home, ".pi", "agent", "sessions", encodePiSessionDir(cwd))
	return newestFileInDir(dir, since, nil)
}

// BuildResumeArgs relaunches pi resuming sessionRef in an interactive pane.
// Verified against `pi --help`: `--session <path|id>` uses a specific session
// file or id. It mirrors BuildArgs' -a trust flag and --name display name (when
// set), but DROPS the task positional and --append-system-prompt: a resumed
// session restores pi's own saved context, so re-injecting instructions would
// duplicate them. Pure: no IO.
func (piAdapter) BuildResumeArgs(spec LaunchSpec, sessionRef string) []string {
	args := []string{"--session", sessionRef, "-a"}
	if spec.PrName != "" {
		args = append(args, "--name", "PR: "+spec.PrName)
	}
	return args
}
