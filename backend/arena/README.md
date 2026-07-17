# Headless tournament arena

The arena runs complete games through `game.State.Apply`; it has no server,
WebSocket, or production configuration dependency. Outcomes are deterministic
for a fixed depth and starting position. Latency percentiles measure only
the tournament contender; throughput counts all decisions.

## Frozen strength corpus

Deterministic engines started from an empty board do not become independent
samples when a loop counter is called a seed. `Balanced` remains useful for
randomized baselines, but replacement strength claims use the checked-in
`testdata/strength-corpus-v1.json` trajectories:

```sh
cd backend
go run ./cmd/arena -corpus arena/testdata/strength-corpus-v1.json \
  -corpus-split heldout -depth 3 -opponent greedy
```

Each suffix starts from an identical authoritative snapshot with the contender
rotated through every player seat. Reports include joint board×seat×phase
buckets, track/stratum marginals, Wilson 95% intervals, illegal/stall/maxed
counts, nodes, completed depth, and latency. Agent factories are fresh per
suffix, so state cannot leak between samples.

The corpus was frozen before contender tuning. Its trajectory-group checksums
are:

- train: `28c65702a6a9664a609465bd14316fe588acfc7749c47d560c0e27527e53edeb`
- heldout: `063a143c57dfbdbce0a64af60303132650dabad57f432260d8389b39aa4d5529`

The competitive 1v1 track covers 5×5, 8×8, 10×10, 12×12, 15×20,
20×15, and 20×20. Multiplayer strata use three and four players through
28×28. The 25×25 and 30×30 cases are tagged `stress` and excluded from win-rate
comparisons; boards beyond the strength caps provide legality/deadline evidence
only.

Every competitive board has two disjoint trajectory families per split. Each
board contributes bounded opening checkpoints plus phase-directed contact /
consolidation and tactical base-threat checkpoints; coverage tests enforce
those properties per board rather than in aggregate.

The generator and declared xorshift seeds live under `arena/cmd/corpusgen`.
Regeneration changes hashes and requires a new corpus version; it must never
silently rewrite v1. Use train for development and heldout only for a
predeclared acceptance run. The CLI defaults to train; heldout must always be
requested explicitly.

Checkpoints from the same trajectory are intentionally correlated. Wilson
intervals are descriptive game-level summaries, not independent-sample
confidence claims, and must be read alongside board, family, seat, and phase
buckets. A release decision cannot turn adjacent checkpoints into additional
independent evidence by increasing a repetition counter.

## Strangler gates

All 12 recent real 12×12 production losses ended in `no_moves` strangulation,
so vs-ai2.34 tunes and gates the evaluator against a strangler — an agent that
races to wall off territory (`MobilityAttacker`) — rather than against the
incumbent, which shares the incumbent's blind spots. Both gates are opt-in via
env vars, use a deterministic 1000-node budget with balanced seats and shared
seeded openings, and report Wilson 95% intervals.

Primary gate — candidate and frozen incumbent each vs `MobilityAttacker` and
`BaseAttacker`; fails if the candidate's Mobility win rate drops to or below
the incumbent's:

```sh
cd backend
VS_STRANGLER=1 go test ./arena -run TestVsStrangler -v -timeout 120m
```

Secondary gate — candidate vs frozen incumbent differential (frozen-vs-frozen
reads 50%):

```sh
cd backend
VS_STRANGLER_DIFF=1 go test ./arena -run TestStrangulationEvalNodeBudget -v -timeout 60m
```

`VS_STRANGLER_OPENINGS` (default 40) and `VS_STRANGLER_NODES` (default 1000,
secondary gate only) override the sample size and node budget.

The secondary gate and the primary gate's log-only BaseAttacker pairs use
SPRT-style sequential early stopping (see below), so lopsided matchups finish
well under the opening cap. The primary gate's MobilityAttacker pairs always
play the full opening set: they feed the hard regression floor, which compares
the two engines' rates directly and needs full paired samples, not noisy
early-stopped point estimates.

## Sequential early stopping

Gate and ladder pairings play balanced seeded-opening pairs in a fixed-seed
permutation and, after each pair, update a running Wilson 95% interval. The
run stops early once at least 8 games are played and the interval lies
entirely above or below the decision threshold (50%, or 60% for the ladder's
hybrid qualification rungs); otherwise it runs to the full opening cap. Everything is deterministic — node-budget engines, fixed
opening order and content — so the same code and seed reproduce the same game
sequence, stop point, and verdict.

Caveat: checking the interval after every pair inflates the type-I error above
the nominal 5% (optional stopping), so intervals printed for early-stopped
runs are descriptive, not exact confidence claims. That is why the strangler
regression floor compares full fixed-n samples instead.

## Hybrid sparring opponents

Two cheap heuristic stranglers extend the fixed baseline roster:

- `MobilityBaseAttacker` — MobilityAttacker's mobility differential plus a
  base-pressure term and capture bonus.
- `CutSeeker` — computes the nearest opponent's base-connected articulation
  points (arena-side Tarjan pass) and prefers moves that capture or adjoin a
  cut cell, falling back to mobility.

A hybrid qualifies for a ladder slot only if it beats the current eval >40%
(i.e. the eval's win rate against it is below 60%). Qualification rungs stop
against the 60% boundary itself, and the verdict comes from that sequential
decision (interval clear of 60%), falling back to the full-cap rate — so the
stop rule tests the same boundary the verdict is read at.

## Ladder report

The pre-merge strength report for eval/search PRs: the current production
eval (deterministic node budget) vs the full ladder — Greedy, Legacy,
BaseAttacker, MobilityAttacker, both hybrids (plus incumbent-vs-hybrid
qualification rows), and the frozen incumbent — printed as one table with
win rates, Wilson 95% intervals, and games-played/cap (showing where early
stopping saved games). Paste the table into the PR body as the strength
baseline.

```sh
cd backend
VS_LADDER=1 go test ./arena -run TestLadderReport -v -timeout 240m
```

`VS_LADDER_NODES` (default 1000) and `VS_LADDER_OPENINGS` (default 40, cap =
2x openings) override the node budget and sample size.

## Owner-loss corpus

Every 1v1 game a human wins against the bot is a proven hole. `replayimport
-fetch` harvests them all in one command: it pulls the live feed, keeps only
human wins (`result==1`, no third player), dedupes against what is already
committed, replays each through the authoritative rules, writes it as a frozen
testdata anchor, and pins its terminal-position fingerprint in
`testdata/owner-corpus.json` (the data-form allowlist).

```sh
cd backend
go run ./cmd/replayimport -fetch 20   # limit must be 5, 10, or 20
```

The feed is a rolling window, so run it periodically to accumulate new losses;
output is deterministic (only new games are written, manifest sorted by id).
`TestOwnerLossCorpusAnchors` pins every entry (terminal fingerprint, bot
eliminated in seat 2). The standing passivity dashboard prints per-game bot
attack-move counts:

```sh
VS_OWNER_CORPUS=1 go test ./arena -run TestOwnerLossCorpusDiagnostic -v
```

The archived-bytes path (`-input saved.json`) is unchanged for reviewing exact
fetched bytes before import.

## Production regressions

Immutable production fixtures were imported from `GET /last_games?limit=20`
on 2026-07-15. They preserve `no_moves`, resignation, and `illegal_move` as
different outcomes. Board-rule `no_moves` must reconstruct a terminal state;
resignation, timeout, disconnect, and illegal-move validate only their legal
prefix and remain protocol outcomes. Game `4d85f7c0…` contains a source event
after authoritative game-over, so the importer records the observed turn count
and omitted post-terminal move instead of pretending that suffix is legal.

`production-motifs-v1.json` freezes within-turn positions for consolidation,
backup routes, thin tendrils, base-rooted small cuts, counter-capture exposure,
harmful own-base halos, opponent-base siege choices, and translated/reflected
structural tests. These annotations are evaluator inputs, not claims that an
exact historical move generalizes unchanged to another board.
They remain outside aggregate strength win rates because they were selected
after observing production outcomes; including them would bias the arena. They
are frozen train-only regression evidence for later structural translations.

The post-self-elimination-fix 12×12 set adds eight authoritative `no_moves`
games (`fd6627c8…`, `3d739acb…`, `e854f8aa…`, `e7b2f1d4…`, `6bf1f3aa…`,
`4558d2fe…`, `913c33f7…`, and `550cfd27…`). `836204cc…` is retained only as
an `illegal_move` legal-prefix/protocol fixture. The controlled 5×5 and 5×7
resignation persistence sentinels from the same response are deliberately not
part of arena evidence. These outcome-selected motifs do not modify either
frozen strength-corpus split or its group hash.
Negative causal motif points identify the state before the recorded bad move;
the following replay action is the queryable historical mistake.

CI gate:

```sh
cd backend
go test ./arena -run TestStrengthGate -count=1 -v
```

Larger balanced evidence run (180 two-player games plus 3/4-player smoke):

```sh
cd backend
go run ./cmd/arena -seeds 10 -depth 3
```

The default `ci` matrix is intentionally small and deterministic. Run the
broader variable-size and wall-clock gate manually before an engine release:

```sh
go run ./cmd/arena -matrix full -production -seeds 2
```

The legacy full probe matrix includes 5x5, both 5x10 orientations, 8x8, 10x10,
15x20, 25x25, and 30x30. The UI maximums 5x50, 50x5, and 50x50 are separate
single-decision legality/no-stall/deadline probes; they are not deep win-rate
requirements. Reports include wins, illegal, stalled and maxed games, searched
nodes, completed-turn depth, and latency.

Every randomized-baseline board/seed pairing is played twice with swapped seats. The command exits
non-zero for any illegal action, incomplete smoke game, less than 85% wins over
the frozen legacy-compatible baseline, or less than 75% over greedy tactical.
Wall-clock latency varies by hardware; fixed-depth outcomes do not.

The production evidence path uses the same `search.Choose` entry point and
`search.ProductionBudget` deadline as the deployed bot:

```sh
cd backend
go run ./cmd/arena -production -seeds 1 -opponent legacy
go run ./cmd/arena -production -seeds 1 -opponent greedy
```

Keep the fixed-depth suite as the reproducible CI gate. Production runs verify
the deployed anytime path separately; on the reference runner, six balanced
games per baseline produced 100% wins, zero illegal/maxed/stalled games, and
approximately 601 ms p95 contender-decision latency.
