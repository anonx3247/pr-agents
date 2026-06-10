package cli

import (
	"crypto/rand"
	"encoding/hex"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/anonx3247/pr-agents/internal/core"
	"github.com/anonx3247/pr-agents/internal/harness"
	"github.com/anonx3247/pr-agents/internal/tmux"
)

// dispatchOpts holds the parsed dispatch flags.
type dispatchOpts struct {
	name     string
	task     string
	mode     string
	base     string
	stackOn  string
	branch   string
	simplify bool
	harness  string
	launcher string
}

// dispatchPlan is the resolved branch/base/mode for a dispatch, computed purely
// from the opts plus injectable git predicates so it can be unit-tested.
type dispatchPlan struct {
	branch string
	base   string
	mode   core.Mode
}

// planDispatch resolves the stacking mode, base branch, and a unique working
// branch. It is PURE given the injected predicates: defaultBranch resolves the
// repo default branch, and branchExists reports whether a branch already exists.
func planDispatch(
	o dispatchOpts,
	entries []core.PrEntry,
	session string,
	defaultBranch func() (string, error),
	branchExists core.BranchExists,
) (dispatchPlan, error) {
	mode := core.Mode(o.mode)
	if mode == "" {
		mode = core.ModeIndependent
	}
	switch mode {
	case core.ModeIndependent, core.ModeStack, core.ModeGraphite:
	default:
		return dispatchPlan{}, fmt.Errorf("invalid mode %q (want independent|stack|graphite)", o.mode)
	}

	base := o.base
	if base == "" && (mode == core.ModeStack || mode == core.ModeGraphite) {
		if ref := resolveStackRef(o.stackOn, entries, session); ref != nil {
			base = ref.Branch
		}
	}
	if base == "" {
		db, err := defaultBranch()
		if err != nil {
			return dispatchPlan{}, err
		}
		base = db
	}

	desired := o.branch
	if desired == "" {
		desired = "pi/pr-" + core.Slugify(o.name)
	}
	branch := core.UniqueBranch(desired, branchExists)

	return dispatchPlan{branch: branch, base: base, mode: mode}, nil
}

// resolveStackRef picks the entry a stacked PR builds on: the explicit stackOn
// ref if given, else the most recently dispatched depth-1 entry in this session.
func resolveStackRef(stackOn string, entries []core.PrEntry, session string) *core.PrEntry {
	if stackOn != "" {
		return core.FindEntry(entries, stackOn)
	}
	scoped := core.EntriesForSession(entries, session)
	var best *core.PrEntry
	for i := range scoped {
		if scoped[i].Depth != 1 {
			continue
		}
		if best == nil || scoped[i].CreatedAt > best.CreatedAt {
			best = &scoped[i]
		}
	}
	return best
}

// buildLaunchCommand assembles the pane shell command: the launcher tokens
// (unquoted command words) followed by the shell-quoted harness args, with a
// trailing `; exec <shell>` so the pane survives the harness exiting. Pure.
func buildLaunchCommand(launcher string, args []string, shell string) string {
	if shell == "" {
		shell = "bash"
	}
	parts := strings.Fields(launcher)
	for _, a := range args {
		parts = append(parts, core.Shq(a))
	}
	return strings.Join(parts, " ") + "; exec " + shell
}

// buildTaskMessage renders the seed task message handed to the worker.
func buildTaskMessage(name, branch, base string, mode core.Mode, task string, simplify bool) string {
	lines := []string{
		"You are the dedicated subagent for ONE pull request.",
		"",
		"PR title: " + name,
		"Branch: " + branch + " (already checked out here)",
		"Base branch: " + base,
		"Stacking mode: " + string(mode),
		"",
		"Task:",
		task,
		"",
	}
	footer := "Follow the pr-worker skill. Make an atomic commit after every coherent change."
	if simplify {
		footer += " Before opening the PR, simplify your diff and commit it as a `refactor: simplify` commit."
	}
	footer += " When you open the PR, run `pr-agents set-pr-number <n>` and `pr-agents mark-pushed`."
	lines = append(lines, footer)
	return strings.Join(lines, "\n")
}

// genID returns an 8-hex-char id for a new registry entry.
func genID() string {
	var b [4]byte
	if _, err := rand.Read(b[:]); err != nil {
		// Fall back to a timestamp-derived id; collisions are astronomically
		// unlikely and UniqueBranch already guards the branch namespace.
		return fmt.Sprintf("%08x", time.Now().UnixNano()&0xffffffff)
	}
	return hex.EncodeToString(b[:])
}

func runDispatch(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("dispatch", flag.ContinueOnError)
	fs.SetOutput(stderr)
	var o dispatchOpts
	var taskFile string
	fs.StringVar(&o.name, "name", "", "Short PR title (required)")
	fs.StringVar(&o.task, "task", "", "Full, self-contained instructions for the worker")
	fs.StringVar(&taskFile, "task-file", "", "Read the task from a file instead of --task")
	fs.StringVar(&o.mode, "mode", "independent", "Stacking mode: independent|stack|graphite")
	fs.StringVar(&o.base, "base", "", "Base branch (defaults to stack ref or repo default)")
	fs.StringVar(&o.stackOn, "stack-on", "", "PR id/branch to stack on (stack/graphite)")
	fs.StringVar(&o.branch, "branch", "", "Override the working branch name")
	fs.BoolVar(&o.simplify, "simplify", false, "Ask the worker to simplify its diff before opening")
	fs.StringVar(&o.harness, "harness", os.Getenv(core.EnvHarness), "Harness adapter: pi|claude|codex")
	fs.StringVar(&o.launcher, "launcher", os.Getenv(core.EnvLauncher), "Launch-command prefix before the harness args")
	if err := fs.Parse(args); err != nil {
		return 2
	}

	if o.name == "" {
		fmt.Fprintln(stderr, "pr-agents dispatch: --name is required")
		return 2
	}
	if taskFile != "" {
		raw, err := os.ReadFile(taskFile)
		if err != nil {
			fmt.Fprintf(stderr, "pr-agents dispatch: reading --task-file: %v\n", err)
			return 1
		}
		o.task = string(raw)
	}
	if strings.TrimSpace(o.task) == "" {
		fmt.Fprintln(stderr, "pr-agents dispatch: --task or --task-file is required")
		return 2
	}
	if !tmux.InsideTmux() {
		fmt.Fprintln(stderr, "pr-agents dispatch: not inside tmux; run `pr-agents start` first")
		return 1
	}

	if o.harness == "" {
		o.harness = "pi"
	}
	adapter, err := harness.Get(o.harness)
	if err != nil {
		fmt.Fprintf(stderr, "pr-agents dispatch: %v\n", err)
		return 1
	}
	if o.launcher == "" {
		o.launcher = adapter.DefaultLauncher()
	}

	cwd, err := os.Getwd()
	if err != nil {
		fmt.Fprintf(stderr, "pr-agents dispatch: %v\n", err)
		return 1
	}
	all, err := core.LoadRegistry(cwd)
	if err != nil {
		fmt.Fprintf(stderr, "pr-agents dispatch: %v\n", err)
		return 1
	}

	// Depth enforcement via cwd→registry (env is best-effort only). A helper
	// (depth 2) context must refuse to dispatch.
	ctxEntry := core.ResolveContextFromCwd(all, cwd)
	depth := core.Depth()
	parentID := "root"
	if ctxEntry != nil {
		depth = ctxEntry.Depth
		parentID = ctxEntry.ID
	}
	if depth >= 2 {
		fmt.Fprintln(stderr, "pr-agents dispatch: helpers (depth 2) cannot dispatch further subagents")
		return 1
	}

	root, err := core.RepoRoot(cwd)
	if err != nil {
		fmt.Fprintf(stderr, "pr-agents dispatch: %v\n", err)
		return 1
	}
	session := resolveSession(all, cwd)

	plan, err := planDispatch(o, all, session,
		func() (string, error) { return core.DefaultBranch(root) },
		core.GitBranchExists(root))
	if err != nil {
		fmt.Fprintf(stderr, "pr-agents dispatch: %v\n", err)
		return 1
	}

	// Keep .worktrees/ out of git status before creating anything in the repo.
	core.EnsureWorktreesIgnored(root)
	wtDir := core.WorktreesDirFrom(root)
	if err := os.MkdirAll(wtDir, 0o755); err != nil {
		fmt.Fprintf(stderr, "pr-agents dispatch: %v\n", err)
		return 1
	}
	worktree := filepath.Join(wtDir, core.Slugify(plan.branch))

	if err := core.AddWorktree(root, plan.branch, worktree, plan.base); err != nil {
		fmt.Fprintf(stderr, "pr-agents dispatch: failed to create worktree: %v\n", err)
		return 1
	}

	if plan.mode == core.ModeGraphite {
		// Best-effort: register the branch in the Graphite stack. A missing gt
		// is non-fatal — the worker falls back to gh.
		if err := core.GtTrack(worktree, plan.base); err != nil {
			fmt.Fprintf(stderr, "pr-agents dispatch: warning: gt track failed (%v); worker will fall back to gh\n", err)
		}
	}

	// Render the worker instructions and decide flag vs file injection.
	instructions, err := harness.Instructions(harness.RoleWorker, harness.InstructionData{
		Base: plan.base,
		Mode: string(plan.mode),
	})
	if err != nil {
		core.RemoveWorktree(root, worktree)
		core.DeleteBranch(root, plan.branch)
		fmt.Fprintf(stderr, "pr-agents dispatch: %v\n", err)
		return 1
	}

	spec := harness.LaunchSpec{
		Task:     buildTaskMessage(o.name, plan.branch, plan.base, plan.mode, o.task, o.simplify),
		PrName:   o.name,
		Worktree: worktree,
	}
	instructionsPath := ""
	if adapter.InstructionMode() == harness.InstructionFile {
		instructionsPath = filepath.Join(worktree, adapter.InstructionFileName())
		if err := os.WriteFile(instructionsPath, []byte(instructions), 0o644); err != nil {
			core.RemoveWorktree(root, worktree)
			core.DeleteBranch(root, plan.branch)
			fmt.Fprintf(stderr, "pr-agents dispatch: writing instructions: %v\n", err)
			return 1
		}
	} else {
		spec.InstructionsText = instructions
	}

	id := genID()
	simplifyEnv := "0"
	if o.simplify {
		simplifyEnv = "1"
	}
	env := map[string]string{
		core.EnvDepth:    fmt.Sprintf("%d", depth+1),
		core.EnvSession:  session,
		core.EnvID:       id,
		core.EnvHarness:  o.harness,
		core.EnvLauncher: o.launcher,
		core.EnvSimplify: simplifyEnv,
		core.EnvMode:     string(plan.mode),
		core.EnvBase:     plan.base,
		core.EnvBranch:   plan.branch,
		core.EnvName:     o.name,
	}
	spec.Env = env

	command := buildLaunchCommand(o.launcher, adapter.BuildArgs(spec, instructionsPath), os.Getenv("SHELL"))
	title := core.PaneTitle(core.PaneTitleArgs{PrName: o.name, Branch: plan.branch})
	window := core.WindowName(core.PaneTitleArgs{PrName: o.name, Branch: plan.branch})

	paneID, err := tmux.OpenWindow(worktree, command, title, window, env)
	if err != nil {
		core.RemoveWorktree(root, worktree)
		core.DeleteBranch(root, plan.branch)
		fmt.Fprintf(stderr, "pr-agents dispatch: failed to open tmux window: %v\n", err)
		return 1
	}

	entry := core.PrEntry{
		ID:        id,
		SessionID: session,
		PrName:    o.name,
		Branch:    plan.branch,
		Base:      plan.base,
		Mode:      plan.mode,
		PaneID:    paneID,
		Worktree:  worktree,
		Depth:     depth + 1,
		ParentID:  parentID,
		Simplify:  o.simplify,
		Status:    core.StatusWorking,
		CreatedAt: nowRFC3339(),
	}
	if err := core.SaveRegistry(cwd, append(all, entry)); err != nil {
		fmt.Fprintf(stderr, "pr-agents dispatch: %v\n", err)
		return 1
	}

	fmt.Fprintf(stdout, "Dispatched PR subagent:\n")
	fmt.Fprintf(stdout, "  id:       %s\n", id)
	fmt.Fprintf(stdout, "  name:     %s\n", o.name)
	fmt.Fprintf(stdout, "  branch:   %s\n", plan.branch)
	fmt.Fprintf(stdout, "  base:     %s\n", plan.base)
	fmt.Fprintf(stdout, "  mode:     %s\n", plan.mode)
	fmt.Fprintf(stdout, "  worktree: %s\n", worktree)
	fmt.Fprintf(stdout, "  pane:     %s\n", paneID)
	return 0
}
