package main

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/google/uuid"
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

	escapedPath := url.PathEscape(filepath.ToSlash(dbPath))
	escapedPath = strings.ReplaceAll(escapedPath, "%2F", "/")
	escapedPath = strings.ReplaceAll(escapedPath, "%3A", ":")

	dsn := "file:" + escapedPath + "?_pragma=journal_mode(WAL)&_pragma=busy_timeout(5000)"

	var err error
	db, err = sql.Open("sqlite", dsn)
	if err != nil {
		log.Fatalf("Failed to open database: %v", err)
	}

	// Validate returned journal mode
	var journalMode string
	if err = db.QueryRow("PRAGMA journal_mode;").Scan(&journalMode); err != nil {
		log.Printf("Failed to query journal_mode: %v", err)
	} else if strings.ToLower(journalMode) != "wal" {
		log.Printf("Warning: SQLite journal_mode is %s, expected WAL", journalMode)
	} else {
		log.Printf("SQLite journal_mode successfully configured to %s", journalMode)
	}

	// Constrain pool to 1 connection.
	// Trade-off: SQLite is a single-writer database. Constraining the pool to 1 connection
	// serializes all read/write database requests within this application, eliminating
	// database lock contention and SQLITE_BUSY errors, with negligible read latency penalty
	// for our low-traffic '/last_games' endpoint.
	db.SetMaxOpenConns(1)

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
		pgn_content TEXT,
		rejected_attempt TEXT
	);
	`

	_, err = db.Exec(createTableSQL)
	if err != nil {
		log.Fatalf("Failed to create table: %v", err)
	}
	// Existing installations predate rejected-attempt diagnostics.
	if _, err = db.Exec(`ALTER TABLE games ADD COLUMN rejected_attempt TEXT`); err != nil && !isDuplicateColumnError(err) {
		log.Fatalf("Failed to migrate games table: %v", err)
	}

	// A stable opaque database identity minted once inside the mounted file.
	// Unlike a filesystem path it travels WITH the data, so a WS welcome and a
	// /last_games read that report the same db_id provably share one volume.
	dbIdentity = ensureDBIdentity()
	persistHealth.setDBID(dbIdentity)

	// Durable outbox lives alongside the database on the same mounted volume.
	spool.init(filepath.Dir(dbPath))
	persistHealth.setOutboxDepth(spool.depth())
	// Quarantined (lost) records survive restarts: surface the persistent
	// unhealthy state immediately, before any new game closes.
	persistHealth.setQuarantineDepth(spool.quarantineDepth())

	// The resolved absolute path is operator-only; log it, never expose it.
	resolved := dbPath
	if abs, absErr := filepath.Abs(dbPath); absErr == nil {
		resolved = abs
	}
	log.Printf("Database initialized successfully at %s (db_id=%s)", resolved, dbIdentity)
}

// dbIdentity is the opaque UUID stored inside the database's metadata table.
var dbIdentity string

// ensureDBIdentity reads the persistent database UUID, minting one on first init.
// The mint is race-safe: INSERT OR IGNORE keeps the first writer's value, then a
// SELECT returns whichever value is durably stored.
func ensureDBIdentity() string {
	if _, err := db.Exec(`CREATE TABLE IF NOT EXISTS db_metadata (key TEXT PRIMARY KEY, value TEXT)`); err != nil {
		log.Fatalf("Failed to create metadata table: %v", err)
	}
	// Fast path: once minted, the identity never changes — read it without a write
	// so a reopen while another writer holds the DB lock does not block on a write.
	var id string
	err := db.QueryRow(`SELECT value FROM db_metadata WHERE key = 'db_uuid'`).Scan(&id)
	if err == nil && id != "" {
		return id
	}
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		log.Fatalf("Failed to read database identity: %v", err)
	}
	// First init: mint race-safely. INSERT OR IGNORE lets concurrent first-inits
	// converge on a single durable value, which the following SELECT returns.
	if _, err := db.Exec(`INSERT OR IGNORE INTO db_metadata (key, value) VALUES ('db_uuid', ?)`, uuid.NewString()); err != nil {
		log.Fatalf("Failed to mint database identity: %v", err)
	}
	if err := db.QueryRow(`SELECT value FROM db_metadata WHERE key = 'db_uuid'`).Scan(&id); err != nil {
		log.Fatalf("Failed to read database identity: %v", err)
	}
	return id
}

// PersistGameOnce captures and saves a game's terminal state exactly once.
// The SQLite write is synchronous so a terminal game is durable before hub
// cleanup or process shutdown. Failed writes remain retryable.
func PersistGameOnce(game *Game, termination string) bool {
	game.persistenceMu.Lock()
	defer game.persistenceMu.Unlock()

	if game.persisted {
		return true
	}
	if game.persistenceTermination == "" {
		game.persistenceTermination = termination
	}
	if game.EndTime.IsZero() {
		game.EndTime = time.Now()
	}
	if err := saveGame(game, game.persistenceTermination); err != nil {
		persistHealth.recordFailure(err)
		log.Printf("event=persist_outcome result=failure game=%s termination=%s error=%q",
			game.ID, game.persistenceTermination, err.Error())
		return false
	}
	game.persisted = true
	persistHealth.recordSuccess(game.ID)
	log.Printf("event=persist_outcome result=success game=%s termination=%s", game.ID, game.persistenceTermination)
	return true
}

// saveGame builds an immutable terminal record from the finalized game and
// commits it. Kept for callers that persist directly from a live *Game.
func saveGame(game *Game, termination string) error {
	rec, err := buildTerminalRecord(game, termination)
	if err != nil {
		return err
	}
	return saveRecord(rec)
}

// buildTerminalRecord snapshots everything needed to persist a completed game
// into a self-contained value, so durability no longer depends on live state.
func buildTerminalRecord(game *Game, termination string) (terminalRecord, error) {
	pgnContent, err := generatePGN(game)
	if err != nil {
		return terminalRecord{}, fmt.Errorf("generate move history: %w", err)
	}

	p1Name, p2Name, p3Name, p4Name := "", "", "", ""
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

	rejected := ""
	if game.RejectedAttempt != nil {
		encoded, err := json.Marshal(game.RejectedAttempt)
		if err != nil {
			return terminalRecord{}, fmt.Errorf("encode rejected attempt: %w", err)
		}
		rejected = string(encoded)
	}

	return terminalRecord{
		ID:           game.ID,
		StartedAt:    game.StartTime,
		EndedAt:      game.EndTime,
		Rows:         game.Rows,
		Cols:         game.Cols,
		Player1Name:  p1Name,
		Player2Name:  p2Name,
		Player3Name:  p3Name,
		Player4Name:  p4Name,
		Result:       game.Winner,
		Termination:  termination,
		PGNContent:   pgnContent,
		RejectedJSON: rejected,
	}, nil
}

// saveRecord inserts one immutable terminal record. The games PRIMARY KEY makes
// re-inserting the same id a constraint error, so replay never duplicates a row.
func saveRecord(rec terminalRecord) error {
	if db == nil {
		return fmt.Errorf("database not initialized")
	}
	var rejected any
	if rec.RejectedJSON != "" {
		rejected = rec.RejectedJSON
	}
	insertSQL := `
		INSERT INTO games (id, started_at, ended_at, rows, cols, player1_name, player2_name, player3_name, player4_name, result, termination, pgn_content, rejected_attempt)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		`
	_, err := db.Exec(insertSQL,
		rec.ID, rec.StartedAt, rec.EndedAt, rec.Rows, rec.Cols,
		rec.Player1Name, rec.Player2Name, rec.Player3Name, rec.Player4Name,
		rec.Result, rec.Termination, rec.PGNContent, rejected,
	)
	return err
}

// gameRowExists reports whether a durable games row already exists for id.
func gameRowExists(id string) (bool, error) {
	if db == nil {
		return false, fmt.Errorf("database not initialized")
	}
	var one int
	err := db.QueryRow(`SELECT 1 FROM games WHERE id = ?`, id).Scan(&one)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return true, nil
}

func isDuplicateColumnError(err error) bool {
	return err != nil && strings.Contains(strings.ToLower(err.Error()), "duplicate column name")
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
	Turn   int       `json:"turn"`
	Player int       `json:"player"`
	Moves  []PGNMove `json:"moves"`
}

type PGNMove struct {
	Type       string    `json:"type"`
	Row        int       `json:"row"`
	Col        int       `json:"col"`
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
