package harness

import (
	"bufio"
	"encoding/json"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

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

// SessionRef locates the Codex session whose recorded cwd matches. Unlike pi and
// claude, Codex's store is NOT cwd-namespaced: it writes rollout files at
// ~/.codex/sessions/YYYY/MM/DD/rollout-<ts>-<uuid>.jsonl. We scan rollout files
// newest-first (mtime >= since), read the FIRST JSONL line (a
// `"type":"session_meta"` record), and stop at the first whose payload.cwd == cwd.
// The returned ref is payload.id (the session uuid), which `codex resume
// <SESSION_ID>` accepts. ok=false when the store is absent or nothing matches.
func (codexAdapter) SessionRef(cwd string, since time.Time) (string, bool) {
	home, err := sessionStoreHome()
	if err != nil {
		return "", false
	}
	root := filepath.Join(home, ".codex", "sessions")

	type rollout struct {
		path string
		mod  time.Time
	}
	var rollouts []rollout
	_ = filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return nil
		}
		name := d.Name()
		if !strings.HasPrefix(name, "rollout-") || !strings.HasSuffix(name, ".jsonl") {
			return nil
		}
		info, err := d.Info()
		if err != nil || info.ModTime().Before(since) {
			return nil
		}
		rollouts = append(rollouts, rollout{path: p, mod: info.ModTime()})
		return nil
	})
	sort.Slice(rollouts, func(i, j int) bool { return rollouts[i].mod.After(rollouts[j].mod) })

	for _, r := range rollouts {
		if id, ok := codexRolloutSessionID(r.path, cwd); ok {
			return id, true
		}
	}
	return "", false
}

// codexRolloutSessionID reads the first JSONL line of a rollout file and returns
// payload.id when it is a session_meta record whose payload.cwd matches cwd.
func codexRolloutSessionID(path, cwd string) (string, bool) {
	f, err := os.Open(path)
	if err != nil {
		return "", false
	}
	defer f.Close()

	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 0, 64*1024), 8*1024*1024)
	if !sc.Scan() {
		return "", false
	}
	var meta struct {
		Type    string `json:"type"`
		Payload struct {
			ID  string `json:"id"`
			Cwd string `json:"cwd"`
		} `json:"payload"`
	}
	if err := json.Unmarshal(sc.Bytes(), &meta); err != nil {
		return "", false
	}
	if meta.Type != "session_meta" || meta.Payload.Cwd != cwd {
		return "", false
	}
	return meta.Payload.ID, true
}

// BuildResumeArgs relaunches Codex resuming sessionRef in an interactive pane.
// Verified against `codex resume --help`: `codex resume [SESSION_ID] [PROMPT]`
// resumes a previous interactive session by uuid. We pass only the session id
// (no PROMPT seed) so the resumed session restores its own context. Note this
// is the SUBCOMMAND form, unlike BuildArgs' bare-positional launch. Pure: no IO.
func (codexAdapter) BuildResumeArgs(_ LaunchSpec, sessionRef string) []string {
	return []string{"resume", sessionRef}
}
