# Feature Request: Game History Navigation

This feature will allow users to navigate through the history of a game, both in single-player and multiplayer modes. The primary use case is to review past moves, especially if a player was distracted and missed an opponent's move.

## Functionality:

1.  **Navigation Buttons**: Add "Previous Move" and "Next Move" buttons (or similar navigation controls) to the game interface. The navigation bar containing these buttons should be compact and mobile-friendly.
2.  **Local-only**: This functionality should be entirely client-side. It will not interact with the server or send any data. It will replay the stored game states locally in the browser.
3.  **Visual Indication**: When the user is viewing a historical game state (i.e., not the current state), there must be a clear visual indicator on the UI to signify this. This could be a banner, a change in background, or highlighting.
4.  **No Moves in Historical State**: While viewing a historical state, the user should not be able to make any new moves. The game input mechanism should be disabled.
5.  **Return to Current State**: A way to quickly return to the current live game state should be provided (e.g., a "Return to Current" button or by navigating forward until the end of history is reached). Once back at the current state, moves should be re-enabled.

## Implementation Notes:

*   The game state history will need to be stored locally (e.g., in an array of game states).
*   The UI will need to be re-rendered based on the historical state being viewed.
*   Consider how this interacts with different game modes (AI, local, multiplayer) – the functionality should be consistent across all.