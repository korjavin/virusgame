# Feature Request: Bot Templates (Python and JavaScript)

This feature aims to provide readily usable bot templates for developers, specifically in Python and JavaScript. These templates will serve as a starting point for creating new bots, handling the communication protocol with the game server, and performing basic move validation. The actual game-playing logic will be a simple stub (e.g., making random valid moves).

## Functionality:

1.  **Protocol Implementation**: Each template must include the necessary code to communicate with the game server, understanding the game state messages and sending move commands according to the established protocol. This involves handling WebSocket connections, message parsing, and serialization.
2.  **Stub Game Logic**: The core AI logic within the templates should be a minimal, functional stub. For instance, it could implement a random valid move generator. This ensures the bot is operational out-of-the-box.
3.  **Valid Move Validation**: Crucially, the bot's move generation (even the random stub) must include client-side validation to ensure that any proposed move is legal according to the game rules. This prevents the bot from sending invalid moves to the server and helps developers understand the validation process.
4.  **Language-Specific Templates**:
    *   **Python Template**: A Python script (`.py`) demonstrating the above functionalities.
    *   **JavaScript Template**: A Node.js or browser-compatible JavaScript file (`.js`) demonstrating the above functionalities.

## Implementation Notes:

*   Refer to the existing protocol documentation (if any) and `backend/client.go` for Go bot implementation details.
*   The templates should be well-commented and easy to understand for new bot developers.
*   The validation logic in the templates should mirror the `isValidMove` logic in `script.js` or `backend/hub.go` to ensure consistency.
*   The templates should be runnable with minimal setup (e.g., `python bot.py` or `node bot.js`).
