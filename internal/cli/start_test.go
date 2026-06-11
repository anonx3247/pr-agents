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

func TestResumeFlag(t *testing.T) {
	tests := []struct {
		name    string
		value   string // the value flag.Set passes
		wantSet bool
		wantID  string
	}{
		{"bare flag", "true", true, ""},
		{"explicit id", "abc123", true, "abc123"},
		{"empty value", "", true, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var r resumeFlag
			if err := r.Set(tt.value); err != nil {
				t.Fatalf("Set(%q) error: %v", tt.value, err)
			}
			if r.set != tt.wantSet || r.id != tt.wantID {
				t.Errorf("Set(%q) = {set:%v id:%q}, want {set:%v id:%q}", tt.value, r.set, r.id, tt.wantSet, tt.wantID)
			}
		})
	}
	// Zero value: not set, no id.
	var zero resumeFlag
	if zero.set || zero.id != "" {
		t.Errorf("zero value = {set:%v id:%q}, want {set:false id:\"\"}", zero.set, zero.id)
	}
}

func TestRewriteResumeForReexec(t *testing.T) {
	tests := []struct {
		name    string
		args    []string
		session string
		want    []string
	}{
		{"no resume appends explicit", []string{"start", "--harness", "pi"}, "abc", []string{"start", "--harness", "pi", "--resume=abc"}},
		{"bare resume replaced", []string{"start", "--resume"}, "abc", []string{"start", "--resume=abc"}},
		{"explicit resume replaced", []string{"start", "--resume=old", "--harness", "pi"}, "abc", []string{"start", "--harness", "pi", "--resume=abc"}},
		{"single dash resume stripped", []string{"start", "-resume=old"}, "abc", []string{"start", "--resume=abc"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := rewriteResumeForReexec(tt.args, tt.session); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("rewriteResumeForReexec(%#v, %q) = %#v, want %#v", tt.args, tt.session, got, tt.want)
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
