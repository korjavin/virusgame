#Bot-Hoster Service - Architecture Plan V2
## Bots as First-Class Players

## Overview

A **bot player farm** that maintains a pool of bot clients that connect to the game server using the **exact same WebSocket interface as human players**. This allows:
- Bots to be indistinguishable from humans in the protocol
- Independent developers to create their own bot implementations
- Easy ecosystem growth and bot competitions
- Zero special handling in backend for bots

---

## Philosophy: Bots = Users

**Core Principle**: Bots connect to `/ws` just like humans, receive the same messages, and send the same commands.

**Key Difference**: Bots listen for special `bot_wanted` signals and auto-join lobbies that need them.

```
┌─────────────────────────────────────────────────┐
│           Main Backend (/ws)                    │
│                                                 │
│  All clients connect to same endpoint:          │
│  - Human players (browsers)                     │
│  - Bot players (bot-hoster service)             │
│  - Future: Independent bot implementations      │
│                                                 │
└────────┬────────────────────────────────────────┘
         │
         │ Same WebSocket Protocol
         │
    ┌────┴──────┬──────────┬──────────────┐
    │           │          │              │
┌───▼────┐ ┌───▼────┐ ┌───▼─────┐  ┌────▼─────┐
│ Human  │ │ Human  │ │   Bot   │  │   Bot    │
│Browser │ │Browser │ │Hoster 1 │  │ Hoster 2 │
└────────┘ └────────┘ │Pool: 10 │  │Pool: 10  │
                      │bots     │  │bots      │
                      └─────────┘  └──────────┘
```

---

## Current vs New Architecture

### Current System (Resource Problem)

```
1. User creates lobby
2. Host clicks "Add Bot"
3. Backend creates LobbyPlayer{IsBot: true, User: nil}
4. Game starts
5. On bot turn → calculateBotMove() runs ON BACKEND
6. Backend applies move
```

**Problem**: AI runs on game server, causing CPU exhaustion.

### New System (Bots as Players)

```
1. User creates lobby
2. Host clicks "Add Bot"
3. Backend broadcasts: {type: "bot_wanted", lobbyId: "...", botSettings: {...}}
4. Bot-hoster sees signal → assigns available bot
5. Bot connects/joins: {type: "join_lobby", lobbyId: "..."}
6. Bot appears in lobby as regular player (with AI- prefix name)
7. Game starts
8. On bot turn → bot calculates move locally, sends: {type: "move", ...}
9. Backend validates and applies move (same as human)
10. Game ends → bot returns to pool
```

**Benefits**:
- ✅ Bots use same protocol as humans
- ✅ No special backend handling
- ✅ Independent developers can write bots
- ✅ Bot AI runs on bot-hoster hardware
- ✅ Can scale bot-hosters independently

---

## Protocol Changes

### New Message Types

#### 1. Bot Request Signal (Backend → All Clients)

**Sent when**: Host clicks "Add Bot" button

```json
{
  "type": "bot_wanted",
  "lobbyId": "uuid",
  "requestId": "uuid",
  "botSettings": {
    "materialWeight": 100.0,
    "mobilityWeight": 50.0,
    "positionWeight": 30.0,
    "redundancyWeight": 40.0,
    "cohesionWeight": 25.0,
    "searchDepth": 5
  },
  "rows": 20,
  "cols": 20
}
```

**Who processes**:
- Human clients: Ignore (don't show in UI)
- Bot clients: Check if available, if yes → join lobby

#### 2. Bot Availability (Bot → Backend) [Optional]

**Sent when**: Bot-hoster wants to advertise available bots

```json
{
  "type": "bot_available",
  "botId": "uuid",
  "capabilities": ["multiplayer"],
  "version": "1.0.0"
}
```

This is optional - bots can just listen for `bot_wanted` and join directly.

---

## Bot-Hoster Service Architecture

### Concept: Bot Player Pool

The bot-hoster maintains a **pool of bot clients**, each with its own WebSocket connection to the game server.

```
Bot-Hoster Service
├── Bot Pool Manager
│   ├── Bot 1 (WebSocket to /ws) [IDLE]
│   ├── Bot 2 (WebSocket to /ws) [IN_LOBBY]
│   ├── Bot 3 (WebSocket to /ws) [IN_GAME]
│   ├── Bot 4 (WebSocket to /ws) [IDLE]
│   └── Bot N (WebSocket to /ws) [IDLE]
│
├── Message Listener
│   └── Listens for "bot_wanted" on all bot connections
│
└── AI Engine
    └── Calculates moves when bots are playing
```

### Bot Lifecycle

```
1. STARTUP: Bot connects to /ws
2. REGISTERED: Receives welcome message with userId (AI-BraveOctopus42)
3. IDLE: Listening for bot_wanted signals
4. SIGNALED: Receives bot_wanted for lobby
5. JOINING: Sends join_lobby message
6. IN_LOBBY: Waiting for game start
7. IN_GAME: Playing moves using AI
8. GAME_END: Returns to IDLE state
9. (Optional) DISCONNECT: If pool is full, disconnect to free resources
```

### State Management

```go
type BotState int

const (
    BotIdle BotState = iota
    BotInLobby
    BotInGame
)

type Bot struct {
    ID           string
    Username     string
    UserID       string  // From backend welcome message
    WS           *websocket.Conn
    State        BotState
    CurrentLobby string
    CurrentGame  string
    YourPlayer   int
    BotSettings  *BotSettings
    AIEngine     *AIEngine

    // Game state (maintained locally like a human client would)
    Board        [][]interface{}
    GamePlayers  []GamePlayerInfo
    PlayerBases  []CellPos
    CurrentTurn  int
}
```

---

## Implementation Details

### Bot-Hoster Service Structure

```
bot-hoster/
├── main.go                 # Entry point, starts bot pool
├── manager.go              # Bot pool manager
├── bot_client.go           # Individual bot WebSocket client
├── ai/
│   └── bot.go             # AI engine (copy from backend/bot.go)
├── protocol.go            # Message types (copy from backend/types.go)
├── config.go              # Configuration
├── Dockerfile
├── docker-compose.yml
├── go.mod
└── README.md
```

### Core Components

#### 1. main.go - Entry Point

```go
package main

import (
    "log"
    "os"
    "os/signal"
)

func main() {
    config := LoadConfig()
    manager := NewBotManager(config)

    // Start bot pool
    log.Printf("Starting bot-hoster with %d bots", config.PoolSize)
    if err := manager.Start(); err != nil {
        log.Fatal(err)
    }

    // Wait for interrupt
    c := make(chan os.Signal, 1)
    signal.Notify(c, os.Interrupt)
    <-c

    log.Println("Shutting down bot-hoster...")
    manager.Stop()
}
```

#### 2. manager.go - Bot Pool Manager

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

func (m *BotManager) Start() error {
    // Create and connect all bots
    for i := 0; i < m.config.PoolSize; i++ {
        bot := NewBot(m.config.BackendURL, m)
        if err := bot.Connect(); err != nil {
            log.Printf("Failed to connect bot %d: %v", i, err)
            continue
        }

        m.mu.Lock()
        m.bots = append(m.bots, bot)
        m.mu.Unlock()

        // Start bot message loop
        go bot.Run()

        log.Printf("Bot %d connected: %s", i+1, bot.Username)
    }

    log.Printf("Bot pool ready: %d/%d bots connected", len(m.bots), m.config.PoolSize)
    return nil
}

func (m *BotManager) Stop() {
    m.mu.Lock()
    defer m.mu.Unlock()

    for _, bot := range m.bots {
        bot.Disconnect()
    }
}

func (m *BotManager) FindIdleBot() *Bot {
    m.mu.RLock()
    defer m.mu.RUnlock()

    for _, bot := range m.bots {
        if bot.State == BotIdle {
            return bot
        }
    }
    return nil
}

func (m *BotManager) HandleBotWanted(msg *Message) {
    bot := m.FindIdleBot()
    if bot == nil {
        log.Printf("No idle bots available for lobby %s", msg.LobbyID)
        return
    }

    log.Printf("Assigning bot %s to lobby %s", bot.Username, msg.LobbyID)
    bot.JoinLobby(msg.LobbyID, msg.BotSettings)
}
```

#### 3. bot_client.go - Individual Bot

```go
package main

import (
    "encoding/json"
    "log"
    "time"

    "github.com/gorilla/websocket"
)

type Bot struct {
    ID           string
    Username     string
    UserID       string
    WS           *websocket.Conn
    State        BotState
    Manager      *BotManager

    // Current activity
    CurrentLobby string
    CurrentGame  string
    YourPlayer   int
    BotSettings  *BotSettings

    // Game state
    Board        [][]interface{}
    GamePlayers  []GamePlayerInfo
    PlayerBases  []CellPos
    Rows         int
    Cols         int

    // AI
    AIEngine     *AIEngine

    // Channels
    send         chan []byte
    done         chan bool
}

func NewBot(backendURL string, manager *BotManager) *Bot {
    return &Bot{
        ID:      generateID(),
        Manager: manager,
        State:   BotIdle,
        send:    make(chan []byte, 256),
        done:    make(chan bool),
    }
}

func (b *Bot) Connect() error {
    ws, _, err := websocket.DefaultDialer.Dial(b.Manager.config.BackendURL, nil)
    if err != nil {
        return err
    }

    b.WS = ws
    return nil
}

func (b *Bot) Run() {
    defer b.Disconnect()

    // Start writer goroutine
    go b.writePump()

    // Read messages
    for {
        select {
        case <-b.done:
            return
        default:
            var msg Message
            err := b.WS.ReadJSON(&msg)
            if err != nil {
                log.Printf("Bot %s read error: %v", b.Username, err)
                return
            }

            b.handleMessage(&msg)
        }
    }
}

func (b *Bot) writePump() {
    ticker := time.NewTicker(54 * time.Second)
    defer ticker.Stop()

    for {
        select {
        case message := <-b.send:
            if err := b.WS.WriteMessage(websocket.TextMessage, message); err != nil {
                return
            }
        case <-ticker.C:
            if err := b.WS.WriteMessage(websocket.PingMessage, nil); err != nil {
                return
            }
        case <-b.done:
            return
        }
    }
}

func (b *Bot) handleMessage(msg *Message) {
    switch msg.Type {
    case "welcome":
        b.UserID = msg.UserID
        b.Username = msg.Username
        log.Printf("Bot registered: %s (ID: %s)", b.Username, b.UserID)

    case "bot_wanted":
        // Only respond if we're idle
        if b.State == BotIdle {
            b.Manager.HandleBotWanted(msg)
        }

    case "lobby_joined":
        b.State = BotInLobby
        b.CurrentLobby = msg.Lobby.LobbyID
        log.Printf("Bot %s joined lobby %s", b.Username, b.CurrentLobby)

    case "multiplayer_game_start":
        b.State = BotInGame
        b.CurrentGame = msg.GameID
        b.YourPlayer = msg.YourPlayer
        b.Board = make([][]interface{}, msg.Rows)
        for i := range b.Board {
            b.Board[i] = make([]interface{}, msg.Cols)
        }
        b.GamePlayers = msg.GamePlayers
        b.PlayerBases = msg.PlayerBases  // Assuming backend sends this
        b.Rows = msg.Rows
        b.Cols = msg.Cols

        // Initialize AI engine
        b.AIEngine = NewAIEngine(b.BotSettings)

        log.Printf("Bot %s game started as player %d", b.Username, b.YourPlayer)

    case "move_made":
        // Update local board state
        b.applyMove(msg.Row, msg.Col, msg.Player)

    case "turn_change":
        // Is it our turn?
        if msg.Player == b.YourPlayer {
            log.Printf("Bot %s calculating move...", b.Username)
            go b.makeMove()
        }

    case "game_end":
        log.Printf("Bot %s game ended, winner: %d", b.Username, msg.Winner)
        b.State = BotIdle
        b.CurrentGame = ""
        b.CurrentLobby = ""
        b.Board = nil

    case "player_eliminated":
        // Update game state
        for i := range b.GamePlayers {
            if b.GamePlayers[i].PlayerIndex == msg.EliminatedPlayer {
                b.GamePlayers[i].IsActive = false
            }
        }
    }
}

func (b *Bot) JoinLobby(lobbyID string, botSettings *BotSettings) {
    b.BotSettings = botSettings
    b.sendMessage(&Message{
        Type:    "join_lobby",
        LobbyID: lobbyID,
    })
}

func (b *Bot) makeMove() {
    // Use AI engine to calculate best move
    gameState := &GameState{
        Board:       b.Board,
        Rows:        b.Rows,
        Cols:        b.Cols,
        PlayerBases: b.PlayerBases,
        Players:     b.GamePlayers,
    }

    row, col, ok := b.AIEngine.CalculateMove(gameState, b.YourPlayer)

    if !ok {
        log.Printf("Bot %s has no valid moves!", b.Username)
        // Could send resign message here
        return
    }

    // Send move
    b.sendMessage(&Message{
        Type:   "move",
        GameID: b.CurrentGame,
        Row:    &row,
        Col:    &col,
    })

    log.Printf("Bot %s sent move: (%d, %d)", b.Username, row, col)
}

func (b *Bot) applyMove(row, col *int, player int) {
    if row == nil || col == nil {
        return
    }

    r, c := *row, *col
    cell := b.Board[r][c]

    if cell == nil {
        b.Board[r][c] = player
    } else {
        b.Board[r][c] = fmt.Sprintf("%d-fortified", player)
    }
}

func (b *Bot) sendMessage(msg *Message) {
    data, err := json.Marshal(msg)
    if err != nil {
        log.Printf("Bot %s marshal error: %v", b.Username, err)
        return
    }

    select {
    case b.send <- data:
    case <-time.After(time.Second):
        log.Printf("Bot %s send timeout", b.Username)
    }
}

func (b *Bot) Disconnect() {
    close(b.done)
    b.WS.Close()
}
```

#### 4. config.go - Configuration

```go
package main

import (
    "os"
    "strconv"
)

type Config struct {
    BackendURL  string
    PoolSize    int
}

func LoadConfig() *Config {
    poolSize, _ := strconv.Atoi(getEnv("BOT_POOL_SIZE", "10"))

    return &Config{
        BackendURL: getEnv("BACKEND_URL", "ws://localhost:8080/ws"),
        PoolSize:   poolSize,
    }
}

func getEnv(key, defaultValue string) string {
    if value := os.Getenv(key); value != "" {
        return value
    }
    return defaultValue
}
```

---

## Backend Changes

### Minimal Changes Required

The beautiful thing about this approach: **almost no backend changes needed!**

#### Change 1: Broadcast bot_wanted Event

**File**: `backend/hub.go`

**Modify**: `handleAddBot()` function

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

    // Get bot settings from message
    botSettings := msg.BotSettings
    if botSettings == nil {
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
    botWantedMsg := Message{
        Type:        "bot_wanted",
        LobbyID:     lobby.ID,
        RequestID:   uuid.New().String(),
        BotSettings: botSettings,
        Rows:        lobby.Rows,
        Cols:        lobby.Cols,
    }
    h.broadcast(&botWantedMsg)

    log.Printf("Broadcasted bot_wanted for lobby %s", lobby.ID)

    // Note: We don't create a LobbyPlayer here anymore!
    // The bot will join via regular join_lobby message
}
```

#### Change 2: Mark Bot Users (Optional)

**File**: `backend/hub.go`

When a user with username starting with "AI-" connects, we can optionally mark them:

```go
func (h *Hub) handleConnect(client *Client) {
    username := GenerateRandomName()

    // Check if this is a bot (username starts with AI-)
    isBot := strings.HasPrefix(username, "AI-")

    userID := uuid.New().String()
    user := &User{
        ID:       userID,
        Username: username,
        Client:   client,
        InGame:   false,
        IsBot:    isBot,  // Optional: add this field to User struct
    }

    // ... rest of the function
}
```

This is optional - bots can be treated exactly like humans. But it's useful for:
- UI showing bot indicators
- Analytics/statistics
- Rate limiting (optional)

#### Change 3: Remove Old Bot Logic (Optional)

Since bots are now real users, we can remove:
- `handleBotMoveRequest()` - not needed
- `handleBotMoveResult()` - not needed
- `calculateBotMove()` calls from Hub - not needed

Bots send regular `move` messages that go through existing `handleMove()`.

**That's it!** No other backend changes needed.

---

## Frontend Changes

### Change 1: Ignore bot_wanted Messages

**File**: `multiplayer.js`

```javascript
handleMessage(msg) {
    switch (msg.type) {
        // ... existing cases ...

        case 'bot_wanted':
            // Human clients ignore this message
            // Only bot clients will process it
            break;

        // ... rest of the cases ...
    }
}
```

### Change 2: Show Bot Indicator (Optional)

**File**: `lobby.js`

Already implemented! Your `LobbyPlayerInfo` has `isBot` field.

---

## Deployment Configuration

### Bot-Hoster docker-compose.yml

```yaml
version: '3.8'

services:
  bot-hoster:
    build: .
    container_name: virusgame-bot-hoster-1
    restart: unless-stopped
    environment:
      - BACKEND_URL=ws://virusgame-backend:8080/ws
      - BOT_POOL_SIZE=10
    deploy:
      resources:
        limits:
          cpus: '2.0'
          memory: 2G
        reservations:
          cpus: '1.0'
          memory: 1G
```

### Scaling: Multiple Bot-Hosters

```bash
# Host A (10 bots)
cd bot-hoster
BOT_POOL_SIZE=10 docker-compose up -d

# Host B (10 bots)
cd bot-hoster
docker-compose -p bot-hoster-2 up -d

# Now you have 20 bots available!
```

Each bot-hoster independently:
1. Connects its bots to the backend
2. Listens for `bot_wanted` signals
3. Assigns idle bots to lobbies
4. First available bot responds (race condition is OK - backend validates)

---

## Benefits of This Approach

### 1. Ecosystem Friendly

**Independent Developers Can Build Bots!**

Anyone can write a bot that:
1. Connects to your `/ws` endpoint
2. Listens for `bot_wanted` messages
3. Joins lobbies and plays games
4. Uses any AI strategy they want

Example: A Python bot developer:
```python
import websocket
import json

class MyBot:
    def on_message(self, ws, message):
        msg = json.loads(message)

        if msg['type'] == 'bot_wanted':
            # Join the lobby!
            ws.send(json.dumps({
                'type': 'join_lobby',
                'lobbyId': msg['lobbyId']
            }))

        if msg['type'] == 'turn_change' and msg['player'] == self.my_player:
            # My turn - calculate move
            move = self.calculate_move()
            ws.send(json.dumps({
                'type': 'move',
                'gameId': self.game_id,
                'row': move['row'],
                'col': move['col']
            }))
```

### 2. Zero Backend Special Handling

Backend treats bots exactly like humans:
- Same validation
- Same game logic
- Same message protocol
- No special cases

### 3. Natural Scaling

Want more bots? Just deploy more bot-hosters:
- Each hoster maintains its own pool
- Multiple hosters = distributed bot capacity
- No coordination needed between hosters

### 4. Bot Competitions

Could host bot tournaments:
- Different bot implementations compete
- All use same protocol
- Leaderboards and statistics
- Community engagement

### 5. Resource Isolation

AI computation happens on bot-hoster machines:
- Main backend only does game logic
- Bot-hosters can be on different hardware
- Scale compute independently from game logic

---

## Migration Path

### Phase 1: Bot-Hoster Service (2 days)

**Goal**: Working bot-hoster that can join lobbies and play games

Tasks:
- [ ] Create project structure
- [ ] Implement Bot client with WebSocket connection
- [ ] Implement bot pool manager
- [ ] Copy AI engine from backend
- [ ] Handle bot_wanted signals
- [ ] Implement game playing logic
- [ ] Test bot can join lobby and play

### Phase 2: Backend Integration (1 hour)

**Goal**: Backend broadcasts bot_wanted signals

Tasks:
- [ ] Modify `handleAddBot()` to broadcast bot_wanted
- [ ] Test signal is received by bot clients
- [ ] (Optional) Remove old bot logic

### Phase 3: Testing (1 day)

**Goal**: Verify everything works end-to-end

Tasks:
- [ ] Test human + bot in same lobby
- [ ] Test multiple bots in one game
- [ ] Test bot pool exhaustion handling
- [ ] Test multiple bot-hosters
- [ ] Load test with many concurrent games

### Phase 4: Documentation (1 day)

**Goal**: Enable external bot development

Tasks:
- [ ] Write bot protocol documentation
- [ ] Write bot development guide
- [ ] Create example bot in different language (Python/Node.js)
- [ ] API documentation for bot developers

---

## Bot Protocol Documentation (for External Devs)

### How to Build Your Own Bot

**Step 1**: Connect to WebSocket
```
wss://your-server.com/ws
```

**Step 2**: Receive Welcome Message
```json
{
  "type": "welcome",
  "userId": "your-user-id",
  "username": "AI-BraveOctopus42"
}
```

**Step 3**: Listen for bot_wanted Signal
```json
{
  "type": "bot_wanted",
  "lobbyId": "lobby-uuid",
  "botSettings": {
    "materialWeight": 100.0,
    "mobilityWeight": 50.0,
    "positionWeight": 30.0,
    "redundancyWeight": 40.0,
    "cohesionWeight": 25.0,
    "searchDepth": 5
  },
  "rows": 20,
  "cols": 20
}
```

**Step 4**: Join the Lobby
```json
{
  "type": "join_lobby",
  "lobbyId": "lobby-uuid"
}
```

**Step 5**: Play the Game

When it's your turn:
```json
{
  "type": "turn_change",
  "player": 2  // Your player number
}
```

Calculate and send your move:
```json
{
  "type": "move",
  "gameId": "game-uuid",
  "row": 5,
  "col": 8
}
```

**Full Protocol**: See `MULTIPLAYER.md` for complete message reference.

---

## Performance & Scaling

### Resource Usage Per Bot-Hoster

**10 Bot Pool**:
- Memory: ~500MB - 1GB (50-100MB per bot)
- CPU: 1-2 cores (when bots are playing)
- Network: Minimal (~1KB/s per bot)

**20 Bot Pool**:
- Memory: ~1-2GB
- CPU: 2-4 cores
- Network: Minimal

### Scaling Example

**Scenario**: Need 100 concurrent bot players

**Solution**: Deploy 5 bot-hosters with 20 bots each
- Host A: 20 bots
- Host B: 20 bots
- Host C: 20 bots
- Host D: 20 bots
- Host E: 20 bots
- **Total**: 100 bots available

Each hoster independently listens for bot_wanted and assigns bots.

---

## Comparison: V1 vs V2

| Aspect | V1 (Compute Service) | V2 (Bot Players) |
|--------|---------------------|------------------|
| **Bot Identity** | Not users, just compute workers | Real users with userId |
| **Protocol** | Custom compute protocol | Same as human players |
| **Backend Changes** | +200 lines (worker management) | -20 lines (broadcast bot_wanted) |
| **External Bots** | Not possible | Easy - just implement protocol |
| **Game State** | Backend sends snapshot | Bot maintains locally |
| **Move Validation** | Backend trusts result | Backend validates like humans |
| **Scaling** | Via worker registration | Via bot pool size |
| **Complexity** | Medium (two protocols) | Low (one protocol) |

**V2 is superior** because:
- ✅ Simpler (one protocol for everything)
- ✅ Extensible (external devs can build bots)
- ✅ Natural (bots = players)
- ✅ Less backend code
- ✅ Same security model (backend validates all moves)

---

## Security Considerations

### Bot Validation

Backend still validates all bot moves:
- Move is valid for current game state
- Move is on bot's turn
- Bot has authority to move

Bots can't cheat because backend is authoritative.

### Rate Limiting (Optional)

Could add rate limiting for bot connections:
- Max N bots per IP
- Max M lobby joins per bot per hour

But since you said "no security needed", this is optional.

### Bot Identification

Bots can be identified by:
- Username prefix "AI-"
- Optional `isBot` flag in User struct
- Behavior patterns (instant moves, perfect play)

---

## Future Enhancements

### 1. Bot Marketplace

Users can:
- Submit their bot implementations
- Rate and review bots
- Choose which bot to add to lobby
- Bot leaderboards

### 2. Bot Customization

When adding bot, user chooses:
- Bot difficulty (AI settings)
- Bot strategy (aggressive, defensive, balanced)
- Bot personality (move speed, chat messages)

### 3. Bot Tournaments

Automated tournaments:
- Bracket-style competition
- ELO ratings
- Prize pools
- Streaming/spectating

### 4. Bot Analytics

Track and display:
- Win rates per bot
- Average game length
- Most popular bots
- Bot vs human statistics

---

## Implementation Priority

### Must Have (MVP)
1. ✅ Bot-hoster service with pool management
2. ✅ Bot WebSocket client implementation
3. ✅ Bot AI engine (copy from backend)
4. ✅ Bot responds to bot_wanted signals
5. ✅ Bot joins lobby and plays game
6. ✅ Backend broadcasts bot_wanted
7. ✅ Docker deployment

### Nice to Have
1. ⏳ Bot reconnection logic
2. ⏳ Graceful bot pool scaling
3. ⏳ Bot health monitoring
4. ⏳ Multiple bot-hoster coordination
5. ⏳ Bot developer documentation

### Future
1. 🔮 External bot API
2. 🔮 Bot marketplace
3. 🔮 Bot tournaments
4. 🔮 Bot analytics

---

## Success Criteria

1. ✅ Bot connects to /ws like human player
2. ✅ Bot receives bot_wanted signal
3. ✅ Bot joins lobby successfully
4. ✅ Bot appears in lobby like human
5. ✅ Bot plays full game using move protocol
6. ✅ Bot returns to pool after game
7. ✅ Multiple bots can be in same game
8. ✅ Bot-hoster can be deployed on separate host
9. ✅ Multiple bot-hosters work simultaneously
10. ✅ External developer can build bot using protocol

---

## Conclusion

This V2 architecture is **significantly better** than V1 because:

### Simplicity
- One protocol for all clients (human + bot)
- No special backend handling for bots
- Fewer lines of code

### Extensibility
- Independent developers can build bots
- Any language can implement bot (Python, JS, Rust, etc.)
- Ecosystem can grow organically

### Natural Design
- Bots are first-class citizens
- Same validation and security
- Same game experience

### Scalability
- Deploy more bot-hosters = more bots
- No coordination needed
- Pure horizontal scaling

**This is the right architecture for a bot-enabled competitive game!**