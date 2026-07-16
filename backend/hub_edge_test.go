package main

import (
	"encoding/json"
	"testing"
	"time"
)

func TestHubLogic_IllegalMove(t *testing.T) {
	h := newHub()
	// No h.run() needed for logic test if we don't rely on channel processing

	// Setup 1v1 game
	u1 := &User{ID: "u1", Username: "P1", Client: &Client{send: make(chan []byte, 10)}}
	u2 := &User{ID: "u2", Username: "P2", Client: &Client{send: make(chan []byte, 10)}}

	game := &Game{
		ID:      "illegal-test",
		Player1: u1,
		Player2: u2,
		Board:   make(Board, 5),
		Rows:    5, Cols: 5,
		IsMultiplayer: false,
		MoveHistory:   []MoveAction{},
	}
	for i := range game.Board {
		game.Board[i] = make([]CellValue, 5)
	}
	// Give P1 some pieces
	game.Board[0][0] = NewCell(1, CellFlagBase)
	game.Board[0][1] = NewCell(1, CellFlagNormal)

	h.games[game.ID] = game
	u1.InGame = true
	u1.GameID = game.ID

	// Call handleIllegalMove
	h.handleIllegalMove(game, 1, "Testing illegal move")

	// Verify P1 pieces removed
	if game.Board[0][0] != 0 || game.Board[0][1] != 0 {
		t.Error("Illegal move should remove player pieces")
	}

	// Verify Game Over (since 1v1)
	if !game.GameOver {
		t.Error("1v1 game should end on illegal move")
	}
	if game.Winner != 2 {
		t.Errorf("Winner should be 2, got %d", game.Winner)
	}
}

func TestHubLogic_MoveTimeout(t *testing.T) {
	h := newHub()

	u1 := &User{ID: "u1", Username: "P1", Client: &Client{send: make(chan []byte, 10)}}
	u2 := &User{ID: "u2", Username: "P2", Client: &Client{send: make(chan []byte, 10)}}

	// 1. Multiplayer Timeout
	// Need to register players in game
	p1 := &LobbyPlayer{User: u1, Index: 0}
	p2 := &LobbyPlayer{User: u2, Index: 1}

	game := &Game{
		ID:            "timeout-test",
		IsMultiplayer: true,
		Players:       [4]*LobbyPlayer{p1, p2, nil, nil},
		Board:         make(Board, 5),
		Rows:          5, Cols: 5,
		CurrentPlayer: 1,
		ActivePlayers: 2,
		MoveHistory:   []MoveAction{},
	}
	for i := range game.Board {
		game.Board[i] = make([]CellValue, 5)
	}
	// Give P1 pieces
	game.Board[0][0] = NewCell(1, CellFlagBase)

	h.games[game.ID] = game
	u1.InGame = true
	u1.GameID = game.ID

	// Simulate timeout message
	msg := &Message{
		GameID: game.ID,
		Player: 1,
	}

	h.handleMoveTimeout(msg)

	// In multiplayer, human timeout calls handleResign.
	// handleResign checks if user is in game.
	// We need to verify P1 resigned.
	// Pieces should be killed (neutral).
	if !game.Board[0][0].IsKilled() {
		t.Error("Timeout/Resign should kill pieces in multiplayer")
	}

	// 2. Bot Timeout
	botPlayer := &LobbyPlayer{IsBot: true, Index: 2} // Player 3
	game.Players[2] = botPlayer
	game.Board[0][4] = NewCell(3, CellFlagBase)
	game.CurrentPlayer = 3
	game.GameOver = false

	msgBot := &Message{GameID: game.ID, Player: 3}
	h.handleMoveTimeout(msgBot)

	// Bot pieces should be removed (0), not killed?
	// handleMoveTimeout for bot:
	// "Remove all pieces for this bot... game.Board[i][j] = 0"
	if game.Board[0][4] != 0 {
		t.Error("Bot timeout should remove pieces")
	}
}

func TestHubIntegration_InvalidMoves(t *testing.T) {
	h := newHub()
	go h.run()

	c1 := &Client{hub: h, send: make(chan []byte, 256)}
	c2 := &Client{hub: h, send: make(chan []byte, 256)}
	h.register <- c1
	h.register <- c2
	waitForMessage(t, c1, "welcome")
	waitForMessage(t, c2, "welcome")

	// Start Game
	sendMessage(h, c1, &Message{Type: "challenge", TargetUserID: c2.user.ID, Rows: 5, Cols: 5})
	msg := waitForMessage(t, c2, "challenge_received")
	sendMessage(h, c2, &Message{Type: "accept_challenge", ChallengeID: msg.ChallengeID})
	waitForMessage(t, c1, "game_start")
	waitForMessage(t, c2, "game_start")

	// Drain users_update messages that are broadcast after game start
	drainWelcome(c1)
	drainWelcome(c2)

	// Game setup: P1 at (0,0), P2 at (4,4)
	// P1 turn.

	// 1. Invalid Move: Disconnected
	r, c := 2, 2
	sendMessage(h, c1, &Message{Type: "move", GameID: c1.user.GameID, Row: &r, Col: &c})

	// Should receive error "Defeated by illegal move"
	// And game end.

	// Wait for possible error or game end
	// Note: handleIllegalMove sends "error" message to user, then "game_end"

	// We might need to drain unrelated messages?
	// Or loop until we find error/game_end

	foundError := false
	foundEnd := false

	timeout := time.After(1 * time.Second)
	for !foundEnd {
		select {
		case msgBytes := <-c1.send:
			var m Message
			json.Unmarshal(msgBytes, &m)
			t.Logf("Received message type: %s", m.Type)
			if m.Type == "error" {
				foundError = true
			}
			if m.Type == "game_end" {
				foundEnd = true
			}
		case <-timeout:
			t.Error("Timeout waiting for illegal move consequence")
			return
		}
	}

	if !foundError {
		t.Error("Expected error message for illegal move")
	}
}

func TestHubIntegration_GameEndMultiplayer(t *testing.T) {
	h := newHub()
	go h.run()

	// Create 3 players
	clients := make([]*Client, 3)
	for i := 0; i < 3; i++ {
		clients[i] = &Client{hub: h, send: make(chan []byte, 256)}
		h.register <- clients[i]
	}
	for i := 0; i < 3; i++ {
		waitForMessage(t, clients[i], "welcome")
	}

	// Start Multiplayer Game
	sendMessage(h, clients[0], &Message{Type: "create_lobby", Rows: 5, Cols: 5})
	msg := waitForMessage(t, clients[0], "lobby_created")
	lobbyID := msg.LobbyID

	sendMessage(h, clients[1], &Message{Type: "join_lobby", LobbyID: lobbyID})
	waitForMessage(t, clients[1], "lobby_joined")

	sendMessage(h, clients[2], &Message{Type: "join_lobby", LobbyID: lobbyID})
	waitForMessage(t, clients[2], "lobby_joined")

	sendMessage(h, clients[0], &Message{Type: "start_multiplayer_game"})

	// Wait for game start
	for i := 0; i < 3; i++ {
		waitForMessage(t, clients[i], "multiplayer_game_start")
	}
	drainWelcome(clients[0]) // users_update

	// P1 Resigns
	gameID := clients[0].user.GameID
	sendMessage(h, clients[0], &Message{Type: "resign", GameID: gameID})

	// Should NOT end game yet (2 players left)
	// Check users_update not sent implies game not ended? No.
	// Check players count?
	// We can check if P2 receives turn_change (if P1 was current)
	// P1 starts at 1. So P2 should get turn.
	waitForMessage(t, clients[1], "turn_change")

	// P2 Resigns
	sendMessage(h, clients[1], &Message{Type: "resign", GameID: gameID})

	// Now only P3 left. Game should end.
	waitForMessage(t, clients[2], "game_end")
}
