package main

// AI strategy implementation
// Customize this to create your unique bot!

// findValidMove returns the first valid move found
func (b *Bot) findValidMove() (int, int) {
    if len(b.Board) == 0 {
        return 0, 0
    }
    for row := 0; row < len(b.Board); row++ {
        for col := 0; col < len(b.Board[0]); col++ {
            if b.isValidMove(row, col) {
                return row, col
            }
        }
    }
    return 0, 0
}

// isValidMove checks if a move is legal
func (b *Bot) isValidMove(row, col int) bool {
    // Cell must be empty or opponent's (not fortified)
    cell := b.Board[row][col]

    // Empty is always valid if adjacent
    if cell == nil {
        return b.isAdjacentToMyTerritory(row, col)
    }

    // TODO: Add more validation logic
    return false
}

// isAdjacentToMyTerritory checks if cell touches my territory
func (b *Bot) isAdjacentToMyTerritory(row, col int) bool {
    // Check 8 neighbors
    for i := -1; i <= 1; i++ {
        for j := -1; j <= 1; j++ {
            if i == 0 && j == 0 {
                continue
            }

            nr, nc := row+i, col+j
            if nr >= 0 && nr < len(b.Board) && nc >= 0 && nc < len(b.Board[0]) {
                cell := b.Board[nr][nc]
                if cell != nil {
                     // In the main game code, the board stores strings for fortified cells (e.g., "1-fortified")
                     // or integers for simple ownership (e.g., 1).
                     // However, the JSON unmarshals to interface{}.
                     // For the template, we'll keep it simple and assume we just check against YourPlayer.
                     // But we must handle the type assertion carefully or just check if it's ours.
                     // Here we'll do a simple check.
                     switch v := cell.(type) {
                     case int:
                         if v == b.YourPlayer {
                             return true
                         }
                     case float64: // JSON numbers are float64
                         if int(v) == b.YourPlayer {
                             return true
                         }
                     }
                }
            }
        }
    }
    return false
}

// TODO: Implement more sophisticated AI
// Ideas:
// - Minimax algorithm
// - Alpha-beta pruning
// - Board evaluation function
// - Opening book
// - Endgame tables
