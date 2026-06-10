package harness

import "testing"

func TestEncodeClaudeProjectDir(t *testing.T) {
	tests := []struct {
		name string
		cwd  string
		want string
	}{
		{"simple", "/Users/anas", "-Users-anas"},
		{"nested", "/Users/anas/dev/agent-sandbox", "-Users-anas-dev-agent-sandbox"},
		{"dotted worktree collapses slash-dot", "/Users/anas/dev/x/.worktrees/y", "-Users-anas-dev-x--worktrees-y"},
		{"dot dir", "/Users/anas/dev/bmb/.claude/worktrees/add", "-Users-anas-dev-bmb--claude-worktrees-add"},
		{"underscores become dash", "/tmp/my_dir", "-tmp-my-dir"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := encodeClaudeProjectDir(tt.cwd); got != tt.want {
				t.Errorf("encodeClaudeProjectDir(%q) = %q, want %q", tt.cwd, got, tt.want)
			}
		})
	}
}

func TestEncodePiSessionDir(t *testing.T) {
	tests := []struct {
		name string
		cwd  string
		want string
	}{
		{"simple", "/Users/anas", "--Users-anas--"},
		{"nested", "/Users/anas/dev/agent-sandbox", "--Users-anas-dev-agent-sandbox--"},
		{"dotted worktree keeps dot", "/Users/anas/dev/x/.worktrees/y", "--Users-anas-dev-x-.worktrees-y--"},
		{"trailing slash trimmed", "/Users/anas/", "--Users-anas--"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := encodePiSessionDir(tt.cwd); got != tt.want {
				t.Errorf("encodePiSessionDir(%q) = %q, want %q", tt.cwd, got, tt.want)
			}
		})
	}
}

func TestSessionStoreHomeOverride(t *testing.T) {
	orig := sessionHomeOverride
	t.Cleanup(func() { sessionHomeOverride = orig })

	sessionHomeOverride = "/tmp/fake-home"
	got, err := sessionStoreHome()
	if err != nil {
		t.Fatalf("sessionStoreHome() error: %v", err)
	}
	if got != "/tmp/fake-home" {
		t.Errorf("sessionStoreHome() = %q, want /tmp/fake-home", got)
	}
}
