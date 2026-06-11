package cli

import (
	"flag"
	"fmt"
	"io"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/anonx3247/pr-agents/internal/core"
	"github.com/anonx3247/pr-agents/internal/daemon"
	"github.com/anonx3247/pr-agents/internal/tmux"
)

// runDaemon runs the per-session background daemon: a long-lived loop that polls
// GitHub PR state, CI checks, and review activity, driving all cross-agent
// messaging via tmux send-keys. It is resilient (every tick is wrapped) and
// exits cleanly when not inside tmux (nothing to drive).
func runDaemon(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("daemon", flag.ContinueOnError)
	fs.SetOutput(stderr)
	session := fs.String("session", os.Getenv(core.EnvSession), "Orchestrator session id to scope the registry")
	orchPane := fs.String("orchestrator-pane", "", "The orchestrator's own tmux pane id (notifications target it)")
	harnessKind := fs.String("harness", os.Getenv(core.EnvHarness), "Fallback harness kind for session capture (selector, not a launch command)")
	ghInterval := fs.Duration("gh-interval", daemon.DefaultGhInterval, "GitHub PR-state poll interval")
	reviewInterval := fs.Duration("review-interval", daemon.DefaultReviewInterval, "Review/CI poll interval")
	noDock := fs.Bool("no-dock", false, "Disable dock auto-flip / layout maintenance")
	if err := fs.Parse(args); err != nil {
		return 2
	}

	// Nothing to drive outside tmux: every notification is a tmux send-keys, so
	// exit cleanly so the best-effort spawn from `start` is harmless.
	if !tmux.InsideTmux() {
		fmt.Fprintln(stdout, "pr-agents daemon: not inside tmux; nothing to do")
		return 0
	}

	cwd, err := os.Getwd()
	if err != nil {
		fmt.Fprintf(stderr, "pr-agents daemon: %v\n", err)
		return 1
	}

	// Best-effort: the project's stacking strategy decides whether the gh-state
	// poller reads the whole stack from Graphite in one shot. A missing/unreadable
	// config leaves it empty (the github/independent path).
	projCfg, _ := core.LoadProjectConfig(cwd)

	cfg := daemon.Config{
		Session:          *session,
		OrchestratorPane: *orchPane,
		Harness:          *harnessKind,
		GhInterval:       *ghInterval,
		ReviewInterval:   *reviewInterval,
		NoDock:           *noDock,
		Strategy:         projCfg.Strategy,
	}
	d := daemon.NewReal(cfg, cwd)

	stop := make(chan struct{})
	sig := make(chan os.Signal, 1)
	signal.Notify(sig, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		<-sig
		close(stop)
	}()

	fmt.Fprintf(stdout, "pr-agents daemon: polling session %s (gh %s, review %s)\n",
		cfg.Session, cfg.GhInterval.Round(time.Second), cfg.ReviewInterval.Round(time.Second))
	d.Run(stop, stderr)
	return 0
}
