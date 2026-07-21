# Java NNUE — distilled from GoBot's static eval

Archived weights for the Java bot's NNUE (project: `nnue-trainer`), distilled from
this repo's hand-tuned static evaluator so we don't lose them.

- **Net**: 864 → 256 → 1, clipped-ReLU, single scalar output. Input = 12×12×6
  planes (empty / own-normal / opp-normal / own-fortified / opp-fortified / neutral),
  matching the Java `BoardFeatureMapper`.
- **Label**: `search.StaticEval(state, sideToMove)` (see `backend/search/evaluate.go`),
  z-normalized. Higher = better for the side to move.
- **Pipeline**: `backend/arena/cmd/staticevalgen` (emits board+eval JSONL) →
  nnue-trainer `make_distill_dataset.py` (→ 864 features) → `train.py`.
- **Fit**: val MSE **0.015** vs the eval target (constant-predictor baseline 1.01) —
  the net reproduces GoBot's static eval almost exactly (~98.5% of variance).

## Important result
Near-perfect eval fit but **0/7 vs GoBot in play** (bead `nnue-trainer-ntd.8`).
Reproducing the static eval does NOT transfer playing strength: GoBot's edge is its
**search stack** (quiescence, opening book, TT, PVS/pruning, SPSA-tuned), not the
static eval alone. A static eval is co-designed with its search. Kept as the
**warm-start for phase-2 RL** (`nnue-trainer-ntd.7`).
