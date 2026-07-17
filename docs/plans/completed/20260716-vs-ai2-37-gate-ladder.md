# vs-ai2.37: Gate ladder + SPRT early stopping

## Overview
Keep arena strength measurement honest as the bot improves. Three deliverables,
ALL in `backend/arena/**` (test/measurement code only — zero changes to
`search/**`, `game/**`, `cmd/**` production paths):

1. **Sequential early-stopping (SPRT-style)** for the gate tests: play balanced
   seeded-opening pairs in a fixed deterministic order, update a running Wilson
   95% CI after each pair, stop early when the CI lies entirely above/below the
   decision threshold, cap at the full n. Deterministic: same (code, seed) =>
   same game sequence => same verdict.
2. **Hybrid sparring opponents**: two cheap heuristic stranglers —
   (a) mobility+base mix, (b) a cut-seeker preferring moves that attack the
   candidate's articulation points (reimplemented arena-side; search's
   articulation code is unexported and off-limits). Measure current eval AND
   frozen incumbent vs each; an opponent qualifies for a ladder slot only if it
   beats the current (space-race) eval >40%.
3. **Ladder report test**: one env-opt-in test running the current production
   eval (node-budget deterministic) vs the full ladder (greedy, legacy,
   BaseAttacker, MobilityAttacker, qualifying hybrids, frozen incumbent) over
   seeded openings, printing ONE table with win rates + Wilson CIs +
   games-played (demonstrating early stopping). This is THE pre-merge strength
   report for future eval/search PRs.

## Context (from discovery)
- `backend/arena/arena.go`: `Play(Match)`, `Report`, `Report.Add`,
  `Wilson95(wins,games) Interval`, `Interval{Low,High}` (percent). Node-budget
  results are wall-clock-independent (deterministic).
- `backend/arena/agents.go`: baseline agents `MobilityAttacker`, `BaseAttacker`,
  `Greedy`, `Legacy`, `Random`, plus telemetry wrappers `TelemetryNodeBudget(nodes, frozen)`
  (candidate=false / frozen incumbent=true) and `Instrument(Agent)`. Helpers
  `immediateMobility`, `opponentBaseDistance`, `basePosition`, `activeCount`.
- `backend/arena/strangulation_eval_test.go`: `playBalancedOpenings(t,label,openings,a,b) Report`
  (plays both seats of `randomLegalOpening(t, i+1)` snapshots, 12x12),
  `stranglerOpenings(t)` (default 40, env `VS_STRANGLER_OPENINGS`),
  `randomLegalOpening(t,seed)`. `TestStrangulationEvalNodeBudget` (env `VS_STRANGLER_DIFF`).
- `backend/arena/strangler_gate_test.go`: `TestVsStrangler` (env `VS_STRANGLER`),
  candidate+incumbent vs MobilityAttacker+BaseAttacker at 1000 nodes; regression
  floor `candidate/MobilityAttacker > incumbent/MobilityAttacker`.
- Adjacency is 8-neighbour (king moves), matching search's `neighbors`. Move
  targets are `game.Pos`; `state.At(pos)` returns `(Cell, bool)` with
  `Cell.Owner`/`Cell.Kind` (`Empty`/`Normal`/`Base`/`Fortified`). Bases at
  `basePosition(state, player)`.
- Articulation logic in `search/evaluate.go` (`analyzeWithConnectivity`,
  `articulationPointsInto`) is UNEXPORTED — must reimplement cheaply arena-side.

## Development Approach
- **Testing approach**: Regular. The deliverables ARE tests; each new helper
  ships one small deterministic self-check in the same task.
- Deterministic node budgets and fixed-seed opening order throughout — NEVER
  assert on wall-clock (machine runs parallel work; node-budget results are
  load-immune, timing is not).
- Small focused changes; run `go build ./... && go vet ./... && go test ./arena/...`
  after each task; all tests pass before the next task.
- Zero production-path edits: only `backend/arena/**`.

## Testing Strategy
- **Unit/self-checks**: every new helper gets one deterministic assert-based
  test (no new frameworks). Reuse existing `Wilson95`, `Play`, `Report`.
- The two slow gates and the ladder stay env-opt-in (skip by default) so
  `go test ./...` remains fast; a tiny opening count keeps wiring checks quick.
- No e2e (no UI).

## Progress Tracking
- Mark completed items `[x]` immediately.
- `➕` new tasks, `⚠️` blockers.

## What Goes Where
- Implementation Steps: all arena code + tests.
- Post-Completion: run the full ladder report and paste its table into the PR.

## Implementation Steps

### Task 1: Sequential early-stopping engine
- [x] Add `backend/arena/sequential_test.go` (package `arena`) with a pure
      decision helper `wilsonDecision(wins, games int, thresholdPct float64, minGames int) (stop bool, above bool)`:
      returns stop=true only when `games >= minGames` AND the Wilson95 interval
      is entirely above (`Low > thresholdPct`) or entirely below (`High < thresholdPct`)
      the threshold; `above` reports which side.
- [x] Add `sequentialResult` struct (`Report`; plus `ThresholdPct float64`,
      `Stopped bool`, `Above bool`) and
      `playSequentialOpenings(t *testing.T, label string, maxOpenings int, thresholdPct float64, minGames int, a, b TelemetryAgent) sequentialResult`:
      iterate opening indices in a FIXED-SEED permutation
      (`rand.New(rand.NewSource(sequentialOrderSeed)).Perm(maxOpenings)`, const
      `sequentialOrderSeed`), for each index play BOTH seats of
      `randomLegalOpening(t, uint64(idx)+1)` (reuse existing snapshot logic —
      same snapshots as `playBalancedOpenings`), `report.Add` each, then after
      the pair call `wilsonDecision`; stop early when it says stop, else run to
      `maxOpenings` (cap). Fail on illegal/stalled/maxed like `playBalancedOpenings`.
- [x] `TestSequentialEarlyStopDeterministic`: with a deterministic pair of fake
      `TelemetryAgent`s (e.g. `a` always wins vs a passive `b`) assert the same
      games-played and verdict across two runs; assert a lopsided matchup stops
      BELOW the 2*maxOpenings cap (early stopping saves games); assert a coin-flip
      matchup (agents that split by seat) runs to the cap. Use small maxOpenings.
- [x] run `go test ./arena/... -run Sequential` — must pass before next task.

### Task 2: Hybrid sparring opponents (mobility+base, cut-seeker)
- [x] Add `backend/arena/sparring.go` (package `arena`): `MobilityBaseAttacker(state) (game.Action, bool)`
      — score = `immediateMobility(next, actor)` minus opponent-reply mobility
      (like MobilityAttacker) PLUS a base-pressure term `-k*opponentBaseDistance(state, actor, target)`
      and a capture bonus; deterministic board-order tie-break. Cheap, no search.
- [x] Add arena-side connectivity helper in `sparring.go`:
      `opponentArticulations(state game.State, victim game.Player) map[game.Pos]bool`
      — BFS the victim's base-connected component over 8-neighbours, run a simple
      iterative/recursive Tarjan articulation-point pass over that component,
      return the cut cells. Keep it a plain cheap heuristic (recompute per call;
      `ponytail:` comment noting O(cells) recompute, memoise only if the ladder
      gets slow).
- [x] Add `CutSeeker(state) (game.Action, bool)`: pick the highest-priority
      victim (nearest active opponent base), compute its articulation points,
      score moves that capture an articulation cell or land 8-adjacent to one
      highest; fall back to MobilityAttacker-style mobility when no cut is
      reachable; deterministic tie-break. Cheap heuristic only.
- [x] `TestSparringAgentsLegalAndDeterministic`: for several `randomLegalOpening`
      snapshots, assert `MobilityBaseAttacker` and `CutSeeker` return legal moves
      (`state.Apply` succeeds) and identical action on repeated calls; add one
      crafted position with a known articulation point and assert `CutSeeker`
      targets/adjoins it.
- [x] run `go test ./arena/... -run Sparring` — must pass before next task.

### Task 3: Wire early stopping into the existing gates
- [x] Update `TestVsStrangler` (`strangler_gate_test.go`) to measure each
      (engine, opponent) pair with `playSequentialOpenings` (threshold 50%),
      log `wins/games win% wilson95 games-played/cap` per pair, and keep the
      regression floor `candidate/MobilityAttacker win% > incumbent/MobilityAttacker win%`.
      Keep env gating (`VS_STRANGLER`, `VS_STRANGLER_OPENINGS`).
- [x] Update `TestStrangulationEvalNodeBudget` (`strangulation_eval_test.go`) to
      use `playSequentialOpenings` (candidate vs frozen incumbent, threshold 50%)
      and log the CI + games-played/cap. Keep env gating + `VS_STRANGLER_NODES`.
- [x] Sanity-run both with a tiny opening count and the env flags set to confirm
      wiring (`VS_STRANGLER=1 VS_STRANGLER_OPENINGS=4`, `VS_STRANGLER_DIFF=1 VS_STRANGLER_OPENINGS=4`).
- [x] run `go test ./arena/...` (default, gates skipped) — must pass before next task.

### Task 4: Hybrid qualification + ladder report
- [x] Add `backend/arena/ladder_test.go` `TestLadderReport` gated by env
      `VS_LADDER=1` (skip otherwise). Node budget default 1000 (env
      `VS_LADDER_NODES`), openings cap default from `stranglerOpenings`-style env
      `VS_LADDER_OPENINGS`. Roster of opponents (all `TelemetryAgent`):
      Greedy, Legacy(seed), BaseAttacker, MobilityAttacker, MobilityBaseAttacker,
      CutSeeker, frozen-incumbent (`TelemetryNodeBudget(nodes, true)`).
- [x] For each opponent run `playSequentialOpenings` for the current eval
      (`TelemetryNodeBudget(nodes, false)`, threshold 50%). For the two hybrids
      ALSO run frozen incumbent vs the hybrid (qualification needs eval AND
      incumbent numbers). Compute qualifies = eval win% < 60 (i.e. hybrid beats
      the eval >40%).
- [x] Print ONE aligned table via `t.Log`: columns `opponent | wins/games |
      win% | wilson95 [low,high] | games-played/cap | qualifies` (qualifies blank
      for fixed rungs, yes/no for hybrids). Assert no illegal/stalled/maxed games.
- [x] run `VS_LADDER=1 VS_LADDER_OPENINGS=4 go test ./arena/ -run TestLadderReport -v`
      to confirm the table renders and early stopping shows games<cap somewhere.
      (Table renders; at 4 openings the cap=8 equals minGames=8 so every rung
      runs to cap — games<cap demonstration happens at the real count in Task 5.)

### Task 5: Verify acceptance criteria + document
- [x] `go build ./...`, `go vet ./...`, `go test ./...`, `go test -race ./arena/...`
      all green from `backend/`.
- [x] Run the ladder report once at a real opening count and capture the table
      for the PR body; confirm at least one clear-cut opponent shows
      games-played < cap (early stopping saved games).
      (40 openings, PASS in 176.7s; table in `ladder-report-full.log`. Early
      stopping saved games on 8 of 9 rungs, e.g. eval vs Greedy 12/80, eval vs
      incumbent 8/80; only eval vs MobilityAttacker ran to 40/80.)
- [x] Update `backend/arena/README.md`: document the ladder report, early
      stopping, the two hybrid opponents, and the new env vars
      (`VS_LADDER`, `VS_LADDER_NODES`, `VS_LADDER_OPENINGS`).

## Technical Details
- Wilson stop rule (percent space): stop when `games >= minGames` and
  (`Wilson95(wins,games).Low > thresholdPct` OR `.High < thresholdPct`).
  `minGames` guards against a premature stop on the first pair (use e.g. 8).
- Determinism: opening ORDER is a fixed-seed permutation; opening CONTENT reuses
  `randomLegalOpening(t, idx+1)`; agents are node-budget/heuristic (no wall
  clock). Same code+seed => identical sequence, stop point, and verdict.
- Articulation (arena-side): BFS victim's base-connected component (8-neighbour),
  Tarjan low-link DFS over that subgraph; a non-root vertex `u` is a cut vertex
  if some child `v` has `low[v] >= disc[u]`; the root is a cut vertex iff it has
  >1 DFS child. Plain and cheap — recompute per decision.
- All new opponents are `Agent`s wrapped with `Instrument` at test call sites.

## Post-Completion
**Manual verification**:
- Full ladder report table pasted into the draft PR description as the
  pre-merge strength baseline for the current (space-race) eval.
- Note which hybrids qualified (>40% vs eval) and thus stay in the ladder.
