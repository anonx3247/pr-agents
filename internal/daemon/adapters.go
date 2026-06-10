package daemon

import (
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"strconv"
	"strings"
	"time"

	"github.com/anonx3247/pr-agents/internal/core"
	"github.com/anonx3247/pr-agents/internal/tmux"
)

// ghTimeout bounds every gh invocation so a hung gh never blocks a tick.
const ghTimeout = 15 * time.Second

// realGH implements GH by shelling out to the gh CLI. Every method tolerates a
// missing/unauthenticated gh and degrades to ok=false.
type realGH struct{}

// runGh runs `gh args...` in cwd and returns trimmed stdout, or ok=false.
func runGh(cwd string, args ...string) (string, bool) {
	ctx, cancel := context.WithTimeout(context.Background(), ghTimeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, "gh", args...)
	cmd.Dir = cwd
	out, err := cmd.Output()
	if err != nil {
		return "", false
	}
	return strings.TrimSpace(string(out)), true
}

func (realGH) PrState(worktree string, number int) (*core.PrStateJSON, bool) {
	out, ok := runGh(worktree, "pr", "view", strconv.Itoa(number), "--json", "state,mergedAt,closedAt,url")
	if !ok {
		return nil, false
	}
	var j core.PrStateJSON
	if err := json.Unmarshal([]byte(out), &j); err != nil {
		return nil, false
	}
	return &j, true
}

func (realGH) CiChecks(cwd string, number int) (string, []core.CiCheck, bool) {
	headOut, ok := runGh(cwd, "pr", "view", strconv.Itoa(number), "--json", "headRefOid")
	if !ok {
		return "", nil, false
	}
	var head struct {
		HeadRefOid string `json:"headRefOid"`
	}
	if err := json.Unmarshal([]byte(headOut), &head); err != nil || head.HeadRefOid == "" {
		return "", nil, false
	}

	// Preferred: `gh pr checks --json name,state,bucket,link`.
	if out, ok := runGh(cwd, "pr", "checks", strconv.Itoa(number), "--json", "name,state,bucket,link"); ok {
		var raw []struct {
			Name, State, Bucket, Link string
		}
		if err := json.Unmarshal([]byte(out), &raw); err == nil {
			checks := make([]core.CiCheck, 0, len(raw))
			for _, c := range raw {
				if c.Name == "" {
					continue
				}
				checks = append(checks, core.CiCheck{Name: c.Name, State: c.State, Bucket: c.Bucket, Link: c.Link})
			}
			return head.HeadRefOid, checks, true
		}
	}

	// Fallback for older gh: map statusCheckRollup conclusions to buckets.
	rollOut, ok := runGh(cwd, "pr", "view", strconv.Itoa(number), "--json", "statusCheckRollup")
	if !ok {
		return head.HeadRefOid, nil, true
	}
	var roll struct {
		StatusCheckRollup []struct {
			Name       string `json:"name"`
			Context    string `json:"context"`
			Conclusion string `json:"conclusion"`
			State      string `json:"state"`
			DetailsURL string `json:"detailsUrl"`
			TargetURL  string `json:"targetUrl"`
		} `json:"statusCheckRollup"`
	}
	if err := json.Unmarshal([]byte(rollOut), &roll); err != nil {
		return head.HeadRefOid, nil, true
	}
	checks := make([]core.CiCheck, 0, len(roll.StatusCheckRollup))
	for _, c := range roll.StatusCheckRollup {
		name := c.Name
		if name == "" {
			name = c.Context
		}
		if name == "" {
			continue
		}
		rawState := c.Conclusion
		if rawState == "" {
			rawState = c.State
		}
		link := c.DetailsURL
		if link == "" {
			link = c.TargetURL
		}
		checks = append(checks, core.CiCheck{
			Name:   name,
			State:  strings.ToLower(rawState),
			Bucket: core.RollupBucket(strings.ToUpper(rawState)),
			Link:   link,
		})
	}
	return head.HeadRefOid, checks, true
}

func (realGH) ReviewActivity(owner, repo string, number int, cwd string) (core.FetchedReviewActivity, bool) {
	var act core.FetchedReviewActivity
	any := false

	// Inline comments via the REST API (paginated).
	if out, ok := runGh(cwd, "api", "--paginate",
		"repos/"+owner+"/"+repo+"/pulls/"+strconv.Itoa(number)+"/comments"); ok {
		any = true
		var raw []struct {
			ID           int    `json:"id"`
			Body         string `json:"body"`
			Path         string `json:"path"`
			Line         *int   `json:"line"`
			OriginalLine *int   `json:"original_line"`
			CreatedAt    string `json:"created_at"`
			InReplyToID  *int   `json:"in_reply_to_id"`
			User         struct {
				Login string `json:"login"`
			} `json:"user"`
		}
		if err := json.Unmarshal([]byte(out), &raw); err == nil {
			for _, c := range raw {
				if c.ID == 0 {
					continue
				}
				line := c.Line
				if line == nil {
					line = c.OriginalLine
				}
				act.Inline = append(act.Inline, core.InlineComment{
					ID: c.ID, User: c.User.Login, Body: c.Body, Path: c.Path,
					Line: line, CreatedAt: c.CreatedAt, InReplyToID: c.InReplyToID,
				})
			}
		}
	}

	if out, ok := runGh(cwd, "pr", "view", strconv.Itoa(number), "--json", "reviews"); ok {
		any = true
		var rv struct {
			Reviews []struct {
				ID     *int   `json:"id"`
				Body   string `json:"body"`
				State  string `json:"state"`
				Author struct {
					Login string `json:"login"`
				} `json:"author"`
				SubmittedAt string `json:"submittedAt"`
			} `json:"reviews"`
		}
		if err := json.Unmarshal([]byte(out), &rv); err == nil {
			for _, r := range rv.Reviews {
				act.Reviews = append(act.Reviews, core.ReviewSummary{
					ID: r.ID, Author: r.Author.Login, Body: r.Body, State: r.State, SubmittedAt: r.SubmittedAt,
				})
			}
		}
	}

	if out, ok := runGh(cwd, "pr", "view", strconv.Itoa(number), "--json", "comments"); ok {
		any = true
		var ic struct {
			Comments []struct {
				ID     *int   `json:"id"`
				Body   string `json:"body"`
				Author struct {
					Login string `json:"login"`
				} `json:"author"`
				CreatedAt string `json:"createdAt"`
			} `json:"comments"`
		}
		if err := json.Unmarshal([]byte(out), &ic); err == nil {
			for _, c := range ic.Comments {
				act.IssueComments = append(act.IssueComments, core.IssueComment{
					ID: c.ID, Author: c.Author.Login, Body: c.Body, CreatedAt: c.CreatedAt,
				})
			}
		}
	}

	return act, any
}

func (realGH) OwnerRepo(cwd string) (string, string, bool) {
	out, ok := runGh(cwd, "repo", "view", "--json", "nameWithOwner")
	if !ok {
		return "", "", false
	}
	var j struct {
		NameWithOwner string `json:"nameWithOwner"`
	}
	if err := json.Unmarshal([]byte(out), &j); err != nil {
		return "", "", false
	}
	parts := strings.SplitN(j.NameWithOwner, "/", 2)
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		return "", "", false
	}
	return parts[0], parts[1], true
}

// ResolveOwnerRepo resolves owner/name for the current repo in cwd via gh, or
// ok=false when gh is unavailable. Exported so worker-side verbs (reply-review)
// can reuse the same resolution the daemon uses.
func ResolveOwnerRepo(cwd string) (owner, repo string, ok bool) {
	return realGH{}.OwnerRepo(cwd)
}

// PostReviewReply posts an inline reply to a review comment thread via gh,
// passing the body over stdin so it is always treated as a literal string (no
// shell/quoting issues). Returns the created reply's numeric id and ok=true, or
// ok=false on any failure. A reply does NOT resolve the thread.
func PostReviewReply(owner, repo string, prNumber, commentID int, body, cwd string) (int, bool) {
	ctx, cancel := context.WithTimeout(context.Background(), ghTimeout)
	defer cancel()
	endpoint := fmt.Sprintf("repos/%s/%s/pulls/%d/comments/%d/replies", owner, repo, prNumber, commentID)
	cmd := exec.CommandContext(ctx, "gh", "api", "-X", "POST", endpoint, "-F", "body=@-")
	cmd.Dir = cwd
	cmd.Stdin = strings.NewReader(body)
	out, err := cmd.Output()
	if err != nil {
		return 0, false
	}
	var j struct {
		ID int `json:"id"`
	}
	if err := json.Unmarshal(out, &j); err != nil || j.ID == 0 {
		return 0, false
	}
	return j.ID, true
}

// realTmux implements Tmuxer over the tmux package.
type realTmux struct{}

func (realTmux) SendToPane(pane, msg string) bool       { return tmux.SendToPane(pane, msg) }
func (realTmux) PaneAlive(pane string) bool             { return tmux.PaneAlive(pane) }
func (realTmux) JoinPane(src, target string) bool       { return tmux.JoinPane(src, target) }
func (realTmux) BreakPane(src, name string) bool        { return tmux.BreakPane(src, name) }
func (realTmux) SelectLayoutMainVertical(target string) { tmux.SelectLayoutMainVertical(target) }
func (realTmux) SetMainPaneWidth(target, width string)  { tmux.SetMainPaneWidth(target, width) }

// realStore implements Store over the shared registry rooted at cwd.
type realStore struct{ cwd string }

func (s realStore) Load() []core.PrEntry {
	entries, err := core.LoadRegistry(s.cwd)
	if err != nil {
		return nil
	}
	return entries
}

func (s realStore) Update(id string, patch func(*core.PrEntry)) {
	_, _, _ = core.UpdateEntry(s.cwd, id, patch)
}

// NewReal constructs a Daemon wired to the real gh/tmux/registry adapters,
// rooted at cwd. This is the production entry point used by the CLI verb.
func NewReal(cfg Config, cwd string) *Daemon {
	return New(cfg, realGH{}, realTmux{}, realStore{cwd: cwd}, cwd)
}
