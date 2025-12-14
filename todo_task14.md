# Feature Request: Lobby Quick Chat Buttons

This feature introduces a primitive chat system within the multiplayer lobby, utilizing pre-defined buttons to send quick messages to all users currently in the lobby. This allows for basic interaction and expression without a full-fledged text chat.

## Functionality:

1.  **Pre-defined Message Buttons**: Add a set of buttons to the lobby interface. Each button, when clicked, will send a specific pre-defined message.
2.  **Broadcast to Lobby**: Messages sent via these buttons should be broadcast to all other users present in the same lobby.
3.  **Suggested Messages**:
    *   "Let's start the game."
    *   "Move faster."
    *   "I will win against all of you."
    *   "Oh, how scary."
4.  **Rate Limiting**: Implement a rate limit to prevent spamming. A user should not be able to send more than a certain number of messages (e.g., 3 messages) within a given time frame (e.g., 10-20 seconds).
5.  **Display Messages**: Received messages should be displayed in a visible area within the lobby UI for all participants to see.

## Implementation Notes:

*   **Frontend**:
    *   Add buttons to `lobby.js` and `index.html`.
    *   Implement client-side logic to send messages via WebSocket.
    *   Implement client-side display of received messages.
    *   Implement client-side rate limit enforcement (and potentially provide visual feedback if rate limited).
*   **Backend**:
    *   Extend WebSocket handling in `hub.go` to process "quick chat" messages.
    *   Implement server-side rate limiting for robustness.
    *   Broadcast the message to all clients in the sender's lobby.
    *   Consider the message format for these quick chats (e.g., a simple JSON object with sender ID and message ID/text).