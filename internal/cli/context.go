package cli

import (
	"os"

	"github.com/anonx3247/pr-agents/internal/core"
)

// resolveSession returns the session id used to scope registry views. It
// prefers the PRA_SESSION env var (set orchestrator-side and inherited by
// workers), else falls back to the session id of the entry that owns cwd (so a
// worker inside a sandboxed worktree recovers its session from its path without
// PRA_* crossing the boundary). Returns "" when neither is available.
func resolveSession(entries []core.PrEntry, cwd string) string {
	if s := os.Getenv(core.EnvSession); s != "" {
		return s
	}
	if e := core.ResolveContextFromCwd(entries, cwd); e != nil {
		return e.SessionID
	}
	return ""
}

// sessionEntries loads the registry from cwd and returns the entries scoped to
// the resolved session, along with cwd and the resolved session id.
func sessionEntries() (entries []core.PrEntry, cwd, session string, err error) {
	cwd, err = os.Getwd()
	if err != nil {
		return nil, "", "", err
	}
	all, err := core.LoadRegistry(cwd)
	if err != nil {
		return nil, cwd, "", err
	}
	session = resolveSession(all, cwd)
	return core.EntriesForSession(all, session), cwd, session, nil
}
