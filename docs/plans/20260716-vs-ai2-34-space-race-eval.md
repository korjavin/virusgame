# vs-ai2.34: anti-strangulation space-race eval + strangler gates

## Overview

All 12 recent real 12x12 production losses end in no_moves strangulation; the
production eval wins only 17.5-22.5% vs the trivial `arena.MobilityAttacker`
strangler under a deterministic node budget. The coefficient search is DONE
(results below): a Voronoi space-race term (shared multi-source BFS, count of
open cells each player reaches strictly first) at weight 32 wins 69.5% vs the
strangler (n=200 games, w95 [63,75]) and 80% head-to-head vs the frozen
incumbent. This plan finalizes that term with a FIXED constant, deletes the
losing sweep hooks, promotes the two measurement gates into committed
env-opt-in arena tests, and re-pins the eval/search goldens.

An independent post-mortem of the 12 real losses confirmed the space-race
differential flags doom at/before mobility in all 12, with 3-7 turns of
warning; mobility is lagging/deceptive and maxCutLoss (fragility) does not
detect strangulation at all.

## Context (from discovery)

Working tree already contains (committed as WIP baseline on this branch):

- `backend/search/evaluate.go` — `spaceRace()` (shared multi-source BFS,
  nearest-owns partition, contested=-2, tempo-bias comment) wired into
  `evaluateAllWithWorkspace` behind `spaceRaceCoef`, plus THROWAWAY sweep vars
  `fragilityCoef` / `mobilityWeight` / `strangulationDanger` and
  `structuralFragilityPenalty()` — the last three are RULED OUT and must be
  removed.
- `backend/search/zz_tune33.go` — throwaway env hooks; DELETE entirely.
- `backend/arena/zz_strangler_test.go` — sweep harness; replace with the clean
  committed gate.
- `backend/arena/strangulation_eval_test.go` — from vs-ai2.32;
  `TestStrangulationEvalNodeBudget` + `randomLegalOpening` stay (cleaned),
  `TestStrangulationEvalHeadToHead` (wall-clock, ~7% biased) must be dropped.

Measured results, deterministic 12x12, N=1000-node budget, balanced seats,
seeded openings (n = games = 2x openings), vs MobilityAttacker:

| variant | n=80 win rate |
|---|---|
| incumbent / all hooks off (baseline sanity: identical 18/80) | 22.5% |
| fragility coef 1 / 2 | 36.2% / 21.2% |
| danger 900 / 1800 / 3600 / 7200 | 23.8 / 22.5 / 23.8 / 35.0% |
| mobw 4 / 8 / 16 | 27.5 / 35.0 / 36.2% |
| frag1+mobw8 | 51.2% |
| frag1+danger1800, mobw8+danger1800 | 37.5 / 41.2% |
| space 2 / 4 / 6 / 8 / 12 / 16 / 24 | 26.2 / 31.2 / 38.8 / 41.2 / 52.5 / 55.0 / 62.5% |
| **space 32** | **66.2%** (n=200 confirm: **69.5%** w95[63,75]) |
| space 48 | 61.2% (past the peak) |
| space16+mobw8, space6+frag1, space8+frag1 | 48.8 / 31.2 / 41.2% (combos hurt) |

Validation already measured for space=32: differential gate vs frozen
incumbent (n=80, 1000 nodes) 80.0% w95[70.0,87.3]; vs BaseAttacker (n=80)
68.8% w95[58,78] (incumbent: 7.5%); small-board `cmd/arena -production
-opponent=greedy` 60/60=100% wins (p95 604ms only because 3 parallel jobs
saturated the 4-core box — re-run sequentially in Task 5).

Goldens that WILL break with the new constant and must be RE-PINNED (exact
new values, never weakened to ranges):

- `backend/search/evaluate_test.go`: `TestEvaluateWorkspaceGoldenStates`
  (exact score vectors), `TestEvaluateWorkspaceMatchesOriginMainOracle`
  (sha256 digest + comment).
- `backend/search/search_test.go`: `TestSearchMatchesOriginMainAtFixedDepthAndNodes`
  (exact Result structs + comment). Other behavioral tests in search_test.go
  may change chosen moves — inspect each: re-pin exact-value pins; semantic
  assertions (legality, no self-elimination, forced wins) must genuinely pass.

## HARD LANDMINES (do not violate)

- `backend/search/incumbent/*` stays BYTE-FROZEN. Do not touch
  `backend/game/state.go` or `backend/search/search.go` (search_test.go
  re-pinning is fine; the search algorithm itself is not).
- The shipped code uses FIXED constants — no env hooks outside _test files.
- The `4558d2fe` turn-18 `avoids_losing_continuation` regression is beyond any
  static eval's horizon; leave it skipped, do not chase it.
- Wall-clock (600ms) measurement is ~7% biased; all tuning gates stay
  node-budget deterministic.
- Small-board `cmd/arena -production` runs enforce p95 <= 600ms; run them
  SEQUENTIALLY on an otherwise idle machine or the latency check false-fails.
- All go commands run from `backend/`.

## Development Approach

- Testing approach: Regular (code first, then tests); measurements already done.
- Complete each task fully before moving to the next.
- Every task with code changes includes its tests; all tests pass before the
  next task starts.
- Update this plan file when scope changes.

## Progress Tracking
- Mark completed items with `[x]` immediately when done
- Add newly discovered tasks with ➕ prefix
- Document issues/blockers with ⚠️ prefix

## Implementation Steps

### Task 1: Finalize the space-race term with a fixed constant; delete losing hooks
- [x] in `backend/search/evaluate.go`: replace the four sweep vars with a
      single `const spaceRaceWeight = 32` (documented: chosen by the vs-ai2.34
      sweep, peak of the 2..48 curve, 69.5% vs strangler at n=200); delete
      `fragilityCoef`, `mobilityWeight` (restore the literal `1` weight in the
      mobility term), `strangulationDanger` and its `if` branch, and
      `structuralFragilityPenalty()`
- [x] make the space term unconditional: always compute
      `space := spaceRace(...)` and add
      `normalized(space[player-1], area, spaceRaceWeight)`; keep the
      Voronoi/tempo-bias comments as written
- [x] delete `backend/search/zz_tune33.go` entirely
- [x] add `TestSpaceRacePartition` in `backend/search/evaluate_test.go`: small
      hand-built board; assert each player's first-reach count, that
      equidistant cells are contested (counted for nobody), and that walls
      (non-empty cells) block the BFS
- [x] run `go build ./... && go test ./search/...` — expect ONLY the three
      known golden/oracle tests failing (fixed in Task 2); `TestSpaceRacePartition`
      and all other tests must pass
- ➕ [x] `TestLocalBaseSafetyIsStableAcrossBoardSizes` also failed: it compared
      full `evaluate()` across board sizes, and the space-race term is global
      by design (same local structure claims a different board fraction on
      6x6 vs 6x12). Fixed properly (not weakened): the test now subtracts the
      space term's exact contribution and keeps pinning the local
      base-safety terms it was written for.

### Task 2: Re-pin eval/search goldens to the new evaluator
- [x] re-pin `TestEvaluateWorkspaceGoldenStates` want-vectors in
      `backend/search/evaluate_test.go` to the actual new outputs (terminal /
      eliminated mate-score sentinels must be unchanged — if they changed,
      something is wrong, stop and investigate)
- [x] re-pin the digest in `TestEvaluateWorkspaceMatchesOriginMainOracle`;
      update its comment to say it pins the vs-ai2.34 space-race evaluator
      (self-consistency oracle), no longer origin/main bf74a44
- [x] re-pin `TestSearchMatchesOriginMainAtFixedDepthAndNodes` expected
      `Result` structs in `backend/search/search_test.go`; update comment
      likewise; verify re-pinned actions are still legal moves
- [x] run `go test ./search/... ./arena/... ./game/...`; for any OTHER
      failure: exact-value pin → re-pin; semantic assertion → investigate and
      fix properly, never weaken or skip
- [x] run `go test ./...` from backend — must be fully green
- ➕ only the `eliminated` golden vector changed (symmetric fixtures cancel the
  space term across players): now `{-500000000, -768, 768, -500000000}`;
  sentinels unchanged. New oracle digest `fbb86460…`. Search pins: same
  actions as before (legality now asserted in-test via `state.Apply`), scores
  minimax 26644 (depth+nodes), maxn 6242 (depth) / 9425 (nodes).

### Task 3: Promote the primary strangler gate into backend/arena
- [x] replace `backend/arena/zz_strangler_test.go` with
      `backend/arena/strangler_gate_test.go`: `TestVsStrangler`, opt-in via
      `VS_STRANGLER=1`, default 40 openings (`VS_STRANGLER_OPENINGS`
      override), deterministic N=1000-node budget, balanced seats, SAME seeded
      openings for candidate and frozen incumbent, each vs `MobilityAttacker`
      AND `BaseAttacker`, logging wins/games + Wilson 95% CI per (engine,
      opponent) pair; proper doc comment: what a strangler is, why win-rate vs
      a strangler (not vs the incumbent) is the objective, reproduce command
- [x] keep the measurement non-failing except on illegal/stalled decisions
      (same convention as `TestStrangulationEvalNodeBudget`), fail also if the
      candidate's Mobility win-rate is <= the incumbent's (regression floor)
- [x] run it once at 4 openings (`VS_STRANGLER=1 VS_STRANGLER_OPENINGS=4
      go test ./arena -run TestVsStrangler -v`) to prove wiring; full n=40 run
      happens in Task 5
- [x] run `go test ./arena/...` (without env) — gate must skip cleanly
- ➕ wiring run (n=4 openings, 8 games/pair): candidate vs Mobility 6/8=75.0%,
  vs Base 3/8=37.5%; incumbent vs Mobility 1/8=12.5%, vs Base 2/8=25.0% —
  directionally consistent with the n=200 sweep; full-n numbers land in Task 5.

### Task 4: Promote the secondary incumbent-differential gate
- [x] clean `backend/arena/strangulation_eval_test.go`: DELETE
      `TestStrangulationEvalHeadToHead` (wall-clock, ~7% seat/timing bias —
      say so in the file comment); keep `TestStrangulationEvalNodeBudget` and
      `randomLegalOpening`
- [x] rename its env vars to the vs-ai2.34 convention: opt-in
      `VS_STRANGLER_DIFF=1`, `VS_STRANGLER_OPENINGS`, `VS_STRANGLER_NODES`
      (defaults 40 openings / 1000 nodes); update the doc comment with the
      reproduce command and the parity property (frozen-vs-frozen reads 50%)
- [x] wiring check at 4 openings, then `go test ./arena/...` without env —
      must skip cleanly; `go vet ./...` clean
- ➕ wiring run (4 openings, 8 games, 1000 nodes): candidate 5/8=62.5%
  w95[30.6, 86.3] vs frozen incumbent, illegal=0 stalled=0; gate skips
  without env; vet clean. Full-n run lands in Task 5.

### Task 5: Verify acceptance criteria (full battery, sequential)
- [x] `go build ./... && go vet ./... && go test ./...` — green
- [x] `go test -race ./search/... ./arena/...` — green
- [x] primary gate at n>=40 openings:
      `VS_STRANGLER=1 go test ./arena -run TestVsStrangler -v -timeout 120m`;
      record all four win rates + Wilson CIs in this plan; candidate must be
      >50% vs MobilityAttacker (expected ~62-70%), incumbent ~17-25%
- [x] secondary gate: `VS_STRANGLER_DIFF=1 go test ./arena -run
      TestStrangulationEvalNodeBudget -v -timeout 60m`; candidate >=47.5%
      (expected ~80%); record the number
- [x] small boards, SEQUENTIALLY, nothing else running:
      `go run ./cmd/arena -production -opponent=greedy` (>=75% required) then
      `go run ./cmd/arena -production -opponent=legacy` (>=85% required);
      record win rates
- [x] ⚠️ small-board latency check — OWNER-AUTHORIZED FIX (supersedes earlier
      guidance): the binary's gate requires p95 <= 600ms while
      `search.ProductionBudget` is itself 600ms; the anytime search saturates
      its budget BY DESIGN (p50 reads ~600.7ms), so the check fails on pure
      stop-jitter (preview runs with the space term: greedy 60/60=100% p95
      604.9ms, legacy 58/60=96.7% p95 601.4ms — exit 1 in both purely on
      latency). The owner has authorized making this gate robust: in
      `backend/cmd/arena/main.go` change the two hardcoded
      `Percentile(95) <= 600*time.Millisecond` checks (legacy + greedy) to
      `Percentile(95) <= search.ProductionBudget + 50*time.Millisecond`
      (stop-jitter tolerance), with a short comment citing the vs-ai2.34 owner
      authorization. Do NOT change `search.ProductionBudget` itself in this PR
      (a real budget increase is vs-ai2.36 scope), and keep the existing
      per-decision `searchBudget` (850ms) hard cap as-is. After the change
      both small-board runs must exit 0 with win rates >=75% (greedy) /
      >=85% (legacy) and illegal=0 stalled=0. Record exact win rates and
      p50/p95 numbers honestly in this plan.
- [x] record every measured number with ➕ notes in this plan file
- ➕ full-battery results (2026-07-16, otherwise-idle 4-core box):
  - build/vet/`go test ./...` green; `go test -race ./search/... ./arena/...` green.
  - primary gate (n=40 openings, 80 games/pair, 1000 nodes, PASS in 379s):
    candidate vs MobilityAttacker 53/80=66.2% w95[55.4, 75.7];
    candidate vs BaseAttacker 55/80=68.8% w95[57.9, 77.8];
    incumbent vs MobilityAttacker 18/80=22.5% w95[14.7, 32.8];
    incumbent vs BaseAttacker 6/80=7.5% w95[3.5, 15.4].
  - secondary gate (80 games, 1000 nodes, PASS in 216s): candidate 64/80=80.0%
    w95[70.0, 87.3] vs frozen incumbent, illegal=0 stalled=0 — matches the
    n=80 discovery measurement exactly.
  - small boards, sequential, with the owner-authorized latency cap
    (`search.ProductionBudget + 50ms`) in `cmd/arena/main.go`:
    greedy 60/60=100.0% p50=600.7ms p95=601.0ms max=607.5ms, exit 0;
    legacy 59/60=98.3% p50=600.7ms p95=601.0ms max=604.0ms, exit 0;
    illegal=0 maxed=0 stalled=0 in both. Both smoke matrices clean.

### Task 6: Documentation touch-up
- [ ] add a short "Strangler gates" section to `backend/arena/README.md`: the
      two opt-in env vars, one reproduce command each, and the vs-ai2.34
      rationale (tune against a strangler, not the incumbent)
- [ ] verify no stray references to the deleted env hooks
      (`VS_AI2_33_FRAG|VS_AI2_32_MOBW|VS_AI2_32_DANGER|VS_AI2_34_SPACE|VS_AI2_32_MEASURE|VS_AI2_32_NODEGATE`)
      remain outside docs/plans: `git grep` for them

## Technical Details

- `spaceRace` seeds one BFS queue with every active player's base-connected
  cells (distance 0), expands through `game.Empty` cells only using the
  existing 8-neighbour `neighbors()`, and counts strictly-first arrivals per
  player; ties at equal distance mark the cell contested (`-2`). Buffers
  (`spaceDist []int16`, `spaceOwner []int8`, `spaceQueue []int`) live in
  `evalWorkspace` (per-searcher, not shared) and are sized in `ensure()`.
- The term enters raw scores as `normalized(count, area, spaceRaceWeight)` =
  `count*32*1000/area`, i.e. ~889 points per cell on 12x12 — deliberately
  dominant over the ~7-point linear mobility term; the existing cross-player
  raw differencing turns it into the own-minus-opp space differential.
- Wilson CI helper: `arena.Wilson95`. Agents: `arena.TelemetryNodeBudget(n,
  frozen)`, `arena.Instrument(MobilityAttacker|BaseAttacker)`.

## Post-Completion
*No checkboxes — informational.*

- Executor (not ralphex): push branch, open DRAFT PR
  "vs-ai2.34: anti-strangulation eval + strangler gate", comment results on bd
  vs-ai2.34. Do not merge.
- Production soak: watch real 12x12 games for no_moves losses after deploy.
- PR note: the space differential has a known ±40-50 cell tempo-phase bias in
  raw per-turn measurement; the shipped term is side-to-move-independent
  (shared BFS both players, same position) so the bias cancels in same-ply
  comparisons — documented in the spaceRace comment.
