package main

const (
    CellFlagNormal    byte = 0x00
    CellFlagBase      byte = 0x10
    CellFlagFortified byte = 0x20
    CellFlagKilled    byte = 0x30

    FlagMask   byte = 0x30
    PlayerMask byte = 0x0F
)

type CellValue byte

func NewCell(player int, flag byte) CellValue {
    return CellValue(flag | byte(player))
}

func (c CellValue) Player() int {
    return int(byte(c) & PlayerMask)
}

func (c CellValue) Flag() byte {
    return byte(c) & FlagMask
}

func (c CellValue) IsBase() bool {
    return c.Flag() == CellFlagBase
}

func (c CellValue) IsFortified() bool {
    return c.Flag() == CellFlagFortified
}

func (c CellValue) IsKilled() bool {
    return c.Flag() == CellFlagKilled
}

func (c CellValue) CanBeAttacked() bool {
    return c.Flag() == CellFlagNormal
}

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
    if cell == 0 {
        return b.isAdjacentToMyTerritory(row, col)
    }

    // If not empty, we can attack if it's not fortified/base and not ours
    if cell.Player() != b.YourPlayer && cell.CanBeAttacked() {
        return b.isAdjacentToMyTerritory(row, col)
    }

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
                if cell != 0 && cell.Player() == b.YourPlayer {
                    return true
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
