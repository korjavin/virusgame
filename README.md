# Virus Game

This is a web-based implementation of the turn-based strategy game "Virus" (also known as "Война вирусов"). It is built using plain HTML, CSS, and JavaScript.

The game is played on a 10x10 grid. Two players, represented by 'X' and 'O', take turns to expand their territory. Each player has three moves per turn, which can be used to place new pieces, kill opponent's pieces, or place neutral pieces.

## How to Play

1.  Open the `index.html` file in a web browser.
2.  Player 1 starts with three 'X' pieces in the top-left corner, and Player 2 starts with three 'O' pieces in the bottom-right corner.
3.  Players take turns making three moves. A move can be one of the following:
    *   **Place:** Place a new piece adjacent to one of your existing pieces.
    *   **Kill:** Kill an opponent's piece that is adjacent to one of your pieces.
    *   **Neutral:** Place a neutral piece on any empty cell.
4.  The goal of the game is to eliminate all of the opponent's pieces.

## Running the Game

To run the game, you can either:

1.  Open the `index.html` file directly in a web browser.
2.  Run a simple web server in the project directory. For example:

    ```bash
    npx http-server
    ```
