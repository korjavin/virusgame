---
name: develop-codex
description: Implement one Beads task directly in an isolated worktree, verify it, and open a draft PR. Never use ralphex.
---

# Develop Codex

Ralphex is retired. Do not invoke it or create ralphex plan, progress, worktree, or review artifacts.

## Preconditions

1. Read repository guidance, the assigned bead, and applicable skills such as Ponytail.
2. Do not claim, close, or mutate Beads; the architect owns task state.
3. Fetch `origin main` and work on the assigned isolated branch/worktree from current `origin/main`.
4. Never push main, force-push, or alter another worktree.

## Direct implementation

1. Trace the complete affected flow and callers before editing.
2. Form a concise private plan from the acceptance criteria; ask only if a missing product decision changes the solution.
3. Apply Ponytail full: reuse, standard library, installed dependency, then minimum custom code.
4. Implement the smallest coherent root-cause solution without speculative config, frameworks, services, or abstractions.
5. Add focused deterministic tests for non-trivial logic and validate trust boundaries.
6. Run formatting and the architect's exact verification commands; diagnose failures rather than weakening tests.
7. Self-review the full diff for invariants, acceptance, accidental churn, error/concurrency paths, and complexity.
8. Commit intentionally, push the feature branch, and open a draft PR referencing the bead.
9. Watch CI and fix red checks with up to two focused passes before reporting a concrete blocker.

## Checkpoints

Report: investigation complete; implementation complete; verification commands/results; self-review findings; branch/commits/draft PR/CI; honest deferrals.

## PR and completion

- Title begins with the Beads ID.
- Body covers purpose, implementation, verification, decisions, risks, and deferrals.
- Keep the PR draft until architect review.
- Do not merge or close the bead.
- Remain available for review corrections.

Implementation is direct. No ralphex.
