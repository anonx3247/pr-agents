package core

import (
	"strings"
	"testing"
)

func intPtr(n int) *int { return &n }

func TestSlugify(t *testing.T) {
	tests := []struct{ in, want string }{
		{"Hello World", "hello-world"},
		{"Foo___Bar  Baz", "foo-bar-baz"},
		{"MixedCASE123", "mixedcase123"},
		{"  spaced  ", "spaced"},
		{"--leading-and-trailing--", "leading-and-trailing"},
		{"", "pr"},
		{"!!!@@@###", "pr"},
		{"café münchen", "caf-m-nchen"},
	}
	for _, tt := range tests {
		if got := Slugify(tt.in); got != tt.want {
			t.Errorf("Slugify(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
}

func TestSlugifyCapsAt48(t *testing.T) {
	if got := Slugify(strings.Repeat("a", 100)); len(got) != 48 {
		t.Errorf("Slugify(100 a's) length = %d, want 48", len(got))
	}
}

func TestShq(t *testing.T) {
	tests := []struct{ in, want string }{
		{"hello", "'hello'"},
		{"it's", `'it'\''s'`},
		{"a'b'c", `'a'\''b'\''c'`},
		{"", "''"},
	}
	for _, tt := range tests {
		if got := Shq(tt.in); got != tt.want {
			t.Errorf("Shq(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
}

func TestBuildEnv(t *testing.T) {
	got := BuildEnv(map[string]string{"B": "2", "A": "1", "C": "it's"})
	want := `A='1' B='2' C='it'\''s'`
	if got != want {
		t.Errorf("BuildEnv = %q, want %q", got, want)
	}
	if BuildEnv(nil) != "" {
		t.Errorf("BuildEnv(nil) = %q, want empty", BuildEnv(nil))
	}
}

func TestCapTail(t *testing.T) {
	if got := CapTail("hello", 10); got != "hello" {
		t.Errorf("CapTail within cap = %q", got)
	}
	if got := CapTail("exactly10!", 10); got != "exactly10!" {
		t.Errorf("CapTail at cap = %q", got)
	}
	out := CapTail("abcdefghij", 5)
	if len([]rune(out)) != 5 {
		t.Errorf("CapTail truncated rune-length = %d, want 5", len([]rune(out)))
	}
	if !strings.HasPrefix(out, "…") || !strings.HasSuffix(out, "j") {
		t.Errorf("CapTail = %q, want leading ellipsis and trailing j", out)
	}
	if got := CapTail("anything", 0); got != "" {
		t.Errorf("CapTail with cap 0 = %q, want empty", got)
	}
	// Unicode tail is preserved by rune, not byte.
	uni := CapTail("héllo wörld", 5)
	if len([]rune(uni)) != 5 {
		t.Errorf("CapTail unicode rune-length = %d, want 5", len([]rune(uni)))
	}
}

func TestPaneTitle(t *testing.T) {
	if got := (PaneTitle(PaneTitleArgs{PrNumber: intPtr(42), PrName: "add tests", Branch: "pi/tests"})); got != "PR#42 add tests (pi/tests)" {
		t.Errorf("PaneTitle with number = %q", got)
	}
	if got := (PaneTitle(PaneTitleArgs{PrName: "add tests", Branch: "pi/tests"})); got != "PR add tests (pi/tests)" {
		t.Errorf("PaneTitle without number = %q", got)
	}
}

func TestWindowName(t *testing.T) {
	if got := (WindowName(PaneTitleArgs{PrNumber: intPtr(12), PrName: "Add Rate Limiter", Branch: "pi/rate"})); got != "pr12-add-rate-limiter" {
		t.Errorf("WindowName with number = %q", got)
	}
	if got := (WindowName(PaneTitleArgs{PrName: "", Branch: "pi/feature"})); got != "pr-pi-feature" {
		t.Errorf("WindowName fallback to branch = %q", got)
	}
	got := WindowName(PaneTitleArgs{PrName: strings.Repeat("x", 80), Branch: "b"})
	if len(got) > 24 {
		t.Errorf("WindowName length = %d, want <= 24", len(got))
	}
	if strings.HasSuffix(got, "-") {
		t.Errorf("WindowName = %q, should not end with dash", got)
	}
}
