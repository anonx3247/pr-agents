package harness

import (
	"reflect"
	"strings"
	"testing"
)

func TestGet(t *testing.T) {
	a, err := Get("pi")
	if err != nil {
		t.Fatalf("Get(pi) error: %v", err)
	}
	if a.Kind() != "pi" {
		t.Errorf("Kind() = %q, want pi", a.Kind())
	}
	if _, err := Get("bogus"); err == nil {
		t.Error("Get(bogus) = nil error, want unknown-harness error")
	}
}

func TestPiBuildArgs(t *testing.T) {
	a, _ := Get("pi")
	tests := []struct {
		name string
		spec LaunchSpec
		want []string
	}{
		{
			name: "full worker command",
			spec: LaunchSpec{Task: "do the thing", InstructionsText: "INSTR", PrName: "my pr"},
			want: []string{"do the thing", "-a", "--append-system-prompt", "INSTR", "--name", "PR: my pr"},
		},
		{
			name: "no instructions, no name",
			spec: LaunchSpec{Task: "t"},
			want: []string{"t", "-a"},
		},
		{
			name: "instructions only",
			spec: LaunchSpec{Task: "t", InstructionsText: "X"},
			want: []string{"t", "-a", "--append-system-prompt", "X"},
		},
		{
			name: "empty task (orchestrator) omits positional",
			spec: LaunchSpec{InstructionsText: "ORCH"},
			want: []string{"-a", "--append-system-prompt", "ORCH"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := a.BuildArgs(tt.spec, "")
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("BuildArgs() = %#v, want %#v", got, tt.want)
			}
		})
	}
}

func TestPiMetadata(t *testing.T) {
	a, _ := Get("pi")
	if a.DefaultLauncher() != "pi" {
		t.Errorf("DefaultLauncher() = %q, want pi", a.DefaultLauncher())
	}
	if a.InstructionMode() != InstructionFlag {
		t.Errorf("InstructionMode() = %q, want flag", a.InstructionMode())
	}
	if a.InstructionFileName() != "" {
		t.Errorf("InstructionFileName() = %q, want empty", a.InstructionFileName())
	}
}

func TestClaudeBuildArgs(t *testing.T) {
	a, _ := Get("claude")
	tests := []struct {
		name string
		spec LaunchSpec
		want []string
	}{
		{
			name: "full worker command",
			spec: LaunchSpec{Task: "do the thing", InstructionsText: "INSTR", PrName: "my pr"},
			want: []string{"do the thing", "--append-system-prompt", "INSTR", "--dangerously-skip-permissions"},
		},
		{
			name: "no instructions",
			spec: LaunchSpec{Task: "t"},
			want: []string{"t", "--dangerously-skip-permissions"},
		},
		{
			name: "empty task (orchestrator) omits positional",
			spec: LaunchSpec{InstructionsText: "ORCH"},
			want: []string{"--append-system-prompt", "ORCH", "--dangerously-skip-permissions"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := a.BuildArgs(tt.spec, "")
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("BuildArgs() = %#v, want %#v", got, tt.want)
			}
		})
	}
}

func TestClaudeMetadata(t *testing.T) {
	a, err := Get("claude")
	if err != nil {
		t.Fatalf("Get(claude) error: %v", err)
	}
	if a.Kind() != "claude" {
		t.Errorf("Kind() = %q, want claude", a.Kind())
	}
	if a.DefaultLauncher() != "claude" {
		t.Errorf("DefaultLauncher() = %q, want claude", a.DefaultLauncher())
	}
	if a.InstructionMode() != InstructionFlag {
		t.Errorf("InstructionMode() = %q, want flag", a.InstructionMode())
	}
	if a.InstructionFileName() != "" {
		t.Errorf("InstructionFileName() = %q, want empty", a.InstructionFileName())
	}
}

func TestCodexBuildArgs(t *testing.T) {
	a, _ := Get("codex")
	tests := []struct {
		name string
		spec LaunchSpec
		want []string
	}{
		{
			name: "full worker command (instructions via AGENTS.md, not argv)",
			spec: LaunchSpec{Task: "do the thing", InstructionsText: "INSTR", PrName: "my pr"},
			want: []string{"do the thing", "--dangerously-bypass-approvals-and-sandbox"},
		},
		{
			name: "no task (orchestrator) omits positional",
			spec: LaunchSpec{InstructionsText: "ORCH"},
			want: []string{"--dangerously-bypass-approvals-and-sandbox"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// instructionsPath is ignored: Codex loads AGENTS.md from cwd.
			got := a.BuildArgs(tt.spec, "/wt/AGENTS.md")
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("BuildArgs() = %#v, want %#v", got, tt.want)
			}
		})
	}
}

func TestCodexMetadata(t *testing.T) {
	a, err := Get("codex")
	if err != nil {
		t.Fatalf("Get(codex) error: %v", err)
	}
	if a.Kind() != "codex" {
		t.Errorf("Kind() = %q, want codex", a.Kind())
	}
	if a.DefaultLauncher() != "codex" {
		t.Errorf("DefaultLauncher() = %q, want codex", a.DefaultLauncher())
	}
	if a.InstructionMode() != InstructionFile {
		t.Errorf("InstructionMode() = %q, want file", a.InstructionMode())
	}
	if a.InstructionFileName() != "AGENTS.md" {
		t.Errorf("InstructionFileName() = %q, want AGENTS.md", a.InstructionFileName())
	}
}

func TestInstructions(t *testing.T) {
	tests := []struct {
		role     Role
		contains []string
	}{
		{RoleOrchestrator, []string{"PR-orchestrator", "pr-agents dispatch", "pr-agents cleanup"}},
		{RoleWorker, []string{"pr-agents tool context", "tool set-pr-number", "tool mark-pushed", "feature-base", "graphite", "gt track --parent feature-base feature-branch"}},
		{RoleHelper, []string{"helper subagent", "cannot"}},
	}
	for _, tt := range tests {
		t.Run(string(tt.role), func(t *testing.T) {
			out, err := Instructions(tt.role, InstructionData{Base: "feature-base", Branch: "feature-branch", Mode: "graphite"})
			if err != nil {
				t.Fatalf("Instructions(%s) error: %v", tt.role, err)
			}
			for _, sub := range tt.contains {
				if !strings.Contains(out, sub) {
					t.Errorf("Instructions(%s) missing %q", tt.role, sub)
				}
			}
		})
	}
}

func TestInstructionsWorkerInterpolation(t *testing.T) {
	out, err := Instructions(RoleWorker, InstructionData{Base: "main", Mode: "independent"})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(out, "{{") {
		t.Errorf("template not fully rendered: %q", out)
	}
	if !strings.Contains(out, "gh pr create --base main") {
		t.Error("worker template did not interpolate base branch")
	}
}

func TestInstructionsUnknownRole(t *testing.T) {
	if _, err := Instructions(Role("bogus"), InstructionData{}); err == nil {
		t.Error("expected error for unknown role")
	}
}
