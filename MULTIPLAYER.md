# Multiplayer Architecture

## Overview
The multiplayer system uses WebSockets to enable real-time gameplay between users. The Go backend manages user connections, game sessions, and move synchronization.

## Architecture

### Backend (Go + WebSockets)
- **WebSocket Server**: Handles all client connections
- **User Manager**: Tracks online users with random generated names
- **Challenge System**: Manages game challenges between users
- **Game Session Manager**: Handles active game state and move validation
- **Move Synchronization**: Broadcasts moves between players in real-time

### Frontend (JavaScript)
- **WebSocket Client**: Connects to backend server
- **Online Users UI**: Displays list of available players (left sidebar)
- **Challenge Notifications**: Shows incoming/outgoing challenges
- **Multiplayer Game Mode**: Replaces local game logic with networked moves
- **Rematch System**: Allows players to quickly start new games

## Message Protocol

### Client → Server

```json
// User connects (automatic on WebSocket connection)
{
  "type": "connect"
}

// Challenge another user
{
  "type": "challenge",
  "targetUserId": "user123"
}

// Accept a challenge
{
  "type": "accept_challenge",
  "challengeId": "challenge456"
}

// Decline a challenge
{
  "type": "decline_challenge",
  "challengeId": "challenge456"
}

// Make a move
{
  "type": "move",
  "gameId": "game789",
  "row": 2,
  "col": 3
}

// Place neutral fields
{
  "type": "neutrals",
  "gameId": "game789",
  "cells": [{row: 1, col: 1}, {row: 1, col: 2}]
}

// Request rematch
{
  "type": "rematch",
  "gameId": "game789"
}
```

### Server → Client

```json
// Welcome message with user ID and name
{
  "type": "welcome",
  "userId": "user123",
  "username": "BraveOctopus42"
}

// Online users list update
{
  "type": "users_update",
  "users": [
    {"userId": "user123", "username": "BraveOctopus42"},
    {"userId": "user456", "username": "CleverTiger88"}
  ]
}

// Incoming challenge notification
{
  "type": "challenge_received",
  "challengeId": "challenge456",
  "fromUserId": "user123",
  "fromUsername": "BraveOctopus42"
}

// Challenge accepted
{
  "type": "game_start",
  "gameId": "game789",
  "opponentId": "user456",
  "opponentUsername": "CleverTiger88",
  "yourPlayer": 1,
  "rows": 10,
  "cols": 10
}

// Challenge declined
{
  "type": "challenge_declined",
  "challengeId": "challenge456"
}

// Opponent made a move
{
  "type": "move_made",
  "gameId": "game789",
  "row": 2,
  "col": 3,
  "player": 2
}

// Game ended
{
  "type": "game_end",
  "gameId": "game789",
  "winner": 1
}

// Rematch request received
{
  "type": "rematch_received",
  "gameId": "game789",
  "fromUserId": "user456"
}
```

## Game Flow

1. **User Connection**
   - User opens page → WebSocket connects to server
   - Server assigns random name (e.g., "BraveOctopus42")
   - Server sends welcome message with userId and username
   - Server broadcasts updated user list to all clients

2. **Challenge Flow**
   - User A clicks on User B in online list
   - Challenge dialog appears, User A confirms
   - Server sends challenge notification to User B
   - User B can accept or decline
   - If accepted, server creates game session and notifies both players

3. **Gameplay**
   - Game starts with initial board state
   - Players take turns making moves
   - Each move is sent to server, validated, and broadcast to opponent
   - Server maintains authoritative game state
   - Win condition checked after each turn

4. **Rematch**
   - After game ends, "Rematch" button appears
   - Clicking sends rematch request to opponent
   - If opponent accepts, new game session starts
   - If opponent declines or doesn't respond, player returns to lobby

## Backend Components

### User
- `ID`: Unique identifier
- `Username`: Random generated name
- `Connection`: WebSocket connection
- `InGame`: Boolean flag

### Challenge
- `ID`: Unique identifier
- `FromUser`: Challenger user ID
- `ToUser`: Challenged user ID
- `Timestamp`: Creation time

### Game
- `ID`: Unique identifier
- `Player1`: User ID
- `Player2`: User ID
- `Board`: Game state (2D array)
- `CurrentPlayer`: 1 or 2
- `MovesLeft`: Remaining moves in turn
- `GameOver`: Boolean flag
- `Winner`: Player number or 0

## Random Name Generation

Names are generated using the pattern: `[Adjective][Animal][Number]`
- Examples: BraveOctopus42, CleverTiger88, WildPhoenix15
- Ensures uniqueness with number suffix
