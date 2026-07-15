package main

import (
	"encoding/json"
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

func closePersistenceTestDB() {
	if db != nil {
		_ = db.Close()
		db = nil
	}
}
