package cli

import (
	"reflect"
	"strings"
	"testing"

	"github.com/anonx3247/pr-agents/internal/core"
)

func TestPlanDispatch(t *testing.T) {
	defaultMain := func() (string, error) { return "main", nil }
	noBranches := func(string) bool { return false }
	entries := []core.PrEntry{
		{ID: "p1", SessionID: "s", Branch: "pi/pr-first", Depth: 1, CreatedAt: "2026-01-01T00:00:00Z"},
		{ID: "p2", SessionID: "s", Branch: "pi/pr-second", Depth: 1, CreatedAt: "2026-01-02T00:00:00Z"},
	}

	tests := []struct {
		name       string
		opts       dispatchOpts
		entries    []core.PrEntry
		wantBranch string
		wantBase   string
		wantMode   core.Mode
		wantErr    bool
	}{
		{
			name:       "independent defaults to repo default branch",
			opts:       dispatchOpts{name: "My PR", mode: "independent"},
			wantBranch: "pi/pr-my-pr",
			wantBase:   "main",
			wantMode:   core.ModeIndependent,
		},
		{
			name:       "empty mode defaults to independent",
			opts:       dispatchOpts{name: "X"},
			wantBranch: "pi/pr-x",
			wantBase:   "main",
			wantMode:   core.ModeIndependent,
		},
		{
			name:       "explicit base wins",
			opts:       dispatchOpts{name: "X", mode: "independent", base: "develop"},
			wantBranch: "pi/pr-x",
			wantBase:   "develop",
			wantMode:   core.ModeIndependent,
		},
		{
			name:       "stack defaults base to most recent depth-1 entry",
			opts:       dispatchOpts{name: "Third", mode: "stack"},
			entries:    entries,
			wantBranch: "pi/pr-third",
			wantBase:   "pi/pr-second",
			wantMode:   core.ModeStack,
		},
		{
			name:       "graphite with stack-on by id",
			opts:       dispatchOpts{name: "Z", mode: "graphite", stackOn: "p1"},
			entries:    entries,
			wantBranch: "pi/pr-z",
			wantBase:   "pi/pr-first",
			wantMode:   core.ModeGraphite,
		},
		{
			name:       "explicit branch override",
			opts:       dispatchOpts{name: "X", branch: "custom/branch"},
			wantBranch: "custom/branch",
			wantBase:   "main",
			wantMode:   core.ModeIndependent,
		},
		{
			name:    "invalid mode errors",
			opts:    dispatchOpts{name: "X", mode: "bogus"},
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := planDispatch(tt.opts, tt.entries, "s", defaultMain, noBranches)
			if tt.wantErr {
				if err == nil {
					t.Fatal("expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("planDispatch error: %v", err)
			}
			if got.branch != tt.wantBranch || got.base != tt.wantBase || got.mode != tt.wantMode {
				t.Errorf("plan = %+v, want branch=%s base=%s mode=%s", got, tt.wantBranch, tt.wantBase, tt.wantMode)
			}
		})
	}
}

func TestPlanDispatchUniqueBranch(t *testing.T) {
	defaultMain := func() (string, error) { return "main", nil }
	taken := func(b string) bool { return b == "pi/pr-x" }
	got, err := planDispatch(dispatchOpts{name: "X"}, nil, "s", defaultMain, taken)
	if err != nil {
		t.Fatal(err)
	}
	if got.branch != "pi/pr-x-2" {
		t.Errorf("branch = %q, want pi/pr-x-2", got.branch)
	}
}

func TestBuildLaunchCommand(t *testing.T) {
	tests := []struct {
		name     string
		launcher string
		args     []string
		shell    string
		want     string
	}{
		{
			name:     "bare pi launcher",
			launcher: "pi",
			args:     []string{"do it", "-a", "--name", "PR: x"},
			shell:    "/bin/zsh",
			want:     "pi 'do it' '-a' '--name' 'PR: x'; exec /bin/zsh",
		},
		{
			name:     "multi-token sandbox launcher",
			launcher: "isara codex run",
			args:     []string{"task"},
			shell:    "",
			want:     "isara codex run 'task'; exec bash",
		},
		{
			name:     "quote escapes single quotes",
			launcher: "pi",
			args:     []string{"it's fine"},
			shell:    "bash",
			want:     `pi 'it'\''s fine'; exec bash`,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := buildLaunchCommand(tt.launcher, tt.args, tt.shell); got != tt.want {
				t.Errorf("buildLaunchCommand = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestBuildTaskMessage(t *testing.T) {
	msg := buildTaskMessage("My PR", "pi/pr-my-pr", "main", core.ModeGraphite, "build the thing", true)
	for _, want := range []string{
		"PR title: My PR",
		"Branch: pi/pr-my-pr (already checked out here)",
		"Base branch: main",
		"Stacking mode: graphite",
		"build the thing",
		"refactor: simplify",
		"pr-agents set-pr-number",
		"pr-agents mark-pushed",
		"pr-agents report-result",
	} {
		if !strings.Contains(msg, want) {
			t.Errorf("task message missing %q", want)
		}
	}
	// Without simplify, no simplify footer.
	msg2 := buildTaskMessage("X", "b", "main", core.ModeIndependent, "t", false)
	if strings.Contains(msg2, "refactor: simplify") {
		t.Error("non-simplify task should not mention refactor: simplify")
	}
}

func TestResolveStackRef(t *testing.T) {
	entries := []core.PrEntry{
		{ID: "p1", SessionID: "s", Branch: "b1", Depth: 1, CreatedAt: "2026-01-01T00:00:00Z"},
		{ID: "p2", SessionID: "s", Branch: "b2", Depth: 1, CreatedAt: "2026-01-03T00:00:00Z"},
		{ID: "h1", SessionID: "s", Branch: "h", Depth: 2, CreatedAt: "2026-01-09T00:00:00Z"},
		{ID: "o1", SessionID: "other", Branch: "x", Depth: 1, CreatedAt: "2026-01-09T00:00:00Z"},
	}
	t.Run("explicit ref", func(t *testing.T) {
		if got := resolveStackRef("p1", entries, "s"); got == nil || got.ID != "p1" {
			t.Errorf("got %v, want p1", got)
		}
	})
	t.Run("latest depth-1 in session", func(t *testing.T) {
		got := resolveStackRef("", entries, "s")
		if got == nil || got.ID != "p2" {
			t.Errorf("got %v, want p2 (ignores depth-2 and other sessions)", got)
		}
	})
	t.Run("no entries", func(t *testing.T) {
		if got := resolveStackRef("", nil, "s"); got != nil {
			t.Errorf("got %v, want nil", got)
		}
	})
}

func TestGenIDShape(t *testing.T) {
	id := genID()
	if len(id) != 8 {
		t.Errorf("genID len = %d, want 8", len(id))
	}
	if id != strings.ToLower(id) {
		t.Errorf("genID = %q, want lowercase hex", id)
	}
}

func TestPiBuildArgsThroughCommand(t *testing.T) {
	// Ensures the pi adapter's argv survives quoting unchanged in order.
	args := []string{"task", "-a", "--append-system-prompt", "INSTR", "--name", "PR: n"}
	got := buildLaunchCommand("pi", args, "bash")
	want := "pi 'task' '-a' '--append-system-prompt' 'INSTR' '--name' 'PR: n'; exec bash"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
	// reflect.DeepEqual guard on the slice itself (defensive).
	if !reflect.DeepEqual(args, []string{"task", "-a", "--append-system-prompt", "INSTR", "--name", "PR: n"}) {
		t.Error("buildLaunchCommand mutated its args slice")
	}
}

func TestDispatchPaneEnv(t *testing.T) {
	env := dispatchPaneEnv("sess1", "codex", "isara codex run")
	want := map[string]string{
		core.EnvSession:  "sess1",
		core.EnvHarness:  "codex",
		core.EnvLauncher: "isara codex run",
	}
	if !reflect.DeepEqual(env, want) {
		t.Fatalf("dispatchPaneEnv = %#v, want %#v", env, want)
	}
	// The worker-targeted PRA_* vars must NOT be emitted: a worker derives its
	// identity/depth from cwd→registry, so these would only mislead.
	for _, dropped := range []string{
		"PRA_DEPTH", "PRA_ID", "PRA_MODE", "PRA_BASE",
		"PRA_BRANCH", "PRA_NAME", "PRA_SIMPLIFY",
	} {
		if _, ok := env[dropped]; ok {
			t.Errorf("dispatch pane env must not emit %s", dropped)
		}
	}
}

func TestResolveHarnessLauncher(t *testing.T) {
	cases := []struct {
		name                      string
		flagH, flagL, envH, envL  string
		rec                       core.SessionRecord
		wantHarness, wantLauncher string
	}{
		{
			name:         "all empty falls back to pi + default launcher",
			wantHarness:  "pi",
			wantLauncher: "pi",
		},
		{
			name:         "env-stripped sandbox recovers from record",
			rec:          core.SessionRecord{Harness: "claude", Launcher: "isara claude run"},
			wantHarness:  "claude",
			wantLauncher: "isara claude run",
		},
		{
			name:         "env wins over record",
			envH:         "codex",
			envL:         "asb -- codex",
			rec:          core.SessionRecord{Harness: "claude", Launcher: "isara claude run"},
			wantHarness:  "codex",
			wantLauncher: "asb -- codex",
		},
		{
			name:         "explicit flags win over env and record",
			flagH:        "claude",
			flagL:        "wrap claude",
			envH:         "codex",
			envL:         "asb -- codex",
			rec:          core.SessionRecord{Harness: "pi", Launcher: "pi"},
			wantHarness:  "claude",
			wantLauncher: "wrap claude",
		},
		{
			name:         "record harness with no launcher falls back to that adapter default",
			rec:          core.SessionRecord{Harness: "codex"},
			wantHarness:  "codex",
			wantLauncher: "codex",
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			h, l, a, err := resolveHarnessLauncher(c.flagH, c.flagL, c.envH, c.envL, c.rec)
			if err != nil {
				t.Fatalf("resolveHarnessLauncher: %v", err)
			}
			if h != c.wantHarness || l != c.wantLauncher {
				t.Errorf("got harness=%q launcher=%q, want %q/%q", h, l, c.wantHarness, c.wantLauncher)
			}
			if a == nil {
				t.Errorf("expected non-nil adapter")
			}
		})
	}
}

func TestResolveDispatchSession(t *testing.T) {
	// Explicit --session wins even when the cwd fallback would derive a wrong id
	// from a stripped env (the sandbox case).
	if got := resolveDispatchSession("S_real", func() string { return "S_pi" }); got != "S_real" {
		t.Errorf("flag should win: got %q want S_real", got)
	}
	// With no flag, fall back to the cwd-derived id.
	if got := resolveDispatchSession("", func() string { return "S_fallback" }); got != "S_fallback" {
		t.Errorf("empty flag should use fallback: got %q want S_fallback", got)
	}
}
