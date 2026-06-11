package core

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

// Graphite-native PR-state reader (pure helpers + tolerant IO).
//
// Graphite's `gt` CLI caches each stack's PR/merge state in the repo's git
// common dir: <git-common-dir>/.graphite_pr_info is JSON with a `prInfos` array,
// one entry per PR in the stack. Field names are NOT documented and vary by gt
// version, so we parse DEFENSIVELY (multiple likely key spellings) and tolerate
// every failure. ParseGraphitePrInfos and ClassifyGraphitePrState are pure (no
// IO) so they can be unit-tested; the IO readers degrade to nil/no-op when gt is
// unavailable.

// GraphitePrInfo is one PR in a Graphite stack, as read from `.graphite_pr_info`.
type GraphitePrInfo struct {
	PrNumber int
	Branch   string
	State    string
	Title    string
	URL      string
}

// graphiteRawPrInfo captures every field spelling we tolerate from one
// `.graphite_pr_info` entry. Pointers on the number fields distinguish absent
// from zero so a missing PR number can be detected and skipped.
type graphiteRawPrInfo struct {
	PrNumber    *int   `json:"prNumber"`
	Number      *int   `json:"number"`
	BranchName  string `json:"branchName"`
	HeadRefName string `json:"headRefName"`
	Branch      string `json:"branch"`
	State       string `json:"state"`
	Title       string `json:"title"`
	URL         string `json:"url"`
}

// ParseGraphitePrInfos parses the raw bytes of `.graphite_pr_info` into one
// GraphitePrInfo per entry in `prInfos`. Defensive: invalid JSON → nil; a
// missing/non-array `prInfos` → empty; null/non-object entries are skipped, as
// are entries with no resolvable PR number. Field names vary by gt version, so
// several likely spellings are tried for the number and branch. The raw `state`
// string is preserved verbatim (classification is separate). Pure: no IO.
func ParseGraphitePrInfos(raw []byte) []GraphitePrInfo {
	var top struct {
		PrInfos []json.RawMessage `json:"prInfos"`
	}
	if err := json.Unmarshal(raw, &top); err != nil {
		return nil
	}
	out := make([]GraphitePrInfo, 0, len(top.PrInfos))
	for _, entry := range top.PrInfos {
		var o graphiteRawPrInfo
		if err := json.Unmarshal(entry, &o); err != nil {
			continue // non-object entry (number/string/array) — skip
		}
		num := o.PrNumber
		if num == nil {
			num = o.Number
		}
		if num == nil {
			continue // no resolvable PR number — skip
		}
		branch := firstNonEmpty(o.BranchName, o.HeadRefName, o.Branch)
		out = append(out, GraphitePrInfo{
			PrNumber: *num,
			Branch:   branch,
			State:    o.State,
			Title:    o.Title,
			URL:      o.URL,
		})
	}
	return out
}

// firstNonEmpty returns the first non-empty string, or "".
func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if v != "" {
			return v
		}
	}
	return ""
}

// ClassifyGraphitePrState classifies a Graphite PR state string into a
// PrStateClass, mirroring ClassifyPrState: case-insensitive MERGED → merged,
// CLOSED → closed, OPEN → open, anything else (including empty) → unknown. Pure.
func ClassifyGraphitePrState(state string) PrStateClass {
	switch strings.ToUpper(state) {
	case "MERGED":
		return PrStateMerged
	case "CLOSED":
		return PrStateClosed
	case "OPEN":
		return PrStateOpen
	default:
		return PrStateUnknown
	}
}

// GraphitePrInfoPath returns the path to Graphite's PR-info cache for this repo:
// <git-common-dir>/.graphite_pr_info.
func GraphitePrInfoPath(cwd string) (string, error) {
	commonDir, err := GitCommonDir(cwd)
	if err != nil {
		return "", err
	}
	return filepath.Join(commonDir, ".graphite_pr_info"), nil
}

// ReadGraphitePrInfos reads and parses `.graphite_pr_info` for this repo.
// Tolerant: a missing file, an unreadable path, or malformed JSON all yield nil.
// IO; not pure.
func ReadGraphitePrInfos(cwd string) []GraphitePrInfo {
	path, err := GraphitePrInfoPath(cwd)
	if err != nil {
		return nil
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil
	}
	return ParseGraphitePrInfos(raw)
}

// graphiteRefreshTimeout bounds the best-effort `gt log short` refresh.
const graphiteRefreshTimeout = 15 * time.Second

// RefreshGraphitePrInfo asks `gt` to refresh its PR-info cache from the Graphite
// service. A read-only `gt log short` bumps `.graphite_pr_info`. Swallows ALL
// errors (gt missing/unauthenticated/not a gt repo) so callers degrade to
// stale/no data. IO; not pure.
func RefreshGraphitePrInfo(cwd string) {
	ctx, cancel := context.WithTimeout(context.Background(), graphiteRefreshTimeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, "gt", "log", "short", "--no-interactive")
	cmd.Dir = cwd
	_ = cmd.Run() // ignore: gt missing/unauth/not a gt repo
}

// FetchGraphitePrStates refreshes (best-effort) then reads the Graphite PR
// states for this repo. This is the entry point the GitHub poller calls for the
// graphite strategy. IO; not pure.
func FetchGraphitePrStates(cwd string) []GraphitePrInfo {
	RefreshGraphitePrInfo(cwd)
	return ReadGraphitePrInfos(cwd)
}
