# Feature Request: Visual Marker for Current Player's Turn

This feature aims to provide a clear and attention-grabbing visual indicator to the player when it is their turn to make a move. This is especially helpful in multiplayer or when playing against AI, to quickly signal whose turn it is.

## Functionality:

1.  **Prominent Visual Signal**: When it is a player's turn, a clear visual marker should be displayed. This could involve:
    *   An element on the screen blinking or flashing.
    *   A distinct animation or transition effect.
    *   A glow or highlight around relevant UI elements (e.g., the status display, current player's score/icon).
    *   A temporary overlay or notification.
2.  **Attention-Grabbing**: The visual signal should be noticeable but not overly intrusive, ensuring the player can easily identify that it's their turn without disrupting the game experience.
3.  **Context-Aware**: The marker should only appear when it's genuinely the player's turn and disappear or change when the turn passes to another player or the AI.

## Implementation Notes:

*   Consider using CSS animations or JavaScript-driven class toggling for visual effects.
*   The implementation should integrate smoothly with existing UI updates and game state changes.
*   Ensure the visual marker is responsive and works well across different screen sizes.