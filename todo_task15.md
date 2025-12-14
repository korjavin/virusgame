# Bug Fix: "New Local Game" State Consistency and Cleanup

There is an issue with the "New Local Game" button where the game state becomes inconsistent if clicked while currently in a multiplayer game or an active local game. The board might not clear, the user might not properly exit the previous game, and artifacts may remain.

## Problem:
*   Clicking "New Local Game" does not deterministically clean up the previous game state.
*   Transitions between multiplayer and local modes are buggy.
*   Artifacts from previous games (history, board state) persist.

## Requirements:

1.  **Confirmation Dialog**:
    *   If the user clicks "New Local Game" while an active game (local or multiplayer) is in progress, a confirmation dialog should appear (e.g., "Are you sure you want to start a new local game? Your current game will be forfeited/left.").

2.  **Auto-Resign / Leave**:
    *   If confirmed, the system must automatically handle the "Resign" or "Leave Game" action for the current session.
    *   If in multiplayer, it should send the leave/resign signal to the server.

3.  **Complete State Reset**:
    *   The game board must be completely cleared.
    *   All game state variables (scores, moves, history, etc.) must be reset to their initial default values.
    *   No artifacts from the previous game should remain.

4.  **History View Cleanup**:
    *   If the user is currently in "History View" (reviewing past moves), this mode must be cancelled. The interface should return to the live/setup state.

5.  **"Leave Game" Consistency**:
    *   Ensure that the existing "Leave Game" functionality also performs this rigorous cleanup to prevent state pollution when returning to the lobby or main menu.

## Implementation Notes:
*   Review `initGame` and `leaveGame` logic in `script.js` and `multiplayer.js`.
*   Ensure a strict order of operations: Confirm -> Network Leave/Resign -> Local State Reset -> New Game Initialization.
