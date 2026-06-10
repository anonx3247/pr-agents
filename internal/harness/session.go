package harness

import (
	"os"
	"path/filepath"
	"strings"
	"time"
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

// newestFileInDir returns the absolute path of the newest regular file in dir
// whose modification time is at or after since and for which match(name) is
// true (a nil match accepts every file). It returns ok=false when dir is
// missing/unreadable or nothing matches, so callers tolerate an absent store
// without panicking.
func newestFileInDir(dir string, since time.Time, match func(name string) bool) (path string, ok bool) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return "", false
	}
	var best time.Time
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		if match != nil && !match(e.Name()) {
			continue
		}
		info, err := e.Info()
		if err != nil {
			continue
		}
		mt := info.ModTime()
		if mt.Before(since) {
			continue
		}
		if !ok || mt.After(best) {
			path, best, ok = filepath.Join(dir, e.Name()), mt, true
		}
	}
	return path, ok
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
