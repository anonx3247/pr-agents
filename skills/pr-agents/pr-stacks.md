# pr-agents stacks (dependent PRs)

When PRs depend on each other, ship them as a **stack** of small PRs instead of
one giant PR. `pr-agents` supports two strategies via the dispatch `--mode` flag.
Every branch still gets its **own git worktree** — `dispatch` runs
`git worktree add -b <branch> <worktree> <base>`, branching each PR off the
previous PR's branch — so the branches are created separately and then tracked
(Graphite) or based (manual) on each other.

The default stacking strategy for a project (`github` vs `graphite`) is stored at
`<repo-root>/.pr-agents.json`. Standalone PRs always use `--mode independent`; an
explicit `--mode` always wins.

## Manual GitHub stack (`--mode stack`)

Dispatch PRs **in order**, each based on the previous branch:

```bash
pr-agents dispatch --name "Part A" --task "…"                       # base = main, branch = pi/pr-part-a
pr-agents dispatch --name "Part B" --task "…" --mode stack --stack-on pi/pr-part-a
pr-agents dispatch --name "Part C" --task "…" --mode stack --stack-on pi/pr-part-b
```

Omit `--stack-on` to stack on the most recent depth-1 entry. Each worker opens
its PR with `gh pr create --base <previous-branch> --head <branch>`.

**Restack after a lower PR changes** (rebase the ones above it):
```bash
git checkout pi/pr-part-b && git rebase pi/pr-part-a && git push --force-with-lease
git checkout pi/pr-part-c && git rebase pi/pr-part-b && git push --force-with-lease
```
When a lower PR merges into `main`, retarget the next one
(`gh pr edit pi/pr-part-b --base main`) and rebase it onto `main`.

## Graphite stack (`--mode graphite`)

Requires `gt`. Dispatch bottom-up exactly as above but with `--mode graphite`:

```bash
gt init --trunk main          # once per repo, if not initialised
pr-agents dispatch --name "Part A" --task "…" --mode graphite
pr-agents dispatch --name "Part B" --task "…" --mode graphite --stack-on pi/pr-part-a
```

- **Each worker** tracks and submits only its OWN branch:
  ```bash
  gt track --parent "<base>" "<branch>"
  gt submit --no-interactive
  ```
- **The orchestrator** owns all cross-stack operations — run
  `gt submit --no-interactive --stack` to create/update the whole stack at once
  (keeps every PR's base correct and updates the Graphite web UI), and own
  `gt restack` / `gt sync` / `gt merge` / navigation. `gt restack`/`gt modify`
  **skip branches checked out in other worktrees**, which is why cross-branch
  restacking is the orchestrator's job, not the worker's.

Always pass `--no-interactive` (or `--quiet`) in automated contexts, and avoid
commands needing an interactive editor/selector unless given explicit arguments.

## Rules

- Keep each PR in the stack small and independently reviewable.
- Dispatch and merge **bottom-up**; never stack on an un-dispatched branch.
- After a stack lands, run `pr-agents cleanup` (and `gt sync` for Graphite) to
  tidy worktrees, branches, panes, and merged local branches.
