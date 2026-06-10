package core

import "strings"

// PrStateClass is a PR's GitHub lifecycle state, as classified from gh JSON.
type PrStateClass string

const (
	PrStateMerged  PrStateClass = "merged"
	PrStateClosed  PrStateClass = "closed"
	PrStateOpen    PrStateClass = "open"
	PrStateUnknown PrStateClass = "unknown"
)

// IsPollable reports whether the orchestrator should poll this entry's GitHub
// state: it must be a PR subagent (depth 1) that signalled Pushed (so the PR
// exists), carry a numeric PR number, and not already be in a terminal state.
// Before Pushed this is false, so zero gh calls are made.
func IsPollable(e PrEntry) bool {
	return e.Depth == 1 &&
		e.Pushed &&
		e.PrNumber != nil &&
		e.Status != StatusMerged &&
		e.Status != StatusClosed &&
		e.Status != StatusStopped
}

// PrStateJSON is the subset of `gh pr view --json state,mergedAt` we classify.
// MergedAt is a pointer so a null/absent value is distinguishable from "".
type PrStateJSON struct {
	State    string  `json:"state"`
	MergedAt *string `json:"mergedAt"`
}

// ClassifyPrState classifies parsed gh PR JSON into a lifecycle state. A
// non-null mergedAt always means merged; otherwise the textual state (gh emits
// MERGED/CLOSED/OPEN, matched case-insensitively) decides. Anything else,
// including a nil pointer, is unknown.
func ClassifyPrState(j *PrStateJSON) PrStateClass {
	if j == nil {
		return PrStateUnknown
	}
	if j.MergedAt != nil {
		return PrStateMerged
	}
	switch strings.ToUpper(j.State) {
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

// ClassifiedEntry pairs a PR entry with its freshly-classified GitHub state.
type ClassifiedEntry struct {
	Entry PrEntry
	State PrStateClass
}

// StateTransition is an entry that newly reached a terminal state.
type StateTransition struct {
	Entry PrEntry
	State PrStateClass // merged or closed
}

// SelectStateTransitions returns the entries whose state has transitioned to a
// TERMINAL state (merged/closed) differing from lastState. The lastState map
// dedups across polling ticks so each genuine transition notifies once.
func SelectStateTransitions(classified []ClassifiedEntry, lastState map[string]PrStateClass) []StateTransition {
	out := make([]StateTransition, 0)
	for _, c := range classified {
		if c.State != PrStateMerged && c.State != PrStateClosed {
			continue
		}
		if lastState[c.Entry.ID] == c.State {
			continue
		}
		out = append(out, StateTransition{Entry: c.Entry, State: c.State})
	}
	return out
}

// ChangedFileStatus is the status of a file in `git diff --name-status`.
type ChangedFileStatus string

const (
	ChangedModified ChangedFileStatus = "modified"
	ChangedAdded    ChangedFileStatus = "added"
	ChangedRenamed  ChangedFileStatus = "renamed"
	ChangedCopied   ChangedFileStatus = "copied"
)

// ChangedFile is one entry from a parsed diff name-status listing.
type ChangedFile struct {
	Path   string
	Status ChangedFileStatus
}

var changedStatusMap = map[byte]ChangedFileStatus{
	'M': ChangedModified,
	'A': ChangedAdded,
	'R': ChangedRenamed,
	'C': ChangedCopied,
}

// ParseChangedFiles parses `git diff --name-status` output into ChangedFiles.
// Renamed (R100\told\tnew) and copied (C100\told\tnew) lines carry two paths;
// the NEW path (3rd tab field) is used. Unrecognized/blank lines are skipped.
func ParseChangedFiles(stdout string) []ChangedFile {
	files := make([]ChangedFile, 0)
	for _, line := range strings.Split(stdout, "\n") {
		if strings.TrimSpace(line) == "" {
			continue
		}
		parts := strings.Split(line, "\t")
		if len(parts) == 0 || parts[0] == "" {
			continue
		}
		status, ok := changedStatusMap[parts[0][0]]
		if !ok {
			continue
		}
		var path string
		if status == ChangedRenamed || status == ChangedCopied {
			if len(parts) > 2 {
				path = parts[2]
			}
		} else if len(parts) > 1 {
			path = parts[1]
		}
		if path != "" {
			files = append(files, ChangedFile{Path: path, Status: status})
		}
	}
	return files
}
