# Backend Architecture

## Overview

The backend is a Go WebSocket server that manages multiplayer Virus game sessions. It uses a single-threaded event loop pattern (Hub) to handle all state mutations safely without locks.

## File Structure

```
backend/
├── main.go      # HTTP server, WebSocket endpoint, static files
├── hub.go       # Central event loop, game logic, message handlers
├── client.go    # WebSocket client, read/write pumps
├── types.go     # Data structures (Game, User, Lobby, Message, etc.)
├── bot.go       # AI bot implementation (minimax algorithm)
└── names.go     # Random username generator
```

## Core Architecture

```
┌─────────────────────────────────────────────────────────────────────┐
│                         HTTP Server (main.go)                       │
│                              :8080                                  │
│   ┌─────────────┐    ┌─────────────────────────────────────────┐   │
│   │ GET /*      │    │ GET /ws (WebSocket upgrade)             │   │
│   │ Static files│    │ Creates Client, starts read/writePump   │   │
│   └─────────────┘    └─────────────────────────────────────────┘   │
└─────────────────────────────────────────────────────────────────────┘
                                    │
                                    ▼
┌─────────────────────────────────────────────────────────────────────┐
│                        Hub (hub.go)                                 │
│                                                                     │
│   ┌─────────────────────────────────────────────────────────────┐   │
│   │                    run() - Event Loop                       │   │
│   │                    (SINGLE GOROUTINE)                       │   │
│   │                                                             │   │
│   │   for {                                                     │   │
│   │       select {                                              │   │
│   │       case client := <-h.register:    // New connection     │   │
│   │       case client := <-h.unregister:  // Disconnection      │   │
│   │       case wrapper := <-h.handleMessage: // All messages    │   │
│   │       }                                                     │   │
│   │   }                                                         │   │
│   └─────────────────────────────────────────────────────────────┘   │
│                                                                     │
│   ┌─────────────────────────────────────────────────────────────┐   │
│   │                    State (NO MUTEX NEEDED)                  │   │
│   │                                                             │   │
│   │   clients    map[*Client]bool      // Connected clients     │   │
│   │   users      map[string]*User      // UserID -> User        │   │
│   │   games      map[string]*Game      // GameID -> Game        │   │
│   │   lobbies    map[string]*Lobby     // LobbyID -> Lobby      │   │
│   │   challenges map[string]*Challenge // ChallengeID -> Chal   │   │
│   └─────────────────────────────────────────────────────────────┘   │
└─────────────────────────────────────────────────────────────────────┘
                                    │
            ┌───────────────────────┼───────────────────────┐
            ▼                       ▼                       ▼
┌───────────────────┐   ┌───────────────────┐   ┌───────────────────┐
│     Client 1      │   │     Client 2      │   │     Client N      │
│  ┌─────────────┐  │   │  ┌─────────────┐  │   │  ┌─────────────┐  │
│  │  readPump   │──┼───┼──│  Reads WS   │  │   │  │  readPump   │  │
│  │  (goroutine)│  │   │  │  messages   │  │   │  │             │  │
│  └─────────────┘  │   │  └─────────────┘  │   │  └─────────────┘  │
│  ┌─────────────┐  │   │  ┌─────────────┐  │   │  ┌─────────────┐  │
│  │  writePump  │  │   │  │  Writes to  │  │   │  │  writePump  │  │
│  │  (goroutine)│  │   │  │  WebSocket  │  │   │  │             │  │
│  └─────────────┘  │   │  └─────────────┘  │   │  └─────────────┘  │
│  ┌─────────────┐  │   │  ┌─────────────┐  │   │  ┌─────────────┐  │
│  │ send chan   │  │   │  │ Buffered    │  │   │  │ send chan   │  │
│  │ []byte, 256 │  │   │  │ channel     │  │   │  │             │  │
│  └─────────────┘  │   │  └─────────────┘  │   │  └─────────────┘  │
└───────────────────┘   └───────────────────┘   └───────────────────┘
```

## Critical Design Principle

**ALL state modifications MUST go through the Hub's `handleMessage` channel.**

This ensures thread-safety without mutexes. The Hub's `run()` loop is the ONLY code that modifies:
- `h.clients`
- `h.users`
- `h.games`
- `h.lobbies`
- `h.challenges`
- Any `Game` or `User` struct fields

### Safe Pattern (DO THIS)

```go
// From any goroutine (timer, bot, etc.):
h.handleMessage <- &MessageWrapper{
    client: nil,  // nil for internal messages
    message: &Message{
        Type:   "internal_action",
        GameID: gameID,
    },
}
```

### Unsafe Pattern (DON'T DO THIS)

```go
// From a goroutine:
go func() {
    game.Board[r][c] = value  // RACE CONDITION!
    h.games[id] = game        // RACE CONDITION!
}()
```

## Message Flow

```
┌──────────┐     WebSocket      ┌──────────┐    Channel     ┌──────────┐
│  Browser │ ──── JSON ──────▶  │  Client  │ ─────────────▶ │   Hub    │
│          │                    │ readPump │  handleMessage │  run()   │
└──────────┘                    └──────────┘                └──────────┘
                                                                  │
                                                                  ▼
                                                            ┌──────────┐
                                                            │ Handler  │
                                                            │ Function │
                                                            └──────────┘
                                                                  │
     ┌────────────────────────────────────────────────────────────┘
     ▼
┌──────────┐    Channel     ┌──────────┐     WebSocket      ┌──────────┐
│   Hub    │ ─────────────▶ │  Client  │ ──── JSON ──────▶  │  Browser │
│sendToUser│   client.send  │ writePump│                    │          │
└──────────┘                └──────────┘                    └──────────┘
```

## Internal Message Types

These messages are sent through `handleMessage` for internal coordination:

| Type | Purpose | Sender |
|------|---------|--------|
| `bot_move` | Request bot move calculation | Hub (triggers async goroutine) |
| `bot_move_result` | Apply calculated bot move | Bot goroutine (after minimax) |
| `move_timeout` | Player ran out of time | Timer callback (via Hub channel) |
| `cleanup_game` | Delete finished game | Cleanup timer (via Hub channel) |

## Bot Move Architecture

Bot moves use an async pattern to avoid blocking the Hub's event loop:

```
┌─────────────────┐     1. bot_move           ┌─────────────────┐
│   Hub (main)    │ ─────────────────────────▶│  Goroutine      │
│   event loop    │   (spawn goroutine)       │                 │
│                 │                           │  copyBoard()    │
│  (continues     │                           │  minimax()      │
│   processing)   │  2. bot_move_result       │  (CPU heavy)    │
│                 │ ◀─────────────────────────│                 │
└─────────────────┘     {row, col}            └─────────────────┘
        │
        ▼
   3. Validate & apply move (fast)
```

Key points:
- `handleBotMoveRequest`: Copies board, spawns goroutine for calculation
- Goroutine: Runs minimax on board copy (safe, read-only)
- `handleBotMoveResult`: Validates move is still valid, applies to game state

## Cleanup Mechanisms

1. **Per-game cleanup timer**: When a game ends, a 10-second timer schedules `cleanup_game` via the Hub channel
2. **User cleanup on new game**: `cleanupUserFromPreviousGame()` removes user from old game before joining new one
3. **Periodic cleanup**: Every 5 minutes, `cleanupStaleGames()` removes orphaned games (no human players, game over, etc.)

## Game Lifecycle

```
┌─────────────┐
│   Lobby     │  Player creates lobby, others join
│   Created   │
└──────┬──────┘
       │ start_multiplayer_game
       ▼
┌─────────────┐
│   Game      │  Game in progress
│   Active    │  - Players make moves
└──────┬──────┘  - Bots make moves (via bot_move message)
       │         - Turn timer runs
       │
       ├─────────────────────────────────────┐
       │ (player eliminated / resigned)      │
       ▼                                     │
┌─────────────┐                              │
│   Game      │  Fewer players remain        │
│   Continues │                              │
└──────┬──────┘                              │
       │                                     │
       │ (only 1 player left)                │
       ▼                                     │
┌─────────────┐                              │
│   Game      │  Winner declared             │
│   Over      │                              │
└──────┬──────┘                              │
       │ cleanup_game (after 10s)
       ▼
┌─────────────┐
│   Game      │  Removed from h.games
│   Deleted   │
└─────────────┘
```

## Bot AI Architecture

```
┌─────────────────────────────────────────────────────────────────────┐
│                        Bot Move Request                             │
│                     (via bot_move message)                          │
└─────────────────────────────────────────────────────────────────────┘
                                │
                                ▼
┌─────────────────────────────────────────────────────────────────────┐
│                        makeBotMove()                                │
│   1. Get valid moves                                                │
│   2. Score moves with scoreMoveQuick() for ordering                 │
│   3. Run minimax with alpha-beta pruning                            │
│   4. Apply best move                                                │
└─────────────────────────────────────────────────────────────────────┘
                                │
                                ▼
┌─────────────────────────────────────────────────────────────────────┐
│                        Minimax Search                               │
│   - Depth: 4-5 (configurable via BotSettings)                       │
│   - Transposition table for memoization                             │
│   - Move pruning (top 15-20 moves per node)                         │
│   - Alpha-beta cutoffs                                              │
└─────────────────────────────────────────────────────────────────────┘
                                │
                                ▼
┌─────────────────────────────────────────────────────────────────────┐
│                      Board Evaluation                               │
│   Weighted sum of:                                                  │
│   - Material (cells + fortifications)                               │
│   - Mobility (available moves)                                      │
│   - Position (aggression toward opponent bases)                     │
│   - Redundancy (cells with multiple connections)                    │
│   - Cohesion (penalize gaps in territory)                           │
└─────────────────────────────────────────────────────────────────────┘
```

## Known Limitations & Future Improvements

### Current Limitations

1. **Board uses `interface{}`** - Type assertions everywhere, slow
2. **Single Hub** - No horizontal scaling
3. **In-memory state** - Server restart loses all games
4. **No authentication** - Anonymous users only

### Potential Improvements

1. **Typed board cells** - Use struct instead of interface{}
2. **Connection pooling** - For multiple game servers
3. **Redis/persistence** - For game state recovery
4. **User accounts** - For rankings, history
5. **Spectator mode** - Watch games in progress

## Configuration

| Constant | Value | Location | Purpose |
|----------|-------|----------|---------|
| `writeWait` | 10s | client.go | WebSocket write deadline |
| `pongWait` | 60s | client.go | WebSocket ping/pong timeout |
| `pingPeriod` | 54s | client.go | WebSocket ping interval |
| `maxMessageSize` | 512 | client.go | Max incoming message size |
| `defaultBotDepth` | 4 | bot.go | Minimax search depth |

## Debugging

### Useful Log Patterns

```bash
# Bot moves
grep "Bot player" server.log

# Turn changes
grep "endTurn" server.log

# Player elimination
grep "eliminated" server.log

# Game lifecycle
grep "Game started\|Game ended\|Cleaned up" server.log

# Connection issues
grep "disconnected\|Failed to send" server.log
```

### Common Issues

1. **"Moves not shown"** - Check for race conditions, ensure all state changes go through Hub
2. **High CPU** - Check bot depth, board size, number of concurrent games
3. **Memory leak** - Check game cleanup, ensure games are deleted after completion
4. **Disconnections** - Check WebSocket timeouts, network stability
