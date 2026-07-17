package main

import (
	"testing"
	"time"
)

// TestResync_LiveGame simulates a client reconnecting (ws.onopen -> "resync")
// while its game is still live: the hub must reply with a game_state message
// carrying the full authoritative snapshot so the client can reconcile its
// board, turn and optimistically-decremented movesLeft.
func TestResync_LiveGame(t *testing.T) {
	h := newHub()
	go h.run()

	c := &Client{hub: h, send: make(chan []byte, 256)}
	h.register <- c
	waitForMessage(t, c, "welcome")
	if c.user == nil {
		t.Fatal("user should be set after connect")
	}

	gameID := "resync-live"
	rows, cols := 5, 5
	board := make(Board, rows)
	for i := range board {
		board[i] = make([]CellValue, cols)
	}
	game := &Game{
		ID:             gameID,
		Player1:        c.user,
		Board:          board,
		Rows:           rows,
		Cols:           cols,
		CurrentPlayer:  2,
		MovesLeft:      2,
		Player1Base:    CellPos{0, 0},
		Player2Base:    CellPos{4, 4},
		MoveHistory:    []MoveAction{},
		LastActionTime: time.Now(),
	}
	game.Board[0][0] = NewCell(1, CellFlagBase)
	game.Board[0][1] = NewCell(1, CellFlagNormal)
	game.Board[4][4] = NewCell(2, CellFlagBase)

	runOnHub(h, func() { h.games[gameID] = game })

	sendMessage(h, c, &Message{Type: "resync", GameID: gameID})

	msg := waitForMessage(t, c, "game_state")
	if msg == nil {
		return
	}
	if msg.Snapshot == nil {
		t.Fatal("resync of a live game must carry an authoritative snapshot")
	}
	// Snapshot must reflect real server state, not a stale client guess.
	if msg.Snapshot.MovesLeft != 2 {
		t.Errorf("snapshot MovesLeft = %d, want 2", msg.Snapshot.MovesLeft)
	}
	if int(msg.Snapshot.Current) != 2 {
		t.Errorf("snapshot Current = %d, want 2", msg.Snapshot.Current)
	}
	if len(msg.Snapshot.Board) != rows || len(msg.Snapshot.Board[0]) != cols {
		t.Errorf("snapshot board dims = %dx%d, want %dx%d",
			len(msg.Snapshot.Board), len(msg.Snapshot.Board[0]), rows, cols)
	}
	if msg.Snapshot.Board[0][1].Owner != 1 {
		t.Errorf("snapshot board[0][1] owner = %d, want 1", msg.Snapshot.Board[0][1].Owner)
	}
}

// TestResync_EndedGame simulates a client reconnecting after the game it thinks
// it is in has already ended and been cleaned up (e.g. auto-resign on the
// disconnect that triggered the reconnect). The hub has no game to snapshot, so
// it must reply with game_end so the client stops rendering a phantom live turn.
func TestResync_EndedGame(t *testing.T) {
	h := newHub()
	go h.run()

	c := &Client{hub: h, send: make(chan []byte, 256)}
	h.register <- c
	waitForMessage(t, c, "welcome")

	sendMessage(h, c, &Message{Type: "resync", GameID: "already-gone"})

	msg := waitForMessage(t, c, "game_end")
	if msg == nil {
		return
	}
	if msg.GameID != "already-gone" {
		t.Errorf("game_end GameID = %q, want %q", msg.GameID, "already-gone")
	}
}
