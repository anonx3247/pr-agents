package cli

import (
	"bytes"
	"os"
	"strings"
	"testing"
)

// TestRunDaemonOutsideTmuxExitsCleanly verifies the daemon is a harmless no-op
// when not inside tmux (so the best-effort spawn from `start` never blocks or
// errors). TMUX is cleared for the duration of the call.
func TestRunDaemonOutsideTmuxExitsCleanly(t *testing.T) {
	saved, had := os.LookupEnv("TMUX")
	os.Unsetenv("TMUX")
	defer func() {
		if had {
			os.Setenv("TMUX", saved)
		}
	}()

	var out, errOut bytes.Buffer
	code := runDaemon([]string{"--session", "abc"}, &out, &errOut)
	if code != 0 {
		t.Fatalf("exit code = %d, want 0; stderr=%q", code, errOut.String())
	}
	if !strings.Contains(out.String(), "nothing to do") {
		t.Errorf("stdout = %q, want 'nothing to do'", out.String())
	}
}
