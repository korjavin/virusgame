# Task 1: Backend - Broadcast bot_wanted Signal

## Context

We're implementing a bot system where bots connect as regular players (not special compute workers). When a lobby host clicks "Add Bot", instead of creating a fake LobbyPlayer on the backend, we broadcast a `bot_wanted` signal that bot clients can respond to.

**Current behavior**: `handleAddBot()` creates a `LobbyPlayer{IsBot: true, User: nil}` directly in the lobby.

**New behavior**: `handleAddBot()` broadcasts a `bot_wanted` message to all connected clients. Bot clients will respond by joining the lobby via regular `join_lobby` message.

## Related Files

- `backend/hub.go` - Contains `handleAddBot()` function (line ~1293)
- `backend/types.go` - Contains `Message` struct and types
- `lobby.js` - Frontend that sends `add_bot` message (line ~114)
- `multiplayer.js` - Frontend WebSocket client

## Goal

Modify the backend to broadcast a `bot_wanted` signal when a lobby host adds a bot, instead of directly creating a bot slot.

## Acceptance Criteria

1. ✅ When lobby host clicks "Add Bot", backend broadcasts `bot_wanted` message to all connected clients
2. ✅ Message includes: lobbyId, botSettings (AI coefficients), rows, cols
3. ✅ Human clients can receive the message (but will ignore it in frontend)
4. ✅ No slot is created immediately - waiting for bot to join via `join_lobby`
5. ✅ Existing lobby functionality still works (humans joining, etc.)
6. ✅ Bot can join lobby via regular `join_lobby` message after seeing signal

## Implementation Steps

### Step 1: Update Message Types (if needed)

Check if `Message` struct in `backend/types.go` has a `RequestID` field. If not, add it:

```go
// backend/types.go
type Message struct {
    // ... existing fields ...
    RequestID     string       `json:"requestId,omitempty"`
    // ... rest of fields ...
}
```

### Step 2: Modify handleAddBot()

**File**: `backend/hub.go`

**Current code** (around line 1293):
```go
func (h *Hub) handleAddBot(user *User, msg *Message) {
    if !user.InLobby {
        h.sendError(user, "You are not in a lobby")
        return
    }

    lobby, exists := h.lobbies[user.LobbyID]
    if !exists {
        return
    }

    // Only host can add bots
    if lobby.Host.ID != user.ID {
        h.sendError(user, "Only the host can add bots")
        return
    }

    // Find empty slot
    slotIndex := -1
    for i := 0; i < lobby.MaxPlayers; i++ {
        if lobby.Players[i] == nil {
            slotIndex = i
            break
        }
    }

    if slotIndex == -1 {
        h.sendError(user, "Lobby is full")
        return
    }

    // Get bot settings from message, or use defaults
    botSettings := msg.BotSettings
    if botSettings == nil {
        // Default bot settings
        botSettings = &BotSettings{
            MaterialWeight:   100.0,
            MobilityWeight:   50.0,
            PositionWeight:   30.0,
            RedundancyWeight: 40.0,
            CohesionWeight:   25.0,
            SearchDepth:      5,
        }
    }

    // Add bot to slot
    lobby.Players[slotIndex] = &LobbyPlayer{
        User:        nil,
        IsBot:       true,
        Symbol:      playerSymbols[slotIndex],
        Ready:       true,
        Index:       slotIndex,
        BotSettings: botSettings,
    }

    h.broadcastLobbyUpdate(lobby)
    h.broadcastLobbiesList()

    log.Printf("Bot added to lobby %s (slot %d)", lobby.ID, slotIndex)
}
```

**New code** (replace the section from "// Add bot to slot" onwards):

```go
func (h *Hub) handleAddBot(user *User, msg *Message) {
    if !user.InLobby {
        h.sendError(user, "You are not in a lobby")
        return
    }

    lobby, exists := h.lobbies[user.LobbyID]
    if !exists {
        return
    }

    // Only host can add bots
    if lobby.Host.ID != user.ID {
        h.sendError(user, "Only the host can add bots")
        return
    }

    // Find empty slot
    slotIndex := -1
    for i := 0; i < lobby.MaxPlayers; i++ {
        if lobby.Players[i] == nil {
            slotIndex = i
            break
        }
    }

    if slotIndex == -1 {
        h.sendError(user, "Lobby is full")
        return
    }

    // Get bot settings from message, or use defaults
    botSettings := msg.BotSettings
    if botSettings == nil {
        // Default bot settings
        botSettings = &BotSettings{
            MaterialWeight:   100.0,
            MobilityWeight:   50.0,
            PositionWeight:   30.0,
            RedundancyWeight: 40.0,
            CohesionWeight:   25.0,
            SearchDepth:      5,
        }
    }

    // NEW: Broadcast bot_wanted signal to all clients
    requestID := uuid.New().String()
    botWantedMsg := Message{
        Type:        "bot_wanted",
        LobbyID:     lobby.ID,
        RequestID:   requestID,
        BotSettings: botSettings,
        Rows:        lobby.Rows,
        Cols:        lobby.Cols,
    }
    h.broadcast(&botWantedMsg)

    log.Printf("Broadcasted bot_wanted for lobby %s (requestId: %s)", lobby.ID, requestID)

    // Note: We don't create a LobbyPlayer here anymore!
    // The bot will join via regular join_lobby message
    // The lobby update will happen when bot actually joins
}
```

### Step 3: Add broadcast Helper (if not exists)

Check if `broadcast()` method exists in `hub.go`. If not, add it:

```go
// backend/hub.go

// broadcast sends a message to all connected clients
func (h *Hub) broadcast(msg *Message) {
    data, err := json.Marshal(msg)
    if err != nil {
        log.Printf("Error marshaling broadcast message: %v", err)
        return
    }

    for client := range h.clients {
        select {
        case client.send <- data:
        default:
            close(client.send)
            delete(h.clients, client)
        }
    }
}
```

**Note**: Check if a similar broadcast function already exists. The existing code has `broadcastUserList()` and `broadcastLobbyUpdate()` which are specific broadcasts. We need a general broadcast function.

### Step 4: Frontend - Ignore bot_wanted Messages

**File**: `multiplayer.js`

In the `handleMessage(msg)` function, add a case for `bot_wanted`:

```javascript
handleMessage(msg) {
    switch (msg.type) {
        case 'welcome':
            // ... existing code ...
            break;

        // ... other existing cases ...

        case 'bot_wanted':
            // Human clients ignore this message
            // Only bot clients will respond to this signal
            console.log('Bot wanted signal received (ignored by human client):', msg.lobbyId);
            break;

        // ... rest of cases ...
    }
}
```

This ensures human clients receive the message but don't crash or show errors.

## Testing Steps

### Test 1: Verify Signal Broadcast

1. Start backend: `cd backend && go run .`
2. Open browser, connect to game
3. Create a lobby
4. Open browser console (F12)
5. Click "Add Bot" button
6. Verify in console: Should see `bot_wanted` message logged
7. Verify in backend logs: Should see "Broadcasted bot_wanted for lobby..."

### Test 2: Verify No Immediate Slot Creation

1. Create lobby
2. Click "Add Bot"
3. Verify: Bot slot should NOT appear immediately in lobby
4. (Will appear later when actual bot joins - that's Task 3)

### Test 3: Verify Humans Can Still Join

1. Create lobby
2. Have another human join the lobby
3. Verify: Human joining still works normally
4. Verify: Lobby updates correctly

### Test 4: Verify Multiple bot_wanted Signals

1. Create lobby (4 slots)
2. Click "Add Bot" three times
3. Verify: Three `bot_wanted` messages broadcasted (check console)
4. Verify: No errors, backend stable

## Edge Cases to Handle

1. **Lobby full**: Already handled - sends error to user
2. **Not in lobby**: Already handled - sends error
3. **Not host**: Already handled - sends error
4. **Nil botSettings**: Already handled - uses defaults

## Rollback Plan

If this causes issues, revert to old behavior by replacing broadcast section with original code:

```go
// Rollback: Create bot directly (old behavior)
lobby.Players[slotIndex] = &LobbyPlayer{
    User:        nil,
    IsBot:       true,
    Symbol:      playerSymbols[slotIndex],
    Ready:       true,
    Index:       slotIndex,
    BotSettings: botSettings,
}
h.broadcastLobbyUpdate(lobby)
h.broadcastLobbiesList()
```

## Dependencies

**Blocked by**: None - can start immediately

**Blocks**:
- Task 3 (Bot-hoster service needs this signal to work)

## Estimated Time

**1-2 hours**

- 30 min: Code changes
- 30 min: Testing
- 30 min: Documentation/verification

## Success Validation

Run these commands to verify success:

```bash
# 1. Code compiles
cd backend
go build .

# 2. No errors on startup
go run .

# 3. Manual testing
# Open http://localhost:8080
# Create lobby → Click "Add Bot" → Check browser console for bot_wanted message
```

## Notes

- This is a **breaking change** for the bot system, but we're building a new bot system from scratch
- Old bot logic (handleBotMoveRequest, calculateBotMove in goroutines) can remain for now - will be removed in cleanup task
- The `bot_wanted` signal will initially have no responders - that's OK, bots will be implemented in Task 3
- Frontend already sends `botSettings` with `add_bot` message (see `lobby.js` line 117), so we have AI coefficients available

## Related Documentation

- `BOT_HOSTER_PLAN_V2.md` - Full architecture plan
- `MULTIPLAYER.md` - Current multiplayer protocol documentation
- `backend/hub.go` - Hub implementation with message handling
