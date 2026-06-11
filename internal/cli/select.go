package cli

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"

	"github.com/anonx3247/pr-agents/internal/core"
	"github.com/anonx3247/pr-agents/internal/tmux"
)

// focusPane brings a pane to the foreground. It is a package var so tests can
// stub it without invoking real tmux.
var focusPane = tmux.Focus

// parseSelection interprets a user's menu input over a list of n choices. It
// returns a zero-based index into the list and ok=true for a valid pick. Blank
// input (or a "q"/"quit"/"cancel" sentinel) is treated as a graceful cancel:
// index -1, ok=false. Any other unparseable or out-of-range input is also a
// non-ok cancel, so the caller never errors on a fat-fingered choice.
func parseSelection(input string, n int) (idx int, ok bool) {
	s := strings.TrimSpace(strings.ToLower(input))
	if s == "" || s == "q" || s == "quit" || s == "cancel" {
		return -1, false
	}
	v, err := strconv.Atoi(s)
	if err != nil || v < 1 || v > n {
		return -1, false
	}
	return v - 1, true
}

func runSelect(args []string, stdout, stderr io.Writer) int {
	entries, _, _, err := sessionEntries()
	if err != nil {
		fmt.Fprintf(stderr, "pr-agents select: %v\n", err)
		return 1
	}
	return selectFrom(liveEntries(entries, paneAlive), os.Stdin, stdout, stderr)
}

// selectFrom renders the numbered menu for the given live agents, reads one
// choice from in, focuses the chosen pane, and returns a process exit code. It
// is the testable core of the select verb: the entry list comes from the caller
// and the tmux focus call goes through the package-level focusPane var.
func selectFrom(live []core.PrEntry, in io.Reader, stdout, stderr io.Writer) int {
	if len(live) == 0 {
		fmt.Fprintln(stdout, "No live PR agents.")
		return 0
	}

	for i, e := range live {
		fmt.Fprintf(stdout, "  %d) %s\n", i+1, selectLabel(e))
	}
	fmt.Fprint(stdout, "Select an agent (blank to cancel): ")

	line, _ := bufio.NewReader(in).ReadString('\n')
	idx, ok := parseSelection(line, len(live))
	if !ok {
		fmt.Fprintln(stdout, "Cancelled.")
		return 0
	}

	e := live[idx]
	if !focusPane(e.PaneID) {
		fmt.Fprintln(stderr, "Pane no longer exists.")
		return 1
	}
	fmt.Fprintf(stdout, "Focused %s (%s).\n", e.PrName, e.PaneID)
	return 0
}

// liveEntries filters entries down to those with a pane that is currently
// alive, preserving order.
func liveEntries(entries []core.PrEntry, alive func(string) bool) []core.PrEntry {
	live := make([]core.PrEntry, 0, len(entries))
	for _, e := range entries {
		if e.PaneID != "" && alive(e.PaneID) {
			live = append(live, e)
		}
	}
	return live
}

// selectLabel renders one menu row: "#PR name (branch)" with sensible
// fallbacks when the PR number is not yet known.
func selectLabel(e core.PrEntry) string {
	pr := "-"
	if e.PrNumber != nil {
		pr = "#" + strconv.Itoa(*e.PrNumber)
	}
	return fmt.Sprintf("%s %s (%s)", pr, e.PrName, e.Branch)
}
