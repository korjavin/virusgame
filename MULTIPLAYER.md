# Multiplayer Architecture

## Overview
The multiplayer system uses WebSockets to enable real-time gameplay between users. It supports two distinct modes of play:
1.  **Direct Challenge (1v1)**: Quick duel requests between two online users.
2.  **Lobby System (2-4 Players)**: Hosted rooms where multiple players can join, with optional AI bots to fill empty slots.

## Architecture

### Backend (Go + WebSockets)
-   **WebSocket Server**: Handles persistent client connections (`client.go`).
-   **Hub**: The central message router (`hub.go`). Manages:
    -   **User List**: Tracks online users and their status (In Lobby, In Game).
    -   **Challenges**: Handles 1v1 requests.
    -   **Lobbies**: Manages creation, joining, and state of game rooms (`Lobby` struct).
    -   **Game Sessions**: Authoritative game state for active matches.
-   **Move Synchronization**: Validates and broadcasts moves to all players in a session.

### Bot Service (Go)
-   **Bot Hoster**: A separate service (`backend/cmd/bot-hoster`) that manages a pool of AI bots.
-   **Operation**: Bots connect to the backend as regular WebSocket clients. When a host requests a bot, the backend broadcasts a `bot_wanted` signal, and an idle bot from the pool joins the lobby.

### Frontend (JavaScript)
-   **`multiplayer.js`**: Core WebSocket client. Handles connection, message dispatching, and game state updates.
-   **`lobby.js`**: Manages the UI and logic for the lobby system (creating rooms, adding bots, starting games).
-   **`script.js`**: renders the game board and handles user input, delegating to `multiplayer.js` when in online mode.

## Lobby System
The lobby system allows for flexible 2-4 player games.
-   **Host**: The user who creates the lobby controls settings (board size) and game start.
-   **Slots**: Lobbies have 4 slots.
-   **Bots**: The host can fill empty slots with AI bots.
-   **Ready State**: The game begins when the host initiates it (usually requiring >1 player).

## Message Protocol

Communication uses JSON payloads over WebSockets.

### Key Message Types

#### Connection & Status
-   `connect`: Initial handshake.
-   `welcome`: Server assigns identity (User ID, Name).
-   `users_update`: Broadcast of online player list.

#### Lobbies
-   `create_lobby`: Client requests a new room.
-   `join_lobby`: Client requests to join a specific room.
-   `leave_lobby`: Client exits current room.
-   `add_bot`: Host requests a bot. Server broadcasts `bot_wanted`.
-   `bot_wanted`: Server signal to bot-hoster service.
-   `remove_bot`: Host removes a bot from the lobby.
-   `start_multiplayer_game`: Host triggers transition from Lobby to Game.

#### Gameplay
-   `move`: Player (Human or Bot) sends `{row, col}`.
-   `move_made`: Server broadcasts confirmed move to all players.
-   `game_start`: Transition to game view, provides initial board and player assignments.
-   `game_end`: Announcements of winner/elimination.

#### Direct Challenge (Legacy/Quick)
-   `challenge`: Target a specific user.
-   `accept_challenge` / `decline_challenge`: Response.

## Game Flow (Lobby)

1.  **Creation**: User A clicks "Create Lobby". Server creates a `Lobby` object and adds User A as Player 1 (Host).
2.  **Joining**: User B sees the lobby in the list and joins. Server adds User B as Player 2.
3.  **Bot Setup**: Host clicks "Add Bot".
    -   Server broadcasts `bot_wanted` to all clients (ignored by humans, processed by bot-hoster).
    -   An available Bot (e.g., "AI-Bot1") sends `join_lobby`.
    -   Server adds Bot as Player 3.
4.  **Start**: Host clicks "Start".
    -   Server instantiates a `Game` object.
    -   Server sends `game_start` to all players (User A, User B, Bot).
5.  **Play**:
    -   Turns rotate P1 -> P2 -> P3 -> ...
    -   Server validates moves.
    -   When it's the Bot's turn, the Bot calculates the move locally and sends a standard `move` message.

## Random Name Generation

Names are generated server-side using `[Adjective][Animal][Number]` (e.g., "WildPhoenix15") to ensure friendly, unique identifiers.
