# JavaScript Bot Template

A starter kit for building JavaScript bots for the game using Node.js.

## Prerequisites

- Node.js 14+

## Setup

1.  Install dependencies:
    ```bash
    npm install
    ```

2.  Configure environment (optional):
    ```bash
    export BACKEND_URL="ws://localhost:8080/ws"
    ```

## Running the Bot

```bash
npm start
```

## Structure

-   `bot.js`: Main entry point. Handles WebSocket connection and protocol.
-   `game.js`: Game logic helper. Implements board state and move validation.

## Implementing Your AI

1.  Open `bot.js`.
2.  Locate the `findMove` method.
3.  Replace the random move logic with your own strategy.
