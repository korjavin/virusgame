# Virus Game

This is a web-based implementation of the turn-based strategy game "Virus" (also known as "Война вирусов"). It is built using plain HTML, CSS, and JavaScript.

The game is played on a 10x10 grid. Two players, represented by 'X' and 'O', take turns to expand their territory. Each player has three moves per turn, which can be used to place new pieces, kill opponent's pieces, or place neutral pieces.

## Rules

The game is a turn-based strategy game where two players, X and O, compete to control the board.

Each player has **three moves** per turn. A move can be either to **grow** into an adjacent empty cell or to **attack** an adjacent opponent's cell.

When an opponent's cell is attacked, it becomes a **fortified cell** for the attacker. Fortified cells cannot be re-taken and can be used for further expansion.

For a complete explanation of the rules, please see [DOCS.md](DOCS.md).

## Running the Game

To run the game, you can either:

1.  Open the `index.html` file directly in a web browser.
2.  Run a simple web server in the project directory. For example:

    ```bash
    npx http-server
    ```
