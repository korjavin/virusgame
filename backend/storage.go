package main

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"time"

	_ "modernc.org/sqlite"
)

var db *sql.DB

// InitDB initializes the SQLite database
func InitDB(dbPath string) {
	// Ensure directory exists
	dir := filepath.Dir(dbPath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		log.Fatalf("Failed to create database directory: %v", err)
	}

	var err error
	db, err = sql.Open("sqlite", dbPath)
	if err != nil {
		log.Fatalf("Failed to open database: %v", err)
	}

	createTableSQL := `
	CREATE TABLE IF NOT EXISTS games (
		id TEXT PRIMARY KEY,
		started_at DATETIME,
		ended_at DATETIME,
		rows INTEGER,
		cols INTEGER,
		player1_name TEXT,
		player2_name TEXT,
		player3_name TEXT,
		player4_name TEXT,
		result INTEGER,
		termination TEXT,
		pgn_content TEXT
	);
	`

	_, err = db.Exec(createTableSQL)
	if err != nil {
		log.Fatalf("Failed to create table: %v", err)
	}

	log.Println("Database initialized successfully at", dbPath)
}

// SaveGame saves the game to the database
func SaveGame(game *Game, termination string) {
	if db == nil {
		log.Println("Database not initialized, skipping save")
		return
	}

	// Extract data synchronously to avoid race conditions
	pgnContent, err := generatePGN(game)
	if err != nil {
		log.Printf("Error generating PGN: %v", err)
		return
	}

	// Get player names
	p1Name := ""
	p2Name := ""
	p3Name := ""
	p4Name := ""

	if game.IsMultiplayer {
		if game.Players[0] != nil {
			p1Name = getPlayerNameSafe(game.Players[0])
		}
		if game.Players[1] != nil {
			p2Name = getPlayerNameSafe(game.Players[1])
		}
		if game.Players[2] != nil {
			p3Name = getPlayerNameSafe(game.Players[2])
		}
		if game.Players[3] != nil {
			p4Name = getPlayerNameSafe(game.Players[3])
		}
	} else {
		if game.Player1 != nil {
			p1Name = game.Player1.Username
		}
		if game.Player2 != nil {
			p2Name = game.Player2.Username
		}
	}

	gameID := game.ID
	startTime := game.StartTime
	rows := game.Rows
	cols := game.Cols
	winner := game.Winner
	endTime := time.Now()

	// Run saving in a separate goroutine to avoid blocking the game loop
	// using ONLY captured local variables
	go func() {
		// Insert into database
		insertSQL := `
		INSERT INTO games (id, started_at, ended_at, rows, cols, player1_name, player2_name, player3_name, player4_name, result, termination, pgn_content)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		`

		_, err = db.Exec(insertSQL,
			gameID,
			startTime,
			endTime,
			rows,
			cols,
			p1Name,
			p2Name,
			p3Name,
			p4Name,
			winner,
			termination,
			pgnContent,
		)

		if err != nil {
			log.Printf("Error saving game to database: %v", err)
		} else {
			log.Printf("Game %s saved to database", gameID)
		}
	}()
}
func getPlayerNameSafe(player *LobbyPlayer) string {
	if player.User != nil {
		return player.User.Username
	}
	if player.IsBot {
		return fmt.Sprintf("Bot %d", player.Index+1)
	}
	return "Unknown"
}

// PGN structure definitions
type PGNTurn struct {
	Turn    int       `json:"turn"`
	Player  int       `json:"player"`
	Moves   []PGNMove `json:"moves"`
}

type PGNMove struct {
	Type       string    `json:"type"`
	Row        int       `json:"row,omitempty"`
	Col        int       `json:"col,omitempty"`
	Cells      []CellPos `json:"cells,omitempty"`
	DurationCS int       `json:"duration_cs"`
}

func generatePGN(game *Game) (string, error) {
	var turns []PGNTurn
	var currentTurn *PGNTurn

	// Assuming game.MoveHistory contains flat list of moves
	// We need to group them by turn
	// But actually, the prompt example shows:
	// "Sequence Number: Turn number"
	// "Player": Who made the move

	// The prompt JSON structure:
	/*
	[
	  {
	    "turn": 1,
	    "player": 1,
	    "moves": [ ... ]
	  },
	  ...
	]
	*/

	// We'll iterate through MoveHistory and reconstruct this structure.
	// Since 3 moves constitute a turn, we can group them.
	// However, a player might make fewer than 3 moves if game ends or they pass (though pass isn't explicit in current rules unless implied by turn change?)
	// Actually, the turn change logic is in `endTurn` or `handleNeutrals`.
	// The `MoveAction` struct we will add to types.go will just record the action.
	// We need to infer turns.
	// Or we can store `TurnNumber` in `MoveAction`.

	// Let's look at `MoveAction` again (to be defined in types.go):
	/*
	type MoveAction struct {
		Player int
		Type string
		Row int
		Col int
		Cells []CellPos
		Time time.Time
		DurationCS int
		TurnNumber int // Global turn number or per-player turn count?
	}
	*/

	// Let's assume we add `TurnNumber` to `MoveAction`.

	// Grouping logic:
	lastTurnNum := -1
	lastPlayer := -1

	for _, action := range game.MoveHistory {
		// Start a new turn block if turn number or player changes
		// (Player change should coincide with turn number change usually, but in 1v1 turn number might just increment globally)

		// Wait, how do we track Turn Number in the game?
		// The game logic doesn't seem to have an explicit "Turn Number" counter in `Game` struct.
		// We should add `TurnCount` to `Game` struct and increment it in `endTurn`.

		if currentTurn == nil || action.TurnNumber != lastTurnNum || action.Player != lastPlayer {
			if currentTurn != nil {
				turns = append(turns, *currentTurn)
			}
			currentTurn = &PGNTurn{
				Turn:   action.TurnNumber,
				Player: action.Player,
				Moves:  []PGNMove{},
			}
			lastTurnNum = action.TurnNumber
			lastPlayer = action.Player
		}

		pgnMove := PGNMove{
			Type:       action.Type,
			Row:        action.Row,
			Col:        action.Col,
			Cells:      action.Cells,
			DurationCS: action.DurationCS,
		}
		currentTurn.Moves = append(currentTurn.Moves, pgnMove)
	}

	if currentTurn != nil {
		turns = append(turns, *currentTurn)
	}

	bytes, err := json.Marshal(turns)
	if err != nil {
		return "", err
	}
	return string(bytes), nil
}
