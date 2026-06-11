# pr-agents cleanup

Removes the leftovers of finished PR work — worktrees, local branches, and tmux
panes for PRs that are now **merged** or **closed** — plus orphaned worktrees
under `<repo>/.worktrees/`.

## How to run

```bash
pr-agents cleanup --dry    # preview what would be removed
pr-agents cleanup          # apply
```

Run it from the **main repo** as the orchestrator, not from inside a worktree.

## What it does

For every PR subagent in the current session scope:

1. Decide if it is finished:
   - with a PR number → ask GitHub via `gh pr view <n> --json state,mergedAt`
     (MERGED / CLOSED → finished);
   - otherwise → check whether its branch is merged into the default branch.
2. If finished: kill its tmux pane, remove its worktree, delete its local branch,
   and drop it from the registry. Depth-2 helpers are dropped when their parent
   PR is reaped or their pane is gone.
3. Prune orphaned `<repo>/.worktrees/` directories and run `git worktree prune`.

Still-open PRs (branch not merged) are left untouched and reported under "Still
active".

## Notes

- `gh` is used for PR state; without it, cleanup falls back to "branch merged
  into the default branch" detection only.
- A concurrent session's worktrees are never pruned (cleanup guards against the
  full registry).
- For Graphite stacks, additionally run `gt sync` to delete merged branches
  Graphite tracks locally.
