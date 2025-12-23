# Continuity Ledger

## Goal
**Fix Neutral Field Logic and Harden Game Validation**
- Fix a bug where bots could retake neutral fields.
- Implement strict server-side validation to defeat players who attempt illegal moves.
- Add backend unit tests to verify validation logic.
- Ensure bots explicitly avoid neutral fields.

## Constraints/Assumptions
- **Neutral Fields**: Immutable `CellFlagKilled` (0x30). Cannot be attacked or used for connectivity.
- **Illegal Moves**: Result in immediate elimination/defeat for the offending player.
- **Testing**: Need to establish a new `go test` suite for the backend.

## Key Decisions
- **Defeat by Illegal Move**: Instead of silently rejecting illegal moves, the server will now actively eliminate the player. This protects the game integrity against buggy or malicious clients.
- **Strict Validation**: Explicit checks for `IsKilled()` will be added to `isValidMove` in both backend and bot code.
- **Connectivity Check**: `handleMove` now enforces connectivity checks via `isValidMove`, closing a major loophole.

## State
- **Done**:
  - Implemented V2 Bot Architecture.
  - Implemented Bot Hoster.
  - Created backend unit tests (`backend/hub_test.go`) covering validation logic.
  - Implemented "Defeat on Illegal Move" in `backend/hub.go`.
  - Added strict validation (neutral check + connectivity) to `backend/hub.go` and `backend/cmd/bot-hoster/ai_engine.go`.
  - Updated `BOT_DEVELOPMENT_GUIDE.md` with strict rules.
  - Merged PR #44.
- **Now**:
  - Monitoring for regressions.
- **Next**:
  - Verify visually in frontend if possible (optional).
  - Deploy and monitor.

## Open Questions
- None.

## Working Set
- `backend/hub.go`
- `backend/hub_test.go`
- `backend/cmd/bot-hoster/ai_engine.go`
- `BOT_DEVELOPMENT_GUIDE.md`