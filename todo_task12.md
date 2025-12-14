# Feature Request: Challenge Management Improvements

This feature request focuses on improving the challenge system by adding auto-decline functionality and preventing duplicate challenges.

## Functionality:

1.  **Auto-Decline Challenges**:
    *   Challenges sent to other users should automatically expire and be declined if not accepted or rejected within a specific time limit (e.g., 60 seconds).
    *   This prevents stale challenges from hanging indefinitely.
    *   Both the sender and the receiver should be notified (or the UI updated) when a challenge expires.

2.  **Prevent Duplicate Challenges**:
    *   A user should not be able to send a new challenge to a specific user if there is already an active (pending) challenge sent to that same user.
    *   The UI should reflect this restriction (e.g., disable the "Challenge" button for that user or show an error message).

## Implementation Notes:

*   **Server-side**:
    *   Implement a timer or timestamp check for challenges.
    *   Add logic to check for existing pending challenges between the sender and receiver before creating a new one.
    *   Clean up expired challenges periodically or on access.
*   **Client-side**:
    *   Update the UI to handle challenge expiration events.
    *   Prevent the user from sending multiple challenges to the same person.
