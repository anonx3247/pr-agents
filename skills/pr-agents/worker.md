# pr-agents worker (one PR per subagent)

You were dispatched by `pr-agents dispatch` as the dedicated subagent for **one
pull request**. You run in an isolated git worktree on your own branch, in a
tmux pane the orchestrator watches. This file is the worker companion to the
main [SKILL.md](SKILL.md).

## Recover your identity from the cwd

Your identity is NOT in your environment — it is keyed on your worktree path in
the shared registry. Read it with:

```bash
pr-agents tool context          # human-readable
pr-agents tool context --json   # id, prNumber, branch, base, mode, simplify, depth, sessionId
```

Use `branch`, `base`, and `mode` from there to drive your push/PR commands. The
seed task message you received also states them inline.

## Atomic commits — always

After **every** coherent, self-contained change:

```bash
git add -A
git commit -m "<type>: <concise description>"
```

- Many small commits, not one big one — the history must read clearly.
- Keep each commit green: run the project's build/tests/linters first.
- Use conventional prefixes (`feat:`, `fix:`, `refactor:`, `test:`, `docs:`,
  `chore:`). Never `git commit --amend` a commit you have already pushed.

## Steps

1. **Implement** the task, committing atomically as you go.
2. **Verify**: run the project's tests/build/lint; fix and re-commit until green.
3. **Simplify (if requested)**: when `mode`/dispatch opted in (`--simplify`,
   shown as `simplify: true` in `pr-agents tool context`), simplify your diff and
   commit it as an atomic `refactor: simplify` commit before opening the PR.
4. **Push and open the PR** for your `mode`:

   **independent** — plain GitHub PR off the base branch:
   ```bash
   git push -u origin "<branch>"
   gh pr create --base "<base>" --head "<branch>" --title "<name>" --fill
   ```

   **stack** — manual stacked GitHub PR; your `<base>` is the previous PR's
   branch:
   ```bash
   git push -u origin "<branch>"
   gh pr create --base "<base>" --head "<branch>" --title "<name>" --fill
   ```

   **graphite** — track your own branch and submit just it (the orchestrator owns
   stack-wide ops):
   ```bash
   gt track --parent "<base>" "<branch>"   # if not already tracked
   gt submit --no-interactive               # your own branch only
   ```
   See [pr-stacks.md](pr-stacks.md) for the full stacking workflow.

5. **Record the PR and signal pushed** (so your pane is labelled and the daemon
   starts polling your PR for merge/close/CI/reviews):
   ```bash
   pr-agents tool set-pr-number <n> --url "<pr-url>"
   pr-agents tool mark-pushed
   ```
   Get the number/url from the `gh pr create` / `gt submit` output, or
   `gh pr view --json number,url`.

6. **Report your result** as your FINAL step so the daemon notifies the
   orchestrator you finished:
   ```bash
   pr-agents tool report-result "Branch <branch>, PR #<n> <url>. <what you did, how you verified, follow-ups>."
   ```

> These worker-plumbing verbs live under the `tool` parent command
> (`pr-agents tool …`). Running them at the top level prints a hint pointing at
> the `tool` form.

## Stay available after opening the PR

The daemon keeps watching your PR while your pane is alive and sends fresh tasks
into it via tmux:

- **Review comments** → it lists each new inline comment and asks you to address
  them in code, commit, push, and reply to each thread:
  ```bash
  pr-agents tool reply-review <commentId> "How you addressed it (or a clarifying question)."
  ```
  Replies do **not** resolve threads — leave resolving to the human reviewer.
  `<commentId>` is the numeric id from the `rc:<id>` in the task.
- **CI failures** → it lists each failing check (deduped per head commit) and asks
  you to reproduce locally, fix, commit, and `git push`. Inspect logs with
  `gh run view --log-failed` when needed. **Never disable or weaken checks to
  make CI pass.**

A still-failing check re-notifies after you push a fix. This only works while
your pane/process is alive.

## Helpers (optional, one level only)

For a focused sub-task you may dispatch a helper in this same worktree (depth 2):

```bash
pr-agents dispatch --name "review" --task "Review the diff for edge cases and report findings."
```

Helpers cannot dispatch further subagents.

## Boundaries

- Stay in this worktree and on this branch; do not touch other branches/worktrees
  or the main checkout.
- Do only this PR's work. Note out-of-scope discoveries in your reported result.
- If blocked or unsure, state the question in your pane output — the orchestrator
  can `pr-agents send` instructions back to you.
