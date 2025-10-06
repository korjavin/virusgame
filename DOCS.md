# Documentation

## Game Rules

The game is played on a 10x10 grid. There are two players, Player 1 (X) and Player 2 (O).

### Initial Setup

Player 1 starts with three pieces at the top-left corner of the board:
- (0, 0), (0, 1), (1, 0)

Player 2 starts with three pieces at the bottom-right corner of the board:
- (9, 9), (9, 8), (8, 9)

### Turns

Players take turns making three moves. A player can choose to pass their turn at any time.

### Moves

A move can be one of the following:

*   **Place:** Place a new piece on an empty cell. The new piece must be adjacent (horizontally, vertically, or diagonally) to one of the player's existing pieces.
*   **Kill:** Remove an opponent's piece from the board. The opponent's piece must be adjacent to one of the player's existing pieces. A player can also "chain-kill" through a line of already killed enemy pieces.
*   **Neutral:** Place a neutral piece on any empty cell. Neutral pieces cannot be killed or moved.

### Winning the Game

A player wins the game when the other player has no more pieces on the board.

## How to Play

1.  Open the `index.html` file in a web browser.
2.  The game will start automatically.
3.  The status display at the top of the page will show whose turn it is and how many moves are left.
4.  To make a move, click on the desired cell on the board.
5.  To switch between "Place" and "Kill" modes, use the "Switch to Kill Mode" / "Switch to Place Mode" button.
6.  To place a neutral piece, click the "Place Neutral" button and then click on an empty cell.
7.  To pass your turn, click the "Pass Turn" button.
