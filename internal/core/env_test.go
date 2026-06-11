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
		EnvLauncher,
	} {
		if len(name) < 5 || name[:4] != "PRA_" {
			t.Errorf("env constant %q is not PRA_-prefixed", name)
		}
	}
}
