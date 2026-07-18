# NNUE-lite inference + canary (vs-ai2.56, stage 3)

Stage 3 wires the pure-Go int8 forward pass into search's leaf eval, behind an
env flag, and documents how a trained net ships. Stages 1-2 (data + training)
are in [nnue-datagen.md](nnue-datagen.md).

## Policy: canary-first, owner judges

Per `bd` memory `vs-ai2-goodhart-scripted-gates`: scripted-opponent gate fitness
does NOT transfer to strength against the owner (vs-ai2.52 dominated every gate,
then lost 3 straight to the owner by turtling — Goodhart). So:

- Eval changes ship **canary-first**; the **owner is the only promotion judge**.
- Gates (build/vet/test, oracle byte-identity, cost) are **sanity floors**, not
  strength proof. Never conclude the net is stronger from gate numbers.
- The weights committed today are a **smoke placeholder** (300-position train,
  spearman ~0.45) — plumbing validation only. Strength is meaningless until the
  full nnuegen run + train.

## Full flow

```
nnuegen (full 1-5M run, in progress) ──► shard-*.jsonl
        │
        ▼
nnue-train ──► backend/search/nnueweights/weights.go   (int8 Go arrays + Predict)
        │
        ▼
canary build: VS_NNUE=1  ──►  evaluateWithWorkspace routes to the net
        │
        ▼
owner plays the canary ──► owner judges ──► promote or iterate data→train
```

## Inference path

- `backend/nnuefeat` — the game-only feature extractor, shared by the labeler
  (via `arena`, which re-exports it) and search. One definition of the feature
  vector, so training and inference feed byte-identical inputs. `nnuefeat.Input`
  flattens a position to the 76-wide (`Seats`×`FeatureCount`) network input.
  Moved here from `arena` in stage 3 because `arena` imports `search` — search
  cannot import back without a cycle.
- `backend/search/nnueweights` — generated int8 weights + `Predict` forward pass.
- `backend/search/nnue.go` — `nnueEnabled` (read once from `VS_NNUE` at init) and
  `nnueEvaluate`. `evaluateWithWorkspace` branches to the net only when enabled.

**Default path is byte-identical to origin-main.** With `VS_NNUE` unset,
`nnueEnabled` is false and the leaf eval is untouched — proven by
`TestEvaluateWorkspaceMatchesOriginMainOracle` (unchanged digest).

Enable the candidate path:

```bash
VS_NNUE=1 <run the engine / arena / server>
```

`VS_NNUE=0` or unset ⇒ frozen eval. Any other non-empty value ⇒ net.

Perspective: the net predicts the mover's deep-score; for the 2-player game the
other seat negates it (`nnueEvaluate`). A real 4-player mapping is deferred — the
smoke net's strength is meaningless, so it is not worth building yet.

## How the weights swap in

The weights are a normal committed Go source file. To ship a new net:

```bash
# 1. generate the corpus (see nnue-datagen.md); full run is 1-5M positions
go run ./backend/arena/cmd/nnuegen -out /path/shards -positions 2000000 -budget 2000 -corpus <manifest>

# 2. train + export straight into the package (overwrites the placeholder)
cd tools/nnue-train
go run . -data /path/shards -export ../../backend/search/nnueweights/weights.go -package nnueweights

# 3. build the canary and hand it to the owner
git checkout -b canary/nnue-<date>
git commit -am "canary: nnue weights <date> (full run)"
# owner runs it with VS_NNUE=1 and judges
```

Regenerate the smoke placeholder (what stage 3 committed) cheaply from the
committed fixture:

```bash
mkdir -p /tmp/smoke && cp backend/arena/cmd/nnuegen/testdata/smoke.jsonl /tmp/smoke/shard-000.jsonl
cd tools/nnue-train && go run . -data /tmp/smoke -export ../../backend/search/nnueweights/weights.go -package nnueweights -epochs 30 -hidden 32
```

## Branch naming

Candidate nets live on `canary/nnue-<descriptor>` branches (e.g.
`canary/nnue-20260718-fullrun`). One net per branch so the owner can A/B them.
Never merge a canary to `main` on gate numbers alone — merge only after the owner
judges it beats BOTH live exploit vectors (en-prise gifting and base-turtling).

## Cost

Micro-benchmarks (`backend/search/nnue_bench_test.go`, 8x8 mid-game leaf; box
under heavy load, so absolute ns are noisy — relative holds):

| path | ns/op | allocs/op |
|------|-------|-----------|
| classic eval (frozen) | ~12,300 | 0 |
| NNUE forward pass only (`Predict`) | ~4,000 | 0 |
| NNUE full path (`Input` + `Predict`) | ~27,000 | 132 |

The **int8 forward pass is cheap** (~0.3x the classic eval, zero-alloc) —
comfortably inside the ~2x cost gate. The full-path 2.2x overshoot is entirely
**redundant feature re-extraction**: `nnuefeat.Input` recomputes connectivity /
articulation / space-race from scratch (132 allocs), the exact quantities the
classic eval already computes zero-alloc in its workspace.

Optimization when the net actually ships (deferred, not built now): feed the net
from the workspace metrics search already has, sharing extraction. That drops the
NNUE path to ~forward-only and could make it net cheaper than the classic eval.
Secondary levers: narrower quantization, feature pruning. Not worth building for
a placeholder.
