package core

import (
	"reflect"
	"testing"
)

func strPtr(s string) *string { return &s }

func TestIsPollable(t *testing.T) {
	n := 7
	base := func(patch func(*PrEntry)) PrEntry {
		e := PrEntry{Depth: 1, Pushed: true, PrNumber: &n, Status: StatusOpen}
		patch(&e)
		return e
	}
	tests := []struct {
		name string
		e    PrEntry
		want bool
	}{
		{"pushed+number+open", base(func(*PrEntry) {}), true},
		{"not pushed", base(func(e *PrEntry) { e.Pushed = false }), false},
		{"merged terminal", base(func(e *PrEntry) { e.Status = StatusMerged }), false},
		{"closed terminal", base(func(e *PrEntry) { e.Status = StatusClosed }), false},
		{"stopped terminal", base(func(e *PrEntry) { e.Status = StatusStopped }), false},
		{"missing number", base(func(e *PrEntry) { e.PrNumber = nil }), false},
		{"non-depth-1", base(func(e *PrEntry) { e.Depth = 2 }), false},
	}
	for _, tt := range tests {
		if got := IsPollable(tt.e); got != tt.want {
			t.Errorf("%s: IsPollable = %v, want %v", tt.name, got, tt.want)
		}
	}
}

func TestClassifyPrState(t *testing.T) {
	tests := []struct {
		name string
		j    *PrStateJSON
		want PrStateClass
	}{
		{"mergedAt wins over state", &PrStateJSON{State: "OPEN", MergedAt: strPtr("2026-01-01")}, PrStateMerged},
		{"MERGED", &PrStateJSON{State: "MERGED"}, PrStateMerged},
		{"CLOSED", &PrStateJSON{State: "CLOSED"}, PrStateClosed},
		{"OPEN", &PrStateJSON{State: "OPEN"}, PrStateOpen},
		{"lowercase open", &PrStateJSON{State: "open"}, PrStateOpen},
		{"nil", nil, PrStateUnknown},
		{"DRAFT", &PrStateJSON{State: "DRAFT"}, PrStateUnknown},
		{"empty", &PrStateJSON{}, PrStateUnknown},
	}
	for _, tt := range tests {
		if got := ClassifyPrState(tt.j); got != tt.want {
			t.Errorf("%s: ClassifyPrState = %q, want %q", tt.name, got, tt.want)
		}
	}
}

func TestSelectStateTransitions(t *testing.T) {
	mk := func(id string) PrEntry { return PrEntry{ID: id} }
	classified := []ClassifiedEntry{
		{Entry: mk("a"), State: PrStateMerged},
		{Entry: mk("b"), State: PrStateClosed},
		{Entry: mk("c"), State: PrStateOpen},
	}
	out := SelectStateTransitions(classified, map[string]PrStateClass{"a": PrStateOpen})
	type pair struct {
		id    string
		state PrStateClass
	}
	got := make([]pair, len(out))
	for i, tr := range out {
		got[i] = pair{tr.Entry.ID, tr.State}
	}
	want := []pair{{"a", PrStateMerged}, {"b", PrStateClosed}}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("transitions = %v, want %v", got, want)
	}

	// Dedup: already known terminal => skip.
	dedup := SelectStateTransitions(
		[]ClassifiedEntry{{Entry: mk("a"), State: PrStateMerged}},
		map[string]PrStateClass{"a": PrStateMerged},
	)
	if len(dedup) != 0 {
		t.Errorf("dedup = %v, want empty", dedup)
	}

	// Non-terminal states ignored.
	none := SelectStateTransitions(
		[]ClassifiedEntry{{Entry: mk("a"), State: PrStateOpen}, {Entry: mk("b"), State: PrStateUnknown}},
		nil,
	)
	if len(none) != 0 {
		t.Errorf("non-terminal = %v, want empty", none)
	}
}

func TestParseChangedFiles(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want []ChangedFile
	}{
		{"empty", "", []ChangedFile{}},
		{"modified and added", "M\tsrc/a.go\nA\tsrc/b.go", []ChangedFile{
			{"src/a.go", ChangedModified}, {"src/b.go", ChangedAdded},
		}},
		{"renamed uses new path", "R100\told.go\tnew.go", []ChangedFile{
			{"new.go", ChangedRenamed},
		}},
		{"copied uses new path", "C075\tsrc.go\tcopy.go", []ChangedFile{
			{"copy.go", ChangedCopied},
		}},
		{"skips blank and unknown", "\nD\tgone.go\nM\tkept.go\n", []ChangedFile{
			{"kept.go", ChangedModified},
		}},
		{"renamed missing new path is skipped", "R100\tonly.go", []ChangedFile{}},
	}
	for _, tt := range tests {
		got := ParseChangedFiles(tt.in)
		if !reflect.DeepEqual(got, tt.want) {
			t.Errorf("%s: ParseChangedFiles = %+v, want %+v", tt.name, got, tt.want)
		}
	}
}
