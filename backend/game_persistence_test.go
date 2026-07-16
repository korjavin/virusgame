package main

import (
	"database/sql"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"sync"
	"testing"
	"time"
)

func TestOneOnOneTerminalPathsPersistCompleteGameExactlyOnce(t *testing.T) {
	InitDB(filepath.Join(t.TempDir(), "games.db"))
	t.Cleanup(func() {
		if db != nil {
			_ = db.Close()
			db = nil
		}
	})

	tests := []struct {
		name        string
		zeroMoves   bool
		termination string
		winner      int
		finish      func(*Hub, *Game, *User)
	}{
		{
			name: "normal elimination", termination: "normal", winner: 1,
			finish: func(h *Hub, game *Game, _ *User) { h.checkWinCondition(game) },
		},
		{
			name: "no moves", termination: "no_moves", winner: 1,
			finish: func(h *Hub, game *Game, _ *User) { h.endTurn(game) },
		},
		{
			name: "illegal move", termination: "illegal_move", winner: 2,
			finish: func(h *Hub, game *Game, _ *User) { h.handleIllegalMove(game, 1, "test") },
		},
		{
			name: "resignation", termination: "resignation", winner: 2,
			finish: func(h *Hub, game *Game, player1 *User) {
				h.handleResign(player1, &Message{GameID: game.ID})
			},
		},
		{
			name: "disconnect with zero moves", termination: "disconnect", winner: 2, zeroMoves: true,
			finish: func(h *Hub, _ *Game, player1 *User) { h.handleDisconnect(player1.Client) },
		},
		{
			name: "timeout", termination: "timeout", winner: 2,
			finish: func(h *Hub, game *Game, _ *User) {
				h.handleMoveTimeout(&Message{GameID: game.ID, Player: 1})
			},
		},
	}

	for i, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			h := newHub()
			player1 := persistenceTestUser("human", "Human")
			player2 := persistenceTestUser("bot", "OnlineBot")
			game := persistenceTestGame(test.name, player1, player2)
			if test.zeroMoves {
				game.MoveHistory = nil
			}
			if test.name == "normal elimination" || test.name == "no moves" {
				game.Board[1][1] = 0
			}
			h.games[game.ID] = game
			h.users[player1.ID] = player1
			h.users[player2.ID] = player2

			test.finish(h, game, player1)
			if !game.persisted {
				t.Fatal("terminal path did not persist game")
			}
			if !PersistGameOnce(game, "duplicate_signal") {
				t.Fatal("duplicate terminal signal did not observe durable row")
			}

			var (
				count, rows, cols, winner    int
				started, ended               time.Time
				p1, p2, termination, content string
			)
			err := db.QueryRow(`
				SELECT COUNT(*), started_at, ended_at, rows, cols,
				       player1_name, player2_name, result, termination, pgn_content
				FROM games WHERE id = ?`, game.ID).Scan(
				&count, &started, &ended, &rows, &cols,
				&p1, &p2, &winner, &termination, &content,
			)
			if err != nil {
				t.Fatalf("query saved game: %v", err)
			}
			if count != 1 || rows != 2 || cols != 2 || p1 != "Human" || p2 != "OnlineBot" ||
				winner != test.winner || termination != test.termination {
				t.Fatalf("unexpected saved row: count=%d board=%dx%d players=%q/%q winner=%d termination=%q case=%d",
					count, rows, cols, p1, p2, winner, termination, i)
			}
			if started.IsZero() || ended.IsZero() || ended.Before(started) {
				t.Fatalf("invalid timestamps: started=%v ended=%v", started, ended)
			}

			var turns []PGNTurn
			if err := json.Unmarshal([]byte(content), &turns); err != nil {
				t.Fatalf("decode move history: %v", err)
			}
			if test.zeroMoves {
				if len(turns) != 0 {
					t.Fatalf("expected empty move history, got %#v", turns)
				}
				return
			}
			if len(turns) != 1 || turns[0].Player != 1 || len(turns[0].Moves) != 2 ||
				turns[0].Moves[0].Type != "place" || turns[0].Moves[1].Type != "attack" {
				t.Fatalf("move history not preserved in order: %#v", turns)
			}
		})
	}
}

func persistenceTestUser(id, username string) *User {
	client := &Client{send: make(chan []byte, 32)}
	user := &User{ID: id, Username: username, Client: client, InGame: true}
	client.user = user
	return user
}

func persistenceTestGame(id string, player1, player2 *User) *Game {
	start := time.Now().Add(-time.Minute).UTC()
	game := &Game{
		ID: id, Player1: player1, Player2: player2,
		Rows: 2, Cols: 2, Board: make(Board, 2),
		CurrentPlayer: 1, MovesLeft: 3, StartTime: start, LastActionTime: start,
		MoveHistory: []MoveAction{
			{Player: 1, Type: "place", Row: 0, Col: 1, DurationCS: 3, TurnNumber: 1},
			{Player: 1, Type: "attack", Row: 1, Col: 0, DurationCS: 5, TurnNumber: 1},
		},
	}
	for row := range game.Board {
		game.Board[row] = make([]CellValue, 2)
	}
	game.Board[0][0] = NewCell(1, CellFlagBase)
	player1.GameID = id
	player2.GameID = id
	return game
}

// TestEliminateDisconnectedPlayersPersistsOnce covers the remaining distinct 1v1
// terminal producer: a player with pieces but no legal move loses and the game is
// committed exactly once as "no_moves".
func TestEliminateDisconnectedPlayersPersistsOnce(t *testing.T) {
	InitDB(filepath.Join(t.TempDir(), "elim.db"))
	t.Cleanup(closePersistenceTestDB)

	h := newHub()
	player1 := persistenceTestUser("human", "Human")
	player2 := persistenceTestUser("bot", "OnlineBot")
	game := persistenceTestGame("elim-nomoves", player1, player2)
	// Player 2 owns a stray piece with no base, so it can make no legal move.
	game.Board[0][1] = NewCell(2, CellFlagNormal)
	h.games[game.ID] = game

	h.eliminateDisconnectedPlayers(game)

	if !game.GameOver || game.Winner != 1 {
		t.Fatalf("expected player 1 to win, gameOver=%t winner=%d", game.GameOver, game.Winner)
	}
	if !game.persisted {
		t.Fatal("terminal producer did not persist game")
	}
	if !PersistGameOnce(game, "duplicate") {
		t.Fatal("duplicate terminal signal did not observe durable row")
	}
	var count int
	termination := ""
	if err := db.QueryRow("SELECT COUNT(*), termination FROM games WHERE id = ?", game.ID).Scan(&count, &termination); err != nil {
		t.Fatal(err)
	}
	if count != 1 || termination != "no_moves" {
		t.Fatalf("unexpected row: count=%d termination=%q", count, termination)
	}
}

// TestCleanupUserRequiresCustodyBeforeDelete proves cleanupUserFromPreviousGame
// cannot drop a GameOver game from memory until its terminal record has durable
// custody (games row or outbox).
func TestCleanupUserRequiresCustodyBeforeDelete(t *testing.T) {
	InitDB(filepath.Join(t.TempDir(), "custody.db"))
	t.Cleanup(closePersistenceTestDB)
	h := newHub()

	user := persistenceTestUser("human", "Human")
	board := make(Board, 2)
	for i := range board {
		board[i] = make([]CellValue, 2)
	}
	board[0][0] = NewCell(1, CellFlagBase)
	game := &Game{
		ID: "custody-game", Board: board, Rows: 2, Cols: 2,
		IsMultiplayer: true, GameOver: true, Winner: 1,
		persistenceTermination: "normal",
		Players:                [4]*LobbyPlayer{{User: user, Index: 0}, {IsBot: true, Index: 1}, nil, nil},
		MoveHistory:            []MoveAction{{Player: 1, Type: "place", Row: 0, Col: 1, TurnNumber: 1}},
		StartTime:              time.Now().Add(-time.Minute),
	}
	h.games[game.ID] = game
	user.InGame = true
	user.GameID = game.ID

	// Force both durable stores to fail: no DB, and a spool whose durable write
	// errors. Restore via t.Cleanup so a failed assertion cannot leak global state
	// (db/fs) into later tests.
	good := db
	t.Cleanup(func() { db = good })
	rec := installRecordingFS(t)
	rec.failAt = "rename"
	db = nil

	h.cleanupUserFromPreviousGame(user)
	if _, exists := h.games[game.ID]; !exists {
		t.Fatal("GameOver game dropped from memory without durable custody")
	}

	// Restore durability; the periodic cleanup now commits and releases it.
	db = good
	rec.failAt = ""
	h.cleanupStaleGames()
	if _, exists := h.games[game.ID]; exists {
		t.Fatal("game not released after durable custody achieved")
	}
	var count int
	if err := db.QueryRow("SELECT COUNT(*) FROM games WHERE id = ?", game.ID).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Fatalf("expected 1 durable row, got %d", count)
	}
}

func TestPersistGameOnceFailureRetryAndConflict(t *testing.T) {
	player1 := persistenceTestUser("human", "Human")
	player2 := persistenceTestUser("bot", "OnlineBot")
	game := persistenceTestGame("retry-game", player1, player2)
	game.Winner = 2
	presetEndTime := time.Date(2026, time.July, 15, 12, 0, 0, 0, time.UTC)
	game.EndTime = presetEndTime

	previousDB := db
	db = nil
	if PersistGameOnce(game, "disconnect") {
		t.Fatal("save unexpectedly succeeded without a database")
	}
	if game.persisted {
		t.Fatal("failed save must remain retryable")
	}
	db = previousDB

	InitDB(filepath.Join(t.TempDir(), "retry.db"))
	t.Cleanup(closePersistenceTestDB)
	if !PersistGameOnce(game, "wrong_retry_reason") {
		t.Fatal("retry did not persist game")
	}

	conflict := persistenceTestGame(game.ID, player1, player2)
	conflict.Winner = 1
	if PersistGameOnce(conflict, "stale_conflict") {
		t.Fatal("conflicting pre-existing game id must be an explicit failure")
	}
	var (
		termination string
		endedAt     time.Time
	)
	if err := db.QueryRow("SELECT termination, ended_at FROM games WHERE id = ?", game.ID).Scan(&termination, &endedAt); err != nil {
		t.Fatal(err)
	}
	if termination != "disconnect" {
		t.Fatalf("conflict changed durable row: %q", termination)
	}
	if !endedAt.Equal(presetEndTime) {
		t.Fatalf("pre-set end time changed: got %v want %v", endedAt, presetEndTime)
	}
}

func TestPersistGameOnceConcurrentDuplicates(t *testing.T) {
	InitDB(filepath.Join(t.TempDir(), "concurrent.db"))
	t.Cleanup(closePersistenceTestDB)
	game := persistenceTestGame("concurrent-game", persistenceTestUser("p1", "Human"), persistenceTestUser("p2", "OnlineBot"))
	game.Winner = 2

	const callers = 16
	var wg sync.WaitGroup
	results := make(chan bool, callers)
	for i := 0; i < callers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			results <- PersistGameOnce(game, "resignation")
		}()
	}
	wg.Wait()
	close(results)
	for result := range results {
		if !result {
			t.Fatal("duplicate caller did not observe durable success")
		}
	}
	var count int
	if err := db.QueryRow("SELECT COUNT(*) FROM games WHERE id = ?", game.ID).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Fatalf("expected one durable row, got %d", count)
	}
}

func TestClientCannotForgeMoveTimeout(t *testing.T) {
	InitDB(filepath.Join(t.TempDir(), "forged-timeout.db"))
	t.Cleanup(closePersistenceTestDB)
	h := newHub()
	player1 := persistenceTestUser("human", "Human")
	player2 := persistenceTestUser("bot", "OnlineBot")
	game := persistenceTestGame("forged-timeout", player1, player2)
	h.games[game.ID] = game

	h.handleClientMessage(player1.Client, &Message{Type: "move_timeout", GameID: game.ID, Player: 1})
	if game.GameOver || game.persisted {
		t.Fatal("connected client forged a timeout result")
	}
	var count int
	if err := db.QueryRow("SELECT COUNT(*) FROM games WHERE id = ?", game.ID).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 0 {
		t.Fatalf("forged timeout created %d training rows", count)
	}
}

func TestMultiplayerCleanupPersistenceRetryAndVisibility(t *testing.T) {
	InitDB(filepath.Join(t.TempDir(), "multiplayer_cleanup.db"))
	t.Cleanup(closePersistenceTestDB)

	h := newHub()
	player1 := persistenceTestUser("human", "Human")
	player2 := persistenceTestUser("bot", "OnlineBot")

	gameID := "multi-cleanup-game"
	board := make(Board, 2)
	for i := range board {
		board[i] = make([]CellValue, 2)
	}
	board[0][0] = NewCell(1, CellFlagBase)
	board[1][1] = NewCell(2, CellFlagBase)

	game := &Game{
		ID:            gameID,
		Board:         board,
		CurrentPlayer: 1,
		MovesLeft:     3,
		GameOver:      false,
		Winner:        0,
		Rows:          2,
		Cols:          2,
		IsMultiplayer: true,
		Players: [4]*LobbyPlayer{
			{User: player1, Symbol: "X", IsBot: false, Index: 0},
			{User: player2, Symbol: "O", IsBot: true, Index: 1},
			nil,
			nil,
		},
		ActivePlayers:  2,
		StartTime:      time.Now().Add(-time.Minute),
		LastActionTime: time.Now().Add(-time.Minute),
		TurnCount:      1,
		MoveHistory: []MoveAction{
			{Player: 1, Type: "place", Row: 0, Col: 1, DurationCS: 3, TurnNumber: 1},
		},
	}
	h.games[game.ID] = game
	h.users[player1.ID] = player1
	h.users[player2.ID] = player2

	// 1. Simulate game end due to elimination/normal win condition
	// Let's make active players <= 1 by eliminating player 2
	game.Board[1][1] = 0
	h.checkMultiplayerStatus(game)

	// Game should be marked GameOver
	if !game.GameOver {
		t.Fatal("expected multiplayer game to end")
	}

	// Database should have the persisted game
	var count int
	if err := db.QueryRow("SELECT COUNT(*) FROM games WHERE id = ?", game.ID).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Fatalf("expected 1 game in DB, got %d", count)
	}

	// 2. Test Next-Request Visibility: calling /last_games endpoint should immediately return the game
	response := performRecentGamesTestRequest(db, http.MethodGet, "/last_games?limit=5", "")
	if response.Code != http.StatusOK {
		t.Fatalf("status = %d", response.Code)
	}
	var payload recentGamesResponse
	if err := json.Unmarshal(response.Body.Bytes(), &payload); err != nil {
		t.Fatal(err)
	}
	if len(payload.Games) < 1 || payload.Games[0].ID != game.ID {
		t.Fatalf("expected game %s to be returned by /last_games, got games: %+v", game.ID, payload.Games)
	}
}

func TestMultiplayerCleanupFailureRetry(t *testing.T) {
	InitDB(filepath.Join(t.TempDir(), "multiplayer_cleanup_fail.db"))
	t.Cleanup(closePersistenceTestDB)

	h := newHub()
	player1 := persistenceTestUser("human", "Human")
	player2 := persistenceTestUser("bot", "OnlineBot")

	gameID := "multi-cleanup-fail"
	board := make(Board, 2)
	for i := range board {
		board[i] = make([]CellValue, 2)
	}
	board[0][0] = NewCell(1, CellFlagBase)
	board[1][1] = NewCell(2, CellFlagBase)

	game := &Game{
		ID:            gameID,
		Board:         board,
		CurrentPlayer: 1,
		MovesLeft:     3,
		GameOver:      false,
		Winner:        0,
		Rows:          2,
		Cols:          2,
		IsMultiplayer: true,
		Players: [4]*LobbyPlayer{
			{User: player1, Symbol: "X", IsBot: false, Index: 0},
			{User: player2, Symbol: "O", IsBot: true, Index: 1},
			nil,
			nil,
		},
		ActivePlayers:  2,
		StartTime:      time.Now().Add(-time.Minute),
		LastActionTime: time.Now().Add(-time.Minute),
		TurnCount:      1,
		MoveHistory: []MoveAction{
			{Player: 1, Type: "place", Row: 0, Col: 1, DurationCS: 3, TurnNumber: 1},
		},
	}
	h.games[game.ID] = game

	// Simulate DB failure by setting db to nil
	oldDB := db
	db = nil

	// End the game
	game.Board[1][1] = 0
	h.checkMultiplayerStatus(game)

	if !game.GameOver {
		t.Fatal("expected multiplayer game to end")
	}
	if game.persisted {
		t.Fatal("expected game.persisted to be false since DB was nil")
	}
	// The completed game must not be lost: custody is transferred to the durable
	// outbox even though the games table write failed.
	if spool.depth() != 1 {
		t.Fatalf("terminal record not spooled during DB outage: depth=%d", spool.depth())
	}

	// Cleanup runs while the DB is still down. Custody is already durable in the
	// outbox, so the game may leave memory without being lost.
	h.handleCleanupGame(&Message{GameID: gameID})
	if _, exists := h.games[gameID]; exists {
		t.Fatal("game retained in memory despite durable outbox custody")
	}

	// Restore DB and replay the outbox — the record commits exactly once.
	db = oldDB
	h.replayOutbox()
	if spool.depth() != 0 {
		t.Fatalf("outbox not drained after recovery: depth=%d", spool.depth())
	}

	var count int
	if err := db.QueryRow("SELECT COUNT(*) FROM games WHERE id = ?", game.ID).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Fatalf("expected 1 game in DB, got %d", count)
	}
}

func Test1v1DisconnectDoesNotOverwriteWinner(t *testing.T) {
	InitDB(filepath.Join(t.TempDir(), "disconnect_winner.db"))
	t.Cleanup(closePersistenceTestDB)

	h := newHub()
	player1 := persistenceTestUser("human", "Human")
	player2 := persistenceTestUser("bot", "OnlineBot")

	game := persistenceTestGame("winner-overwritten", player1, player2)
	h.games[game.ID] = game
	h.users[player1.ID] = player1
	h.users[player2.ID] = player2

	// Game ends normally with Player 1 as winner
	game.GameOver = true
	game.Winner = 1
	if !PersistGameOnce(game, "normal") {
		t.Fatal("failed to persist game")
	}

	// Reset persisted flag to simulate a retry (as if the first write failed)
	game.persisted = false
	// Clear from DB
	_, _ = db.Exec("DELETE FROM games WHERE id = ?", game.ID)

	// Now player 1 disconnects
	h.handleDisconnect(player1.Client)

	// Verify that the game is persisted, but the Winner is still 1 (not 2) and the termination is still "normal"
	var (
		count, winner int
		termination   string
	)
	err := db.QueryRow("SELECT COUNT(*), result, termination FROM games WHERE id = ?", game.ID).Scan(&count, &winner, &termination)
	if err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Fatalf("expected 1 game, got %d", count)
	}
	if winner != 1 {
		t.Fatalf("winner was overwritten: got %d, want 1", winner)
	}
	if termination != "normal" {
		t.Fatalf("termination was overwritten: got %s, want normal", termination)
	}
}

func TestClientCannotForgeCleanupGame(t *testing.T) {
	InitDB(filepath.Join(t.TempDir(), "forged-cleanup.db"))
	t.Cleanup(closePersistenceTestDB)
	h := newHub()
	player1 := persistenceTestUser("human", "Human")
	player2 := persistenceTestUser("bot", "OnlineBot")
	game := persistenceTestGame("forged-cleanup", player1, player2)
	h.games[game.ID] = game

	// A client attempts to send cleanup_game message for a live game
	h.handleClientMessage(player1.Client, &Message{Type: "cleanup_game", GameID: game.ID})

	// The game must not be deleted or persisted since the client is not nil
	if _, exists := h.games[game.ID]; !exists {
		t.Fatal("connected client forged cleanup_game and deleted the live game")
	}
	if game.persisted {
		t.Fatal("connected client forged cleanup_game and persisted the live game")
	}

	var count int
	if err := db.QueryRow("SELECT COUNT(*) FROM games WHERE id = ?", game.ID).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 0 {
		t.Fatalf("forged cleanup created %d training rows", count)
	}
}

func TestActiveOrphanGameFinalizedAsAbandoned(t *testing.T) {
	InitDB(filepath.Join(t.TempDir(), "active-cleanup.db"))
	t.Cleanup(closePersistenceTestDB)

	h := newHub()
	player1 := persistenceTestUser("human", "Human")
	player2 := persistenceTestUser("bot", "OnlineBot")
	game := persistenceTestGame("active-cleanup-id", player1, player2)
	h.games[game.ID] = game

	// Simulate the game being orphaned (both players disconnected/client nil)
	player1.Client = nil
	player2.Client = nil

	// Run cleanup
	h.cleanupStaleGames()

	// The orphaned active game must NOT vanish silently: it is finalized as an
	// explicit "abandoned" lifecycle record and then removed from memory.
	if _, exists := h.games[game.ID]; exists {
		t.Fatal("finalized orphan game was not removed from memory after durable write")
	}

	var (
		count       int
		termination string
		content     string
	)
	if err := db.QueryRow("SELECT COUNT(*), termination, pgn_content FROM games WHERE id = ?", game.ID).
		Scan(&count, &termination, &content); err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Fatalf("orphan game was not persisted, count=%d", count)
	}
	if termination != "abandoned" {
		t.Fatalf("orphan termination = %q, want abandoned", termination)
	}
	// Full move history is retained, not truncated.
	var turns []PGNTurn
	if err := json.Unmarshal([]byte(content), &turns); err != nil {
		t.Fatalf("decode move history: %v", err)
	}
	if len(turns) != 1 || len(turns[0].Moves) != 2 {
		t.Fatalf("abandoned game history not preserved: %#v", turns)
	}
}

func TestSQLiteLockContention(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "app_contention.db")

	// 1. Initialize the global DB via the actual app InitDB function
	InitDB(dbPath)
	t.Cleanup(closePersistenceTestDB)

	// 2. Open an external connection to the same database file to simulate another process/writer
	extDB, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatal(err)
	}
	defer extDB.Close()

	// 3. Acquire a write lock on the SQLite database using the external connection.
	// We do this by starting a transaction and writing a row.
	tx, err := extDB.Begin()
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = tx.Rollback() }()

	// We insert a dummy row into games to hold the write lock
	_, err = tx.Exec(`
		INSERT INTO games (id, started_at, ended_at, rows, cols, player1_name, player2_name, result, termination, pgn_content)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		"lock-holder-id", time.Now(), time.Now(), 10, 10, "P1", "P2", 1, "normal", "[]")
	if err != nil {
		t.Fatal(err)
	}

	// 4. Force a reopen/replacement of the app connection to verify the DSN pragma configuration
	// is properly applied on every connection creation.
	if db != nil {
		_ = db.Close()
		db = nil
	}
	InitDB(dbPath) // Re-runs InitDB, opening a fresh connection pool

	// 5. Try to write to the global DB (using PersistGameOnce).
	// Because the external transaction is holding a write lock, the global DB connection
	// must wait/block rather than failing with SQLITE_BUSY immediately.
	player1 := persistenceTestUser("p1", "Human")
	player2 := persistenceTestUser("p2", "OnlineBot")
	game := persistenceTestGame("app-persist-game", player1, player2)
	game.Winner = 1

	start := time.Now()
	errChan := make(chan error, 1)

	// Release the external lock after 150ms
	go func() {
		time.Sleep(150 * time.Millisecond)
		errChan <- tx.Commit()
	}()

	// Perform write via actual global DB created by InitDB
	if !PersistGameOnce(game, "normal") {
		t.Fatal("PersistGameOnce failed under lock contention")
	}

	duration := time.Since(start)
	if duration < 150*time.Millisecond {
		t.Fatalf("PersistGameOnce did not wait for the external lock to be released, duration=%v", duration)
	}

	// Verify the external commit was successful
	if commitErr := <-errChan; commitErr != nil {
		t.Fatalf("External commit failed: %v", commitErr)
	}

	// Verify the row written by PersistGameOnce exists
	var count int
	err = db.QueryRow("SELECT COUNT(*) FROM games WHERE id = ?", game.ID).Scan(&count)
	if err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Fatalf("game was not successfully persisted, count=%d", count)
	}
}

func performRecentGamesTestRequest(database *sql.DB, method, target, encoding string) *httptest.ResponseRecorder {
	request := httptest.NewRequest(method, target, nil)
	request.Header.Set("Accept-Encoding", encoding)
	response := httptest.NewRecorder()
	recentGamesHandler(database).ServeHTTP(response, request)
	return response
}

func closePersistenceTestDB() {
	if db != nil {
		_ = db.Close()
		db = nil
	}
}
