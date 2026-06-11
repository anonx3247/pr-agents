package core

import (
	"testing"
)

func TestDepthFrom(t *testing.T) {
	tests := []struct {
		raw  string
		want int
	}{
		{"", 0},
		{"0", 0},
		{"1", 1},
		{"2", 2},
		{"42", 42},
		{"-1", -1},
		{"notanumber", 0},
		{"1.5", 0},
		{" 1", 0},
	}
	for _, tt := range tests {
		if got := depthFrom(tt.raw); got != tt.want {
			t.Errorf("depthFrom(%q) = %d, want %d", tt.raw, got, tt.want)
		}
	}
}

func TestHarnessArgsRoundTrip(t *testing.T) {
	tests := []struct {
		name string
		args []string
	}{
		{"nil", nil},
		{"empty", []string{}},
		{"single flag", []string{"--full-auto"}},
		{"flag with value", []string{"--model", "o3"}},
		{"arg with spaces and quotes", []string{"--system", `say "hi" now`}},
		{"many", []string{"--full-auto", "--model", "gpt-5", "--search"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := DecodeHarnessArgs(EncodeHarnessArgs(tt.args))
			if len(tt.args) == 0 {
				if got != nil {
					t.Errorf("round-trip of empty = %#v, want nil", got)
				}
				return
			}
			if len(got) != len(tt.args) {
				t.Fatalf("round-trip = %#v, want %#v", got, tt.args)
			}
			for i := range tt.args {
				if got[i] != tt.args[i] {
					t.Errorf("arg[%d] = %q, want %q", i, got[i], tt.args[i])
				}
			}
		})
	}
}

func TestEncodeHarnessArgsEmpty(t *testing.T) {
	if s := EncodeHarnessArgs(nil); s != "" {
		t.Errorf("EncodeHarnessArgs(nil) = %q, want empty", s)
	}
	if s := EncodeHarnessArgs([]string{}); s != "" {
		t.Errorf("EncodeHarnessArgs([]) = %q, want empty", s)
	}
}

func TestDecodeHarnessArgsMalformed(t *testing.T) {
	for _, raw := range []string{"not json", "{}", "[1,2]"} {
		if got := DecodeHarnessArgs(raw); got != nil {
			t.Errorf("DecodeHarnessArgs(%q) = %#v, want nil", raw, got)
		}
	}
}

func TestDepthReadsEnv(t *testing.T) {
	t.Setenv(EnvDepth, "1")
	if got := Depth(); got != 1 {
		t.Errorf("Depth() = %d, want 1", got)
	}
	t.Setenv(EnvDepth, "")
	if got := Depth(); got != 0 {
		t.Errorf("Depth() with empty env = %d, want 0", got)
	}
}

func TestEnvConstantsArePrefixed(t *testing.T) {
	for _, name := range []string{
		EnvDepth, EnvSession, EnvID, EnvMode, EnvBase, EnvBranch, EnvName, EnvSimplify, EnvHarness,
		EnvLauncher, EnvHarnessArgs,
	} {
		if len(name) < 5 || name[:4] != "PRA_" {
			t.Errorf("env constant %q is not PRA_-prefixed", name)
		}
	}
}
