package core

import (
	"reflect"
	"strings"
	"testing"
)

func TestSelectNewCiFailures(t *testing.T) {
	checks := []CiCheck{
		{Name: "build", Bucket: "fail", State: "failure", Link: "http://x/1"},
		{Name: "test", Bucket: "pass"},
		{Name: "lint", Bucket: "fail", State: "failure"},
		{Name: "build", Bucket: "fail"}, // dup within tick
	}
	seen := map[string]bool{"ci:abc:lint": true}
	got := SelectNewCiFailures(checks, "abc", seen)

	if len(got.Failures) != 1 || got.Failures[0].Name != "build" {
		t.Fatalf("failures = %#v, want only build", got.Failures)
	}
	if !reflect.DeepEqual(got.NewKeys, []string{"ci:abc:build"}) {
		t.Errorf("newKeys = %#v", got.NewKeys)
	}
}

func TestSelectNewCiFailuresPerCommit(t *testing.T) {
	checks := []CiCheck{{Name: "build", Bucket: "fail"}}
	// Same check, new head sha => re-surfaces (new key) even though old key seen.
	seen := map[string]bool{"ci:old:build": true}
	got := SelectNewCiFailures(checks, "new", seen)
	if len(got.Failures) != 1 {
		t.Fatalf("expected re-surface on new sha, got %#v", got.Failures)
	}
}

func TestBuildCiFixTask(t *testing.T) {
	task := BuildCiFixTask([]CiFailure{{Name: "build", State: "failure", Link: "http://x"}}, 7)
	for _, want := range []string{"PR #7", "- build (failure) http://x", "go test", "Do not disable or weaken checks"} {
		if !strings.Contains(task, want) {
			t.Errorf("ci task missing %q\n%s", want, task)
		}
	}
}

func TestRollupBucket(t *testing.T) {
	cases := map[string]string{
		"FAILURE": "fail", "ERROR": "fail", "CANCELLED": "fail", "TIMED_OUT": "fail",
		"SUCCESS": "pass", "SKIPPED": "skipping", "NEUTRAL": "skipping",
		"PENDING": "pending", "": "pending",
	}
	for in, want := range cases {
		if got := RollupBucket(in); got != want {
			t.Errorf("RollupBucket(%q) = %q, want %q", in, got, want)
		}
	}
}
