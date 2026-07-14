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

Every board/seed pairing is played twice with swapped seats. The command exits
non-zero for any illegal action, incomplete smoke game, less than 85% wins over
the frozen legacy-compatible baseline, or less than 75% over greedy tactical.
Wall-clock latency varies by hardware; fixed-depth outcomes do not.
