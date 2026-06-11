package harness

// codexAdapter drives the Codex CLI harness. Codex has no
// --append-system-prompt flag; instead it auto-loads an AGENTS.md file from its
// working directory. So instructions are injected via a FILE: dispatch/start
// write AGENTS.md into the worktree (and exclude it from git so the PR diff
// stays clean), and Codex picks it up automatically. The bare launcher is
// "codex".
//
// Flags verified against the installed Codex CLI (`codex --help`, v0.x):
//   - "codex [OPTIONS] [PROMPT]" — a bare positional argument seeds an
//     INTERACTIVE session with that prompt (no subcommand means options/prompt
//     are forwarded to the interactive TUI; `codex exec` is the headless path we
//     deliberately avoid so a human can watch/steer the pane);
//   - --dangerously-bypass-approvals-and-sandbox — "Skip all confirmation
//     prompts and execute commands without sandboxing", i.e. fully autonomous
//     "YOLO" mode with no per-action approval prompts. We use it deliberately:
//     a worker runs unattended in an ISOLATED worktree, and any real OS-level
//     sandbox is re-imposed per pane via the launcher PREFIX (e.g.
//     "isara codex run") — codex's own in-process sandbox is not the trust
//     boundary here, so bypassing it lets the worker make progress without
//     stopping for approvals;
//   - Codex reads AGENTS.md from its working root automatically, which is how
//     the file-injected instructions reach it (no argv flag needed).
type codexAdapter struct{}

func init() { register(codexAdapter{}) }

func (codexAdapter) Kind() string { return "codex" }

func (codexAdapter) DefaultLauncher() string { return "codex" }

func (codexAdapter) InstructionMode() InstructionMode { return InstructionFile }

func (codexAdapter) InstructionFileName() string { return "AGENTS.md" }

// BuildArgs mirrors the Codex worker command:
//
//	<task> --dangerously-bypass-approvals-and-sandbox
//
// The task is the positional initial prompt seeding an interactive pane.
// Instructions are NOT passed on argv — they reach Codex via the AGENTS.md file
// written into the worktree by dispatch/start, so instructionsPath is ignored
// here. An empty Task is omitted so the orchestrator launches interactively (no
// seed message). Pure: no IO.
func (codexAdapter) BuildArgs(spec LaunchSpec, _ string) []string {
	args := []string{}
	if spec.Task != "" {
		args = append(args, spec.Task)
	}
	args = append(args, "--dangerously-bypass-approvals-and-sandbox")
	return args
}
