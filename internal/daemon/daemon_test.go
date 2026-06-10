package daemon

import (
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/anonx3247/pr-agents/internal/core"
)

func intp(n int) *int { return &n }

// --- fakes ---------------------------------------------------------------

type fakeCi struct {
	sha    string
	checks []core.CiCheck
}

type fakeGH struct {
	state  map[int]*core.PrStateJSON
	ci     map[int]fakeCi
	review map[int]core.FetchedReviewActivity
	owner  string
	repo   string
	hasOwn bool
}

func (f *fakeGH) PrState(_ string, n int) (*core.PrStateJSON, bool) {
	j, ok := f.state[n]
	return j, ok
}
func (f *fakeGH) CiChecks(_ string, n int) (string, []core.CiCheck, bool) {
	c, ok := f.ci[n]
	if !ok {
		return "", nil, false
	}
	return c.sha, c.checks, true
}
func (f *fakeGH) ReviewActivity(_, _ string, n int, _ string) (core.FetchedReviewActivity, bool) {
	r, ok := f.review[n]
	return r, ok
}
func (f *fakeGH) OwnerRepo(_ string) (string, string, bool) {
	return f.owner, f.repo, f.hasOwn
}

type sent struct {
	pane string
	msg  string
}

type fakeTmux struct {
	mu      sync.Mutex
	alive   map[string]bool
	sends   []sent
	joins   [][2]string
	breaks  [][2]string
	layouts []string
}

func (f *fakeTmux) SendToPane(pane, msg string) bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.sends = append(f.sends, sent{pane, msg})
	return true
}
func (f *fakeTmux) PaneAlive(pane string) bool { return f.alive[pane] }
func (f *fakeTmux) JoinPane(src, target string) bool {
	f.joins = append(f.joins, [2]string{src, target})
	return true
}
func (f *fakeTmux) BreakPane(src, name string) bool {
	f.breaks = append(f.breaks, [2]string{src, name})
	return true
}
func (f *fakeTmux) SelectLayoutMainVertical(target string) { f.layouts = append(f.layouts, target) }
func (f *fakeTmux) SetMainPaneWidth(_, _ string)           {}

func (f *fakeTmux) messagesTo(pane string) []string {
	out := []string{}
	for _, s := range f.sends {
		if s.pane == pane {
			out = append(out, s.msg)
		}
	}
	return out
}

type fakeStore struct {
	entries []core.PrEntry
}

func (s *fakeStore) Load() []core.PrEntry { return s.entries }
func (s *fakeStore) Update(id string, patch func(*core.PrEntry)) {
	for i := range s.entries {
		if s.entries[i].ID == id {
			patch(&s.entries[i])
			return
		}
	}
}

// worker is a convenience builder for a pollable depth-1 entry.
func worker(id string, num int) core.PrEntry {
	return core.PrEntry{
		ID: id, SessionID: "s", Depth: 1, Pushed: true, PrNumber: intp(num),
		Status: core.StatusOpen, PaneID: "%" + id, Worktree: "/wt/" + id,
	}
}

func newDaemon(cfg Config, gh GH, tm Tmuxer, st Store) *Daemon {
	cfg.Session = "s"
	d := New(cfg, gh, tm, st, "/repo")
	return d
}

// --- tests ---------------------------------------------------------------

func TestPollPrStateNotifiesOnTerminalTransition(t *testing.T) {
	st := &fakeStore{entries: []core.PrEntry{worker("a", 1), worker("b", 2)}}
	gh := &fakeGH{state: map[int]*core.PrStateJSON{
		1: {State: "MERGED"},
		2: {State: "OPEN"},
	}}
	tm := &fakeTmux{alive: map[string]bool{"orch": true}}
	d := newDaemon(Config{OrchestratorPane: "orch"}, gh, tm, st)

	d.pollPrState()

	msgs := tm.messagesTo("orch")
	if len(msgs) != 1 || !strings.Contains(msgs[0], "PR #1") {
		t.Fatalf("want one cleanup notice for PR #1, got %#v", msgs)
	}
	// Status persisted.
	if st.entries[0].Status != core.StatusMerged {
		t.Errorf("entry a status = %q, want merged", st.entries[0].Status)
	}

	// Second poll: no new transition => no new message.
	tm.sends = nil
	d.pollPrState()
	if got := tm.messagesTo("orch"); len(got) != 0 {
		t.Errorf("expected no repeat notification, got %#v", got)
	}
}

func TestPollPrStateSkipsUnpushed(t *testing.T) {
	e := worker("a", 1)
	e.Pushed = false
	st := &fakeStore{entries: []core.PrEntry{e}}
	gh := &fakeGH{state: map[int]*core.PrStateJSON{1: {State: "MERGED"}}}
	tm := &fakeTmux{alive: map[string]bool{"orch": true}}
	d := newDaemon(Config{OrchestratorPane: "orch"}, gh, tm, st)
	d.pollPrState()
	if len(tm.sends) != 0 {
		t.Errorf("unpushed entry should not be polled, got %#v", tm.sends)
	}
}

func TestPollFinishedNotifiesOnce(t *testing.T) {
	e := worker("a", 1)
	e.ResultSeq = intp(0)
	e.LastResult = "all done"
	st := &fakeStore{entries: []core.PrEntry{e}}
	tm := &fakeTmux{alive: map[string]bool{"orch": true}}
	d := newDaemon(Config{OrchestratorPane: "orch"}, &fakeGH{}, tm, st)

	d.pollFinished()
	msgs := tm.messagesTo("orch")
	if len(msgs) != 1 || !strings.Contains(msgs[0], "all done") {
		t.Fatalf("want one finished notice, got %#v", msgs)
	}
	tm.sends = nil
	d.pollFinished()
	if len(tm.sends) != 0 {
		t.Errorf("finished should notify once, got %#v", tm.sends)
	}
}

func TestPollWorkersReviewAndCi(t *testing.T) {
	st := &fakeStore{entries: []core.PrEntry{worker("a", 1)}}
	gh := &fakeGH{
		owner: "o", repo: "r", hasOwn: true,
		review: map[int]core.FetchedReviewActivity{
			1: {Inline: []core.InlineComment{{ID: 10, Path: "x.go", Body: "fix"}}},
		},
		ci: map[int]fakeCi{
			1: {sha: "sha1", checks: []core.CiCheck{{Name: "build", Bucket: "fail", State: "failure"}}},
		},
	}
	tm := &fakeTmux{alive: map[string]bool{"%a": true}}
	d := newDaemon(Config{OrchestratorPane: "orch"}, gh, tm, st)

	d.pollWorkers()
	msgs := tm.messagesTo("%a")
	if len(msgs) != 2 {
		t.Fatalf("want a review task + a CI task, got %d: %#v", len(msgs), msgs)
	}
	joined := strings.Join(msgs, "\n")
	if !strings.Contains(joined, "[rc:10]") || !strings.Contains(joined, "build (failure)") {
		t.Errorf("missing review/ci content: %#v", msgs)
	}
	// Seen-set persisted: a second poll re-surfaces nothing.
	tm.sends = nil
	d.pollWorkers()
	if got := tm.messagesTo("%a"); len(got) != 0 {
		t.Errorf("expected dedup, got %#v", got)
	}
}

// fakeResolver records its calls and returns a canned ref/ok keyed by cwd, so
// session capture is tested without scanning the real session stores.
type fakeResolver struct {
	refByCwd map[string]string
	calls    int
}

func (f *fakeResolver) resolve(_, cwd string, _ time.Time) (string, bool) {
	f.calls++
	ref, ok := f.refByCwd[cwd]
	return ref, ok
}

func TestCaptureSessionsCapturesOnce(t *testing.T) {
	e := worker("a", 1)
	st := &fakeStore{entries: []core.PrEntry{e}}
	tm := &fakeTmux{alive: map[string]bool{"%a": true}}
	fr := &fakeResolver{refByCwd: map[string]string{"/wt/a": "sess-ref-a"}}
	d := newDaemon(Config{Harness: "pi"}, &fakeGH{}, tm, st)
	d.resolveSession = fr.resolve

	d.captureSessions()
	if got := st.entries[0].WorkerSessionRef; got != "sess-ref-a" {
		t.Fatalf("WorkerSessionRef = %q, want sess-ref-a", got)
	}
	if got := st.entries[0].WorkerSessionHarness; got != "pi" {
		t.Errorf("WorkerSessionHarness = %q, want pi", got)
	}

	// A second tick is a no-op: the entry now has a ref, so the resolver is not
	// consulted again.
	before := fr.calls
	d.captureSessions()
	if fr.calls != before {
		t.Errorf("resolver called again after capture: calls went %d -> %d", before, fr.calls)
	}
}

func TestCaptureSessionsUsesEntryHarness(t *testing.T) {
	// Entry dispatched with codex while the daemon's own harness is pi: capture
	// must resolve via the ENTRY's harness and stamp it on WorkerSessionHarness.
	e := worker("a", 1)
	e.Harness = "codex"
	st := &fakeStore{entries: []core.PrEntry{e}}
	tm := &fakeTmux{alive: map[string]bool{"%a": true}}
	var gotKind string
	d := newDaemon(Config{Harness: "pi"}, &fakeGH{}, tm, st)
	d.resolveSession = func(kind, _ string, _ time.Time) (string, bool) {
		gotKind = kind
		return "ref", true
	}
	d.captureSessions()
	if gotKind != "codex" {
		t.Errorf("resolver kind = %q, want codex (entry harness)", gotKind)
	}
	if got := st.entries[0].WorkerSessionHarness; got != "codex" {
		t.Errorf("WorkerSessionHarness = %q, want codex", got)
	}
}

func TestCaptureSessionsLegacyEntryFallsBackToDaemonHarness(t *testing.T) {
	// Legacy entry with no Harness: capture falls back to the daemon's harness.
	st := &fakeStore{entries: []core.PrEntry{worker("a", 1)}}
	tm := &fakeTmux{alive: map[string]bool{"%a": true}}
	var gotKind string
	d := newDaemon(Config{Harness: "pi"}, &fakeGH{}, tm, st)
	d.resolveSession = func(kind, _ string, _ time.Time) (string, bool) {
		gotKind = kind
		return "ref", true
	}
	d.captureSessions()
	if gotKind != "pi" || st.entries[0].WorkerSessionHarness != "pi" {
		t.Errorf("kind=%q harness=%q, want pi/pi", gotKind, st.entries[0].WorkerSessionHarness)
	}
}

func TestCaptureSessionsNotFoundRetries(t *testing.T) {
	e := worker("a", 1)
	st := &fakeStore{entries: []core.PrEntry{e}}
	tm := &fakeTmux{alive: map[string]bool{"%a": true}}
	fr := &fakeResolver{refByCwd: map[string]string{}} // resolver finds nothing yet
	d := newDaemon(Config{Harness: "pi"}, &fakeGH{}, tm, st)
	d.resolveSession = fr.resolve

	d.captureSessions()
	if st.entries[0].WorkerSessionRef != "" {
		t.Errorf("WorkerSessionRef = %q, want empty (left for retry)", st.entries[0].WorkerSessionRef)
	}
	// The entry is still eligible, so a later tick retries (resolver called again).
	before := fr.calls
	d.captureSessions()
	if fr.calls <= before {
		t.Errorf("resolver not retried: calls stayed at %d", fr.calls)
	}
}

func TestPollWorkersSkipsDeadPane(t *testing.T) {
	st := &fakeStore{entries: []core.PrEntry{worker("a", 1)}}
	gh := &fakeGH{owner: "o", repo: "r", hasOwn: true,
		review: map[int]core.FetchedReviewActivity{1: {Inline: []core.InlineComment{{ID: 1, Body: "x"}}}}}
	tm := &fakeTmux{alive: map[string]bool{}} // %a not alive
	d := newDaemon(Config{OrchestratorPane: "orch"}, gh, tm, st)
	d.pollWorkers()
	if len(tm.sends) != 0 {
		t.Errorf("dead worker pane should be skipped, got %#v", tm.sends)
	}
}
