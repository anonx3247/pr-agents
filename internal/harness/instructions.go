package harness

import (
	"embed"
	"fmt"
	"strings"
	"text/template"
)

// instructionFS holds the harness-agnostic role instruction templates. They are
// written in terms of the `pr-agents` CLI so they apply to any harness.
//
//go:embed instructions/*.md
var instructionFS embed.FS

// InstructionData is the small interpolation context for instruction templates.
// Only the worker template currently uses Base/Mode; the others ignore them.
type InstructionData struct {
	// Base is the PR's base branch.
	Base string
	// Branch is the PR's working branch (used by the graphite `gt track` step).
	Branch string
	// Mode is the stacking mode: independent | stack | graphite | helper.
	Mode string
	// Session, Harness, and Launcher carry the orchestrator's OWN resolved
	// identity into the orchestrator instructions so it dispatches with explicit
	// `--session/--harness/--launcher` flags. `start` resolves these OUTSIDE the
	// sandbox (where the real values are known) and injects them via argv/AGENTS,
	// which DO cross the sandbox boundary — unlike the PRA_* env vars. Only the
	// orchestrator template uses them.
	Session  string
	Harness  string
	Launcher string
}

// templateFor maps a role to its embedded template file name.
func templateFor(role Role) (string, bool) {
	switch role {
	case RoleOrchestrator:
		return "instructions/orchestrator.md", true
	case RoleWorker:
		return "instructions/worker.md", true
	case RoleHelper:
		return "instructions/helper.md", true
	default:
		return "", false
	}
}

// Instructions renders the instruction template for role, interpolating data.
// The result is the full instruction text to inject via flag or write to a file.
func Instructions(role Role, data InstructionData) (string, error) {
	name, ok := templateFor(role)
	if !ok {
		return "", fmt.Errorf("unknown role %q", role)
	}
	raw, err := instructionFS.ReadFile(name)
	if err != nil {
		return "", err
	}
	tmpl, err := template.New(string(role)).Parse(string(raw))
	if err != nil {
		return "", err
	}
	var sb strings.Builder
	if err := tmpl.Execute(&sb, data); err != nil {
		return "", err
	}
	return sb.String(), nil
}
