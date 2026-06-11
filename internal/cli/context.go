package cli

import (
	"os"
	"time"

	"github.com/anonx3247/pr-agents/internal/core"
	"github.com/anonx3247/pr-agents/internal/harness"
)

// scopeRefResolver builds a core.ScopeRefResolver that locates the
// orchestrator's own resumable session ref for cwd via the harness adapter,
// degrading to ok=false on an unknown harness or a SessionRef miss. since is the
// zero time so any existing session for cwd is accepted (the harness reopens the
// same one on resume → a stable scope id). An empty kind defaults to "pi".
func scopeRefResolver(kind, cwd string) core.ScopeRefResolver {
	return func() (string, bool) {
		if kind == "" {
			kind = "pi"
		}
		a, err := harness.Get(kind)
		if err != nil {
			return "", false
		}
		return a.SessionRef(cwd, time.Time{})
	}
}

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
	// Orchestrator (no owning entry in the main repo): re-derive the scope from
	// the harness's own resumable session ref so a resume re-scopes to the same
	// registry entries. Falls back to "" (no random mint) for read-only verbs.
	return core.ResolveScopeID(
		scopeRefResolver(os.Getenv(core.EnvHarness), cwd),
		"",
		func() string { return "" },
		false,
	)
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
