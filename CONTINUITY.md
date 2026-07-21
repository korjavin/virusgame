# Continuity Ledger

## Goal
**Enhance Bot Telemetry: Add Diagnostic Fields to Protocol**
- Add optional diagnostic fields (`score`, `depth`, `nodesEvaluated`, `timeMs`, `alternativeMoves`) to the WebSocket message protocol.
- Allow bots to broadcast their search metadata for debug and replay value.

## Constraints/Assumptions
- **Non-breaking Changes:** All new JSON fields use `omitempty` to remain backward-compatible with older clients.
- **Bot Behavior:** Only built-in bots running on `bot-hoster` automatically populate these values right now.
- **Struct Uniformity:** The structs must match across `backend/types.go`, `bot-client.go`, and the standard Go bot template.

## Key Decisions
- Created a new `AlternativeMove` struct for consistency.
- Standard search values inside `bot-hoster` are marshaled into the new message fields by passing `timeMs` manually.
- The `Score` is cast to a `float64` to comply with the issue request, though the engine calculates it natively as an `int`.

## State
- **Done**:
  - Updated `backend/types.go` struct.
  - Updated `backend/cmd/bot-hoster/bot_client.go` struct and updated `calculateAndQueueAction` / `actionMessage` logic.
  - Fixed integration tests inside `bot_search_test.go` broken by the parameter change.
  - Updated `bot-templates/go/protocol.go`.
  - Tests pass, compilation checks out.
- **Next**:
  - Submit.

## Open Questions
- None.

## Working Set
- `backend/types.go`
- `backend/cmd/bot-hoster/bot_client.go`
- `backend/cmd/bot-hoster/bot_search_test.go`
- `bot-templates/go/protocol.go`
