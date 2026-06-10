package core

import (
	"fmt"
	"strings"
)

// CI-failure poll (pure helpers + types).
//
// Once a worker's PR is pushed, the daemon reads its CI check status and, when
// checks FAIL, hands the worker a task to reproduce the failure locally (with
// the same gate CI runs), fix it, commit, and push. Failures are deduped ONCE
// PER COMMIT via "ci:<headSha>:<name>" keys (stored in the same seen-set as
// review ids), so a still-failing check after a fix-push gets a new sha => new
// key => re-notifies, while a passing run never notifies. The selection of NEW
// failures and the task message are pure (no gh/exec/IO).

// CiCheck is a single CI check on the PR's head commit.
type CiCheck struct {
	Name string
	// State is the raw state, e.g. failure/cancelled/timed_out/success/pending.
	State string
	// Bucket is gh's coarse classification: pass/fail/pending/skipping.
	Bucket string
	// Link is the details URL for the check run.
	Link string
}

// CiFailure is a failing CI check surfaced to the worker.
type CiFailure struct {
	Name  string
	State string
	Link  string
}

// NewCiSelection is the result of SelectNewCiFailures: the failures + their
// per-commit seen-keys.
type NewCiSelection struct {
	Failures []CiFailure
	NewKeys  []string
}

// SelectNewCiFailures selects the NEW CI failures for the PR's current head
// commit against the seen-set. Only Bucket == "fail" checks count as failures
// (pending/pass/skipping are ignored). Each failure is keyed
// "ci:<headSha>:<name>" so it is deduped ONCE PER COMMIT: after a fix-push the
// head sha changes, so a check that is still failing produces a NEW key and
// re-surfaces, while a passing run never produces a key. Returns only failures
// whose key is not already seen. Pure: no IO.
func SelectNewCiFailures(checks []CiCheck, headSha string, seen map[string]bool) NewCiSelection {
	failures := make([]CiFailure, 0)
	newKeys := make([]string, 0)
	added := make(map[string]bool)
	for _, c := range checks {
		if c.Bucket != "fail" {
			continue
		}
		key := fmt.Sprintf("ci:%s:%s", headSha, c.Name)
		if seen[key] || added[key] {
			continue
		}
		added[key] = true
		failures = append(failures, CiFailure{Name: c.Name, State: c.State, Link: c.Link})
		newKeys = append(newKeys, key)
	}
	return NewCiSelection{Failures: failures, NewKeys: newKeys}
}

// BuildCiFixTask builds the task message handed to the worker when CI fails. It
// lists each failing check as "- <name> (<state>) <link>" plus the fixed
// instructions: reproduce locally with the Go gate, fix, commit, push, and (if
// needed) inspect logs — never weaken checks to make CI pass. Pure: no IO.
func BuildCiFixTask(failures []CiFailure, prNumber int) string {
	lines := []string{fmt.Sprintf("CI is failing on PR #%d. The following checks failed:", prNumber), ""}
	for _, f := range failures {
		link := ""
		if f.Link != "" {
			link = " " + f.Link
		}
		lines = append(lines, fmt.Sprintf("- %s (%s)%s", f.Name, f.State, link))
	}
	lines = append(lines, "", strings.Join([]string{
		"Reproduce locally by running the gate — `gofmt -l .`, `go vet ./...`, `go build ./...`, `go test",
		"./...` — fix the cause, commit (e.g. `fix: resolve CI failure`), and `git push`. If the failure is",
		"environment-specific or unclear from the gate, inspect logs with `gh run view --log-failed` (find",
		"the run via `gh run list --branch <branch>`). Do not disable or weaken checks to make CI pass.",
	}, " "))
	return strings.Join(lines, "\n")
}

// RollupBucket maps a statusCheckRollup conclusion/state (uppercased) to gh's
// coarse bucket. Used by the CI fetcher's fallback path when
// `gh pr checks --json` is unavailable on older gh. Pure.
func RollupBucket(stateUpper string) string {
	switch stateUpper {
	case "FAILURE", "ERROR", "CANCELLED", "TIMED_OUT", "ACTION_REQUIRED", "STARTUP_FAILURE":
		return "fail"
	case "SUCCESS":
		return "pass"
	case "SKIPPED", "NEUTRAL":
		return "skipping"
	default:
		return "pending"
	}
}
