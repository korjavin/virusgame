# Headless tournament arena

The arena runs complete games through `game.State.Apply`; it has no server,
WebSocket, or production configuration dependency. Outcomes are deterministic
for a fixed depth, board list, and seed count. Latency percentiles measure only
the tournament contender; throughput counts all decisions.

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

The full matrix includes 5x5, both 5x10 orientations, 8x8, 10x10, 15x20,
25x25, 50x50, and 50-edge stress rectangles. Reports include wins, illegal,
stalled and maxed games, searched nodes, completed-turn depth, and latency.

Every board/seed pairing is played twice with swapped seats. The command exits
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
