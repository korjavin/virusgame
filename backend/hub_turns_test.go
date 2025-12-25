package main

import (
	"testing"
)

// Tests for turn rotation and attack mechanics in multiplayer
func TestHub_TurnRotation_Multiplayer(t *testing.T) {
	h := newHub()
	go h.run()

	clients := make([]*Client, 3)
	for i := 0; i < 3; i++ {
		clients[i] = &Client{hub: h, send: make(chan []byte, 256)}
		h.register <- clients[i]
		drainWelcome(clients[i])
	}

	// Create multiplayer game manually
	gameID := "mp-turns"
	game := &Game{
		ID: gameID,
		IsMultiplayer: true,
		ActivePlayers: 3,
		Rows: 5, Cols: 5,
		CurrentPlayer: 1,
		MovesLeft: 1, // Set to 1 so next move ends turn
		Board: make(Board, 5),
	}
	for i := range game.Board {
		game.Board[i] = make([]CellValue, 5)
	}

	// Setup players
	for i := 0; i < 3; i++ {
		game.Players[i] = &LobbyPlayer{User: clients[i].user, Index: i, Symbol: "P"}
		clients[i].user.InGame = true
		clients[i].user.GameID = gameID
	}

	// Set bases and connected pieces
	game.Board[0][0] = NewCell(1, CellFlagBase)
	game.Board[0][1] = NewCell(1, CellFlagNormal) // P1 can move from here

	game.Board[4][4] = NewCell(2, CellFlagBase)
	game.Board[4][3] = NewCell(2, CellFlagNormal) // P2 can move

	game.Board[0][4] = NewCell(3, CellFlagBase)
	game.Board[1][4] = NewCell(3, CellFlagNormal) // P3 can move

	game.PlayerBases[0] = CellPos{0,0}
	game.PlayerBases[1] = CellPos{4,4}
	game.PlayerBases[2] = CellPos{0,4}

	h.games[gameID] = game

	// P1 makes a move (movesLeft=1 -> 0 -> Turn Change)
	r, c := 1, 1
	sendMessage(h, clients[0], &Message{
		Type: "move",
		GameID: gameID,
		Row: &r, Col: &c,
	})

	// Wait for move_made
	waitForMessage(t, clients[0], "move_made")

	// Should see turn_change to Player 2
	msg := waitForMessage(t, clients[0], "turn_change")
	if msg.Player != 2 {
		t.Errorf("Expected turn change to Player 2, got %d", msg.Player)
	}

	// Drain P1 messages from C2 (move_made and turn_change) so we don't read stale ones later
	waitForMessage(t, clients[1], "move_made")
	waitForMessage(t, clients[1], "turn_change")

	if game.CurrentPlayer != 2 {
		t.Error("Internal state should be Player 2")
	}

	// P2 makes a move (movesLeft=3 -> 2)
	// P2 moves to (4,2)
	r2, c2 := 4, 2
	sendMessage(h, clients[1], &Message{
		Type: "move",
		GameID: gameID,
		Row: &r2, Col: &c2,
	})

	// C2 should receive ITS OWN move_made now
	waitForMessage(t, clients[1], "move_made")

	// Wait a bit for processing to update internal state (movesLeft)
	// move_made is broadcasted, but we need to check internal state
	// It's usually sync in hub thread but we read it from test thread.
	// Since handleMessage processes sequentially, if we got move_made, state should be updated.

	if game.CurrentPlayer != 2 {
		t.Error("Turn should still be Player 2")
	}

	if game.MovesLeft != 2 {
		t.Errorf("Expected 2 moves left, got %d", game.MovesLeft)
	}
}

func TestHub_Attack(t *testing.T) {
	h := newHub()
	go h.run()

	clients := make([]*Client, 2)
	for i := 0; i < 2; i++ {
		clients[i] = &Client{hub: h, send: make(chan []byte, 256)}
		h.register <- clients[i]
		drainWelcome(clients[i])
	}

	gameID := "test-attack"
	game := &Game{
		ID: gameID,
		Rows: 5, Cols: 5,
		CurrentPlayer: 1,
		MovesLeft: 3,
		Board: make(Board, 5),
		Player1: clients[0].user,
		Player2: clients[1].user,
		Player1Base: CellPos{0,0},
		Player2Base: CellPos{4,4},
	}
	for i := range game.Board {
		game.Board[i] = make([]CellValue, 5)
	}
	h.games[gameID] = game
	clients[0].user.InGame = true
	clients[0].user.GameID = gameID

	// Setup P1 attacking P2
	game.Board[0][0] = NewCell(1, CellFlagBase)
	game.Board[0][1] = NewCell(1, CellFlagNormal)

	// P2 piece at (0,2)
	game.Board[0][2] = NewCell(2, CellFlagNormal)

	// P1 attacks (0,2)
	r, c := 0, 2
	sendMessage(h, clients[0], &Message{
		Type: "move",
		GameID: gameID,
		Row: &r, Col: &c,
	})

	waitForMessage(t, clients[0], "move_made")

	// Check if cell is fortified
	cell := game.Board[0][2]
	if cell.Player() != 1 {
		t.Errorf("Expected cell to be owned by Player 1, got %d", cell.Player())
	}
	if !cell.IsFortified() {
		t.Error("Expected cell to be fortified after attack")
	}
}
