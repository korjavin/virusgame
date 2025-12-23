package main

import (
	"testing"
)

// TestNewCell verifies that NewCell creates cells with correct flags and player IDs
func TestNewCell(t *testing.T) {
	// Test Normal Cell
	c1 := NewCell(1, CellFlagNormal)
	if c1.Player() != 1 {
		t.Errorf("Expected player 1, got %d", c1.Player())
	}
	if c1.Flag() != CellFlagNormal {
		t.Errorf("Expected normal flag, got %x", c1.Flag())
	}
	if !c1.CanBeAttacked() {
		t.Error("Normal cell should be attackable")
	}

	// Test Neutral/Killed Cell
	cNeutral := NewCell(0, CellFlagKilled)
	if cNeutral.Player() != 0 {
		t.Errorf("Expected player 0 for neutral, got %d", cNeutral.Player())
	}
	if !cNeutral.IsKilled() {
		t.Error("Expected IsKilled() to be true")
	}
	if cNeutral.CanBeAttacked() {
		t.Error("Neutral cell should NOT be attackable")
	}
}

// TestIsValidMove verifies the validation logic
func TestIsValidMove(t *testing.T) {
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
	} else {
		t.Log("Correctly rejected move to Neutral cell")
	}

	// Test 4: Attacking Own Base
	if h.isValidMove(game, 0, 0, 1) {
		t.Error("Expected move to own Base to be INVALID")
	}
}

// TestIsConnectedToBase verifies BFS
func TestIsConnectedToBase(t *testing.T) {
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
