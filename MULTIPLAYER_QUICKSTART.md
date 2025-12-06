# Multiplayer Quickstart Guide

## What's New

Your Virus Game supports robust real-time multiplayer with:
-   **2-4 Player Support**: Play 1v1 or chaotic 4-player free-for-alls.
-   **Lobby System**: Create rooms, invite friends, or play with bots.
-   **AI Bots**: Fill empty slots in your multiplayer games with computer opponents.
-   **Live User List**: See who's online and challenge them directly.

## Quick Start

### 1. Start the Server

```bash
./start-server.sh
# OR
cd backend && go run .
```

### 2. Join the Action

Open `http://localhost:8080` in your browser. Open multiple tabs to simulate multiple players.

## Ways to Play

### Option A: The Lobby (Recommended for >2 players)
1.  **Create**: Click the **"Multiplayer"** tab and then **"Create Lobby"**.
2.  **Wait/Join**: Other players will see your lobby in the list and can click to join.
3.  **Add Bots**: As the host, click **"Add Bot"** to fill empty slots with AI.
4.  **Start**: Once you have at least 2 participants (players or bots), click **"Start Game"**.

### Option B: Direct Challenge (Best for 1v1)
1.  **Find**: Look at the **"Online Players"** list in the sidebar.
2.  **Challenge**: Click **"Challenge"** next to a player's name.
3.  **Accept**: The other player accepts the popup request.
4.  **Play**: The game starts immediately.

## How to Play
-   **Turn-Based**: Wait for your turn (indicated by the status bar).
-   **Moves**: You get 3 moves per turn.
-   **Sync**: All moves are synchronized instantly across all screens.
-   **Winning**: Eliminate all opponents to win!

## Troubleshooting
-   **"Disconnected"**: Ensure the Go server is running (`./start-server.sh`).
-   **Lag**: The game uses WebSockets. A stable network connection is required.