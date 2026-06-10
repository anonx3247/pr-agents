package core

import (
	"fmt"
	"strings"
)

// Review-comment poll (pure helpers + types).
//
// The daemon polls each live worker's PR for new reviewer feedback and, when
// new inline comments arrive, hands the worker a task to address them, push,
// and reply (without resolving threads). The selection of what is NEW and the
// task message are kept pure (no gh/exec/IO) so they can be unit-tested.

// InlineComment is an actionable inline review comment (a file/line comment on
// the PR diff).
type InlineComment struct {
	ID          int
	User        string
	Body        string
	Path        string
	Line        *int // nil when the comment is not anchored to a line
	CreatedAt   string
	InReplyToID *int
}

// ReviewSummary is a review summary (gh pr view --json reviews), surfaced as
// context.
type ReviewSummary struct {
	ID          *int
	Author      string
	Body        string
	State       string
	SubmittedAt string
}

// IssueComment is a general PR (issue) comment (gh pr view --json comments),
// surfaced as context.
type IssueComment struct {
	ID        *int
	Author    string
	Body      string
	CreatedAt string
}

// FetchedReviewActivity is everything the poller fetches for one PR in a tick.
type FetchedReviewActivity struct {
	Inline        []InlineComment
	Reviews       []ReviewSummary
	IssueComments []IssueComment
}

// NewReviewSelection is the result of SelectNewReviewItems: the actionable
// inline comments, the context notes, and EVERY surfaced key (so the caller can
// mark them all seen).
type NewReviewSelection struct {
	Actionable   []InlineComment
	ContextNotes []string
	NewIDs       []string
}

// SelectNewReviewItems selects the NEW review items from a fetched tick against
// the seen-set. Ids are keyed distinctly so the three kinds never collide:
// inline comments "rc:<id>", review summaries "rv:<id|submittedAt+author>",
// issue comments "ic:<id|createdAt+author>". Inline comments are the actionable
// items; non-empty COMMENTED/CHANGES_REQUESTED review bodies and issue comments
// become context notes. NewIDs carries EVERY surfaced key (actionable + context)
// so the caller can mark them all seen and never re-surface them. Pure: no IO.
func SelectNewReviewItems(fetched FetchedReviewActivity, seen map[string]bool) NewReviewSelection {
	actionable := make([]InlineComment, 0)
	contextNotes := make([]string, 0)
	newIDs := make([]string, 0)
	added := make(map[string]bool)

	add := func(key string) bool {
		if seen[key] || added[key] {
			return false
		}
		added[key] = true
		newIDs = append(newIDs, key)
		return true
	}

	for _, c := range fetched.Inline {
		key := fmt.Sprintf("rc:%d", c.ID)
		if add(key) {
			actionable = append(actionable, c)
		}
	}

	for _, r := range fetched.Reviews {
		body := strings.TrimSpace(r.Body)
		if body == "" {
			continue
		}
		state := strings.ToUpper(r.State)
		if state != "COMMENTED" && state != "CHANGES_REQUESTED" {
			continue
		}
		key := "rv:" + reviewKey(r)
		if add(key) {
			author := r.Author
			if author == "" {
				author = "?"
			}
			contextNotes = append(contextNotes, fmt.Sprintf("review by %s (%s): %s", author, state, body))
		}
	}

	for _, c := range fetched.IssueComments {
		body := strings.TrimSpace(c.Body)
		if body == "" {
			continue
		}
		key := "ic:" + issueKey(c)
		if add(key) {
			author := c.Author
			if author == "" {
				author = "?"
			}
			contextNotes = append(contextNotes, fmt.Sprintf("comment by %s: %s", author, body))
		}
	}

	return NewReviewSelection{Actionable: actionable, ContextNotes: contextNotes, NewIDs: newIDs}
}

// reviewKey derives the dedup id-suffix for a review summary: its numeric id, or
// "submittedAt+author" when the id is absent.
func reviewKey(r ReviewSummary) string {
	if r.ID != nil {
		return fmt.Sprintf("%d", *r.ID)
	}
	return r.SubmittedAt + "+" + r.Author
}

// issueKey derives the dedup id-suffix for an issue comment: its numeric id, or
// "createdAt+author" when the id is absent.
func issueKey(c IssueComment) string {
	if c.ID != nil {
		return fmt.Sprintf("%d", *c.ID)
	}
	return c.CreatedAt + "+" + c.Author
}

// BuildReviewTask builds the task message handed to the worker when new inline
// review comments arrive. It lists each comment as
// "- [rc:<id>] <path>:<line> — <body>" plus any context notes, then the fixed
// instructions: address with code, run the Go gate, commit, push, and REPLY to
// each thread via the harness reply tool or `pr-agents reply-review` (NOT raw
// gh, so the reply is recorded in the seen-set and never re-surfaced) WITHOUT
// resolving threads. Pure.
func BuildReviewTask(actionable []InlineComment, contextNotes []string, prNumber int) string {
	lines := []string{
		fmt.Sprintf("New review feedback on PR #%d. Address each reviewer comment below.", prNumber),
		"",
		"Inline review comments:",
	}
	for _, c := range actionable {
		loc := c.Path
		if c.Line != nil {
			loc = fmt.Sprintf("%s:%d", c.Path, *c.Line)
		}
		lines = append(lines, fmt.Sprintf("- [rc:%d] %s — %s", c.ID, loc, c.Body))
	}
	if len(contextNotes) > 0 {
		lines = append(lines, "", "Additional context:")
		for _, n := range contextNotes {
			lines = append(lines, "- "+n)
		}
	}
	lines = append(lines, "", strings.Join([]string{
		"Address each comment with code changes; run the gate — `gofmt -l .`, `go vet ./...`, `go test ./...`;",
		"commit (e.g. `fix: address review feedback`); push with `git push`; then REPLY to EACH inline",
		"thread using your harness's reply tool (`reply_to_review_comment`) or `pr-agents reply-review",
		"<commentId> <body>` — do NOT reply with raw `gh`, because these record your reply so the daemon",
		"does not re-surface it as new feedback. commentId is the numeric id from `rc:<id>` and body is a",
		"short explanation of the fix or a clarifying question. Do NOT resolve threads — leave that to the",
		"reviewer. If a comment is ambiguous or architectural, reply asking for clarification instead of",
		"guessing.",
	}, " "))
	return strings.Join(lines, "\n")
}

// MergeSeen returns the union of existing and newIDs, preserving existing order
// then appending the new ids in order. Pure: no IO. The caller persists the
// result on the entry's SeenReviewIds.
func MergeSeen(existing, newIDs []string) []string {
	seen := make(map[string]bool, len(existing))
	out := make([]string, 0, len(existing)+len(newIDs))
	for _, id := range existing {
		if !seen[id] {
			seen[id] = true
			out = append(out, id)
		}
	}
	for _, id := range newIDs {
		if !seen[id] {
			seen[id] = true
			out = append(out, id)
		}
	}
	return out
}
