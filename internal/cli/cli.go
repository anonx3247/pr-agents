// Package cli implements the root command, subcommand dispatch, and version
// reporting for the pr-agents binary. It deliberately uses only the Go stdlib
// flag package plus a small hand-rolled dispatch table — no third-party CLI
// framework — to keep dependencies minimal.
package cli

import (
	"flag"
	"fmt"
	"io"
	"sort"
)

// Version is the pr-agents version string. Overridable at build time via
// -ldflags "-X github.com/anonx3247/pr-agents/internal/cli.Version=...".
var Version = "0.0.0-dev"

// command describes a single subcommand: its summary (for help) and a run
// function receiving the remaining args plus the output/error writers.
type command struct {
	summary string
	run     func(args []string, stdout, stderr io.Writer) int
}

// commands is the dispatch table. select remains a stub until a later PR
// implements the interactive picker.
var commands = map[string]command{
	"version":       {"Print the pr-agents version", runVersion},
	"start":         {"Launch the orchestrator in a tmux session", runStart},
	"dispatch":      {"Create a worktree + branch + pane and hand off one PR", runDispatch},
	"list":          {"List PR agents with number/name/branch/status", runList},
	"peek":          {"Read a PR agent's recent pane output", runPeek},
	"send":          {"Send a message to a running PR agent", runSend},
	"stop":          {"Interrupt or kill a PR agent", runStop},
	"focus":         {"Move tmux focus to a PR agent's pane", runFocus},
	"cleanup":       {"Remove merged/closed PR worktrees", runCleanup},
	"context":       {"Print the current PR identity resolved from the cwd", runContext},
	"set-pr-number": {"Record the PR number on the current worker's entry", runSetPrNumber},
	"mark-pushed":   {"Mark the current worker's PR as pushed (starts polling)", runMarkPushed},
	"report-result": {"Record the current worker's final result text", runReportResult},
	"reply-review":  {"Reply to a reviewer's inline comment thread", runReplyReview},
	"select":        {"Interactively select a PR agent", stub("select")},
	"daemon":        {"Run the per-session background daemon", runDaemon},
}

// Run is the entrypoint invoked by main. It parses the root flags, resolves the
// subcommand, and returns a process exit code.
func Run(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("pr-agents", flag.ContinueOnError)
	fs.SetOutput(stderr)
	showVersion := fs.Bool("version", false, "Print the pr-agents version and exit")
	fs.Usage = func() { usage(stderr) }

	if err := fs.Parse(args); err != nil {
		return 2
	}
	if *showVersion {
		return runVersion(nil, stdout, stderr)
	}

	rest := fs.Args()
	if len(rest) == 0 {
		usage(stderr)
		return 2
	}

	name := rest[0]
	cmd, ok := commands[name]
	if !ok {
		fmt.Fprintf(stderr, "pr-agents: unknown command %q\n\n", name)
		usage(stderr)
		return 2
	}
	return cmd.run(rest[1:], stdout, stderr)
}

func runVersion(_ []string, stdout, _ io.Writer) int {
	fmt.Fprintf(stdout, "pr-agents %s\n", Version)
	return 0
}

// stub returns a run function that reports the command is not yet implemented.
func stub(name string) func([]string, io.Writer, io.Writer) int {
	return func(_ []string, _ io.Writer, stderr io.Writer) int {
		fmt.Fprintf(stderr, "pr-agents %s: not implemented\n", name)
		return 1
	}
}

func usage(w io.Writer) {
	fmt.Fprintln(w, "pr-agents — harness-agnostic PR orchestration")
	fmt.Fprintln(w)
	fmt.Fprintln(w, "Usage: pr-agents [--version] <command> [args]")
	fmt.Fprintln(w)
	fmt.Fprintln(w, "Commands:")
	names := make([]string, 0, len(commands))
	for name := range commands {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		fmt.Fprintf(w, "  %-14s %s\n", name, commands[name].summary)
	}
}
