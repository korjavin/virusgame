# Virus Game Rules

This document outlines the rules for the Virus Game.

## Objective

The objective of the game is to be the last player standing by eliminating your opponents' pieces or rendering them unable to move.

## Players and Setup

*   **Players**: 2 to 4 players can participate.
*   **Symbols**:
    *   Player 1: **X**
    *   Player 2: **O**
    *   Player 3: **△** (Triangle)
    *   Player 4: **□** (Square)
*   **Grid**: The game is played on a customizable grid (default 10x10).
*   **Starting Position**: Each player starts with a single **base cell** in a corner of the board.

## Gameplay

Players take turns in a fixed order (P1 -> P2 -> P3 -> P4). Each player has **three moves** per turn.

A move consists of one of the following actions:

1.  **Grow:** Place a new piece in an empty cell adjacent to one of your existing pieces. The piece you are expanding from **must be part of a chain connected to your initial base cell**.

2.  **Attack:** Attack an opponent's piece in a cell adjacent to one of your existing pieces. The piece you are attacking from **must be part of a chain connected to your initial base cell**. When you attack an opponent's cell, it is converted into a **fortified cell** of your color.

### Base Connection Rule
Crucially, you can only Grow or Attack from a cell that is "live" — meaning it is connected via a chain of your own pieces back to your original base. If a group of your cells gets cut off from your base, you cannot use them to expand until you reconnect them.

## Fortified Cells

-   When you successfully attack an opponent's cell, it becomes a fortified cell.
-   Fortified cells are permanently owned by the attacking player and **cannot be re-taken** for the rest of the game.
-   Fortified cells are visually distinct (often a solid background).
-   You can use your fortified cells to grow and attack from, just like your normal pieces.

## Special Ability: Neutrals

Once per game, each player has the ability to place **two neutral fields** on their own, non-fortified cells. This action replaces your entire turn (all three moves).

-   **How to use**: Click the "Put Neutrals" button and select two of your own cells.
-   **Effect**: These cells become permanent neutral blocks that no one can enter or attack. This is a strategic defensive move to block an opponent's advance.
-   **Requirement**: You must have at least two non-fortified cells to use this ability.

## Winning the Game

A player is eliminated if they lose all their pieces or (in some variations) if their base is captured or isolated such that they cannot make any moves. The last player remaining on the board wins.
