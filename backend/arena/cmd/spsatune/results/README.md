# spsatune determinism, OwnerBot objective, warm start — 2026-07-17 (vs-ai2.52b)

## Determinism — investigated, already order-independent (no fix needed)

The tuner was flagged as nondeterministic (two overnight logs diverging at k=02,
same flags/seed). **Investigation could not reproduce it on current `main`.** The
harness was already order-independent and the divergence was a mid-development
binary artifact (the two original logs were produced 11 min apart while the tuner
code — including the Legacy-floor 85→70 recalibration — was still being changed
and recompiled).

Evidence:

- `go build -race ./arena/cmd/spsatune` + a full run: **0 data races**.
- Each ladder game is single-threaded and pure: `search.chooseNodeBudget` has no
  goroutines/wall-clock; `game.LegalActions` and the heuristic/OwnerBot agents
  scan slices with map *lookups* (no tie-affecting map iteration). No per-process
  nondeterminism source in the fitness path.
- `arena.PlaySequentialOpenings` dispatches games across a worker pool but folds
  results and applies the SPRT/Wilson early stop **strictly in permutation order**
  (windowed; results past the stop index are discarded). Worker count cannot
  change which games are counted — verified: `-workers 1` and `-workers 4` produce
  identical output.
- **Cross-process proof** at the exact overnight regime (`nodes=2000 openings=12
  floor-openings=10 seed=7`): two separate invocations are **byte-identical**
  (stdout trace + output JSON).

Proof command (run from `backend/`; the only stdout difference is the `wrote
<path>` line):

```
go build -o /tmp/spsatune ./arena/cmd/spsatune
for i in 1 2; do /tmp/spsatune -iters 3 -openings 12 -floor-openings 10 \
  -nodes 2000 -seed 7 -workers 4 -out /tmp/det-$i.json > /tmp/det-$i.log; done
diff <(grep -v '^wrote ' /tmp/det-1.log) <(grep -v '^wrote ' /tmp/det-2.log) \
  && diff /tmp/det-1.json /tmp/det-2.json && echo BYTE-IDENTICAL
```

CI enforces the cheap in-process form: `TestSPSAReproducible` (`-iters 3`,
`workers=4`) asserts two loop runs marshal identically.

## OwnerBot objective rung

`arena.OwnerBot` (the owner proxy distilled from the loss corpus) is now a
fitness rung, **weight 3** (the other stranglers are weight 2, Greedy/Legacy/
incumbent weight 1) — the tuner optimizes primarily against the opponent we
actually want to beat. CutSeeker stays a held-out validation opponent, never a
rung.

## Warm start

`-init <path>` seeds theta from a saved vector — either a results JSON (uses
`summary.bestTheta`) or a bare `EvalParams` map. The default vector maps back to
the all-1.0 scaled-space cold start exactly.

---

# spsatune smoke run — 2026-07-17

Bounded SPSA smoke run of the eval-constant tuner against the gate-ladder
objective. This is a **smoke run**, not a shipping result: 8-game samples with
wide Wilson intervals, run at a low node budget. Read the verdict before drawing
any conclusion.

## Command

Run from `backend/`:

```
go run ./arena/cmd/spsatune -iters 25 -openings 6 -nodes 1000 -seed 1 \
  -out arena/cmd/spsatune/results/smoke-20260717.json
```

Settings: `iters=25`, `openings=6` (12 games/rung, both seats), `floor-openings=6`
(default), `nodes=1000`, `seed=1`, `workers=GOMAXPROCS`. Wall time ≈ 6m40s.

## Fitness (weighted-average candidate win% over the 12x12 ladder rungs)

| | fitness |
|---|---|
| baseline (default hand-tuned params) | **infeasible** (`-1e6`) |
| best feasible vector found (k=05, thetaMinus) | **87.04** |

The baseline scored the reject sentinel because it **breached the small-board
Legacy-8x8 floor** (candidate must win ≥85% vs the near-random `Legacy` agent on
8x8). Directly measured, the default eval wins only **75%** vs Legacy on 8x8 at
both nodes=1000 (16 games) and nodes=2000 (20 games) — a robust ~10-point gap
below the floor that more search does not close. The 85% Legacy-8x8 floor is
**miscalibrated for the current engine**: it rejects the very params it is meant
to protect. In the trace this shows as 20 of 25 iterations breaching a floor
(mostly `legacy`), so the SPSA gradient rarely updated — the "best" vector is a
single lucky floor-passing perturbation, not a converged optimum.

## Per-rung win% — default vs best

Measured separately at `nodes=1000`, 8 games/rung (`openings=4`), floors at
`floor-openings=8`. (Throwaway harness, not committed. Samples are tiny —
±~30pp Wilson at 8 games.)

| rung | default | best |
|---|---|---|
| floor: Legacy (8x8) | 83.3 (12g) | 100.0 |
| floor: Greedy (8x8) | 87.5 | 87.5 |
| floor: incumbent (12x12) | 68.8 (16g) | 75.0 |
| Greedy | 100.0 | 100.0 |
| Legacy | 75.0 | 75.0 |
| **BaseAttacker** (strangler) | **37.5** | **75.0** |
| **MobilityAttacker** (strangler) | 75.0 | **87.5** |
| **MobilityBaseAttacker** (strangler) | **37.5** | **75.0** |
| **incumbent-h2h** | 62.5 | **87.5** |
| **holdout: CutSeeker** (validation only) | **62.5** | **75.0** |

## Verdict — honest

Suggestive, not proven. On this smoke sample the best vector **improves on the
stranglers** — exactly where the project memory says eval quality separates:
BaseAttacker +37.5pp, MobilityBaseAttacker +37.5pp, MobilityAttacker +12.5pp,
incumbent-h2h +25pp — and the held-out CutSeeker rose +12.5pp (a genuine
out-of-sample signal, since CutSeeker never entered fitness). The Legacy 12x12
and Greedy rungs were unchanged.

But every caveat applies: 8-game samples, one lucky floor-passing perturbation
rather than a converged SPSA trajectory, and a baseline reported "infeasible"
only because the Legacy-8x8 floor sits ~10pp above what the engine actually
achieves. Nothing here ships.

## Follow-ups (separate beads, not this smoke run)

1. **Recalibrate the Legacy-8x8 floor** before any real run. At 85% it rejects
   the hand-tuned baseline (measured 75%). Drop it to ~70%, or remove it — the
   Greedy-8x8 (75%) and incumbent-h2h (50%) floors already guard strength — so
   SPSA has a feasible region to optimize within instead of breaching ~80% of
   iterations.
2. **Full overnight run** at production opening counts and higher node budgets,
   once the floor is fixed, so the strangler signal above can be confirmed with
   tight Wilson intervals.
3. If a dominating vector survives, ship the new constants through the standard
   pre-merge ladder battery (`VS_LADDER=1`, `VS_STRANGLER=1`) — only if every
   rung ≥ current within CI.
