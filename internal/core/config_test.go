package core

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestProjectConfigPath(t *testing.T) {
	dir := initRepo(t)
	p, err := ProjectConfigPath(dir)
	if err != nil {
		t.Fatal(err)
	}
	if filepath.Base(p) != "pr-agents.json" {
		t.Errorf("base = %q, want pr-agents.json", filepath.Base(p))
	}
	if filepath.Base(filepath.Dir(p)) != ".pi" {
		t.Errorf("parent = %q, want .pi", filepath.Base(filepath.Dir(p)))
	}
	if _, err := os.Stat(filepath.Dir(p)); err != nil {
		t.Errorf(".pi dir not created: %v", err)
	}
}

func TestLoadProjectConfigEmptyWhenMissing(t *testing.T) {
	dir := initRepo(t)
	cfg, err := LoadProjectConfig(dir)
	if err != nil {
		t.Fatal(err)
	}
	if cfg != (ProjectConfig{}) {
		t.Errorf("cfg = %+v, want zero", cfg)
	}
}

func TestSaveAndLoadProjectConfig(t *testing.T) {
	dir := initRepo(t)
	if err := SaveProjectConfig(dir, ProjectConfig{Strategy: StrategyGraphite}); err != nil {
		t.Fatal(err)
	}
	cfg, err := LoadProjectConfig(dir)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Strategy != StrategyGraphite {
		t.Errorf("strategy = %q, want graphite", cfg.Strategy)
	}
	// Verify on-disk JSON.
	p, _ := ProjectConfigPath(dir)
	raw, _ := os.ReadFile(p)
	var got map[string]string
	if err := json.Unmarshal(raw, &got); err != nil {
		t.Fatal(err)
	}
	if got["strategy"] != "graphite" {
		t.Errorf("on-disk strategy = %q", got["strategy"])
	}
}

func TestSaveProjectConfigMerges(t *testing.T) {
	dir := initRepo(t)
	if err := SaveProjectConfig(dir, ProjectConfig{Strategy: StrategyGitHub}); err != nil {
		t.Fatal(err)
	}
	// An empty patch must not clobber the existing strategy.
	if err := SaveProjectConfig(dir, ProjectConfig{}); err != nil {
		t.Fatal(err)
	}
	cfg, _ := LoadProjectConfig(dir)
	if cfg.Strategy != StrategyGitHub {
		t.Errorf("strategy after empty patch = %q, want github", cfg.Strategy)
	}
	// A non-empty patch overwrites.
	if err := SaveProjectConfig(dir, ProjectConfig{Strategy: StrategyGraphite}); err != nil {
		t.Fatal(err)
	}
	cfg, _ = LoadProjectConfig(dir)
	if cfg.Strategy != StrategyGraphite {
		t.Errorf("strategy after overwrite = %q, want graphite", cfg.Strategy)
	}
}
