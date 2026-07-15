---
name: architect-codex
description: Architect autonomous Virusgame delivery with Beads, parallel direct or Jules executors, evidence gates, PR review, merge, deployment verification, and production-game learning. Never use ralphex.
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

## Owner operating contract

- Work autonomously through implementation, review, CI, merge, and production verification. Ask the owner only for a material product/authority decision; do not make them monitor agents or run arenas.
- Poll asynchronous executors and PR/CI state proactively. Send concise checkpoints during active work and signal the owner for play testing only after production is verified.
- Treat Claude-owned skills under `~/.claude/skills` as read-only references. Maintain adaptations in repository-owned `.agents/skills`.
- Use every safe concurrency slot for disjoint file ownership. Serialize or integrate competing edits to the same files explicitly.
- Record failures and rejected designs in Beads. Do not open or merge a PR merely because an implementation exists; stop at the first failed hard gate.

## Executor selection and recovery

- Prefer direct `develop-codex` executors for precise branch ownership and review loops.
- Jules is a cheap, asynchronous, failure-prone executor, not a model. Use `jules-beads-executor` when useful.
- A failed Jules session is not exceptional: inspect its remote patch without applying it to a dirty checkout, record useful evidence, correct the prompt, and retry Jules or hand the Bead to a direct executor.
- If a Jules session is slow, launch a second independent Jules session for the same self-contained Bead when capacity is cheap (the owner currently has roughly 100 attempts/day). Keep separate branches/sessions, never let both mutate Beads, and review/select or combine results deliberately. Do not merge duplicate implementations blindly.
- Never wait indefinitely for one executor while independent work is ready.

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

## Virusgame AI delivery policy

- Ignore legacy JS/Go/WASM AI authority. Reuse an idea only when current evidence supports it.
- Default board size is 12x12. Optimize competitive 1v1 through 20x20. Multiplayer UI may allow 28x28; treat it and boards above 20 as legality/deadline smoke unless a Bead explicitly expands optimization scope. Preserve general rule support up to 50x50.
- Use sampled, predeclared arena shards rather than exhaustive board sweeps: 5x5, 12x12, 20x20, then representative multiplayer. Stop at the first failed shard. Never inspect held-out data before the design and TRAIN gates are frozen.
- Exploit available server cores in production-safe designs, but keep deterministic single-worker/node-budget modes for fair comparisons and reproducible CI.
- Learn from complete production games exposed by `/last_games`; persist/import them into the TRAIN/production-motif corpus without contaminating held-out data.
- Preserve owner strategy evidence in tests and design: attackers are vulnerable while extending; value durable backup routes after gaining space; do not fill a ring around one's own base without a causal defense; take immediate base cuts, eliminations, and hardened near-base opportunities.
- Production readiness requires legal fallback, zero illegal/stalled/avoidable self-elimination, variable-board safety, focused/full/race tests, truthful telemetry, sampled strength gates, green CI, merge deployment, and live endpoint/bot verification.

## Status

Keep a compact table every working turn:

```
bead      track      PR    state
vs-x.1    core       #52   CI green -> review
vs-x.2    protocol   -     executor running
```
