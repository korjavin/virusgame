# Task 2: Unify 1v1 and Multiplayer Modes

## Goal
The ultimate goal is to have a single "Multiplayer" mode that supports both quick 1v1 games and custom lobby-based games (up to 4 players). The distinction between "1v1 Mode" and "Multiplayer Mode" in the UI and backend logic should be removed. Everything should fundamentally be a "Lobby" or "Game Session".

## Motivation
- **Simplify UI:** Remove the confusing toggle between "1v1" and "Multiplayer".
- **Simplify Code:** Maintain a single code path for game initialization, state management, and updates, reducing bugs and maintenance burden.
- **Consistent Experience:** Users should have a unified experience whether playing a quick duel or a custom 4-player match.

## Strategy

### 1. Conceptual Unification (Backend & Frontend)
- **"Everything is a Lobby":** 
    - A "Direct Challenge" (1v1) is simply a shortcut to creating a private, 2-player Lobby with specific settings (12x12 by default, standard rules) and auto-inviting the target user.
    - A "Lobby Game" is just a manually configured Lobby.
- **Unified Game Start Logic:** 
    - Currently, there are two different start signals: `game_start` (1v1) and `multiplayer_game_start` (Lobby).
    - **Action:** Deprecate `game_start` and use `multiplayer_game_start` (or a new unified `start_game` message) for *all* games.

### 2. Frontend Changes (`index.html`, `script.js`, `multiplayer.js`, `lobby.js`)

#### UI Overhaul (`index.html`)
- **Remove Sidebar Toggles:** Delete the "Game Mode" toggle (1v1 vs Multiplayer).
- **Unified Sidebar:**
    - **Header:** "Play Online"
    - **Section A: "Active Lobbies"** (List of public custom lobbies).
        - Button: "Create Custom Game" (opens the lobby creation form).
    - **Section B: "Online Players"** (List of users for quick 1v1).
        - "Challenge" button next to a user now triggers the "Create Private Lobby & Invite" flow internally.
- **Game View:** Ensure the game board and controls look and behave identically for 2-player and 4-player games.

#### Logic Refactoring
- **`multiplayer.js`:**
    - Merge `handleGameStart` and `handleMultiplayerGameStart`.
    - Ensure `MultiplayerClient` tracks `lobbyId` for all games.
    - Unify `handleMoveMade` and state updates.
- **`lobby.js`:**
    - This class should likely become the main "Pre-Game Manager".
    - Handle "Challenge" clicks by sending a `create_lobby` (private) + `invite_player` sequence (or a specific `challenge_user` packet that the server translates into this).

### 3. Backend Changes (Go)
- **Refactor `Client` and `Hub`:**
    - Ensure the "Challenge" mechanism utilizes the `Lobby` struct.
    - When User A challenges User B:
        1. Server creates a hidden/private `Lobby`.
        2. Server adds User A.
        3. Server sends invite/challenge to User B.
        4. If B accepts, B joins the Lobby.
        5. Game starts immediately (skip the "waiting in lobby" UI phase for direct challenges if desired, or show it briefly).
- **Unified Message Types:** 
    - Send standard `lobby_update` messages even for 1v1 challenges so the client state is consistent.

### 4. Step-by-Step Implementation Plan

1.  **Backend Refactor (First):** 
    - Modify the `Challenge` handler to internally create a Lobby.
    - Ensure `StartGame` logic is shared.
2.  **Frontend - Protocol Update:**
    - Update `multiplayer.js` to handle the unified start message.
    - Test that 1v1 challenges still work (even if they technically run inside a lobby structure).
3.  **Frontend - UI Unification:**
    - Modify `index.html` to remove the tabs.
    - Rearrange the sidebar to show Lobbies and Users simultaneously (or nicely grouped).
4.  **Cleanup:**
    - Remove legacy variables (`player1NeutralsUsed`, `isMultiplayerGame` vs `multiplayerMode` distinction if possible).
    - Simplify `script.js` to rely solely on the unified `GameHistory` and `GameState`.

## Success Metrics
- A single "Start Game" event handler in the frontend.
- No "1v1" vs "Multiplayer" toggle in the UI.
- All game features (chat, history, neutrals) work identically in both "Quick 1v1" and "Custom Lobby" games.
