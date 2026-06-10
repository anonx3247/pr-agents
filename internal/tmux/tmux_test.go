package tmux

import (
	"reflect"
	"testing"
)

func TestOpenWindowArgs(t *testing.T) {
	tests := []struct {
		name    string
		cwd     string
		command string
		winName string
		env     map[string]string
		want    []string
	}{
		{
			name:    "no env",
			cwd:     "/tmp/wt",
			command: "pi",
			winName: "pr12-foo",
			env:     nil,
			want:    []string{"new-window", "-d", "-P", "-F", "#{pane_id}", "-c", "/tmp/wt", "-n", "pr12-foo", "pi"},
		},
		{
			name:    "env injected sorted",
			cwd:     "/tmp/wt",
			command: "pi run",
			winName: "pr12",
			env:     map[string]string{"PRA_MODE": "graphite", "PRA_ID": "abc", "PRA_DEPTH": "1"},
			want: []string{
				"new-window", "-d", "-P", "-F", "#{pane_id}", "-c", "/tmp/wt", "-n", "pr12",
				"-e", "PRA_DEPTH=1", "-e", "PRA_ID=abc", "-e", "PRA_MODE=graphite", "pi run",
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := OpenWindowArgs(tt.cwd, tt.command, tt.winName, tt.env)
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("OpenWindowArgs() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestOpenPaneArgs(t *testing.T) {
	got := OpenPaneArgs("/tmp/wt", "pi", map[string]string{"PRA_DEPTH": "2"})
	want := []string{
		"split-window", "-h", "-d", "-P", "-F", "#{pane_id}", "-c", "/tmp/wt",
		"-e", "PRA_DEPTH=2", "pi",
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("OpenPaneArgs() = %v, want %v", got, want)
	}
}

func TestCaptureArgs(t *testing.T) {
	tests := []struct {
		lines int
		want  []string
	}{
		{60, []string{"capture-pane", "-p", "-t", "%3", "-S", "-60"}},
		{0, []string{"capture-pane", "-p", "-t", "%3", "-S", "-0"}},
		{-5, []string{"capture-pane", "-p", "-t", "%3", "-S", "-0"}},
	}
	for _, tt := range tests {
		got := CaptureArgs("%3", tt.lines)
		if !reflect.DeepEqual(got, tt.want) {
			t.Errorf("CaptureArgs(%d) = %v, want %v", tt.lines, got, tt.want)
		}
	}
}

func TestSendArgs(t *testing.T) {
	if got, want := SendTextArgs("%1", "hello world"), []string{"send-keys", "-t", "%1", "-l", "--", "hello world"}; !reflect.DeepEqual(got, want) {
		t.Errorf("SendTextArgs() = %v, want %v", got, want)
	}
	if got, want := SendEnterArgs("%1"), []string{"send-keys", "-t", "%1", "Enter"}; !reflect.DeepEqual(got, want) {
		t.Errorf("SendEnterArgs() = %v, want %v", got, want)
	}
}

func TestSetPaneTitleArgs(t *testing.T) {
	got := SetPaneTitleArgs("%2", "PR#5 foo (br)")
	want := []string{"select-pane", "-t", "%2", "-T", "PR#5 foo (br)"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("SetPaneTitleArgs() = %v, want %v", got, want)
	}
}

func TestEnvArgs(t *testing.T) {
	if got := envArgs(nil); got != nil {
		t.Errorf("envArgs(nil) = %v, want nil", got)
	}
	got := envArgs(map[string]string{"B": "2", "A": "1"})
	want := []string{"-e", "A=1", "-e", "B=2"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("envArgs() = %v, want %v", got, want)
	}
}
