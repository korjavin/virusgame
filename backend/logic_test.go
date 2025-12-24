package main

import (
	"testing"
)

// TestLogicIsValidMove verifies the validation logic
func TestLogicIsValidMove(t *testing.T) {
	h := newHub()

	rows, cols := 5, 5
	board := make(Board, rows)
	for i := range board {
		board[i] = make([]CellValue, cols)
	}

	// Setup a game
	game := &Game{
		ID:    "test-game",
		Board: board,
		Rows:  rows,
		Cols:  cols,
		Player1Base: CellPos{0, 0},
		Player2Base: CellPos{rows-1, cols-1},
	}

	// Setup Base for Player 1
	game.Board[0][0] = NewCell(1, CellFlagBase)
	// Setup a connected piece for Player 1 at (0,1)
	game.Board[0][1] = NewCell(1, CellFlagNormal)

	// Test 1: Valid move (expanding to empty cell (1,1))
	// (1,1) is adjacent to (0,1) which is connected to base (0,0)
	if !h.isValidMove(game, 1, 1, 1) {
		t.Error("Expected (1,1) to be a valid move for Player 1")
	}

	// Test 2: Invalid move (too far)
	if h.isValidMove(game, 3, 3, 1) {
		t.Error("Expected (3,3) to be invalid (too far)")
	}

	// Test 3: Attacking Neutral Cell
	// Place a neutral cell at (0,2). (0,1) is adjacent to (0,2).
	game.Board[0][2] = NewCell(0, CellFlagKilled)

	// Player 1 tries to attack (0,2)
	if h.isValidMove(game, 0, 2, 1) {
		t.Error("Expected move to Neutral cell (0,2) to be INVALID")
	}

	// Test 4: Attacking Own Base
	if h.isValidMove(game, 0, 0, 1) {
		t.Error("Expected move to own Base to be INVALID")
	}

	// Test 5: Out of bounds
	if h.isValidMove(game, -1, 0, 1) {
		t.Error("Expected negative row to be INVALID")
	}
	if h.isValidMove(game, 0, cols, 1) {
		t.Error("Expected col out of bounds to be INVALID")
	}
}

// TestLogicIsConnectedToBase verifies BFS
func TestLogicIsConnectedToBase(t *testing.T) {
	h := newHub()
	rows, cols := 5, 5 // Increased size to allow disconnection
	board := make(Board, rows)
	for i := range board {
		board[i] = make([]CellValue, cols)
	}

	game := &Game{
		Board: board,
		Rows: rows,
		Cols: cols,
		Player1Base: CellPos{0, 0},
	}

	game.Board[0][0] = NewCell(1, CellFlagBase)
	game.Board[0][1] = NewCell(1, CellFlagNormal)

	// Connected chain: (0,0)->(0,1)->(0,2)
	game.Board[0][2] = NewCell(1, CellFlagNormal)

	if !h.isConnectedToBase(game, 0, 2, 1) {
		t.Error("Expected (0,2) to be connected to base (0,0)")
	}

	// Disconnected piece at (4,4)
	game.Board[4][4] = NewCell(1, CellFlagNormal)
	if h.isConnectedToBase(game, 4, 4, 1) {
		t.Error("Expected (4,4) to be DISCONNECTED")
	}
}

func TestLogicCheckWinCondition(t *testing.T) {
	h := newHub()
	rows, cols := 3, 3
	board := make(Board, rows)
	for i := range board {
		board[i] = make([]CellValue, cols)
	}

	// Setup dummy clients so sendToUser doesn't panic if called
	// Although checkWinCondition calls sendToUser, which calls sendToClient.
	// If Client is nil, sendToUser checks for nil client.
	// We need dummy Users.
	p1 := &User{ID: "p1", Username: "Player1"}
	p2 := &User{ID: "p2", Username: "Player2"}

	game := &Game{
		ID:      "test-win",
		Board:   board,
		Rows:    rows,
		Cols:    cols,
		Player1: p1,
		Player2: p2,
		Player1Base: CellPos{0, 0},
		Player2Base: CellPos{2, 2},
		MovesLeft: 1,
	}

	// Base cases
	game.Board[0][0] = NewCell(1, CellFlagBase)
	game.Board[2][2] = NewCell(2, CellFlagBase)

	// No winner yet
	h.checkWinCondition(game)
	if game.GameOver {
		t.Error("Game should not be over")
	}

	// Player 1 captures Player 2's base
	// (Note: capturing base means the cell at base position belongs to Player 1)
	game.Board[2][2] = NewCell(1, CellFlagNormal)

	h.checkWinCondition(game)

	if !game.GameOver || game.Winner != 1 {
		t.Errorf("Player 1 should win by base capture. GameOver=%v, Winner=%d", game.GameOver, game.Winner)
	}

	// Reset
	game.GameOver = false
	game.Winner = 0
	game.Board[2][2] = NewCell(2, CellFlagBase)

	// Remove all Player 1 pieces
	game.Board[0][0] = 0
	h.checkWinCondition(game)

	if !game.GameOver || game.Winner != 2 {
		t.Errorf("Player 2 should win (Player 1 eliminated). GameOver=%v, Winner=%d", game.GameOver, game.Winner)
	}
}

func TestLogicCanMakeAnyMove(t *testing.T) {
	h := newHub()
	// Create a small board where P1 has no valid moves
	rows, cols := 3, 3
	board := make(Board, rows)
	for i := range board {
		board[i] = make([]CellValue, cols)
	}
	game := &Game{
		Board: board,
		Rows: rows,
		Cols: cols,
		Player1Base: CellPos{0, 0},
		Player2Base: CellPos{2, 2},
	}
	game.Board[0][0] = NewCell(1, CellFlagBase)

	// Block P1 by surrounding base with P1 pieces (if own pieces block expansion? No, own pieces are connected)
	// Block P1 by surrounding base with Neutral pieces
	game.Board[0][1] = NewCell(0, CellFlagKilled)
	game.Board[1][0] = NewCell(0, CellFlagKilled)
	game.Board[1][1] = NewCell(0, CellFlagKilled) // Diagonals usually don't matter but let's be safe

	if h.canMakeAnyMove(game, 1) {
		t.Error("Player 1 should have NO valid moves")
	}

	// Open a spot
	game.Board[0][1] = NewCell(0, CellFlagNormal) // Empty
	if !h.canMakeAnyMove(game, 1) {
		t.Error("Player 1 should have a valid move now")
	}
}
