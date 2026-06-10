// Command pr-agents is a harness-agnostic PR-orchestration CLI. This is a thin
// entrypoint; all logic lives in internal/cli and internal/core.
package main

import (
	"os"

	"github.com/anonx3247/pr-agents/internal/cli"
)

func main() {
	os.Exit(cli.Run(os.Args[1:], os.Stdout, os.Stderr))
}
