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
		{"select with no agents", []string{"select"}, 0, "No live PR agents.", ""},
		{"unknown command", []string{"bogus"}, 2, "", "unknown command"},
		{"no command", nil, 2, "", "Usage: pr-agents"},
		{"tool usage", []string{"tool"}, 2, "", "Usage: pr-agents tool"},
		{"tool help", []string{"tool", "--help"}, 0, "Subcommands:", ""},
		{"tool unknown sub", []string{"tool", "bogus"}, 2, "", "unknown subcommand"},
		{"moved verb hint", []string{"context"}, 2, "", "moved to `pr-agents tool context`"},
		{"moved set-pr-number hint", []string{"set-pr-number", "1"}, 2, "", "moved to `pr-agents tool set-pr-number`"},
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
	want := []string{"dispatch", "list", "peek", "send", "stop", "focus", "cleanup", "select", "daemon", "version", "start", "tool"}
	for _, name := range want {
		if _, ok := commands[name]; !ok {
			t.Errorf("command %q not registered", name)
		}
	}
}

func TestWorkerPlumbingVerbsAreToolOnly(t *testing.T) {
	moved := []string{"context", "set-pr-number", "mark-pushed", "report-result", "reply-review"}
	for _, name := range moved {
		if _, ok := commands[name]; ok {
			t.Errorf("verb %q must not be registered top-level", name)
		}
		if _, ok := toolCommands[name]; !ok {
			t.Errorf("verb %q not registered under tool", name)
		}
	}
}

func TestToolContextDispatches(t *testing.T) {
	// `tool context` reaches runContext, which fails cleanly (exit 1) outside a
	// worktree. The "pr-agents context:" prefix proves the route landed on
	// runContext rather than the tool usage/unknown-subcommand paths.
	dir := t.TempDir()
	t.Chdir(dir)
	var out, errOut bytes.Buffer
	if code := Run([]string{"tool", "context"}, &out, &errOut); code != 1 {
		t.Fatalf("exit = %d, want 1; stderr=%s", code, errOut.String())
	}
	if !strings.Contains(errOut.String(), "pr-agents context:") {
		t.Errorf("stderr = %q, want pr-agents context: prefix", errOut.String())
	}
}
