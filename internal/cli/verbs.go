package cli

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"
	"text/tabwriter"

	"github.com/anonx3247/pr-agents/internal/core"
	"github.com/anonx3247/pr-agents/internal/tmux"
)

// paneAlive is the liveness predicate used by the verbs. It is a package var so
// tests can stub it without invoking real tmux.
var paneAlive = tmux.PaneAlive

// listRow is one row of `list` output (also the --json shape).
type listRow struct {
	ID       string `json:"id"`
	PrNumber *int   `json:"prNumber,omitempty"`
	Name     string `json:"name"`
	Branch   string `json:"branch"`
	Mode     string `json:"mode"`
	Status   string `json:"status"`
	Live     bool   `json:"live"`
}

func buildListRows(entries []core.PrEntry, alive func(string) bool) []listRow {
	rows := make([]listRow, 0, len(entries))
	for _, e := range entries {
		rows = append(rows, listRow{
			ID:       e.ID,
			PrNumber: e.PrNumber,
			Name:     e.PrName,
			Branch:   e.Branch,
			Mode:     string(e.Mode),
			Status:   string(e.Status),
			Live:     e.PaneID != "" && alive(e.PaneID),
		})
	}
	return rows
}

func runList(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("list", flag.ContinueOnError)
	fs.SetOutput(stderr)
	asJSON := fs.Bool("json", false, "Output machine-readable JSON")
	if err := fs.Parse(args); err != nil {
		return 2
	}

	entries, _, _, err := sessionEntries()
	if err != nil {
		fmt.Fprintf(stderr, "pr-agents list: %v\n", err)
		return 1
	}
	rows := buildListRows(entries, paneAlive)

	if *asJSON {
		enc := json.NewEncoder(stdout)
		enc.SetIndent("", "  ")
		if err := enc.Encode(rows); err != nil {
			fmt.Fprintf(stderr, "pr-agents list: %v\n", err)
			return 1
		}
		return 0
	}

	if len(rows) == 0 {
		fmt.Fprintln(stdout, "No PR agents.")
		return 0
	}
	tw := tabwriter.NewWriter(stdout, 0, 2, 2, ' ', 0)
	fmt.Fprintln(tw, "PR\tNAME\tBRANCH\tMODE\tSTATUS\tPANE")
	for _, r := range rows {
		pr := "-"
		if r.PrNumber != nil {
			pr = "#" + strconv.Itoa(*r.PrNumber)
		}
		pane := "dead"
		if r.Live {
			pane = "live"
		}
		fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%s\t%s\n", pr, r.Name, r.Branch, r.Mode, r.Status, pane)
	}
	tw.Flush()
	return 0
}

// withEntry resolves ref against the session-scoped registry and invokes fn,
// or prints the standard not-found error and returns exit code 1.
func withEntry(ref string, stdout, stderr io.Writer, fn func(e core.PrEntry) int) int {
	entries, _, _, err := sessionEntries()
	if err != nil {
		fmt.Fprintf(stderr, "pr-agents: %v\n", err)
		return 1
	}
	e := core.FindEntry(entries, ref)
	if e == nil {
		fmt.Fprintf(stderr, "No PR agent matching %q.\n", ref)
		return 1
	}
	return fn(*e)
}

func runPeek(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("peek", flag.ContinueOnError)
	fs.SetOutput(stderr)
	lines := fs.Int("lines", 60, "Number of recent pane lines to capture")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	rest := fs.Args()
	if len(rest) == 0 {
		fmt.Fprintln(stderr, "pr-agents peek: usage: peek <id> [--lines N]")
		return 2
	}
	return withEntry(rest[0], stdout, stderr, func(e core.PrEntry) int {
		out, ok := tmux.CapturePane(e.PaneID, *lines)
		if !ok {
			fmt.Fprintln(stderr, "Pane no longer exists.")
			return 1
		}
		fmt.Fprintln(stdout, out)
		return 0
	})
}

func runSend(args []string, stdout, stderr io.Writer) int {
	if len(args) < 2 {
		fmt.Fprintln(stderr, "pr-agents send: usage: send <id> <message...>")
		return 2
	}
	ref := args[0]
	message := strings.Join(args[1:], " ")
	return withEntry(ref, stdout, stderr, func(e core.PrEntry) int {
		if !tmux.SendToPane(e.PaneID, message) {
			fmt.Fprintln(stderr, "Pane no longer exists.")
			return 1
		}
		fmt.Fprintf(stdout, "Sent to %s (%s).\n", e.PrName, e.PaneID)
		return 0
	})
}

func runStop(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("stop", flag.ContinueOnError)
	fs.SetOutput(stderr)
	kill := fs.Bool("kill", false, "Kill the pane entirely (default: interrupt)")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	rest := fs.Args()
	if len(rest) == 0 {
		fmt.Fprintln(stderr, "pr-agents stop: usage: stop <id> [--kill]")
		return 2
	}
	mode := tmux.StopInterrupt
	if *kill {
		mode = tmux.StopKill
	}
	return withEntry(rest[0], stdout, stderr, func(e core.PrEntry) int {
		if !tmux.StopPane(e.PaneID, mode) {
			fmt.Fprintln(stderr, "Pane no longer exists.")
			return 1
		}
		verb := "Interrupted"
		if *kill {
			verb = "Killed"
		}
		fmt.Fprintf(stdout, "%s %s (%s).\n", verb, e.PrName, e.PaneID)
		return 0
	})
}

func runFocus(args []string, stdout, stderr io.Writer) int {
	if len(args) == 0 {
		fmt.Fprintln(stderr, "pr-agents focus: usage: focus <id>")
		return 2
	}
	return withEntry(args[0], stdout, stderr, func(e core.PrEntry) int {
		if !tmux.Focus(e.PaneID) {
			fmt.Fprintln(stderr, "Pane no longer exists.")
			return 1
		}
		fmt.Fprintf(stdout, "Focused %s (%s).\n", e.PrName, e.PaneID)
		return 0
	})
}

// contextOut is the JSON/human shape of the `context` verb output.
type contextOut struct {
	ID        string `json:"id"`
	PrNumber  *int   `json:"prNumber,omitempty"`
	Branch    string `json:"branch"`
	Base      string `json:"base"`
	Mode      string `json:"mode"`
	Simplify  bool   `json:"simplify"`
	Depth     int    `json:"depth"`
	SessionID string `json:"sessionId"`
}

func runContext(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("context", flag.ContinueOnError)
	fs.SetOutput(stderr)
	asJSON := fs.Bool("json", false, "Output machine-readable JSON")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	cwd, err := os.Getwd()
	if err != nil {
		fmt.Fprintf(stderr, "pr-agents context: %v\n", err)
		return 1
	}
	all, err := core.LoadRegistry(cwd)
	if err != nil {
		fmt.Fprintf(stderr, "pr-agents context: %v\n", err)
		return 1
	}
	e := core.ResolveContextFromCwd(all, cwd)
	if e == nil {
		fmt.Fprintln(stderr, "No PR identity for the current directory.")
		return 1
	}
	out := contextOut{
		ID:        e.ID,
		PrNumber:  e.PrNumber,
		Branch:    e.Branch,
		Base:      e.Base,
		Mode:      string(e.Mode),
		Simplify:  e.Simplify,
		Depth:     e.Depth,
		SessionID: e.SessionID,
	}
	if *asJSON {
		enc := json.NewEncoder(stdout)
		enc.SetIndent("", "  ")
		if err := enc.Encode(out); err != nil {
			fmt.Fprintf(stderr, "pr-agents context: %v\n", err)
			return 1
		}
		return 0
	}
	pr := "-"
	if out.PrNumber != nil {
		pr = "#" + strconv.Itoa(*out.PrNumber)
	}
	fmt.Fprintf(stdout, "id:        %s\n", out.ID)
	fmt.Fprintf(stdout, "pr:        %s\n", pr)
	fmt.Fprintf(stdout, "branch:    %s\n", out.Branch)
	fmt.Fprintf(stdout, "base:      %s\n", out.Base)
	fmt.Fprintf(stdout, "mode:      %s\n", out.Mode)
	fmt.Fprintf(stdout, "simplify:  %t\n", out.Simplify)
	fmt.Fprintf(stdout, "depth:     %d\n", out.Depth)
	fmt.Fprintf(stdout, "sessionId: %s\n", out.SessionID)
	return 0
}
