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

- train: `bc7de8478a7ad5392307d01e5a4dfbe4e2143573dd89a694b9776023721c614c`
- heldout: `f6ff85240fb4fb3d8b6c6382bfcc13798f4817b3deae0405e4180facdcdef06a`

The competitive 1v1 track covers 5×5, 8×8, 10×10, 12×12, 15×20,
20×15, and 20×20. Multiplayer strata use three and four players through
28×28. The 25×25 and 30×30 cases are tagged `stress` and excluded from win-rate
comparisons; boards beyond the strength caps provide legality/deadline evidence
only.

The generator and declared xorshift seeds live under `arena/cmd/corpusgen`.
Regeneration changes hashes and requires a new corpus version; it must never
silently rewrite v1. Use train for development and heldout only for a
predeclared acceptance run.

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
