package harness

// piAdapter drives the pi harness. Instructions are injected via the
// --append-system-prompt flag; the bare launcher is "pi".
type piAdapter struct{}

func init() { register(piAdapter{}) }

func (piAdapter) Kind() string { return "pi" }

func (piAdapter) DefaultLauncher() string { return "pi" }

func (piAdapter) InstructionMode() InstructionMode { return InstructionFlag }

func (piAdapter) InstructionFileName() string { return "" }

// BuildArgs mirrors the pi worker command:
//
//	<task> -a --append-system-prompt <instructionsText> --name "PR: <prName>"
//
// "-a" trusts project-local files for this run (a dispatched worktree of a repo
// the user already chose to work in). The instructions are passed inline via
// the flag, so instructionsPath is ignored. An empty Task is omitted so the
// orchestrator launches interactively (no seed message). Pure: no IO.
func (piAdapter) BuildArgs(spec LaunchSpec, _ string) []string {
	args := []string{}
	if spec.Task != "" {
		args = append(args, spec.Task)
	}
	args = append(args, "-a")
	if spec.InstructionsText != "" {
		args = append(args, "--append-system-prompt", spec.InstructionsText)
	}
	if spec.PrName != "" {
		args = append(args, "--name", "PR: "+spec.PrName)
	}
	return args
}
