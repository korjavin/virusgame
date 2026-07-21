# NNUE (Learned Evaluation) Experiment — Log, Results, Decision

**Dates:** 2026-07-18 → 2026-07-20
**Bead:** `vs-ai2.56` (epic `vs-ai2` — Superior variable-board AI opponent)
**Outcome:** NEGATIVE. A small supervised network cannot beat the hand-tuned evaluator in this game. Production is unchanged (strongest hand-tuned build). All tooling is preserved default-off (zero production risk).

---

## 1. Motivation

The hand-tuned evaluator in `backend/search/evaluate.go`, driving iterative-deepening alpha-beta / maxN search, reached its practical ceiling against the owner (a strong human): roughly even in long 1v1 games. Nine hand-crafted eval/search improvements were tried and measured over two days — space-race term, opening book, TT rework, PVS, maxN pruning, quiescence, strangler-aware search, tie-break jitter, SPSA weight tuning — several shipped, but the remaining gap to the owner did not close. The evidence pointed to *missing evaluation features*, not weights or depth. The natural next step is a learned evaluator (NNUE-style): replace or augment the hand-written formula with a small neural network cheap enough to call at every search leaf.

## 2. Method

**Pipeline** (all in-repo, `backend/arena/cmd/nnuegen` + `tools/nnue-train` + `backend/nnuefeat` + `backend/search/nnue.go`):

1. **Data generation** (`nnuegen`): sample positions from self-play at varied node budgets + owner-corpus replays + seeded openings; label each with a deep-search score (offline, high node budget) plus the eventual game outcome; dedupe by state fingerprint. Schema v2 stores the **raw position**, so features/labels are recomputable without re-searching.
2. **Feature extraction** (`nnuefeat`): two input representations tried — (a) **curated**: 26 per-player scalars (territory, mobility, articulation count, max cut-loss / severable mass, front width, space-race partition, threat-gated cut-risk, forward openness…); (b) **board planes**: a mover-anchored 12×12 grid, 9 owner-relative one-hot planes + OOB plane + 2 scalars (1298-wide), the net learning its own features from raw geometry.
3. **Training** (`nnue-train`): dependency-free Go, 2-layer MLP, Adam, int8-quantized export as Go source arrays. Two framings: **replacement** (net predicts the score directly) and **residual** (net predicts `deepScore − staticEval`, so play-time eval = hand-tuned + learned correction and degrades gracefully). Mate-magnitude labels are filtered (they dwarf positional residuals and crush z-normalization).
4. **Inference** (`search/nnue.go`): env-gated (`VS_NNUE=1`), int8 forward pass (~4µs, ~⅓ the classic eval's cost). Default-off ⇒ production byte-identical to the frozen hand-tuned eval (proven by the eval-oracle/golden tests).
5. **Measurement**: uncached paired protocol — net-ON vs net-OFF, same seeded openings, deterministic node budget, `TestVsStrangler` n=48 (MobilityAttacker + BaseAttacker). The frozen `incumbent` package has no NNUE hook, so toggling `VS_NNUE` cleanly isolates the candidate.

## 3. Results

**Baseline (hand-tuned eval, `VS_NNUE` off), n=48 paired:** **70.8%** vs MobilityAttacker, **87.5%** vs BaseAttacker.

| Variant | Input | Framing | Data | Strangler | Base | Verdict |
|---|---|---|---|---|---|---|
| v4 | curated-26 | replacement | 156k, 8×8, deep | 39.6% | 39.6% | far below |
| v5 | curated-26 | residual | 156k, 8×8, deep | 50.0% | 87.5%* | below |
| v7 | curated-26 | residual + mate-filter | 156k, 8×8, deep(20k) | 70.8% (tie) | 64.6% | ties strangler, regresses base |
| plane-8×8 | board-plane | residual, h256 | 41k, 8×8, deep | 64.6% | 70.0% | below (size-mismatched) |
| **plane-12×12** | board-plane | residual, h256 | **50k, 12×12, deep(20k)** | **66.7%** | **72.7%** | **below** |

\* early-stopped small n. Sweeps also covered: replacement vs residual at every input; hidden 64/128/256; data 56k→156k; label budget 3k→20k nodes; aux (outcome) weight 0.05–0.5.

**No configuration beat the hand-tuned baseline on both axes.** The best (v7) merely *tied* on the strangler while regressing base defense.

## 4. Root cause

The training labels are deep-search scores, and that deep search **uses the hand-tuned evaluator at its own leaves**. A network trained to predict those labels therefore asymptotes to the hand-tuned eval — it distills its teacher and cannot exceed it. Supervised distillation was structurally capped from the outset. (The owner anticipated exactly this: "I'm not convinced deeper search is good in our case" — the labels inherit the same evaluator's blind spots, just laundered through lookahead.) The board-plane representation removes the *feature-compression* ceiling but not the *teacher* ceiling, which is why it also plateaus.

## 5. Engineering findings (reusable)

- **Schema v2 (raw positions stored)** is essential — it let us re-label at deeper budgets and re-extract new features (curated → planes) without regenerating data.
- **Mate-band label filtering** is required; mate-magnitude scores destroy z-normalization (Spearman collapses to ~0.04 without it).
- **Residual framing** > replacement at every input richness (degrades gracefully toward the hand-tuned eval).
- **Memory**: dense plane inputs for >~50k positions exceed the 7.5 GB dev box — subsample or stream.
- **Data must match the target board size**: 8×8-trained nets evaluated at 12×12 are uninformative (the default generator board is 8×8; the game/gates are 12×12).
- The search speedups (TT/PVS/pruning) made deep re-labeling of 50k positions take ~75 min, not the hours feared.

## 6. Decision

**Stop the supervised-NNUE line.** It is a measured dead end for *beating* the hand-tuned eval, for the structural reason in §4. Production remains the strongest hand-tuned build; all NNUE code ships default-off with zero production risk and is fully reproducible.

**The only path that breaks the teacher ceiling** is a learning signal not derived from the hand-tuned eval — i.e. **AlphaZero-style reinforcement learning** from self-play *game outcomes* (policy+value net, self-play data loop, no supervised teacher). That is a substantially larger project requiring real training hardware, and is a deliberate future decision, not a variant tweak.

## 7. Artifacts

- Tooling (default-off, zero prod impact): PR #117 (curated-net tooling: residual mode, deep-relabel/residualize, mate-band filter, canary `VS_NNUE` env), branch `canary/nnue-planes` (board-plane extractor + trainer).
- Code: `backend/arena/cmd/nnuegen` (generate / `-relabel` / `-residualize`), `backend/nnuefeat` (curated + board-plane extractors), `tools/nnue-train`, `backend/search/nnue.go` (inference), `backend/search/nnueweights` (weights + loader).
- Durable summary: bd memory `vs-ai2-nnue-verdict`.
