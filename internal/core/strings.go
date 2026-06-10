package core

import (
	"fmt"
	"regexp"
	"sort"
	"strings"
)

var (
	slugNonAlnum = regexp.MustCompile(`[^a-z0-9]+`)
	slugEdges    = regexp.MustCompile(`^-+|-+$`)
	trailingDash = regexp.MustCompile(`-+$`)
)

// Slugify lowercases s, collapses runs of non-alphanumeric characters into a
// single dash, trims leading/trailing dashes, caps the result at 48 characters,
// and falls back to "pr" when nothing usable remains.
func Slugify(s string) string {
	s = strings.ToLower(s)
	s = slugNonAlnum.ReplaceAllString(s, "-")
	s = slugEdges.ReplaceAllString(s, "")
	if len(s) > 48 {
		s = s[:48]
	}
	if s == "" {
		return "pr"
	}
	return s
}

// Shq single-quotes s for safe use in a POSIX shell command, escaping any
// embedded single quotes via the close-escape-reopen idiom.
func Shq(s string) string {
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}

// BuildEnv renders a map of environment variables into a single space-separated
// `KEY='value'` assignment string suitable for prefixing a shell command. Keys
// are emitted in sorted order so the output is deterministic.
func BuildEnv(vars map[string]string) string {
	keys := make([]string, 0, len(vars))
	for k := range vars {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	parts := make([]string, 0, len(keys))
	for _, k := range keys {
		parts = append(parts, k+"="+Shq(vars[k]))
	}
	return strings.Join(parts, " ")
}

// CapTail caps s to at most cap runes, keeping the TAIL (a subagent's final
// summary is usually at the end). When truncated, a leading ellipsis marks the
// cut so the result stays exactly cap runes. A non-positive cap yields "".
func CapTail(s string, cap int) string {
	if cap <= 0 {
		return ""
	}
	runes := []rune(s)
	if len(runes) <= cap {
		return s
	}
	return "…" + string(runes[len(runes)-cap+1:])
}

// PaneTitleArgs carries the fields needed to format a pane title or window name
// without depending on the full PrEntry shape.
type PaneTitleArgs struct {
	PrNumber *int
	PrName   string
	Branch   string
}

// PaneTitle formats a tmux pane title: "PR#<n> <name> (<branch>)" when a PR
// number is set, otherwise "PR <name> (<branch>)".
func PaneTitle(a PaneTitleArgs) string {
	tag := "PR"
	if a.PrNumber != nil {
		tag = fmt.Sprintf("PR#%d", *a.PrNumber)
	}
	return fmt.Sprintf("%s %s (%s)", tag, a.PrName, a.Branch)
}

// WindowName formats a concise tmux window name derived from the PR number,
// name (or branch fallback), slugified, capped at 24 chars with trailing dashes
// trimmed, falling back to "pr".
func WindowName(a PaneTitleArgs) string {
	tag := "pr"
	if a.PrNumber != nil {
		tag = fmt.Sprintf("pr%d", *a.PrNumber)
	}
	label := a.PrName
	if label == "" {
		label = a.Branch
	}
	name := tag + "-" + Slugify(label)
	if len(name) > 24 {
		name = name[:24]
	}
	name = trailingDash.ReplaceAllString(name, "")
	if name == "" {
		return "pr"
	}
	return name
}
