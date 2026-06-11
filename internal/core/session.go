package core

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
)

// SessionRecord is the durable, on-disk record of an orchestrator session's
// resolved harness + launcher. It exists because a sandbox launcher (e.g.
// `isara claude run`, `asb --profile git -- claude`) strips the orchestrator's
// PRA_HARNESS/PRA_LAUNCHER env vars before the sandboxed orchestrator shells out
// to `pr-agents dispatch`. Disk crosses the sandbox boundary (the repo is
// mounted), so a persisted record is the reliable channel — env is not. Mirrors
// the registry's defensive, atomic IO style.
type SessionRecord struct {
	Harness  string `json:"harness,omitempty"`
	Launcher string `json:"launcher,omitempty"`
	// TmuxSocket is the orchestrator's tmux server socket path, derived from the
	// real $TMUX value OUTSIDE the sandbox. A sandbox launcher strips $TMUX, so a
	// sandboxed dispatch cannot locate the tmux server; the persisted socket lets
	// it talk to the same server via `tmux -S <socket>`.
	TmuxSocket string `json:"tmuxSocket,omitempty"`
}

// TmuxSocketFromEnv extracts the tmux server socket path from a $TMUX value.
// tmux sets $TMUX to a comma-separated triple `<socket>,<pid>,<session>`; the
// socket is the substring before the first comma. Returns "" for an empty or
// comma-leading (malformed) value. Pure.
func TmuxSocketFromEnv(tmuxVal string) string {
	if tmuxVal == "" {
		return ""
	}
	if i := strings.IndexByte(tmuxVal, ','); i >= 0 {
		return tmuxVal[:i]
	}
	return tmuxVal
}

// prAgentsFile resolves <git-common-dir>/.pr-agents/<name>, creating the parent
// dir. It is the shared path resolver for the session record + marker files,
// which live beside the registry so the same mounted-repo disk channel carries
// them across a sandbox boundary.
func prAgentsFile(cwd, name string) (string, error) {
	commonDir, err := GitCommonDir(cwd)
	if err != nil {
		return "", err
	}
	dir := filepath.Join(commonDir, ".pr-agents")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	return filepath.Join(dir, name), nil
}

// atomicWriteFile writes data to path atomically: a temp file in the same dir,
// then a rename over the target, so a reader never sees a partial file.
func atomicWriteFile(path string, data []byte) error {
	tmp, err := os.CreateTemp(filepath.Dir(path), "."+filepath.Base(path)+"-*.tmp")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		os.Remove(tmpName)
		return err
	}
	if err := tmp.Close(); err != nil {
		os.Remove(tmpName)
		return err
	}
	if err := os.Rename(tmpName, path); err != nil {
		os.Remove(tmpName)
		return err
	}
	return nil
}

// currentSessionFile is the marker file holding the most recent orchestrator
// session id for this checkout, so env-less verbs (list/resume/cleanup) run by
// a sandboxed orchestrator still resolve the REAL session id rather than a
// harness-ref re-derivation that misses the stripped PRA_HARNESS. It lives
// beside the registry and crosses the sandbox boundary the same way.
const currentSessionFile = "current-session"

// CurrentSessionPath returns <git-common-dir>/.pr-agents/current-session,
// creating the parent dir.
func CurrentSessionPath(cwd string) (string, error) {
	return prAgentsFile(cwd, currentSessionFile)
}

// SaveCurrentSession records sessionID as the checkout's current orchestrator
// session (atomic temp file + rename). A no-op for an empty id.
func SaveCurrentSession(cwd, sessionID string) error {
	if sessionID == "" {
		return nil
	}
	path, err := CurrentSessionPath(cwd)
	if err != nil {
		return err
	}
	return atomicWriteFile(path, []byte(sessionID))
}

// LoadCurrentSession returns the checkout's recorded current orchestrator
// session id, or "" when the marker is missing or unreadable. Tolerant.
func LoadCurrentSession(cwd string) string {
	path, err := CurrentSessionPath(cwd)
	if err != nil {
		return ""
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(raw))
}

// ResolveFromSources returns the first non-empty value among flag (an
// explicitly-passed CLI flag), env (the PRA_* env var), and record (the value
// from the persisted session record), in that precedence order. It returns ""
// when all three are empty, leaving the final harness-specific fallback (e.g.
// "pi" / adapter.DefaultLauncher()) to the caller. Pure.
func ResolveFromSources(flag, env, record string) string {
	for _, v := range []string{flag, env, record} {
		if v != "" {
			return v
		}
	}
	return ""
}

// SessionsPath returns the per-session-records file,
// <git-common-dir>/.pr-agents/sessions.json, creating the parent dir. It lives
// next to the registry so the same disk channel that survives the sandbox
// boundary carries it.
func SessionsPath(cwd string) (string, error) {
	return prAgentsFile(cwd, "sessions.json")
}

// loadSessions reads the sessions map, returning an empty map when the file is
// missing or unparseable (so callers never special-case a fresh repo).
func loadSessions(cwd string) (map[string]SessionRecord, error) {
	path, err := SessionsPath(cwd)
	if err != nil {
		return nil, err
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return map[string]SessionRecord{}, nil
		}
		return nil, err
	}
	var m map[string]SessionRecord
	if err := json.Unmarshal(raw, &m); err != nil {
		return map[string]SessionRecord{}, nil
	}
	if m == nil {
		return map[string]SessionRecord{}, nil
	}
	return m, nil
}

// LoadSessionRecord returns the record for sessionID, with ok=false when there
// is no record (missing file, garbage file, or unknown session). Tolerant: a
// read error other than a true filesystem failure surfaces as ok=false.
func LoadSessionRecord(cwd, sessionID string) (SessionRecord, bool, error) {
	if sessionID == "" {
		return SessionRecord{}, false, nil
	}
	m, err := loadSessions(cwd)
	if err != nil {
		return SessionRecord{}, false, err
	}
	rec, ok := m[sessionID]
	return rec, ok, nil
}

// SessionRecordFor returns the record for sessionID, or a zero SessionRecord
// when there is none (missing, garbage, or unknown session). It is the tolerant
// convenience used by callers that simply want the persisted harness/launcher as
// a fallback and treat absence as "empty".
func SessionRecordFor(cwd, sessionID string) SessionRecord {
	rec, _, _ := LoadSessionRecord(cwd, sessionID)
	return rec
}

// SaveSessionRecord upserts the record for sessionID and writes the sessions map
// atomically (temp file in the same dir, then rename) so a reader never sees a
// partial file. A no-op-safe empty sessionID is rejected to avoid a junk key.
func SaveSessionRecord(cwd, sessionID string, rec SessionRecord) error {
	if sessionID == "" {
		return nil
	}
	path, err := SessionsPath(cwd)
	if err != nil {
		return err
	}
	m, err := loadSessions(cwd)
	if err != nil {
		return err
	}
	m[sessionID] = rec
	data, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		return err
	}
	return atomicWriteFile(path, data)
}
