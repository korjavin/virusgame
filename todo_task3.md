# Task 3: Bot-Hoster Service - Core Implementation

## Context

Implement the bot-hoster service that maintains a pool of bot clients. Each bot is a WebSocket connection to the game backend that:
1. Connects to `/ws` like a human player
2. Listens for `bot_wanted` signals
3. Joins lobbies when requested
4. Plays games using AI
5. Returns to pool after game ends

This is the **heart of the bot system** - the service that runs bot players.

## Prerequisites

- Task 1 completed (Backend broadcasts `bot_wanted` signal)
- Task 2 completed (Docker setup with bot-hoster command)

## Related Files

- `backend/cmd/bot-hoster/main.go` - Entry point (stub exists from Task 2)
- `backend/cmd/bot-hoster/manager.go` - Bot pool manager (stub exists)
- `backend/cmd/bot-hoster/bot_client.go` - NEW: Individual bot client
- `backend/cmd/bot-hoster/config.go` - Config (exists from Task 2)
- `backend/bot.go` - AI engine to reuse
- `backend/types.go` - Message types to import

## Goal

Implement a fully functional bot pool manager that spawns bot clients, connects them to the game server, and manages their lifecycle.

## Acceptance Criteria

1. ✅ Bot-hoster starts and creates N bots (configurable via BOT_POOL_SIZE)
2. ✅ Each bot connects to backend via WebSocket
3. ✅ Each bot receives `welcome` message and stores userId/username
4. ✅ Bots listen for `bot_wanted` signals
5. ✅ When signal received, idle bot joins lobby
6. ✅ Bot appears in lobby like a regular player
7. ✅ Multiple bots can be in the pool
8. ✅ Graceful shutdown on Ctrl+C
9. ✅ Reconnection logic for dropped connections
10. ✅ Logging for all major events

## Architecture

```
BotManager
    │
    ├── Bot 1 [IDLE]      (WebSocket → Backend)
    ├── Bot 2 [IN_LOBBY]  (WebSocket → Backend)
    ├── Bot 3 [IN_GAME]   (WebSocket → Backend)
    └── Bot N [IDLE]      (WebSocket → Backend)
```

## Implementation Steps

### Step 1: Define Bot States and Structures

**File**: `backend/cmd/bot-hoster/bot_client.go`

```go
package main

import (
    "encoding/json"
    "fmt"
    "log"
    "sync"
    "time"

    "github.com/gorilla/websocket"
)

// BotState represents the current state of a bot
type BotState int

const (
    BotIdle BotState = iota
    BotInLobby
    BotInGame
    BotDisconnected
)

func (s BotState) String() string {
    switch s {
    case BotIdle:
        return "IDLE"
    case BotInLobby:
        return "IN_LOBBY"
    case BotInGame:
        return "IN_GAME"
    case BotDisconnected:
        return "DISCONNECTED"
    default:
        return "UNKNOWN"
    }
}

// Bot represents a single bot client
type Bot struct {
    ID           string
    Username     string
    UserID       string
    WS           *websocket.Conn
    State        BotState
    Manager      *BotManager
    BackendURL   string

    // Current activity
    CurrentLobby string
    CurrentGame  string
    YourPlayer   int
    BotSettings  *BotSettings

    // Game state (maintained locally like a human client)
    Board        [][]interface{}
    GamePlayers  []GamePlayerInfo
    PlayerBases  [4]CellPos
    Rows         int
    Cols         int

    // Communication channels
    send         chan []byte
    done         chan bool

    // Synchronization
    mu           sync.RWMutex
}

// Import Message and other types from parent package
type Message struct {
    Type             string           `json:"type"`
    UserID           string           `json:"userId,omitempty"`
    Username         string           `json:"username,omitempty"`
    LobbyID          string           `json:"lobbyId,omitempty"`
    RequestID        string           `json:"requestId,omitempty"`
    BotSettings      *BotSettings     `json:"botSettings,omitempty"`
    Rows             int              `json:"rows,omitempty"`
    Cols             int              `json:"cols,omitempty"`
    GameID           string           `json:"gameId,omitempty"`
    YourPlayer       int              `json:"yourPlayer,omitempty"`
    Player           int              `json:"player,omitempty"`
    Row              *int             `json:"row,omitempty"`
    Col              *int             `json:"col,omitempty"`
    MovesLeft        int              `json:"movesLeft,omitempty"`
    Winner           int              `json:"winner,omitempty"`
    Lobby            *LobbyInfo       `json:"lobby,omitempty"`
    GamePlayers      []GamePlayerInfo `json:"gamePlayers,omitempty"`
    EliminatedPlayer int              `json:"eliminatedPlayer,omitempty"`
}

type BotSettings struct {
    MaterialWeight   float64 `json:"materialWeight"`
    MobilityWeight   float64 `json:"mobilityWeight"`
    PositionWeight   float64 `json:"positionWeight"`
    RedundancyWeight float64 `json:"redundancyWeight"`
    CohesionWeight   float64 `json:"cohesionWeight"`
    SearchDepth      int     `json:"searchDepth"`
}

type LobbyInfo struct {
    LobbyID    string             `json:"lobbyId"`
    HostName   string             `json:"hostName"`
    Players    []LobbyPlayerInfo  `json:"players"`
    MaxPlayers int                `json:"maxPlayers"`
}

type LobbyPlayerInfo struct {
    Username string `json:"username,omitempty"`
    IsBot    bool   `json:"isBot"`
    Symbol   string `json:"symbol"`
}

type GamePlayerInfo struct {
    PlayerIndex int    `json:"playerIndex"`
    Username    string `json:"username"`
    Symbol      string `json:"symbol"`
    IsBot       bool   `json:"isBot"`
    IsActive    bool   `json:"isActive"`
}

type CellPos struct {
    Row int `json:"row"`
    Col int `json:"col"`
}
```

### Step 2: Implement Bot Client - Connection

**File**: `backend/cmd/bot-hoster/bot_client.go` (continued)

```go
// NewBot creates a new bot instance
func NewBot(backendURL string, manager *BotManager) *Bot {
    return &Bot{
        ID:         fmt.Sprintf("bot-%d", time.Now().UnixNano()),
        Manager:    manager,
        BackendURL: backendURL,
        State:      BotDisconnected,
        send:       make(chan []byte, 256),
        done:       make(chan bool),
    }
}

// Connect establishes WebSocket connection to backend
func (b *Bot) Connect() error {
    ws, _, err := websocket.DefaultDialer.Dial(b.BackendURL, nil)
    if err != nil {
        return fmt.Errorf("failed to connect to %s: %w", b.BackendURL, err)
    }

    b.mu.Lock()
    b.WS = ws
    b.State = BotIdle
    b.mu.Unlock()

    log.Printf("[Bot %s] Connected to %s", b.ID, b.BackendURL)
    return nil
}

// Run starts the bot's message loop
func (b *Bot) Run() {
    defer b.Disconnect()

    // Start writer goroutine
    go b.writePump()

    // Read messages from server
    for {
        select {
        case <-b.done:
            log.Printf("[Bot %s] Shutting down", b.Username)
            return
        default:
            var msg Message
            err := b.WS.ReadJSON(&msg)
            if err != nil {
                log.Printf("[Bot %s] Read error: %v", b.Username, err)

                // Attempt to reconnect
                if b.reconnect() {
                    continue
                }
                return
            }

            b.handleMessage(&msg)
        }
    }
}

// writePump sends messages from the send channel to WebSocket
func (b *Bot) writePump() {
    ticker := time.NewTicker(54 * time.Second)
    defer ticker.Stop()

    for {
        select {
        case message, ok := <-b.send:
            if !ok {
                b.WS.WriteMessage(websocket.CloseMessage, []byte{})
                return
            }

            if err := b.WS.WriteMessage(websocket.TextMessage, message); err != nil {
                log.Printf("[Bot %s] Write error: %v", b.Username, err)
                return
            }

        case <-ticker.C:
            // Send ping to keep connection alive
            if err := b.WS.WriteMessage(websocket.PingMessage, nil); err != nil {
                return
            }

        case <-b.done:
            return
        }
    }
}

// reconnect attempts to reconnect the bot
func (b *Bot) reconnect() bool {
    log.Printf("[Bot %s] Attempting to reconnect...", b.ID)

    b.mu.Lock()
    b.State = BotDisconnected
    if b.WS != nil {
        b.WS.Close()
    }
    b.mu.Unlock()

    // Wait before reconnecting
    time.Sleep(5 * time.Second)

    if err := b.Connect(); err != nil {
        log.Printf("[Bot %s] Reconnection failed: %v", b.ID, err)
        return false
    }

    log.Printf("[Bot %s] Reconnected successfully", b.ID)
    return true
}

// Disconnect closes the bot's connection
func (b *Bot) Disconnect() {
    b.mu.Lock()
    defer b.mu.Unlock()

    if b.State == BotDisconnected {
        return
    }

    close(b.done)

    if b.WS != nil {
        b.WS.Close()
    }

    b.State = BotDisconnected
    log.Printf("[Bot %s] Disconnected", b.Username)
}
```

### Step 3: Implement Message Handling

**File**: `backend/cmd/bot-hoster/bot_client.go` (continued)

```go
// handleMessage processes messages from the server
func (b *Bot) handleMessage(msg *Message) {
    switch msg.Type {
    case "welcome":
        b.handleWelcome(msg)

    case "bot_wanted":
        b.handleBotWanted(msg)

    case "lobby_joined":
        b.handleLobbyJoined(msg)

    case "multiplayer_game_start":
        b.handleGameStart(msg)

    case "move_made":
        b.handleMoveMade(msg)

    case "turn_change":
        b.handleTurnChange(msg)

    case "game_end":
        b.handleGameEnd(msg)

    case "player_eliminated":
        b.handlePlayerEliminated(msg)

    case "lobby_closed":
        b.handleLobbyClosed(msg)

    default:
        // Ignore other message types (users_update, etc.)
    }
}

func (b *Bot) handleWelcome(msg *Message) {
    b.mu.Lock()
    b.UserID = msg.UserID
    b.Username = msg.Username
    b.State = BotIdle
    b.mu.Unlock()

    log.Printf("[Bot %s] Registered as %s (ID: %s)", b.ID, b.Username, b.UserID)
}

func (b *Bot) handleBotWanted(msg *Message) {
    b.mu.RLock()
    isIdle := b.State == BotIdle
    b.mu.RUnlock()

    if !isIdle {
        // Bot is busy, ignore signal
        return
    }

    log.Printf("[Bot %s] Received bot_wanted signal for lobby %s", b.Username, msg.LobbyID)

    // Join the lobby
    b.JoinLobby(msg.LobbyID, msg.BotSettings)
}

func (b *Bot) handleLobbyJoined(msg *Message) {
    b.mu.Lock()
    b.State = BotInLobby
    b.CurrentLobby = msg.Lobby.LobbyID
    b.mu.Unlock()

    log.Printf("[Bot %s] Joined lobby %s", b.Username, b.CurrentLobby)
}

func (b *Bot) handleGameStart(msg *Message) {
    b.mu.Lock()
    b.State = BotInGame
    b.CurrentGame = msg.GameID
    b.YourPlayer = msg.YourPlayer
    b.Rows = msg.Rows
    b.Cols = msg.Cols
    b.GamePlayers = msg.GamePlayers

    // Initialize board
    b.Board = make([][]interface{}, b.Rows)
    for i := range b.Board {
        b.Board[i] = make([]interface{}, b.Cols)
    }

    // TODO: Extract PlayerBases from message (might need backend change)
    // For now, assume standard positions
    b.PlayerBases[0] = CellPos{Row: 0, Col: 0}
    b.PlayerBases[1] = CellPos{Row: b.Rows - 1, Col: b.Cols - 1}
    b.PlayerBases[2] = CellPos{Row: 0, Col: b.Cols - 1}
    b.PlayerBases[3] = CellPos{Row: b.Rows - 1, Col: 0}

    b.mu.Unlock()

    log.Printf("[Bot %s] Game started as player %d in game %s",
        b.Username, b.YourPlayer, b.CurrentGame)
}

func (b *Bot) handleMoveMade(msg *Message) {
    if msg.Row == nil || msg.Col == nil {
        return
    }

    b.mu.Lock()
    b.applyMove(*msg.Row, *msg.Col, msg.Player)
    b.mu.Unlock()

    log.Printf("[Bot %s] Move made by player %d at (%d, %d)",
        b.Username, msg.Player, *msg.Row, *msg.Col)
}

func (b *Bot) handleTurnChange(msg *Message) {
    b.mu.RLock()
    isMyTurn := msg.Player == b.YourPlayer
    b.mu.RUnlock()

    if isMyTurn {
        log.Printf("[Bot %s] My turn! Calculating move...", b.Username)
        // TODO: Task 4 will implement AI move calculation
        // For now, just log
    }
}

func (b *Bot) handleGameEnd(msg *Message) {
    b.mu.Lock()
    b.State = BotIdle
    b.CurrentGame = ""
    b.CurrentLobby = ""
    b.Board = nil
    b.mu.Unlock()

    log.Printf("[Bot %s] Game ended. Winner: player %d. Returning to pool.",
        b.Username, msg.Winner)
}

func (b *Bot) handlePlayerEliminated(msg *Message) {
    b.mu.Lock()
    for i := range b.GamePlayers {
        if b.GamePlayers[i].PlayerIndex == msg.EliminatedPlayer {
            b.GamePlayers[i].IsActive = false
        }
    }
    b.mu.Unlock()

    log.Printf("[Bot %s] Player %d eliminated", b.Username, msg.EliminatedPlayer)
}

func (b *Bot) handleLobbyClosed(msg *Message) {
    b.mu.Lock()
    b.State = BotIdle
    b.CurrentLobby = ""
    b.mu.Unlock()

    log.Printf("[Bot %s] Lobby closed. Returning to pool.", b.Username)
}

// applyMove updates the local board state
func (b *Bot) applyMove(row, col, player int) {
    cell := b.Board[row][col]
    if cell == nil {
        b.Board[row][col] = player
    } else {
        b.Board[row][col] = fmt.Sprintf("%d-fortified", player)
    }
}
```

### Step 4: Implement Bot Actions

**File**: `backend/cmd/bot-hoster/bot_client.go` (continued)

```go
// JoinLobby sends a join_lobby message
func (b *Bot) JoinLobby(lobbyID string, botSettings *BotSettings) {
    b.mu.Lock()
    b.BotSettings = botSettings
    b.mu.Unlock()

    msg := Message{
        Type:    "join_lobby",
        LobbyID: lobbyID,
    }

    b.sendMessage(&msg)
    log.Printf("[Bot %s] Sent join_lobby for %s", b.Username, lobbyID)
}

// sendMessage marshals and sends a message
func (b *Bot) sendMessage(msg *Message) {
    data, err := json.Marshal(msg)
    if err != nil {
        log.Printf("[Bot %s] Failed to marshal message: %v", b.Username, err)
        return
    }

    select {
    case b.send <- data:
    case <-time.After(time.Second):
        log.Printf("[Bot %s] Send timeout", b.Username)
    }
}
```

### Step 5: Implement Bot Pool Manager

**File**: `backend/cmd/bot-hoster/manager.go`

```go
package main

import (
    "log"
    "sync"
)

type BotManager struct {
    config  *Config
    bots    []*Bot
    mu      sync.RWMutex
}

func NewBotManager(config *Config) *BotManager {
    return &BotManager{
        config: config,
        bots:   make([]*Bot, 0, config.PoolSize),
    }
}

// Start initializes and connects all bots
func (m *BotManager) Start() error {
    log.Printf("Starting bot pool with size: %d", m.config.PoolSize)

    for i := 0; i < m.config.PoolSize; i++ {
        bot := NewBot(m.config.BackendURL, m)

        if err := bot.Connect(); err != nil {
            log.Printf("Failed to connect bot %d: %v (continuing with remaining bots)", i+1, err)
            continue
        }

        m.mu.Lock()
        m.bots = append(m.bots, bot)
        m.mu.Unlock()

        // Start bot message loop in goroutine
        go bot.Run()

        log.Printf("Bot %d/%d started", i+1, m.config.PoolSize)
    }

    m.mu.RLock()
    connectedCount := len(m.bots)
    m.mu.RUnlock()

    if connectedCount == 0 {
        return fmt.Errorf("no bots connected successfully")
    }

    log.Printf("Bot pool ready: %d/%d bots connected", connectedCount, m.config.PoolSize)
    return nil
}

// Stop gracefully shuts down all bots
func (m *BotManager) Stop() {
    log.Println("Stopping bot pool...")

    m.mu.Lock()
    defer m.mu.Unlock()

    for _, bot := range m.bots {
        bot.Disconnect()
    }

    log.Printf("All %d bots stopped", len(m.bots))
}

// GetStats returns current pool statistics
func (m *BotManager) GetStats() map[string]int {
    m.mu.RLock()
    defer m.mu.RUnlock()

    stats := map[string]int{
        "total":        len(m.bots),
        "idle":         0,
        "in_lobby":     0,
        "in_game":      0,
        "disconnected": 0,
    }

    for _, bot := range m.bots {
        bot.mu.RLock()
        state := bot.State
        bot.mu.RUnlock()

        switch state {
        case BotIdle:
            stats["idle"]++
        case BotInLobby:
            stats["in_lobby"]++
        case BotInGame:
            stats["in_game"]++
        case BotDisconnected:
            stats["disconnected"]++
        }
    }

    return stats
}
```

### Step 6: Update Main Entry Point

**File**: `backend/cmd/bot-hoster/main.go`

Replace the stub with:

```go
package main

import (
    "log"
    "os"
    "os/signal"
    "syscall"
    "time"
)

func main() {
    log.Println("=== Bot-Hoster Service Starting ===")

    config := LoadConfig()
    log.Printf("Configuration:")
    log.Printf("  Backend URL: %s", config.BackendURL)
    log.Printf("  Pool Size: %d", config.PoolSize)

    manager := NewBotManager(config)

    // Start bot pool
    if err := manager.Start(); err != nil {
        log.Fatalf("Failed to start bot manager: %v", err)
    }

    log.Println("=== Bot-Hoster Service Running ===")

    // Print stats periodically
    statsTicker := time.NewTicker(30 * time.Second)
    go func() {
        for range statsTicker.C {
            stats := manager.GetStats()
            log.Printf("Pool stats: Total=%d, Idle=%d, InLobby=%d, InGame=%d, Disconnected=%d",
                stats["total"], stats["idle"], stats["in_lobby"], stats["in_game"], stats["disconnected"])
        }
    }()

    // Wait for interrupt signal
    sigChan := make(chan os.Signal, 1)
    signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)
    <-sigChan

    log.Println("=== Shutdown Signal Received ===")
    statsTicker.Stop()
    manager.Stop()
    log.Println("=== Bot-Hoster Service Stopped ===")
}
```

## Testing Steps

### Test 1: Build and Run

```bash
# Build
cd backend
go build -o bot-hoster ./cmd/bot-hoster

# Run with local backend
export BACKEND_URL=ws://localhost:8080/ws
export BOT_POOL_SIZE=3
./bot-hoster

# Expected output:
# === Bot-Hoster Service Starting ===
# Configuration:
#   Backend URL: ws://localhost:8080/ws
#   Pool Size: 3
# [Bot bot-123] Connected to ws://localhost:8080/ws
# [Bot bot-123] Registered as AI-BraveOctopus42 (ID: xxx)
# Bot 1/3 started
# [Bot bot-456] Connected to ws://localhost:8080/ws
# [Bot bot-456] Registered as AI-CleverWolf19 (ID: yyy)
# Bot 2/3 started
# ...
# Bot pool ready: 3/3 bots connected
# === Bot-Hoster Service Running ===
```

### Test 2: Verify Bots Appear in Backend

```bash
# Backend should log:
# User connected: AI-BraveOctopus42 (xxx)
# User connected: AI-CleverWolf19 (yyy)
# User connected: AI-SwiftFox88 (zzz)
```

### Test 3: Test bot_wanted Signal

```bash
# 1. Start backend
cd backend && go run .

# 2. Start bot-hoster (in another terminal)
cd backend
export BACKEND_URL=ws://localhost:8080/ws
export BOT_POOL_SIZE=2
go run ./cmd/bot-hoster

# 3. Open browser, create lobby, click "Add Bot"

# Expected in bot-hoster logs:
# [Bot AI-BraveOctopus42] Received bot_wanted signal for lobby abc-123
# [Bot AI-BraveOctopus42] Sent join_lobby for abc-123
# [Bot AI-BraveOctopus42] Joined lobby abc-123

# Expected in lobby UI:
# Bot "AI-BraveOctopus42" appears in lobby!
```

### Test 4: Test Multiple Bots

```bash
# Create lobby with 4 slots
# Click "Add Bot" 3 times

# Expected:
# - 3 bots join lobby
# - All show as ready
# - Can start game
```

### Test 5: Test Reconnection

```bash
# 1. Start bot-hoster
# 2. Stop backend (Ctrl+C)
# 3. Restart backend

# Expected in bot-hoster logs:
# [Bot AI-BraveOctopus42] Read error: EOF
# [Bot AI-BraveOctopus42] Attempting to reconnect...
# [Bot AI-BraveOctopus42] Connected to ws://localhost:8080/ws
# [Bot AI-BraveOctopus42] Reconnected successfully
```

### Test 6: Docker Deployment

```bash
# Build image
cd backend
docker build -t virusgame:latest .

# Run backend
docker-compose up -d

# Run bot-hoster
cd ..
docker-compose -f bot-hoster-compose.yml --env-file .env.bot-hoster up

# Check logs
docker logs -f virusgame-bot-hoster
```

## Edge Cases

1. **All bots busy**: When bot_wanted signal comes but all bots are in games
   - Expected: Signal ignored, no bot joins
   - User can click "Add Bot" again later

2. **Bot disconnects during game**: Connection drops while bot is playing
   - Expected: Bot attempts reconnection
   - Backend handles as player disconnect (auto-resign)

3. **Backend not available**: Bot-hoster starts before backend
   - Expected: Connection fails, retries in reconnect loop

4. **Invalid backend URL**: Wrong URL in config
   - Expected: All bots fail to connect, service exits with error

## Dependencies

**Blocked by**:
- Task 1 (Backend broadcasts bot_wanted)
- Task 2 (Docker setup)

**Blocks**:
- Task 4 (AI integration - bots can join but not play yet)

## Estimated Time

**6-8 hours**

- 3 hours: Bot client implementation
- 2 hours: Bot manager and pool
- 2 hours: Testing and debugging
- 1 hour: Documentation

## Success Validation

After completing this task:

```bash
# 1. Bot-hoster starts
./bot-hoster
# ✓ Logs show N bots connected

# 2. Bots appear in backend
# ✓ Backend logs show "User connected: AI-BotName"

# 3. Bots respond to signal
# Create lobby, click "Add Bot"
# ✓ Bot joins lobby within 1 second

# 4. Bot appears in UI
# ✓ Lobby shows bot with AI- prefix name

# 5. Multiple bots work
# Click "Add Bot" 3 times
# ✓ 3 different bots join

# 6. Stats logging works
# Wait 30 seconds
# ✓ See "Pool stats: ..." in logs
```

## Known Limitations (To Be Fixed in Task 4)

- ✅ Bots join lobbies
- ✅ Bots appear in games
- ❌ Bots don't calculate moves yet (no AI integration)
- ❌ Bots don't send moves (Task 4 will add this)

This is expected - Task 4 will add the AI brain!

## Notes

- Each bot maintains its own WebSocket connection
- Bots are stateful - they track their current lobby/game
- The bot pool is thread-safe (uses mutexes)
- Bots automatically return to idle state after games
- Failed bot connections don't stop other bots

## Related Documentation

- `BOT_HOSTER_PLAN_V2.md` - Full architecture
- `backend/hub.go` - Backend message handling
- `backend/types.go` - Message type definitions
