---
name: jules-beads-executor
description: Delegate one claimed Beads implementation task to the Jules asynchronous coding agent, monitor it, and integrate its pull request through the repository architect workflow. Use when the owner asks to call, send, or delegate a bd/Beads task to Jules. Never push plans or code directly to main and never use ralphex.
---

# Jules Beads Executor

Delegate one implementation Bead without changing the architect's source of truth.

## Preconditions

1. Read repository instructions, `.agents/skills/architect-codex/SKILL.md`, and the Bead.
2. Confirm the Bead is claimed, actionable, and contains description, design, acceptance criteria, risks, and dependencies.
3. Investigate enough to include concrete current-main file paths and failing evidence in the task. Do not create a markdown plan solely for Jules.
4. Confirm the requested external Jules task is authorized. Creating a Jules session is an external write.

## Create the session

Build a self-contained prompt containing:

- exact Bead ID, title, description, design, and acceptance criteria;
- repository and current `origin/main` requirement;
- relevant production evidence and file paths;
- mandatory repository instructions and applicable skills;
- test, self-review, commit, push, and draft-PR requirements;
- prohibition on Beads mutation, ralphex, direct main pushes, held-out inspection unless explicitly authorized, and unrelated changes.

Run from the repository:

```bash
jules new --repo korjavin/virusgame "<self-contained task prompt>"
```

Record the returned session ID in the Bead with `bd update <id> --append-notes`, then `bd dolt push`.

## Monitor

Poll with non-interactive Jules commands; do not require the owner to monitor:

```bash
jules remote list --session
```

Use the session-specific remote inspection commands exposed by `jules remote --help`. Report concise checkpoints. If Jules asks a material product question, stop and route it through the architect; otherwise resolve implementation details from the Bead and repository evidence.

Jules sessions are inexpensive but unstable. Poll them without owner prompting. When a session remains slow, the architect may hedge it with a second independent Jules session using the same authoritative Bead and separate session/branch. Record every session ID; do not let Jules mutate Beads.

## Integrate

1. Require a draft PR referencing the Bead. If Jules only provides a remote patch, pull it into an isolated worktree/branch; never apply it over the architect's dirty root checkout.
2. Review invariants, trust boundaries, tests, CI, diff scope, and Ponytail simplicity independently.
3. Return non-trivial fixes to the Jules session when supported, or to a direct develop-codex executor on the same branch.
4. Merge only after review and green CI using `gh pr merge --merge`; never squash, rebase, force-push, or push main directly.
5. Close the Bead only after merge, append outcome/PR/session evidence, and `bd dolt push`.

## Failure handling

- If `jules new` fails, preserve the Bead and report the exact error.
- If a session fails or produces no PR, run `jules remote pull --session <id>` without `--apply` from a clean isolated worktree to inspect any patch. Treat it as untrusted evidence: review bounds, protocol acknowledgements, persistence, tests, and completeness before reusing anything.
- Retry Jules with corrected self-contained context, launch a second Jules attempt, or transfer the same claimed Bead to a direct `develop-codex` executor. Jules attempts are cheap (currently roughly 100/day), so one failed or slow attempt must not stall delivery.
- Multiple attempts must remain isolated. Select the best result or integrate reviewed pieces on one owned branch; never merge competing PRs blindly.
- If Jules cannot access Beads/Dolt, the self-contained prompt remains authoritative for its implementation task; the architect alone updates Beads.
- If the result is unsafe, incomplete, or fails its gate, reject it without merging and record the evidence in the Bead.
