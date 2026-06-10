package core

import (
	"encoding/json"
	"os"
	"path/filepath"
)

// StackStrategy is the default strategy used when the orchestrator stacks
// DEPENDENT PRs: "github" maps to dispatch mode "stack", "graphite" to mode
// "graphite". Standalone PRs stay independent and an explicit mode always wins.
type StackStrategy string

const (
	StrategyGitHub   StackStrategy = "github"
	StrategyGraphite StackStrategy = "graphite"
)

// ProjectConfig is the per-project config stored in the repo so the choice
// travels with the project. It records only the default stacking strategy.
type ProjectConfig struct {
	Strategy StackStrategy `json:"strategy,omitempty"`
}

// ProjectConfigPath returns <repo-root>/.pi/pr-agents.json, creating the .pi
// directory. We keep the same path as the TypeScript implementation so a repo's
// recorded strategy is shared across both tools.
func ProjectConfigPath(cwd string) (string, error) {
	root, err := RepoRoot(cwd)
	if err != nil {
		return "", err
	}
	dir := filepath.Join(root, ".pi")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	return filepath.Join(dir, "pr-agents.json"), nil
}

// LoadProjectConfig reads the project config, returning a zero ProjectConfig
// when the file is missing or unparseable.
func LoadProjectConfig(cwd string) (ProjectConfig, error) {
	path, err := ProjectConfigPath(cwd)
	if err != nil {
		return ProjectConfig{}, err
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return ProjectConfig{}, nil
		}
		return ProjectConfig{}, err
	}
	var cfg ProjectConfig
	if err := json.Unmarshal(raw, &cfg); err != nil {
		return ProjectConfig{}, nil
	}
	return cfg, nil
}

// SaveProjectConfig merges patch into the existing config (only non-zero fields
// of patch overwrite) and writes it back, so a partial update never clobbers
// unrelated fields.
func SaveProjectConfig(cwd string, patch ProjectConfig) error {
	current, err := LoadProjectConfig(cwd)
	if err != nil {
		return err
	}
	if patch.Strategy != "" {
		current.Strategy = patch.Strategy
	}
	path, err := ProjectConfigPath(cwd)
	if err != nil {
		return err
	}
	data, err := json.MarshalIndent(current, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o644)
}
