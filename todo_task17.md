# Feature Request: Multiplayer Board Highlighting (Ping System)

This feature introduces a non-verbal communication mechanic for multiplayer games, allowing players to briefly highlight specific cells on the board to draw attention to them.

## Functionality:

1.  **User Interaction**:
    *   **Desktop**: Right-clicking on a cell.
    *   **Mobile/Touch**: Long-pressing on a cell (or another intuitive gesture suitable for devices without a mouse).
    *   **Constraint**: The action should not trigger a move or standard selection, just the highlight signal.

2.  **Network Broadcast**:
    *   When triggered, the client sends a message to the server indicating the target cell coordinates.
    *   The server broadcasts this "highlight" event to all other players in the same game session.

3.  **Visual Feedback**:
    *   Upon receiving the event, all clients (including the sender) should display a visual indicator on the target cell.
    *   **Style**: A glow, a flashing border, or a temporary color overlay (e.g., "ping" effect).

4.  **Auto-Expiration**:
    *   The highlight must be temporary.
    *   It should automatically fade out or disappear after a short duration (e.g., 1 second).

## Implementation Notes:

*   **Frontend**:
    *   Add `contextmenu` event listener for right-click handling (prevent default context menu).
    *   Add touch event handling for long-press detection on mobile.
    *   Implement the visual effect in CSS/JS.
*   **Protocol**:
    *   Define a new WebSocket message type (e.g., `highlight_cell`) containing `{row, col}`.
*   **Backend**:
    *   Update `hub.go` / `client.go` to relay this message type to lobby participants without storing it as persistent game state.
