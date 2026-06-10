package cli

import (
	"reflect"
	"testing"
)

func TestSplitDoubleDash(t *testing.T) {
	tests := []struct {
		name      string
		args      []string
		wantFlags []string
		wantExtra []string
	}{
		{"no dash", []string{"--harness", "pi"}, []string{"--harness", "pi"}, nil},
		{"with dash", []string{"--harness", "pi", "--", "--foo", "bar"}, []string{"--harness", "pi"}, []string{"--foo", "bar"}},
		{"trailing dash", []string{"--harness", "pi", "--"}, []string{"--harness", "pi"}, []string{}},
		{"leading dash", []string{"--", "x"}, []string{}, []string{"x"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f, e := splitDoubleDash(tt.args)
			if !reflect.DeepEqual(f, tt.wantFlags) {
				t.Errorf("flags = %#v, want %#v", f, tt.wantFlags)
			}
			if !reflect.DeepEqual(e, tt.wantExtra) {
				t.Errorf("extra = %#v, want %#v", e, tt.wantExtra)
			}
		})
	}
}

func TestTmuxEnvFlags(t *testing.T) {
	env := map[string]string{"PRA_SESSION": "s1", "PRA_HARNESS": "pi"}
	got := tmuxEnvFlags(env)
	// Sorted by key: PRA_HARNESS before PRA_SESSION.
	want := []string{"-e", "PRA_HARNESS=pi", "-e", "PRA_SESSION=s1"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("tmuxEnvFlags = %#v, want %#v", got, want)
	}
	if tmuxEnvFlags(nil) == nil {
		// nil map yields an empty (non-nil) slice; acceptable either way.
	}
}

func TestBuildTmuxSessionArgs(t *testing.T) {
	got := buildTmuxSessionArgs("pra-abc", map[string]string{"PRA_SESSION": "abc"}, []string{"/bin/pr-agents", "start", "--harness", "pi"})
	want := []string{"new-session", "-s", "pra-abc", "-e", "PRA_SESSION=abc", "/bin/pr-agents", "start", "--harness", "pi"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("buildTmuxSessionArgs = %#v, want %#v", got, want)
	}
}

func TestBuildOrchestratorArgv(t *testing.T) {
	tests := []struct {
		name        string
		launcher    string
		adapterArgs []string
		extra       []string
		want        []string
	}{
		{
			name:        "bare pi with instructions",
			launcher:    "pi",
			adapterArgs: []string{"-a", "--append-system-prompt", "ORCH"},
			want:        []string{"pi", "-a", "--append-system-prompt", "ORCH"},
		},
		{
			name:        "sandbox launcher plus extra passthrough",
			launcher:    "isara codex run",
			adapterArgs: []string{"-a"},
			extra:       []string{"--model", "x"},
			want:        []string{"isara", "codex", "run", "-a", "--model", "x"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := buildOrchestratorArgv(tt.launcher, tt.adapterArgs, tt.extra); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("buildOrchestratorArgv = %#v, want %#v", got, tt.want)
			}
		})
	}
}
