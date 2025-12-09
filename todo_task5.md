# Task 5: Bot Creation Guide for Independent Developers

## Context

Now that our bot system is working (Tasks 1-4), we want to enable **independent developers** to create their own bots. This opens up the ecosystem for:
- Custom bot strategies
- Bot competitions
- Community contributions
- Third-party bot hosting

This task creates comprehensive documentation and a Go template project that shows developers how to build their own bots.

## Prerequisites

- Task 1-4 completed (Full working bot system)

## Goal

Create documentation and template code that enables any developer to:
1. Understand the bot protocol
2. Create a simple bot in Go
3. Connect their bot to the game server
4. Deploy their bot using Docker

## Deliverables

1. `BOT_DEVELOPMENT_GUIDE.md` - Comprehensive guide
2. `bot-template/` - Go template project
3. `bot-template/README.md` - Quick start for template
4. Example bots with different strategies

## Acceptance Criteria

1. ✅ Developer can read guide and understand protocol in 15 minutes
2. ✅ Developer can clone template and run bot in 5 minutes
3. ✅ Template bot successfully connects and plays games
4. ✅ Guide explains how to customize AI strategy
5. ✅ Docker deployment instructions included
6. ✅ Example of alternative language (Python snippet) shown

## Implementation Steps

### Step 1: Create Bot Development Guide

**File**: `BOT_DEVELOPMENT_GUIDE.md`

```markdown
# Bot Development Guide

Welcome! This guide shows you how to create your own bot for the multiplayer game.

## Table of Contents

1. [Overview](#overview)
2. [Protocol](#protocol)
3. [Quick Start](#quick-start)
4. [Bot Template](#bot-template)
5. [AI Strategies](#ai-strategies)
6. [Deployment](#deployment)
7. [Examples](#examples)

---

## Overview

### What is a Bot?

A bot is a **regular player** that connects via WebSocket, just like a human. The only difference:
- Bots listen for `bot_wanted` signals
- Bots calculate moves using AI (instead of human input)

### Bot Lifecycle

```
1. Connect to ws://server.com/ws
2. Receive welcome message (get userId and username)
3. Listen for bot_wanted signal
4. Join lobby when requested
5. Play game using move messages
6. Return to idle after game ends
```

### Why Build Your Own Bot?

- Test custom AI strategies
- Compete in bot tournaments
- Learn game theory and AI algorithms
- Contribute to the ecosystem

---

## Protocol

### Connection

Connect to the same WebSocket endpoint as humans:

```
ws://your-server.com/ws
```

### Message Format

All messages are JSON over WebSocket.

#### 1. Welcome (Server → Bot)

Received immediately after connecting:

```json
{
  "type": "welcome",
  "userId": "uuid-string",
  "username": "AI-BraveOctopus42"
}
```

Store your `userId` and `username`.

#### 2. Bot Wanted Signal (Server → All Clients)

Sent when a lobby needs a bot:

```json
{
  "type": "bot_wanted",
  "lobbyId": "lobby-uuid",
  "requestId": "request-uuid",
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

**Human clients ignore this.** Bot clients should respond by joining the lobby.

#### 3. Join Lobby (Bot → Server)

```json
{
  "type": "join_lobby",
  "lobbyId": "lobby-uuid"
}
```

#### 4. Lobby Joined (Server → Bot)

```json
{
  "type": "lobby_joined",
  "lobby": {
    "lobbyId": "lobby-uuid",
    "hostName": "Alice",
    "players": [...],
    "maxPlayers": 4
  }
}
```

#### 5. Game Start (Server → Bot)

```json
{
  "type": "multiplayer_game_start",
  "gameId": "game-uuid",
  "yourPlayer": 2,
  "rows": 20,
  "cols": 20,
  "gamePlayers": [
    {"playerIndex": 0, "username": "Alice", "symbol": "X", "isBot": false, "isActive": true},
    {"playerIndex": 1, "username": "AI-Bot", "symbol": "O", "isBot": true, "isActive": true}
  ]
}
```

Store your `yourPlayer` number and initialize your game state.

#### 6. Turn Change (Server → Bot)

```json
{
  "type": "turn_change",
  "gameId": "game-uuid",
  "player": 2,
  "movesLeft": 3
}
```

If `player` equals your `yourPlayer`, it's your turn!

#### 7. Send Move (Bot → Server)

```json
{
  "type": "move",
  "gameId": "game-uuid",
  "row": 5,
  "col": 8
}
```

You have **3 moves per turn**. Send them one at a time.

#### 8. Move Made (Server → Bot)

```json
{
  "type": "move_made",
  "gameId": "game-uuid",
  "player": 2,
  "row": 5,
  "col": 8,
  "movesLeft": 2
}
```

Update your local board state.

#### 9. Game End (Server → Bot)

```json
{
  "type": "game_end",
  "gameId": "game-uuid",
  "winner": 1
}
```

Game over! Return to idle state.

---

## Quick Start

### Prerequisites

- Go 1.21+ installed
- Game server running and accessible

### Clone Template

```bash
git clone https://github.com/yourserver/bot-template.git
cd bot-template
```

### Configure

Edit `.env`:

```env
BACKEND_URL=ws://your-server.com/ws
```

### Run

```bash
go run .
```

Output:
```
Bot connected to ws://your-server.com/ws
Bot registered as AI-BraveOctopus42
Bot waiting for games...
```

### Test

1. Open browser, create lobby
2. Click "Add Bot"
3. Your bot should join automatically!

---

## Bot Template

See `bot-template/` directory for a complete example.

### Structure

```
bot-template/
├── main.go           # Entry point
├── bot.go            # Bot client
├── ai.go             # AI strategy
├── protocol.go       # Message types
├── .env             # Configuration
├── Dockerfile        # Docker build
└── README.md         # Quick start
```

### main.go

```go
package main

import (
    "log"
    "os"
)

func main() {
    backendURL := os.Getenv("BACKEND_URL")
    if backendURL == "" {
        backendURL = "ws://localhost:8080/ws"
    }

    bot := NewBot(backendURL)

    if err := bot.Connect(); err != nil {
        log.Fatal(err)
    }

    log.Println("Bot running... Press Ctrl+C to stop")
    bot.Run()
}
```

### bot.go

```go
package main

import (
    "encoding/json"
    "log"

    "github.com/gorilla/websocket"
)

type Bot struct {
    WS           *websocket.Conn
    BackendURL   string
    UserID       string
    Username     string
    CurrentGame  string
    YourPlayer   int
    Board        [][]interface{}
}

func NewBot(backendURL string) *Bot {
    return &Bot{BackendURL: backendURL}
}

func (b *Bot) Connect() error {
    ws, _, err := websocket.DefaultDialer.Dial(b.BackendURL, nil)
    if err != nil {
        return err
    }
    b.WS = ws
    log.Printf("Bot connected to %s", b.BackendURL)
    return nil
}

func (b *Bot) Run() {
    defer b.WS.Close()

    for {
        var msg Message
        if err := b.WS.ReadJSON(&msg); err != nil {
            log.Printf("Error: %v", err)
            return
        }

        b.handleMessage(&msg)
    }
}

func (b *Bot) handleMessage(msg *Message) {
    switch msg.Type {
    case "welcome":
        b.UserID = msg.UserID
        b.Username = msg.Username
        log.Printf("Bot registered as %s", b.Username)

    case "bot_wanted":
        log.Printf("Bot requested for lobby %s", msg.LobbyID)
        b.joinLobby(msg.LobbyID)

    case "lobby_joined":
        log.Printf("Joined lobby")

    case "multiplayer_game_start":
        b.CurrentGame = msg.GameID
        b.YourPlayer = msg.YourPlayer
        b.Board = make([][]interface{}, msg.Rows)
        for i := range b.Board {
            b.Board[i] = make([]interface{}, msg.Cols)
        }
        log.Printf("Game started, I am player %d", b.YourPlayer)

    case "turn_change":
        if msg.Player == b.YourPlayer {
            b.makeMove()
        }

    case "move_made":
        if msg.Row != nil && msg.Col != nil {
            b.applyMove(*msg.Row, *msg.Col, msg.Player)
        }

    case "game_end":
        log.Printf("Game ended, winner: player %d", msg.Winner)
        b.CurrentGame = ""
    }
}

func (b *Bot) joinLobby(lobbyID string) {
    msg := Message{
        Type:    "join_lobby",
        LobbyID: lobbyID,
    }
    b.sendMessage(&msg)
}

func (b *Bot) makeMove() {
    // Simple AI: find first valid move
    row, col := b.findValidMove()

    msg := Message{
        Type:   "move",
        GameID: b.CurrentGame,
        Row:    &row,
        Col:    &col,
    }

    b.sendMessage(&msg)
    log.Printf("Sent move: (%d, %d)", row, col)
}

func (b *Bot) findValidMove() (int, int) {
    // TODO: Implement your AI here!
    // This simple version just returns (1, 1)
    // See ai.go for a more sophisticated implementation
    return 1, 1
}

func (b *Bot) applyMove(row, col, player int) {
    cell := b.Board[row][col]
    if cell == nil {
        b.Board[row][col] = player
    } else {
        b.Board[row][col] = player // fortified
    }
}

func (b *Bot) sendMessage(msg *Message) {
    data, _ := json.Marshal(msg)
    b.WS.WriteMessage(websocket.TextMessage, data)
}
```

### protocol.go

```go
package main

type Message struct {
    Type         string           `json:"type"`
    UserID       string           `json:"userId,omitempty"`
    Username     string           `json:"username,omitempty"`
    LobbyID      string           `json:"lobbyId,omitempty"`
    GameID       string           `json:"gameId,omitempty"`
    YourPlayer   int              `json:"yourPlayer,omitempty"`
    Player       int              `json:"player,omitempty"`
    Row          *int             `json:"row,omitempty"`
    Col          *int             `json:"col,omitempty"`
    Rows         int              `json:"rows,omitempty"`
    Cols         int              `json:"cols,omitempty"`
    MovesLeft    int              `json:"movesLeft,omitempty"`
    Winner       int              `json:"winner,omitempty"`
    GamePlayers  []GamePlayerInfo `json:"gamePlayers,omitempty"`
    BotSettings  *BotSettings     `json:"botSettings,omitempty"`
    Lobby        *LobbyInfo       `json:"lobby,omitempty"`
}

type GamePlayerInfo struct {
    PlayerIndex int    `json:"playerIndex"`
    Username    string `json:"username"`
    Symbol      string `json:"symbol"`
    IsBot       bool   `json:"isBot"`
    IsActive    bool   `json:"isActive"`
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
    LobbyID    string `json:"lobbyId"`
    HostName   string `json:"hostName"`
    MaxPlayers int    `json:"maxPlayers"`
}
```

---

## AI Strategies

### Strategy 1: Random Bot

Picks random valid moves:

```go
func (b *Bot) findValidMove() (int, int) {
    for row := 0; row < len(b.Board); row++ {
        for col := 0; col < len(b.Board[0]); col++ {
            if b.isValidMove(row, col) {
                return row, col
            }
        }
    }
    return 0, 0
}
```

### Strategy 2: Greedy Bot

Always captures opponent cells when possible:

```go
func (b *Bot) findBestMove() (int, int) {
    // First, look for opponent cells to capture
    for row := 0; row < len(b.Board); row++ {
        for col := 0; col < len(b.Board[0]); col++ {
            if b.isOpponentCell(row, col) && b.canCapture(row, col) {
                return row, col // Capture!
            }
        }
    }

    // Otherwise, expand territory
    return b.findExpansionMove()
}
```

### Strategy 3: Minimax Bot

Use game tree search (see our official bot-hoster for full implementation).

---

## Deployment

### Docker

```dockerfile
FROM golang:1.21-alpine
WORKDIR /app
COPY . .
RUN go build -o bot .
CMD ["./bot"]
```

Build and run:

```bash
docker build -t my-bot .
docker run -e BACKEND_URL=ws://server.com/ws my-bot
```

### Portainer

1. Create stack with your bot's docker-compose.yml
2. Set environment variables
3. Deploy on any host

---

## Examples

### Python Bot

```python
import websocket
import json

class Bot:
    def __init__(self, url):
        self.ws = websocket.WebSocket()
        self.ws.connect(url)
        print(f"Connected to {url}")

    def run(self):
        while True:
            msg = json.loads(self.ws.recv())
            self.handle_message(msg)

    def handle_message(self, msg):
        if msg['type'] == 'welcome':
            print(f"Bot: {msg['username']}")

        elif msg['type'] == 'bot_wanted':
            self.join_lobby(msg['lobbyId'])

        elif msg['type'] == 'turn_change':
            if msg['player'] == self.your_player:
                self.make_move()

    def join_lobby(self, lobby_id):
        self.ws.send(json.dumps({
            'type': 'join_lobby',
            'lobbyId': lobby_id
        }))

    def make_move(self):
        # Simple AI: always move to (1, 1)
        self.ws.send(json.dumps({
            'type': 'move',
            'gameId': self.game_id,
            'row': 1,
            'col': 1
        }))

bot = Bot('ws://localhost:8080/ws')
bot.run()
```

### JavaScript Bot (Node.js)

```javascript
const WebSocket = require('ws');

class Bot {
    constructor(url) {
        this.ws = new WebSocket(url);
        this.ws.on('open', () => console.log('Connected'));
        this.ws.on('message', (data) => this.handleMessage(JSON.parse(data)));
    }

    handleMessage(msg) {
        if (msg.type === 'welcome') {
            console.log(`Bot: ${msg.username}`);
        } else if (msg.type === 'bot_wanted') {
            this.joinLobby(msg.lobbyId);
        } else if (msg.type === 'turn_change') {
            if (msg.player === this.yourPlayer) {
                this.makeMove();
            }
        }
    }

    joinLobby(lobbyId) {
        this.ws.send(JSON.stringify({
            type: 'join_lobby',
            lobbyId: lobbyId
        }));
    }

    makeMove() {
        this.ws.send(JSON.stringify({
            type: 'move',
            gameId: this.gameId,
            row: 1,
            col: 1
        }));
    }
}

const bot = new Bot('ws://localhost:8080/ws');
```

---

## FAQ

### Can I use any programming language?

Yes! As long as you can:
1. Connect via WebSocket
2. Send/receive JSON
3. Implement game logic

### How do bots get selected?

First available idle bot responds to the `bot_wanted` signal.

### Can I run multiple bots?

Yes! Each bot maintains its own WebSocket connection.

### What happens if my bot crashes?

The game continues with remaining players. Reconnect your bot to resume availability.

### Can I charge for my bot service?

That's up to you! You can host bots as a service for others.

---

## Resources

- Official Bot-Hoster: `/backend/cmd/bot-hoster`
- Protocol Docs: `/MULTIPLAYER.md`
- Server Source: `/backend/hub.go`
- AI Implementation: `/backend/bot.go`

---

## Support

Questions? Open an issue on GitHub or join our Discord.

Happy bot building! 🤖
```

### Step 2: Create Bot Template Project

**File**: `bot-template/README.md`

```markdown
# Bot Template

Quick-start template for building your own game bot in Go.

## Quick Start

```bash
# 1. Clone or copy this template
cp -r bot-template my-bot
cd my-bot

# 2. Install dependencies
go mod init my-bot
go get github.com/gorilla/websocket

# 3. Configure
echo "BACKEND_URL=ws://your-server.com/ws" > .env

# 4. Run
go run .
```

## Files

- `main.go` - Entry point
- `bot.go` - Bot client logic
- `ai.go` - AI strategy (customize this!)
- `protocol.go` - Message definitions
- `Dockerfile` - Docker build

## Customize Your Bot

Edit `ai.go` to implement your strategy:

```go
func (b *Bot) calculateBestMove() (int, int) {
    // Your AI logic here!
    // Return best (row, col)
}
```

## Deploy

```bash
docker build -t my-bot .
docker run -e BACKEND_URL=ws://server.com/ws my-bot
```

## Learn More

See `/BOT_DEVELOPMENT_GUIDE.md` for full documentation.
```

**File**: `bot-template/main.go`

[Include full code from guide above]

**File**: `bot-template/bot.go`

[Include full code from guide above]

**File**: `bot-template/protocol.go`

[Include full code from guide above]

**File**: `bot-template/ai.go`

```go
package main

// AI strategy implementation
// Customize this to create your unique bot!

// findValidMove returns the first valid move found
func (b *Bot) findValidMove() (int, int) {
    for row := 0; row < len(b.Board); row++ {
        for col := 0; col < len(b.Board[0]); col++ {
            if b.isValidMove(row, col) {
                return row, col
            }
        }
    }
    return 0, 0
}

// isValidMove checks if a move is legal
func (b *Bot) isValidMove(row, col int) bool {
    // Cell must be empty or opponent's (not fortified)
    cell := b.Board[row][col]

    // Empty is always valid if adjacent
    if cell == nil {
        return b.isAdjacentToMyTerritory(row, col)
    }

    // TODO: Add more validation logic
    return false
}

// isAdjacentToMyTerritory checks if cell touches my territory
func (b *Bot) isAdjacentToMyTerritory(row, col int) bool {
    // Check 8 neighbors
    for i := -1; i <= 1; i++ {
        for j := -1; j <= 1; j++ {
            if i == 0 && j == 0 {
                continue
            }

            nr, nc := row+i, col+j
            if nr >= 0 && nr < len(b.Board) && nc >= 0 && nc < len(b.Board[0]) {
                cell := b.Board[nr][nc]
                if cell != nil && cell == b.YourPlayer {
                    return true
                }
            }
        }
    }
    return false
}

// TODO: Implement more sophisticated AI
// Ideas:
// - Minimax algorithm
// - Alpha-beta pruning
// - Board evaluation function
// - Opening book
// - Endgame tables
```

**File**: `bot-template/Dockerfile`

```dockerfile
FROM golang:1.21-alpine AS builder

WORKDIR /app
COPY . .
RUN go mod download
RUN CGO_ENABLED=0 go build -o bot .

FROM alpine:latest
RUN apk --no-cache add ca-certificates
WORKDIR /root/
COPY --from=builder /app/bot .

CMD ["./bot"]
```

**File**: `bot-template/.env.example`

```env
BACKEND_URL=ws://localhost:8080/ws
```

**File**: `bot-template/go.mod`

```go
module bot-template

go 1.21

require github.com/gorilla/websocket v1.5.0
```

## Testing Steps

### Test 1: Verify Guide Completeness

```bash
# Read BOT_DEVELOPMENT_GUIDE.md
# Verify:
# - Protocol section is complete
# - All message types documented
# - Examples are clear
# - Quick start works
```

### Test 2: Test Template Bot

```bash
cd bot-template
go mod download
export BACKEND_URL=ws://localhost:8080/ws
go run .

# Expected:
# Bot connected to ws://localhost:8080/ws
# Bot registered as AI-BraveOctopus42
# Bot waiting for games...

# In browser:
# Create lobby → Click "Add Bot"
# Bot should join!
```

### Test 3: Docker Build

```bash
cd bot-template
docker build -t test-bot .
docker run -e BACKEND_URL=ws://host.docker.internal:8080/ws test-bot

# Should connect and work
```

### Test 4: External Developer Simulation

1. Give guide + template to someone unfamiliar with codebase
2. They should be able to:
   - Understand protocol in 15 minutes
   - Run template bot in 5 minutes
   - Customize AI in 30 minutes

## Dependencies

**Blocked by**:
- Task 4 (Need working bot system to document)

**Blocks**: None (Documentation task)

## Estimated Time

**4-5 hours**

- 2 hours: Write comprehensive guide
- 1 hour: Create template project
- 1 hour: Test and refine
- 1 hour: Examples in other languages

## Success Criteria

✅ Guide is comprehensive and clear
✅ Template bot works out of the box
✅ External developer can use template successfully
✅ Docker deployment instructions work
✅ Examples in Python/JavaScript are correct

## Deliverables Checklist

- [ ] `BOT_DEVELOPMENT_GUIDE.md` - Main guide
- [ ] `bot-template/README.md` - Template quick start
- [ ] `bot-template/main.go` - Entry point
- [ ] `bot-template/bot.go` - Bot client
- [ ] `bot-template/ai.go` - AI strategy
- [ ] `bot-template/protocol.go` - Messages
- [ ] `bot-template/Dockerfile` - Docker build
- [ ] `bot-template/.env.example` - Config example
- [ ] `bot-template/go.mod` - Dependencies
- [ ] Python example in guide
- [ ] JavaScript example in guide

## Notes

- Template should be minimal but functional
- Focus on clarity over sophistication
- Include comments explaining each part
- Provide multiple AI strategy examples (random, greedy, minimax)
- Emphasize that bots are just regular WebSocket clients

## Related Documentation

- `BOT_HOSTER_PLAN_V2.md` - Architecture
- `MULTIPLAYER.md` - Full protocol reference
- `backend/cmd/bot-hoster` - Official bot implementation (reference)
