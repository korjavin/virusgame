# Task 9: Game History Recording and Analysis (PGN-like Format)

## Objective
Implement a system to save played games into a local database for future analysis, replay, and AI training. The data structure should be comprehensive, capturing all necessary state transitions, timing, and outcomes, similar to PGN (Portable Game Notation) used in Chess.

## Requirements

### Data to Capture
For each game, the following information must be stored:

1.  **Metadata (Header)**
    *   **Game ID**: Unique identifier.
    *   **Date/Time**: Timestamp of when the game started and ended.
    *   **Board Configuration**:
        *   Dimensions (Rows, Columns).
        *   Initial state (if not empty/standard).
    *   **Players**:
        *   Player 1 (Name, ID, Type: Human/AI/Bot).
        *   Player 2 (Name, ID, Type).
        *   Player 3/4 (if applicable).
    *   **Result**:
        *   Winner (Player ID or Draw).
        *   Termination method (Normal, Resignation, Disconnection, Timeout).
        *   Game duration.

2.  **Move Data (The "PGN" Body)**
    *   Ordered list of moves.
    *   For each move:
        *   **Sequence Number**: Turn number.
        *   **Player**: Who made the move.
        *   **Action Type**: Place, Attack.
        *   **Coordinates**: Row, Column.
        *   **Timestamp**: Precise time when the move was received/processed.
        *   **Time Control info**: Time remaining (if applicable).

3.  **Analysis Data (Optional/Future)**
    *   Board state snapshot at key moments (FEN-like string).
    *   AI evaluation scores (if available during play).

### Storage Technology
*   **Database**: SQLite (embedded, zero-configuration, suitable for local analysis).
*   **Schema**: A simple relational schema or a document-like storage (JSON in a TEXT column) if flexibility is needed.

## Proposed Implementation Plan

### Phase 1: Database Schema & Setup
1.  **Initialize SQLite DB**: Create `games.db` in the `backend/` directory.
2.  **Schema Design**:
    ```sql
    CREATE TABLE games (
        id TEXT PRIMARY KEY,
        started_at DATETIME,
        ended_at DATETIME,
        rows INTEGER,
        cols INTEGER,
        player1_name TEXT,
        player2_name TEXT,
        player3_name TEXT,
        player4_name TEXT,
        result TEXT, -- e.g., "1-0", "0-1", "1/2-1/2"
        termination TEXT, -- "checkmate", "resign", "timeout"
        pgn_content TEXT -- JSON or standard string format of moves
    );
    ```

### Phase 2: Backend Integration (`backend/`)
1.  **Modify `Game` Struct**: Ensure it tracks the list of moves and timestamps.
2.  **Game End Trigger**: In `hub.go`, detect when a game ends (win, disconnect, etc.).
3.  **Persistence Layer**:
    *   Create a `storage.go` module.
    *   Function `SaveGame(game *Game)` that writes the game data to SQLite.
    *   Run this asynchronously to avoid blocking the game server loop.

### Phase 3: PGN-like Format Specification
Define a JSON structure for the `pgn_content` to make it easily parsable by analysis tools.

```json
[
  {
    "turn": 1,
    "player": 1,
    "moves": [
      { "type": "place", "row": 5, "col": 5, "ts": 1678888880 },
      { "type": "place", "row": 5, "col": 6, "ts": 1678888881 },
      { "type": "attack", "row": 4, "col": 5, "ts": 1678888882 }
    ]
  },
  ...
]
```

### Phase 4: API for Retrieval (Optional for now)
*   Endpoint to list past games: `GET /api/games`
*   Endpoint to get specific game PGN: `GET /api/games/{id}`

## Success Criteria
*   Every completed multiplayer game is saved to `backend/games.db`.
*   Saved data includes accurate move logs and timestamps.
*   The system handles concurrent game saves without errors.
*   Basic CLI tool or script to dump/read the saved games.

## Future Considerations
*   **Replay Feature**: Frontend ability to load a game from the DB and replay it move-by-move.
*   **Statistics**: Win rates, average game length, opening moves analysis.
*   **AI Training**: Export data for training machine learning models.
