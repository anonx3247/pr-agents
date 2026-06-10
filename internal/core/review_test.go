package core

import (
	"reflect"
	"strings"
	"testing"
)

func intp(n int) *int { return &n }

func TestSelectNewReviewItems(t *testing.T) {
	fetched := FetchedReviewActivity{
		Inline: []InlineComment{
			{ID: 1, Body: "fix this", Path: "a.go", Line: intp(10)},
			{ID: 2, Body: "and this", Path: "b.go"},
			{ID: 1, Body: "dup", Path: "a.go"}, // duplicate id within tick
		},
		Reviews: []ReviewSummary{
			{ID: intp(5), Author: "alice", Body: "needs work", State: "CHANGES_REQUESTED"},
			{ID: intp(6), Author: "bob", Body: "looks ok", State: "APPROVED"}, // not surfaced
			{ID: intp(7), Author: "carol", Body: "", State: "COMMENTED"},      // empty body skipped
		},
		IssueComments: []IssueComment{
			{ID: intp(9), Author: "dave", Body: "ping"},
			{ID: intp(9), Author: "dave", Body: "ping"}, // dup id
		},
	}
	seen := map[string]bool{"rc:2": true} // comment 2 already seen

	got := SelectNewReviewItems(fetched, seen)

	if len(got.Actionable) != 1 || got.Actionable[0].ID != 1 {
		t.Fatalf("actionable = %#v, want only id 1", got.Actionable)
	}
	wantNotes := []string{
		"review by alice (CHANGES_REQUESTED): needs work",
		"comment by dave: ping",
	}
	if !reflect.DeepEqual(got.ContextNotes, wantNotes) {
		t.Errorf("contextNotes = %#v, want %#v", got.ContextNotes, wantNotes)
	}
	wantIDs := []string{"rc:1", "rv:5", "ic:9"}
	if !reflect.DeepEqual(got.NewIDs, wantIDs) {
		t.Errorf("newIDs = %#v, want %#v", got.NewIDs, wantIDs)
	}
}

func TestSelectNewReviewItemsKeylessFallback(t *testing.T) {
	fetched := FetchedReviewActivity{
		Reviews:       []ReviewSummary{{Author: "a", Body: "x", State: "COMMENTED", SubmittedAt: "t1"}},
		IssueComments: []IssueComment{{Author: "b", Body: "y", CreatedAt: "t2"}},
	}
	got := SelectNewReviewItems(fetched, map[string]bool{})
	wantIDs := []string{"rv:t1+a", "ic:t2+b"}
	if !reflect.DeepEqual(got.NewIDs, wantIDs) {
		t.Errorf("newIDs = %#v, want %#v", got.NewIDs, wantIDs)
	}
}

func TestBuildReviewTask(t *testing.T) {
	task := BuildReviewTask(
		[]InlineComment{{ID: 12, Path: "x.go", Line: intp(3), Body: "rename"}},
		[]string{"review by a (CHANGES_REQUESTED): redo"},
		42,
	)
	for _, want := range []string{"PR #42", "[rc:12] x.go:3 — rename", "Additional context:", "redo", "Do NOT resolve threads"} {
		if !strings.Contains(task, want) {
			t.Errorf("review task missing %q\n%s", want, task)
		}
	}
}

func TestMergeSeen(t *testing.T) {
	got := MergeSeen([]string{"a", "b"}, []string{"b", "c", "c"})
	want := []string{"a", "b", "c"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("MergeSeen = %#v, want %#v", got, want)
	}
}
