package cli

import (
	"flag"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/anonx3247/pr-agents/internal/core"
	"github.com/anonx3247/pr-agents/internal/daemon"
	"github.com/anonx3247/pr-agents/internal/tmux"
)

// nowRFC3339 is the timestamp source for worker verbs; a package var so tests
// can stub it for deterministic output.
var nowRFC3339 = func() string { return time.Now().UTC().Format(time.RFC3339) }

// workerEntry resolves the worker's own registry entry from the cwd. Worker
// identity comes from the registry keyed on the worktree path, NOT from env, so
// it survives a sandbox boundary that filters PRA_* vars.
func workerEntry(stderr io.Writer, verb string) (cwd string, e *core.PrEntry, ok bool) {
	cwd, err := os.Getwd()
	if err != nil {
		fmt.Fprintf(stderr, "pr-agents %s: %v\n", verb, err)
		return "", nil, false
	}
	all, err := core.LoadRegistry(cwd)
	if err != nil {
		fmt.Fprintf(stderr, "pr-agents %s: %v\n", verb, err)
		return cwd, nil, false
	}
	e = core.ResolveContextFromCwd(all, cwd)
	if e == nil {
		fmt.Fprintf(stderr, "pr-agents %s: no PR identity for the current directory.\n", verb)
		return cwd, nil, false
	}
	return cwd, e, true
}

// relabelPane updates the pane title to reflect the (possibly changed) entry.
func relabelPane(e *core.PrEntry) {
	if e.PaneID == "" {
		return
	}
	tmux.SetPaneTitle(e.PaneID, core.PaneTitle(core.PaneTitleArgs{
		PrNumber: e.PrNumber,
		PrName:   e.PrName,
		Branch:   e.Branch,
	}))
}

func runSetPrNumber(args []string, stdout, stderr io.Writer) int {
	if len(args) == 0 {
		fmt.Fprintln(stderr, "pr-agents set-pr-number: usage: set-pr-number <n> [--url URL]")
		return 2
	}
	num, err := strconv.Atoi(args[0])
	if err != nil {
		fmt.Fprintf(stderr, "pr-agents set-pr-number: invalid number %q\n", args[0])
		return 2
	}
	fs := flag.NewFlagSet("set-pr-number", flag.ContinueOnError)
	fs.SetOutput(stderr)
	url := fs.String("url", "", "The pull request URL")
	if err := fs.Parse(args[1:]); err != nil {
		return 2
	}
	cwd, e, ok := workerEntry(stderr, "set-pr-number")
	if !ok {
		return 1
	}
	updated, found, err := core.UpdateEntry(cwd, e.ID, func(p *core.PrEntry) {
		p.PrNumber = &num
		if *url != "" {
			p.PrURL = *url
		}
		p.Status = core.StatusOpen
	})
	if err != nil || !found {
		fmt.Fprintf(stderr, "pr-agents set-pr-number: failed to update registry\n")
		return 1
	}
	relabelPane(updated)
	fmt.Fprintf(stdout, "Recorded PR #%d.\n", num)
	return 0
}

func runMarkPushed(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("mark-pushed", flag.ContinueOnError)
	fs.SetOutput(stderr)
	number := fs.Int("number", 0, "The pull request number (optional if already set)")
	url := fs.String("url", "", "The pull request URL")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	cwd, e, ok := workerEntry(stderr, "mark-pushed")
	if !ok {
		return 1
	}
	updated, found, err := core.UpdateEntry(cwd, e.ID, func(p *core.PrEntry) {
		if *number > 0 {
			n := *number
			p.PrNumber = &n
		}
		if *url != "" {
			p.PrURL = *url
		}
		p.Pushed = true
		p.PushedAt = nowRFC3339()
		p.Status = core.StatusOpen
	})
	if err != nil || !found {
		fmt.Fprintf(stderr, "pr-agents mark-pushed: failed to update registry\n")
		return 1
	}
	relabelPane(updated)
	fmt.Fprintln(stdout, "Marked as pushed; the orchestrator will now poll this PR.")
	return 0
}

// runReplyReview posts an inline reply to a reviewer's comment thread on the
// current worker's PR and records the reply id in the seen-set so the daemon
// never re-surfaces it. It does NOT resolve the thread (left to the reviewer).
func runReplyReview(args []string, stdout, stderr io.Writer) int {
	if len(args) < 2 {
		fmt.Fprintln(stderr, "pr-agents reply-review: usage: reply-review <commentId> <body...>")
		return 2
	}
	commentID, err := strconv.Atoi(args[0])
	if err != nil {
		fmt.Fprintf(stderr, "pr-agents reply-review: invalid commentId %q\n", args[0])
		return 2
	}
	body := strings.Join(args[1:], " ")
	cwd, e, ok := workerEntry(stderr, "reply-review")
	if !ok {
		return 1
	}
	if e.PrNumber == nil {
		fmt.Fprintln(stderr, "pr-agents reply-review: this PR has no number yet (run set-pr-number first)")
		return 1
	}
	owner, repo, ok := daemon.ResolveOwnerRepo(cwd)
	if !ok {
		fmt.Fprintln(stderr, "pr-agents reply-review: could not resolve owner/repo (is gh authenticated?)")
		return 1
	}
	replyID, ok := daemon.PostReviewReply(owner, repo, *e.PrNumber, commentID, body, cwd)
	if !ok {
		fmt.Fprintln(stderr, "pr-agents reply-review: failed to post reply via gh")
		return 1
	}
	// Record the reply so the poller's seen-set never re-surfaces our own reply.
	_, _, _ = core.UpdateEntry(cwd, e.ID, func(p *core.PrEntry) {
		p.SeenReviewIds = core.MergeSeen(p.SeenReviewIds, []string{fmt.Sprintf("rc:%d", replyID)})
	})
	fmt.Fprintf(stdout, "Replied to comment %d (reply id %d).\n", commentID, replyID)
	return 0
}

func runReportResult(args []string, stdout, stderr io.Writer) int {
	if len(args) == 0 {
		fmt.Fprintln(stderr, "pr-agents report-result: usage: report-result <text...>")
		return 2
	}
	result := strings.Join(args, " ")
	cwd, e, ok := workerEntry(stderr, "report-result")
	if !ok {
		return 1
	}
	_, found, err := core.UpdateEntry(cwd, e.ID, func(p *core.PrEntry) {
		p.LastResult = result
		p.LastResultAt = nowRFC3339()
		seq := 0
		if p.ResultSeq != nil {
			seq = *p.ResultSeq + 1
		}
		p.ResultSeq = &seq
	})
	if err != nil || !found {
		fmt.Fprintf(stderr, "pr-agents report-result: failed to update registry\n")
		return 1
	}
	fmt.Fprintln(stdout, "Recorded result.")
	return 0
}
