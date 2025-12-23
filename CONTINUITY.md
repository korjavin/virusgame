# Continuity Ledger

## Goal
**Stabilize Lobby System and V2 Bot Architecture**
- Ensure the Lobby System (the core feature) is fully documented and stable.
- Finalize the transition to V2 Bot Architecture (Bots as external clients).
- Bring documentation (`MULTIPLAYER.md`) in line with the implemented code.

## Constraints/Assumptions
- **Architecture**: Go backend with single-threaded Hub (no mutexes for game state), JS frontend, external Bot Hoster service.
- **Protocol**: WebSockets for all real-time communication.
- **Bot Identity**: Bots connect as regular clients with `?bot=true` or via `bot-hoster` service, identified by "AI-" prefix or `isBot` flag.
- **Tests**: Currently relying on manual verification and frontend tests; backend unit tests are missing.

## Key Decisions
- **Bots as First-Class Players**: Bots run as external WebSocket clients (via `bot-hoster`), not as internal backend logic.
- **Lobby System Centrality**: The Lobby is the primary entry point for multiplayer games; bots are requested by hosts within a lobby.
- **Broadcast Signaling**: The backend uses `bot_wanted` broadcast messages to solicit bot joins from the `bot-hoster` pool.

## State
- **Done**:
  - Implemented V2 Bot Architecture (backend `hub.go` changes).
  - Implemented `bot-hoster` service (`backend/cmd/bot-hoster`).
  - Updated `backend/ARCHITECTURE.md`.
  - Implemented frontend Lobby UI for bot management.
- **Now**:
  - Restoring `CONTINUITY.md`.
  - Updating `MULTIPLAYER.md` to reflect V2 architecture.
  - Fixing links in `BOT_DEVELOPMENT_GUIDE.md`.
- **Next**:
  - Add backend unit tests (`go test`).
  - Restore/Create `smoke_test.sh` for end-to-end verification.
  - Verify "Success Criteria" from `BOT_HOSTER_PLAN_V2.md` (Bot scaling, reconnects).

## Open Questions
- None.

## Working Set
- `CONTINUITY.md`
- `MULTIPLAYER.md`
- `BOT_DEVELOPMENT_GUIDE.md`
