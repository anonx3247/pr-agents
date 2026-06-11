package core

import "testing"

func TestParseGraphitePrInfos(t *testing.T) {
	cases := []struct {
		name string
		raw  string
		want []GraphitePrInfo
	}{
		{name: "empty bytes", raw: "", want: nil},
		{name: "garbage json", raw: "}{not json", want: nil},
		{name: "null", raw: "null", want: nil},
		{name: "non-object top", raw: "[1,2,3]", want: nil},
		{name: "missing prInfos", raw: `{"foo":1}`, want: []GraphitePrInfo{}},
		{name: "prInfos not array", raw: `{"prInfos":"nope"}`, want: []GraphitePrInfo{}},
		{
			name: "skips null and non-object entries",
			raw:  `{"prInfos":[null, 42, "x", {"prNumber":7,"state":"OPEN"}]}`,
			want: []GraphitePrInfo{{PrNumber: 7, State: "OPEN"}},
		},
		{
			name: "skips entries with no number",
			raw:  `{"prInfos":[{"state":"OPEN"},{"number":3,"branch":"b"}]}`,
			want: []GraphitePrInfo{{PrNumber: 3, Branch: "b", State: ""}},
		},
		{
			name: "prefers prNumber over number; branch spellings",
			raw:  `{"prInfos":[{"prNumber":9,"number":1,"branchName":"feat","state":"MERGED","title":"t","url":"u"}]}`,
			want: []GraphitePrInfo{{PrNumber: 9, Branch: "feat", State: "MERGED", Title: "t", URL: "u"}},
		},
		{
			name: "branch fallbacks headRefName then branch",
			raw:  `{"prInfos":[{"number":2,"headRefName":"h"},{"number":4,"branch":"plain"}]}`,
			want: []GraphitePrInfo{{PrNumber: 2, Branch: "h"}, {PrNumber: 4, Branch: "plain"}},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := ParseGraphitePrInfos([]byte(tc.raw))
			if len(got) != len(tc.want) {
				t.Fatalf("got %d infos, want %d: %#v", len(got), len(tc.want), got)
			}
			for i := range got {
				if got[i] != tc.want[i] {
					t.Errorf("info[%d] = %#v, want %#v", i, got[i], tc.want[i])
				}
			}
		})
	}
}

func TestClassifyGraphitePrState(t *testing.T) {
	cases := map[string]PrStateClass{
		"MERGED":  PrStateMerged,
		"merged":  PrStateMerged,
		"Closed":  PrStateClosed,
		"OPEN":    PrStateOpen,
		"open":    PrStateOpen,
		"":        PrStateUnknown,
		"DRAFT":   PrStateUnknown,
		"unknown": PrStateUnknown,
	}
	for state, want := range cases {
		if got := ClassifyGraphitePrState(state); got != want {
			t.Errorf("ClassifyGraphitePrState(%q) = %q, want %q", state, got, want)
		}
	}
}
