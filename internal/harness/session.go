package harness

import (
	"os"
	"strings"
)

// sessionHomeOverride, when non-empty, replaces os.UserHomeDir() as the base
// directory that holds each harness's on-disk session store. It exists ONLY so
// unit tests can point the SessionRef scanners at a fake store under a temp dir
// instead of the real "~". Production code never sets it.
var sessionHomeOverride string

// sessionStoreHome resolves the base directory under which the per-harness
// session stores live (".claude", ".pi", ".codex"). It honours the test
// override, else falls back to the user's home directory.
func sessionStoreHome() (string, error) {
	if sessionHomeOverride != "" {
		return sessionHomeOverride, nil
	}
	return os.UserHomeDir()
}

// encodeClaudeProjectDir mirrors Claude Code's project-directory naming scheme:
// the absolute cwd with EVERY non-alphanumeric character replaced by '-'. For
// example "/Users/anas/dev/x/.worktrees/y" encodes to
// "-Users-anas-dev-x--worktrees-y" (note the "/." run collapses to "--"). Pure:
// no IO.
func encodeClaudeProjectDir(cwd string) string {
	var b strings.Builder
	b.Grow(len(cwd))
	for _, r := range cwd {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') {
			b.WriteRune(r)
		} else {
			b.WriteByte('-')
		}
	}
	return b.String()
}

// encodePiSessionDir mirrors pi's session-directory naming scheme: the absolute
// cwd with '/' replaced by '-' (dots and other characters KEPT), wrapped in a
// leading and trailing "--". For example "/Users/anas/dev/agent-sandbox"
// encodes to "--Users-anas-dev-agent-sandbox--" and
// "/Users/anas/dev/x/.worktrees/y" to "--Users-anas-dev-x-.worktrees-y--".
// Pure: no IO.
func encodePiSessionDir(cwd string) string {
	trimmed := strings.Trim(cwd, "/")
	return "--" + strings.ReplaceAll(trimmed, "/", "-") + "--"
}
