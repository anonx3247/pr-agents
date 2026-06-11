package harness

// claudeAdapter drives the Claude Code harness. Instructions are injected via
// the --append-system-prompt flag (no instruction FILE is written into the
// worktree, so the PR diff stays clean); the bare launcher is "claude".
//
// Flags verified against the installed Claude Code CLI (`claude --help`):
//   - a bare positional argument seeds the interactive session with that prompt
//     ("claude [options] [command] [prompt]", interactive by default);
//   - --append-system-prompt appends to the default system prompt;
//   - --permission-mode acceptEdits lets it apply edits autonomously in the
//     worktree without per-action approval (a valid choice alongside "default",
//     "auto", "bypassPermissions"). Verified on claude 2.1.170 that it is NOT
//     mutually exclusive with --dangerously-skip-permissions (the two coexist,
//     exit 0, no warning), and that a repeated --permission-mode is last-wins.
//     This default is therefore safely overridable: the CLI layer appends an
//     operator's passthrough args (`start ... -- <args>`) AFTER these, so a
//     later --permission-mode / --dangerously-skip-permissions takes effect.
//
// Sandboxing is intentionally NOT this adapter's concern. pr-agents never calls
// a sandbox tool; isolation is layered on by the operator overriding the
// configurable launcher PREFIX (e.g. "isara claude run" / "asb -- claude").
// Crucially, every subagent is its OWN separate launch through that same prefix
// (the orchestrator dispatches each worker via `pr-agents dispatch`, which
// re-applies the prefix per pane) — so a sandbox is re-imposed on each subagent
// rather than inherited from the parent process. We therefore use the scoped
// --permission-mode acceptEdits (edit-approval inside the already-isolated
// worktree) and deliberately NOT --dangerously-skip-permissions: the launcher
// prefix is the real trust boundary, and acceptEdits does not need to propagate
// to subagents because each pane is launched fresh with its own flags here.
type claudeAdapter struct{}

func init() { register(claudeAdapter{}) }

func (claudeAdapter) Kind() string { return "claude" }

func (claudeAdapter) DefaultLauncher() string { return "claude" }

func (claudeAdapter) InstructionMode() InstructionMode { return InstructionFlag }

func (claudeAdapter) InstructionFileName() string { return "" }

// BuildArgs mirrors the Claude Code worker command:
//
//	<task> --append-system-prompt <instructionsText> --permission-mode acceptEdits
//
// The task is the positional initial prompt seeding an interactive pane;
// instructions are passed inline via the flag, so instructionsPath is ignored.
// An empty Task is omitted so the orchestrator launches interactively (no seed
// message). Pure: no IO.
func (claudeAdapter) BuildArgs(spec LaunchSpec, _ string) []string {
	args := []string{}
	if spec.Task != "" {
		args = append(args, spec.Task)
	}
	if spec.InstructionsText != "" {
		args = append(args, "--append-system-prompt", spec.InstructionsText)
	}
	args = append(args, "--permission-mode", "acceptEdits")
	return args
}
