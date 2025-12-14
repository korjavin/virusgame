# Bot Development Guide

Welcome! This guide shows you how to create your own bot for the multiplayer game.

## Table of Contents

1. [Overview](#overview)
2. [Protocol](#protocol)
3. [Quick Start](#quick-start)
4. [Bot Templates](#bot-templates)
5. [AI Strategies](#ai-strategies)
6. [Deployment](#deployment)

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

- Go 1.21+ (for Go bot)
- Python 3.8+ (for Python bot)
- Node.js 14+ (for JS bot)
- Game server running and accessible

### Choose a Template

We provide templates for Go, Python, and JavaScript in the `bot-templates/` directory.

#### Go Template

```bash
cd bot-templates/go
# Edit .env to set BACKEND_URL
go run .
```

#### Python Template

```bash
cd bot-templates/python
pip install -r requirements.txt
# Set BACKEND_URL env var if needed
python bot.py
```

#### JavaScript Template

```bash
cd bot-templates/javascript
npm install
# Set BACKEND_URL env var if needed
npm start
```

### Test

1. Open browser, create lobby
2. Click "Add Bot"
3. Your bot should join automatically!

---

## Bot Templates

The repository contains starter kits in the `bot-templates/` directory:

```
bot-templates/
├── go/               # Go implementation
├── python/           # Python implementation
└── javascript/       # JavaScript (Node.js) implementation
```

Each template includes:
-   **WebSocket connection logic**: Handles the communication protocol.
-   **Game State**: Tracks the board, players, and current turn.
-   **Valid Move Logic**: Ensures the bot only attempts legal moves.
-   **Stub AI**: A random move generator you can replace with your own strategy.

### Python Bot Structure

-   `bot.py`: Main entry point. Handles WebSocket connection and protocol.
-   `game.py`: Game logic helper. Implements board state and move validation.

### JavaScript Bot Structure

-   `bot.js`: Main entry point. Handles WebSocket connection and protocol.
-   `game.js`: Game logic helper. Implements board state and move validation.

---

## AI Strategies

### Strategy 1: Random Bot (Included)

Picks random valid moves. This is implemented in the templates by default.

### Strategy 2: Greedy Bot

Always captures opponent cells when possible.

### Strategy 3: Minimax Bot

Use game tree search (see our official bot-hoster for full implementation).

---

## Deployment

### Docker

Each template can be containerized. See the Go template's `Dockerfile` for an example.

### Portainer

1. Create stack with your bot's docker-compose.yml
2. Set environment variables
3. Deploy on any host

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
