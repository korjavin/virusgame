# Virus Game

This is a web-based implementation of the turn-based strategy game "Virus" (also known as "Война вирусов"). It is built using plain HTML, CSS, and JavaScript.

The game is played on a 10x10 grid. Two players, represented by 'X' and 'O', take turns to expand their territory. Each player has three moves per turn, which can be used to place new pieces, kill opponent's pieces, or place neutral pieces.

## Rules

Each player starts with a single base cell in their corner of the board. The goal is to eliminate the opponent by capturing all their cells.

Players take turns making **three moves**. A move can be to **grow** into an adjacent empty cell or to **attack** an adjacent opponent's cell. 

Crucially, any expansion (grow or attack) must originate from a chain of cells that is connected to your initial **base cell**.

When an opponent's cell is attacked, it becomes a **fortified cell** for the attacker. Fortified cells cannot be re-taken.

Once per game, each player can choose to place two **neutral fields** on their own territory instead of making their moves. This can be used to create defensive barriers.

For a complete explanation of the rules, please see [DOCS.md](DOCS.md).

## Running the Game

To run the game, you can either:

1.  Open the `index.html` file directly in a web browser.
2.  Run a simple web server in the project directory. For example:

    ```bash
    npx http-server
    ```
