# NNUE-lite data generation + training (vs-ai2.56, stages 1-2)

Offline pipeline for a learned evaluation network. This covers DATA (`nnuegen`)
and TRAINING (`tools/nnue-train`) only. Stage 3 — pure-Go int8 inference wired
into `evaluateWithWorkspace` — is a separate later bead and is out of scope.

## Pipeline at a glance

```
positions ──► nnuegen ──► shard-*.jsonl ──► nnue-train ──► weights_out.go
 (selfplay/corpus/ladder)   (labeled)         (MLP + int8)   (+ loader stub)
```

## nnuegen — generate the corpus

`backend/arena/cmd/nnuegen`. Samples positions from three sources, labels each
with a deep search score + eventual game outcome, writes deterministic JSONL
shards keyed by worker id (`shard-000.jsonl`, …).

Flags:

| flag | default | meaning |
|------|---------|---------|
| `-out` | (required) | output shard directory |
| `-workers` | 1 | parallel shard writers |
| `-positions` | 5000 | target total positions |
| `-budget` | 2000 | node budget for the deep-score label |
| `-seed` | 1 | base seed; per-worker seed = base + worker index |
| `-boards` | `8x8` | comma-separated board sizes, e.g. `8x8,12x12` |
| `-corpus` | "" | owner-corpus manifest path (enables the corpus source) |
| `-resume` | false | scan existing shards, skip fingerprints already present, append |

### Smoke run (this box)

From `backend/`:

```
go run ./arena/cmd/nnuegen/ -out /tmp/smoke -positions 2000 -budget 500 \
    -seed 7 -workers 4 -boards 8x8,12x12 -corpus arena/testdata/owner-corpus.json
```

Observed: **2000 positions across 4 shards in ~58s wall** (~145s CPU, 4
workers), ~29 MB RSS. Deep-score labeling at `-budget 500` dominates the cost;
`-budget 2000` is roughly 4× slower per position.

### Committed smoke fixture

`backend/arena/cmd/nnuegen/testdata/smoke.jsonl` (300 records). Byte-identical
reproduction, from `backend/`:

```
go run ./arena/cmd/nnuegen/ -out <dir> -positions 300 -budget 500 -seed 42 \
    -boards 8x8,12x12 -corpus arena/testdata/owner-corpus.json
```

Compare `<dir>/shard-000.jsonl` to the committed fixture — they match.

### Full production run (documented, NOT run here)

Architect-scheduled; the SPSA tuner owns CPU on this box. For 1-5M positions at
a high offline budget across the board matrix:

```
go run ./arena/cmd/nnuegen/ -out data/nnue -positions 2000000 -budget 20000 \
    -seed 1 -workers $(nproc) -boards 8x8,10x10,12x12 \
    -corpus arena/testdata/owner-corpus.json
```

Use `-resume` to continue an interrupted run — it scans existing shards and
skips fingerprints already present. Resume dedupe is a per-shard fingerprint
scan (`// ponytail:` O(n) scan noted in `loadFingerprints`); fine at these
sizes.

## JSONL schema

One `Record` per line (see `nnuegen/main.go` package doc for the authoritative
copy):

| field | type | meaning |
|-------|------|---------|
| `fingerprint` | string | `arena.StateFingerprint(state)` — stable dedupe key |
| `rows`, `cols` | int | board dimensions |
| `currentPlayer` | int | seat to move (1-based) |
| `features` | `[4][]float64` | per-seat feature vectors (seat-1 indexed); each is the fixed-order `arena.PlayerFeatures.Features()` slice; inactive seats are JSON `null` |
| `deepScore` | int | `search.ChooseNodeBudget(state, budget).Score` |
| `budget` | uint64 | node budget used to produce `deepScore` |
| `outcome` | `{winner int, placement int}` | eventual game result; `winner 0 / placement 0` = unknown (ladder positions with no completed game). `placement` is the mover's finishing rank (1 = won) |
| `source` | string | `"selfplay"` \| `"corpus"` \| `"ladder"` |

The full 4×K feature matrix is kept whole so no perspective decision is baked
into the data — the trainer picks mover-vs-opponents itself.

### Feature vector (per player, fixed field order)

`arena.PlayerFeatures`, flattened by `Features()` in declaration order:

```
Normal, Fortified, Connected, Disconnected, Mobility, Captures,
BaseExits, BaseOpenings, BaseAnchors, BaseThreat, Threatened, ThreatenedLoss,
ThreatTempo, Articulation, MaxCutLoss, SpaceRace, SealedBase, NeutralUnused,
MovesLeftTempo
```

Booleans flatten to 0/1. Do not reorder without bumping the schema.

## Feature coverage vs evaluate.go (honest notes)

The extractor (`backend/arena/nnuefeatures.go`) mirrors the cheap per-player
quantities `search/evaluate.go` computes, using only the public `game.State`
API (the frozen search internals are unexported and off limits).

**Covered** (same definitions as `analyzeWithConnectivity`): normal, fortified,
connected, disconnected, mobility, captures, baseExits, baseOpenings,
baseAnchors, baseThreat, threatened, threatenedLoss, threatTempo, sealed-base
flag, neutral-unused flag, moves-left tempo; plus the Voronoi space-race
first-reach count (`spaceRace`) and, from the articulation analysis, the cut
COUNT and the MAX single-cut cutLoss.

**Omitted, and why acceptable for a learned vector:**

- **Cross-player predatory-cut term** — evaluate.go's second pass scores one
  player against an *opponent's* articulation cells adjacent to its own
  territory. It is a pairwise interaction a per-player vector cannot hold; the
  learner sees each player's own `Articulation`/`MaxCutLoss` and approximates
  the fragility signal.
- **Exact per-cell threatenedLoss weighting** as folded into the final
  `ratio()`/`normalized()` score (denominators, `threatTempo` multiplier,
  `spaceRaceWeight`, …). We emit raw integer tallies and let the trainer learn
  its own scales — the hand-set `EvalParams` are exactly what the net replaces.
- **Final self-minus-mean-opponent aggregation** and the sealed-base / mate
  special-cases — that is score assembly, not features. We surface the raw
  sealed-base flag and let the trainer combine perspectives.

## nnue-train — train + export int8 weights

`tools/nnue-train` (own `go.mod`, stdlib only). Loads the shards, does a
deterministic 90/10 train/val split by fingerprint hash, trains a 2-layer MLP
(input → hidden → 1, tanh, Adam, MSE on normalized deep score with a small
game-outcome auxiliary term), and exports int8-quantized weights as Go source.

Flags: `-data` (required, dir of `shard-*.jsonl`), `-export` (output `.go`,
`-` for stdout), `-package`, `-hidden` (default 48), `-epochs` (100), `-lr`
(0.001), `-aux` (0.05 outcome-loss weight), `-seed`.

```
go run . -data data/nnue -epochs 100 -export weights_out.go -package nnueweights
```

### Trainer smoke (on the committed fixture)

Copy `smoke.jsonl` to `<dir>/shard-000.jsonl`, then from `tools/nnue-train`:

```
go run . -data <dir> -epochs 60 -seed 1 -export <dir>/weights_out.go
```

Observed loss curve (300 samples, 90/10 split, hidden 48, Adam lr 0.001):

```
epoch   1  train_loss 1.105792  val_loss 1.942668  spearman 0.3547
epoch   2  train_loss 0.949589  val_loss 1.630472  spearman 0.3002
epoch   4  train_loss 0.877095  val_loss 1.694102  spearman 0.3055
epoch   9  train_loss 0.803403  val_loss 1.716866  spearman 0.2821
epoch  19  train_loss 0.737710  val_loss 1.671066  spearman 0.3523
epoch  29  train_loss 0.679604  val_loss 1.594415  spearman 0.4423
epoch  39  train_loss 0.601426  val_loss 1.411393  spearman 0.5759
epoch  49  train_loss 0.555520  val_loss 1.324264  spearman 0.6009
epoch  59  train_loss 0.511115  val_loss 1.267956  spearman 0.6082
epoch  60  train_loss 0.506540  val_loss 1.263035  spearman 0.6073
```

Train and val loss both fall monotonically after warm-up and val Spearman rank
correlation rises from ~0.35 to ~0.61 — the net is learning to rank positions
by deep score on held-out data. Absolute val loss stays above train loss (only
300 samples); the production run's 1-5M positions is what closes that gap.

## Exported weights format + loader-stub contract

`weights_out.go` is generated Go source (`// Code generated … DO NOT EDIT`).
Symmetric per-matrix int8: `scale = max|w|/127`, `q = round(w/scale)`, and
`w ≈ q*scale` on reconstruction. Contents:

- `InputDim`, `HiddenDim` consts.
- `Mean`, `Std` — score de-normalization: `rawScore = out*Std + Mean`.
- `W1 [HiddenDim][InputDim]int8` + `W1Scale [HiddenDim]float64` (per-row scale).
- `B1`, `W2` as `[...]int8` + their scalar scale.
- `B2 float64` (scalar output bias, kept float).
- `Predict(x []float64) float64` — a pure-Go int8 forward pass, emitted **and
  explicitly marked UNUSED BY PRODUCTION**. It is the loader stub Stage 3 (a
  separate bead) adopts into `evaluateWithWorkspace`. The generated source
  compiles standalone (verified in `main_test.go`).

The trainer duplicates the record struct locally rather than importing the
backend module (`// ponytail:` note in `main.go`) so `tools/nnue-train` stays a
zero-dependency module separate from `backend/go.mod`.
