# Project: Virus Game

## Project Overview

This project is a web-based implementation of the turn-based strategy game "Virus" (also known as "Война вирусов"), featuring both single-player (vs AI) and real-time multiplayer modes.

The game supports 2-4 players. Each player, represented by a unique symbol ('X', 'O', '△', '□'), aims to expand their territory and eliminate opponents.

## Architecture

The project follows a full-stack architecture:

### Frontend
*   **HTML/CSS**: `index.html` and `style.css` provide the UI, including the game grid, lobby interface, and control panels.
*   **JavaScript**:
    *   `script.js`: Core game logic (local/AI modes).
    *   `multiplayer.js`: WebSocket client and game state synchronization.
    *   `lobby.js`: UI and logic for the multiplayer lobby system (3-4 players).
    *   `ai.js`: JavaScript-based AI opponent (minimax with alpha-beta pruning).
    *   `translations.js`: Internationalization support.

### Backend (`/backend`)
*   **Language**: Go (Golang).
*   **Core Functionality**:
    *   Serves static files.
    *   Manages WebSocket connections (`hub.go`, `client.go`).
    *   Handles 1v1 challenges and 3-4 player lobbies (`hub.go`).
    *   Authoritative game state validation and synchronization.

### Experimental
*   **WASM**: A WebAssembly version of the AI (`/wasm`) exists but is currently not the primary AI engine.

## Building and Running

### Prerequisites
*   Go 1.21+
*   Node.js (optional, for convenience scripts)

### Running Locally

1.  **Start the Server**:
    ```bash
    ./start-server.sh
    # OR
    cd backend && go run .
    ```
2.  **Play**:
    Open `http://localhost:8080` in your browser.

## Key Features

*   **Modes**:
    *   **Local 1v1**: Play against a friend on the same device.
    *   **vs AI**: Challenge the computer with adjustable difficulty settings.
    *   **Multiplayer**: Online play via WebSockets.
*   **Lobby System**: Create or join lobbies for 3-4 player matches, with bot support.
*   **Customization**: Adjustable board size, AI difficulty parameters.
*   **Localization**: English and German support.

## Development Conventions

*   **State Management**: Game state is mirrored on client and server. The server is the authority in multiplayer.
*   **Communication**: JSON messages over WebSockets.
*   **Styling**: Plain CSS with a focus on responsiveness.