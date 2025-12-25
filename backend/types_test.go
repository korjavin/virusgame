package main

import (
	"encoding/json"
	"testing"
)

func TestCellValue_Methods(t *testing.T) {
	// Test NewCell
	c := NewCell(1, CellFlagBase)
	if c.Player() != 1 {
		t.Errorf("Expected Player 1, got %d", c.Player())
	}
	if !c.IsBase() {
		t.Error("Expected IsBase to be true")
	}
	if c.IsFortified() {
		t.Error("Expected IsFortified to be false")
	}
	if c.IsKilled() {
		t.Error("Expected IsKilled to be false")
	}
	if c.CanBeAttacked() {
		t.Error("Expected Base to not be attackable")
	}

	// Test Fortified
	c = NewCell(2, CellFlagFortified)
	if c.Player() != 2 {
		t.Errorf("Expected Player 2, got %d", c.Player())
	}
	if !c.IsFortified() {
		t.Error("Expected IsFortified to be true")
	}
	if c.CanBeAttacked() {
		t.Error("Expected Fortified to not be attackable")
	}

	// Test Killed (Neutral)
	c = NewCell(0, CellFlagKilled)
	if c.Player() != 0 {
		t.Errorf("Expected Player 0, got %d", c.Player())
	}
	if !c.IsKilled() {
		t.Error("Expected IsKilled to be true")
	}
	if c.CanBeAttacked() {
		t.Error("Expected Killed to not be attackable")
	}

	// Test Normal
	c = NewCell(1, CellFlagNormal)
	if !c.CanBeAttacked() {
		t.Error("Expected Normal cell to be attackable")
	}
	if c.IsBase() || c.IsFortified() || c.IsKilled() {
		t.Error("Expected Normal cell to have no flags")
	}
}

func TestBoard_MarshalJSON(t *testing.T) {
	rows, cols := 2, 2
	board := make(Board, rows)
	for i := range board {
		board[i] = make([]CellValue, cols)
	}
	board[0][0] = NewCell(1, CellFlagBase)
	board[1][1] = NewCell(2, CellFlagNormal)

	data, err := json.Marshal(board)
	if err != nil {
		t.Fatalf("Marshal failed: %v", err)
	}

	// Expected: [[17,0],[0,2]]  (17 = 16 (base) | 1 (p1), 2 = 0 | 2)
	expected := "[[17,0],[0,2]]"
	if string(data) != expected {
		t.Errorf("Expected JSON %s, got %s", expected, string(data))
	}
}
