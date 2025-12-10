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
