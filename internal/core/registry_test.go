package core

import (
	"reflect"
	"testing"
)

func mkEntry(id string) PrEntry {
	return PrEntry{
		ID:        id,
		PrName:    "pr " + id,
		Branch:    "pi/" + id,
		Base:      "main",
		Mode:      ModeStack,
		PaneID:    "%1",
		Worktree:  "/tmp",
		Depth:     1,
		Status:    StatusWorking,
		CreatedAt: "2026-01-01T00:00:00.000Z",
	}
}

func TestRegistryRoundTrip(t *testing.T) {
	dir := initRepo(t)
	entries := []PrEntry{mkEntry("aaa"), mkEntry("bbb")}
	if err := SaveRegistry(dir, entries); err != nil {
		t.Fatalf("SaveRegistry: %v", err)
	}
	got, err := LoadRegistry(dir)
	if err != nil {
		t.Fatalf("LoadRegistry: %v", err)
	}
	if !reflect.DeepEqual(got, entries) {
		t.Errorf("round-trip mismatch:\n got %+v\nwant %+v", got, entries)
	}
}

func TestLoadRegistryEmptyWhenMissing(t *testing.T) {
	dir := initRepo(t)
	got, err := LoadRegistry(dir)
	if err != nil {
		t.Fatalf("LoadRegistry: %v", err)
	}
	if len(got) != 0 {
		t.Errorf("expected empty, got %+v", got)
	}
}

func TestUpdateEntry(t *testing.T) {
	dir := initRepo(t)
	if err := SaveRegistry(dir, []PrEntry{mkEntry("aaa")}); err != nil {
		t.Fatal(err)
	}
	n := 13
	updated, ok, err := UpdateEntry(dir, "aaa", func(e *PrEntry) {
		e.PrNumber = &n
		e.Status = StatusOpen
	})
	if err != nil || !ok {
		t.Fatalf("UpdateEntry ok=%v err=%v", ok, err)
	}
	if updated.PrNumber == nil || *updated.PrNumber != 13 || updated.Status != StatusOpen {
		t.Errorf("updated = %+v", updated)
	}
	reloaded, _ := LoadRegistry(dir)
	if reloaded[0].PrNumber == nil || *reloaded[0].PrNumber != 13 {
		t.Errorf("reloaded PrNumber = %v", reloaded[0].PrNumber)
	}
}

func TestUpdateEntryUnknownID(t *testing.T) {
	dir := initRepo(t)
	if err := SaveRegistry(dir, []PrEntry{mkEntry("aaa")}); err != nil {
		t.Fatal(err)
	}
	_, ok, err := UpdateEntry(dir, "missing", func(*PrEntry) {})
	if err != nil {
		t.Fatal(err)
	}
	if ok {
		t.Error("expected ok=false for unknown id")
	}
}

func TestEntriesForSession(t *testing.T) {
	e := func(id, sid string) PrEntry { x := mkEntry(id); x.SessionID = sid; return x }
	entries := []PrEntry{e("a", "s1"), e("b", "s2"), e("c", "s1"), e("legacy", "")}

	got := EntriesForSession(entries, "s1")
	if ids := idsOf(got); !reflect.DeepEqual(ids, []string{"a", "c"}) {
		t.Errorf("s1 ids = %v, want [a c]", ids)
	}
	if got := EntriesForSession(entries, "other"); len(got) != 0 {
		t.Errorf("unmatched session = %v, want empty", got)
	}
	if got := EntriesForSession(entries, ""); !reflect.DeepEqual(idsOf(got), []string{"legacy"}) {
		t.Errorf("empty session ids = %v, want [legacy]", idsOf(got))
	}
	// Does not mutate input.
	if len(entries) != 4 {
		t.Errorf("input mutated, len = %d", len(entries))
	}
}

func TestFindEntry(t *testing.T) {
	n := 7
	entries := []PrEntry{
		{ID: "abcdef12-3456-7890", PrName: "feature one", Branch: "pi/feature-one", PrNumber: &n},
		{ID: "99999999-0000", PrName: "feature two", Branch: "pi/feature-two"},
	}
	cases := []struct {
		ref  string
		want string // matched PrName, or "" for nil
	}{
		{"abcdef12-3456-7890", "feature one"},
		{"abcdef12", "feature one"},
		{"pi/feature-two", "feature two"},
		{"feature one", "feature one"},
		{"7", "feature one"},
		{"#7", "feature one"},
		{"nope", ""},
		{"#999", ""},
	}
	for _, c := range cases {
		got := FindEntry(entries, c.ref)
		if c.want == "" {
			if got != nil {
				t.Errorf("FindEntry(%q) = %+v, want nil", c.ref, got)
			}
			continue
		}
		if got == nil || got.PrName != c.want {
			t.Errorf("FindEntry(%q) = %v, want %q", c.ref, got, c.want)
		}
	}
}

func idsOf(entries []PrEntry) []string {
	ids := make([]string, len(entries))
	for i, e := range entries {
		ids[i] = e.ID
	}
	return ids
}
