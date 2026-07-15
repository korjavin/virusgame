package main

import (
	"os"
	"testing"
)

func TestStorage_SaveGame(t *testing.T) {
	// Initialize DB with temp file
	dbPath := "test_games_storage.db"
	InitDB(dbPath)
	defer func() {
		if db != nil {
			db.Close()
		}
		os.Remove(dbPath)
	}()

	// Create test game
	game := &Game{
		ID:    "test-save-game-sync",
		Rows:  10,
		Cols:  10,
		Winner: 1,
		IsMultiplayer: false,
		MoveHistory: []MoveAction{
			{Player: 1, Type: "place", Row: 0, Col: 0, TurnNumber: 1},
		},
		Player1: &User{Username: "P1"},
		Player2: &User{Username: "P2"},
	}

	if !PersistGameOnce(game, "normal") {
		t.Fatal("save failed")
	}

	// Verify data is in DB
	var count int
	err := db.QueryRow("SELECT COUNT(*) FROM games WHERE id = ?", game.ID).Scan(&count)
	if err != nil {
		t.Fatalf("Failed to query DB: %v", err)
	}
	if count != 1 {
		t.Errorf("Expected 1 game record, got %d", count)
	}
}
