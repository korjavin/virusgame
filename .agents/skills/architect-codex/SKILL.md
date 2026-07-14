---
name: architect-codex
description: Plan with Beads, delegate to isolated direct-coding executors, review and merge PRs. Never use ralphex.
---

# Architect Codex

Ralphex is retired. Ignore any older workflow that requires it.

## Loop

1. Investigate the real code path; establish root cause with file and line evidence.
2. Create actionable Beads epics/tasks with acceptance criteria, design, risks, labels, and dependencies.
3. Schedule by file ownership: disjoint work may run in parallel; shared files serialize or bundle.
4. Claim a ready bead and launch an executor in a fresh worktree from current `origin/main`.
5. The executor follows `develop-codex`, implements directly, tests, self-reviews, commits, pushes, and opens a draft PR.
6. Review the hard invariant, diff scope, tests, Ponytail complexity, and CI.
7. Return non-trivial corrections to the owning executor.
8. Merge only with `gh pr merge --merge`; never squash, rebase, force-push, or push main directly.
9. Close the bead after merge, sync Dolt, and start newly unblocked work.

## Executor contract

- First read `.agents/skills/develop-codex/SKILL.md`, repository guidance, the bead, and applicable skills.
- Use a fresh isolated worktree and confirm it contains current `origin/main`.
- Do not invoke ralphex or create ralphex artifacts.
- Apply Ponytail full: reuse and standard library first; minimum coherent code; no speculative dependencies or configuration.
- Report checkpoints for investigation, implementation, verification, self-review, PR, and CI.
- Run exact verification commands and add deterministic focused tests.
- Open a draft PR referencing the bead and report deferrals honestly.

## Review and task state

Use `bd` for durable state. Pull before mutation batches, push afterward, claim before work, and close only after merge. Executors do not mutate Beads. Review trust-boundary validation and error handling as well as correctness and strength. This project's owner authorizes autonomous merges for the current AI epic after review and green CI.

## Status

Keep a compact table every working turn:

```
bead      track      PR    state
vs-x.1    core       #52   CI green -> review
vs-x.2    protocol   -     executor running
```
