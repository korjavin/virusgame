package main

import (
	"database/sql"
	"os"
	"path/filepath"
	"testing"

	_ "modernc.org/sqlite"
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
		ID:            "test-save-game-sync",
		Rows:          10,
		Cols:          10,
		Winner:        1,
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

func TestInitDBMigratesLegacyGamesTable(t *testing.T) {
	path := filepath.Join(t.TempDir(), "legacy.db")
	legacy, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	_, err = legacy.Exec(`CREATE TABLE games (
		id TEXT PRIMARY KEY, started_at DATETIME, ended_at DATETIME,
		rows INTEGER, cols INTEGER, player1_name TEXT, player2_name TEXT,
		player3_name TEXT, player4_name TEXT, result INTEGER,
		termination TEXT, pgn_content TEXT
	)`)
	if err != nil {
		t.Fatal(err)
	}
	_ = legacy.Close()

	InitDB(path)
	t.Cleanup(func() { _ = db.Close(); db = nil })
	var found bool
	rows, err := db.Query(`PRAGMA table_info(games)`)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	for rows.Next() {
		var cid, notNull, primaryKey int
		var name, columnType string
		var defaultValue any
		if err := rows.Scan(&cid, &name, &columnType, &notNull, &defaultValue, &primaryKey); err != nil {
			t.Fatal(err)
		}
		found = found || name == "rejected_attempt"
	}
	if !found {
		t.Fatal("legacy database was not migrated with rejected_attempt")
	}
}
