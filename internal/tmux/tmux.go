// Package tmux is a thin, mostly-pure layer over the `tmux` CLI used to manage
// PR-agent panes/windows. The command-argument BUILDERS (OpenWindowArgs,
// OpenPaneArgs, SendArgs, …) are pure and unit-tested; the runners (Tmux,
// TryTmux) and the convenience verbs that call them perform the actual exec.
package tmux

import (
	"context"
	"os"
	"os/exec"
	"sort"
	"strconv"
	"strings"
	"time"
)

// tmuxTimeout bounds every tmux invocation so a hung tmux never blocks the CLI.
const tmuxTimeout = 10 * time.Second

// InsideTmux reports whether the current process is running inside a tmux
// session (the TMUX env var is set by tmux for every pane).
func InsideTmux() bool {
	return os.Getenv("TMUX") != ""
}

// Tmux runs `tmux args...` (bounded by tmuxTimeout) and returns trimmed stdout,
// or an error.
func Tmux(args ...string) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), tmuxTimeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, "tmux", args...)
	out, err := cmd.Output()
	return strings.TrimSpace(string(out)), err
}

// TryTmux runs tmux and returns trimmed stdout, or "" with ok=false on any
// error (mirrors the TS tryTmux: best-effort, never throws).
func TryTmux(args ...string) (string, bool) {
	out, err := Tmux(args...)
	if err != nil {
		return "", false
	}
	return out, true
}

// TmuxSetup configures the session for PR-agent panes: pane titles on the
// border (so panes can be labelled with their PR number/name) and mouse
// support (so a user can click a pane to focus it). Best-effort.
func TmuxSetup() {
	TryTmux("set", "-g", "pane-border-status", "top")
	TryTmux("set", "-g", "pane-border-format", " #{pane_title} ")
	TryTmux("set", "-g", "mouse", "on")
}

// envArgs renders an env map into repeated `-e KEY=VAL` tmux flags, emitted in
// sorted key order so the argv is deterministic (and unit-testable).
func envArgs(env map[string]string) []string {
	if len(env) == 0 {
		return nil
	}
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

// OpenWindowArgs builds the argv for launching command in its OWN background
// tmux window (`-d` keeps focus on the orchestrator). env (if non-empty) is
// injected via repeated `-e KEY=VAL` flags so the launched process inherits the
// PRA_* contract in its pane. Pure.
func OpenWindowArgs(cwd, command, name string, env map[string]string) []string {
	args := []string{"new-window", "-d", "-P", "-F", "#{pane_id}", "-c", cwd, "-n", name}
	args = append(args, envArgs(env)...)
	return append(args, command)
}

// OpenPaneArgs builds the argv for splitting a new pane (right half) running
// command in cwd. env is injected via `-e KEY=VAL` flags. Pure.
func OpenPaneArgs(cwd, command string, env map[string]string) []string {
	args := []string{"split-window", "-h", "-d", "-P", "-F", "#{pane_id}", "-c", cwd}
	args = append(args, envArgs(env)...)
	return append(args, command)
}

// CaptureArgs builds the argv for capturing the recent visible output of a
// pane: the last `lines` rows (clamped to >= 0). Pure.
func CaptureArgs(paneID string, lines int) []string {
	if lines < 0 {
		lines = 0
	}
	return []string{"capture-pane", "-p", "-t", paneID, "-S", "-" + strconv.Itoa(lines)}
}

// SendTextArgs / SendEnterArgs build the two-step send-keys argv: first the
// literal message (`-l --`), then a separate Enter so the message is submitted.
// Pure.
func SendTextArgs(paneID, message string) []string {
	return []string{"send-keys", "-t", paneID, "-l", "--", message}
}

func SendEnterArgs(paneID string) []string {
	return []string{"send-keys", "-t", paneID, "Enter"}
}

// SetPaneTitleArgs builds the argv for setting a pane's title. Pure.
func SetPaneTitleArgs(paneID, title string) []string {
	return []string{"select-pane", "-t", paneID, "-T", title}
}

// JoinPaneArgs builds the argv for joining srcPane into targetPane's window as a
// new pane to the RIGHT of target (`-h`). Used by the daemon to dock the active
// PR-agent pane beside the orchestrator. Pure.
func JoinPaneArgs(srcPane, targetPane string) []string {
	return []string{"join-pane", "-h", "-s", srcPane, "-t", targetPane}
}

// BreakPaneArgs builds the argv for breaking srcPane back out into its OWN
// background window (`-d`) named windowName. Used to undock a docked PR-agent
// pane. Pure.
func BreakPaneArgs(srcPane, windowName string) []string {
	return []string{"break-pane", "-d", "-s", srcPane, "-n", windowName}
}

// OpenWindow launches command in its own background tmux window in cwd, labels
// the pane with title, and returns the new pane id (works with `-t %paneId`
// across windows). env is injected so the process inherits PRA_* in its pane.
func OpenWindow(cwd, command, title, name string, env map[string]string) (string, error) {
	paneID, err := Tmux(OpenWindowArgs(cwd, command, name, env)...)
	if err != nil {
		return "", err
	}
	SetPaneTitle(paneID, title)
	return paneID, nil
}

// OpenPane splits a new pane running command in cwd, labels it, and re-tiles via
// main-vertical so the orchestrator pane stays dominant on the left. Used for
// helper subagents that live alongside their parent PR agent.
func OpenPane(cwd, command, title string, env map[string]string) (string, error) {
	paneID, err := Tmux(OpenPaneArgs(cwd, command, env)...)
	if err != nil {
		return "", err
	}
	SetPaneTitle(paneID, title)
	TryTmux("set-window-option", "-t", paneID, "main-pane-width", "55%")
	TryTmux("select-layout", "-t", paneID, "main-vertical")
	return paneID, nil
}

// PaneAlive reports whether paneID is still a live pane (present in the global
// pane list).
func PaneAlive(paneID string) bool {
	if paneID == "" {
		return false
	}
	out, ok := TryTmux("list-panes", "-a", "-F", "#{pane_id}")
	if !ok {
		return false
	}
	for _, line := range strings.Split(out, "\n") {
		if line == paneID {
			return true
		}
	}
	return false
}

// CapturePane captures the recent visible output of a pane (the last `lines`
// rows), trimming trailing blank lines. Returns "" with ok=false when the pane
// is dead or capture fails.
func CapturePane(paneID string, lines int) (string, bool) {
	if !PaneAlive(paneID) {
		return "", false
	}
	out, ok := TryTmux(CaptureArgs(paneID, lines)...)
	if !ok {
		return "", false
	}
	return strings.TrimRight(out, "\n"), true
}

// SendToPane sends message to a pane as a literal string and then a separate
// Enter to submit it. Returns false when the pane is dead.
func SendToPane(paneID, message string) bool {
	if !PaneAlive(paneID) {
		return false
	}
	TryTmux(SendTextArgs(paneID, message)...)
	TryTmux(SendEnterArgs(paneID)...)
	return true
}

// StopMode selects how StopPane stops a pane.
type StopMode string

const (
	// StopInterrupt sends Escape to abort the current turn while keeping the
	// session alive so it can be re-steered.
	StopInterrupt StopMode = "interrupt"
	// StopKill kills the pane entirely (worktree/branch are kept for inspection).
	StopKill StopMode = "kill"
)

// StopPane stops a pane: interrupt (Escape) or kill (kill-pane). Returns false
// when the pane is already dead.
func StopPane(paneID string, mode StopMode) bool {
	if !PaneAlive(paneID) {
		return false
	}
	if mode == StopKill {
		TryTmux("kill-pane", "-t", paneID)
		return true
	}
	TryTmux("send-keys", "-t", paneID, "Escape")
	return true
}

// SetPaneTitle labels a pane (best-effort).
func SetPaneTitle(paneID, title string) {
	TryTmux(SetPaneTitleArgs(paneID, title)...)
}

// JoinPane docks srcPane to the RIGHT of targetPane's window. Returns false on
// any tmux failure (e.g. the source pane already gone). Best-effort.
func JoinPane(srcPane, targetPane string) bool {
	_, ok := TryTmux(JoinPaneArgs(srcPane, targetPane)...)
	return ok
}

// BreakPane undocks srcPane back into its own hidden background window named
// windowName. Best-effort.
func BreakPane(srcPane, windowName string) bool {
	_, ok := TryTmux(BreakPaneArgs(srcPane, windowName)...)
	return ok
}

// SelectLayoutMainVertical re-tiles targetPane's window with the main-vertical
// layout so the orchestrator stays dominant on the left. Best-effort.
func SelectLayoutMainVertical(targetPane string) {
	TryTmux("select-layout", "-t", targetPane, "main-vertical")
}

// SetMainPaneWidth sets the main-pane-width window option (e.g. "60%") for
// targetPane's window. Best-effort.
func SetMainPaneWidth(targetPane, width string) {
	TryTmux("set-window-option", "-t", targetPane, "main-pane-width", width)
}

// Focus brings a pane full-screen by selecting its window and then the pane.
// Returns false when the pane no longer exists.
func Focus(paneID string) bool {
	TryTmux("select-window", "-t", paneID)
	_, ok := TryTmux("select-pane", "-t", paneID)
	return ok
}
