package cli

import (
	"strings"
	"testing"

	"github.com/anonx3247/pr-agents/internal/core"
)

func TestParseSelection(t *testing.T) {
	cases := []struct {
		name    string
		input   string
		n       int
		wantIdx int
		wantOk  bool
	}{
		{"first pick", "1\n", 3, 0, true},
		{"last pick", "3", 3, 2, true},
		{"surrounding whitespace", "  2  \n", 3, 1, true},
		{"trailing carriage return", "2\r\n", 5, 1, true},
		{"blank cancels", "\n", 3, -1, false},
		{"q cancels", "q", 3, -1, false},
		{"quit cancels", "QUIT", 3, -1, false},
		{"cancel word", "cancel", 3, -1, false},
		{"zero out of range", "0", 3, -1, false},
		{"too high", "4", 3, -1, false},
		{"non-numeric", "abc", 3, -1, false},
		{"empty list", "1", 0, -1, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			idx, ok := parseSelection(tc.input, tc.n)
			if idx != tc.wantIdx || ok != tc.wantOk {
				t.Errorf("parseSelection(%q, %d) = (%d, %t), want (%d, %t)",
					tc.input, tc.n, idx, ok, tc.wantIdx, tc.wantOk)
			}
		})
	}
}

func TestLiveEntries(t *testing.T) {
	alive := func(id string) bool { return id == "%live" }
	entries := []core.PrEntry{
		{ID: "a", PaneID: "%live"},
		{ID: "b", PaneID: "%dead"},
		{ID: "c", PaneID: ""},
		{ID: "d", PaneID: "%live"},
	}
	live := liveEntries(entries, alive)
	if len(live) != 2 {
		t.Fatalf("live = %d, want 2", len(live))
	}
	if live[0].ID != "a" || live[1].ID != "d" {
		t.Errorf("order not preserved: got %s, %s", live[0].ID, live[1].ID)
	}
}

func TestSelectLabel(t *testing.T) {
	withPR := core.PrEntry{PrNumber: intp(7), PrName: "Feat", Branch: "br-a"}
	if got := selectLabel(withPR); got != "#7 Feat (br-a)" {
		t.Errorf("got %q", got)
	}
	noPR := core.PrEntry{PrName: "Draft", Branch: "br-b"}
	if got := selectLabel(noPR); got != "- Draft (br-b)" {
		t.Errorf("got %q", got)
	}
}

func TestSelectFrom(t *testing.T) {
	origFocus := focusPane
	t.Cleanup(func() { focusPane = origFocus })

	live := []core.PrEntry{
		{PrNumber: intp(1), PrName: "One", Branch: "br-1", PaneID: "%1"},
		{PrName: "Two", Branch: "br-2", PaneID: "%2"},
	}

	t.Run("empty list", func(t *testing.T) {
		var out, errb strings.Builder
		if code := selectFrom(nil, strings.NewReader(""), &out, &errb); code != 0 {
			t.Fatalf("code = %d, want 0", code)
		}
		if !strings.Contains(out.String(), "No live PR agents.") {
			t.Errorf("missing empty message: %q", out.String())
		}
	})

	t.Run("valid pick focuses chosen pane", func(t *testing.T) {
		var focused string
		focusPane = func(id string) bool { focused = id; return true }
		var out, errb strings.Builder
		if code := selectFrom(live, strings.NewReader("2\n"), &out, &errb); code != 0 {
			t.Fatalf("code = %d, want 0", code)
		}
		if focused != "%2" {
			t.Errorf("focused = %q, want %%2", focused)
		}
		if !strings.Contains(out.String(), "Focused Two") {
			t.Errorf("missing focus message: %q", out.String())
		}
	})

	t.Run("cancel does not focus", func(t *testing.T) {
		called := false
		focusPane = func(string) bool { called = true; return true }
		var out, errb strings.Builder
		if code := selectFrom(live, strings.NewReader("\n"), &out, &errb); code != 0 {
			t.Fatalf("code = %d, want 0", code)
		}
		if called {
			t.Error("focus should not be called on cancel")
		}
		if !strings.Contains(out.String(), "Cancelled.") {
			t.Errorf("missing cancel message: %q", out.String())
		}
	})

	t.Run("dead pane errors", func(t *testing.T) {
		focusPane = func(string) bool { return false }
		var out, errb strings.Builder
		if code := selectFrom(live, strings.NewReader("1\n"), &out, &errb); code != 1 {
			t.Fatalf("code = %d, want 1", code)
		}
		if !strings.Contains(errb.String(), "no longer exists") {
			t.Errorf("missing error message: %q", errb.String())
		}
	})
}
