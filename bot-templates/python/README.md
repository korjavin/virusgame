# Python Bot Template

A starter kit for building Python bots for the game.

## Prerequisites

- Python 3.8+
- `websockets` library

## Setup

1.  Install dependencies:
    ```bash
    pip install -r requirements.txt
    ```

2.  Configure environment (optional):
    ```bash
    export BACKEND_URL="ws://localhost:8080/ws"
    ```

## Running the Bot

```bash
python bot.py
```

## Structure

-   `bot.py`: Main entry point. Handles WebSocket connection and protocol.
-   `game.py`: Game logic helper. Implements board state and move validation.

## Implementing Your AI

1.  Open `bot.py`.
2.  Locate the `find_move` method.
3.  Replace the random move logic with your own strategy.
