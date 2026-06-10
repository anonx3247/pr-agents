package cli

import (
	"bytes"
	"strings"
	"testing"
)

func TestRun(t *testing.T) {
	tests := []struct {
		name     string
		args     []string
		wantCode int
		inOut    string
		inErr    string
	}{
		{"version flag", []string{"--version"}, 0, "pr-agents " + Version, ""},
		{"version command", []string{"version"}, 0, "pr-agents " + Version, ""},
		{"list", []string{"list"}, 0, "No PR agents.", ""},
		{"dispatch needs name", []string{"dispatch"}, 2, "", "--name is required"},
		{"stub select", []string{"select"}, 1, "", "not implemented"},
		{"unknown command", []string{"bogus"}, 2, "", "unknown command"},
		{"no command", nil, 2, "", "Usage: pr-agents"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var out, errOut bytes.Buffer
			code := Run(tt.args, &out, &errOut)
			if code != tt.wantCode {
				t.Errorf("exit code = %d, want %d", code, tt.wantCode)
			}
			if tt.inOut != "" && !strings.Contains(out.String(), tt.inOut) {
				t.Errorf("stdout = %q, want substring %q", out.String(), tt.inOut)
			}
			if tt.inErr != "" && !strings.Contains(errOut.String(), tt.inErr) {
				t.Errorf("stderr = %q, want substring %q", errOut.String(), tt.inErr)
			}
		})
	}
}

func TestEveryStubCommandIsRegistered(t *testing.T) {
	want := []string{"dispatch", "list", "peek", "send", "stop", "focus", "cleanup", "context", "select", "daemon", "version", "start", "set-pr-number", "mark-pushed", "report-result", "reply-review"}
	for _, name := range want {
		if _, ok := commands[name]; !ok {
			t.Errorf("command %q not registered", name)
		}
	}
}
