package main

import (
	"encoding/json"
	"testing"
	"time"
)

// Helper function to drain welcome messages
func drainWelcome(c *Client) {
	timeout := time.After(100 * time.Millisecond)
	select {
	case <-c.send:
	case <-timeout:
	}
}

// Helper to wait for specific message type
func waitForMessage(t *testing.T, c *Client, msgType string) *Message {
	timeout := time.After(1 * time.Second)
	for {
		select {
		case msgBytes := <-c.send:
			var msg Message
			if err := json.Unmarshal(msgBytes, &msg); err != nil {
				continue // skip invalid json or keep trying
			}
			if msg.Type == msgType {
				return &msg
			}
		case <-timeout:
			t.Errorf("Timeout waiting for message type: %s", msgType)
			return nil
		}
	}
}

// Helper to send message via handleMessage channel
func sendMessage(h *Hub, c *Client, msg *Message) {
	h.handleMessage <- &MessageWrapper{
		client:  c,
		message: msg,
	}
}

// runOnHub executes fixture setup and observations under the same ownership
// discipline as production Hub mutations. The acknowledgement is also a
// deterministic barrier for preceding events.
func runOnHub(h *Hub, apply func()) {
	done := make(chan struct{})
	h.commands <- hubCommand{apply: apply, done: done}
	<-done
}

func TestHubIntegration_Connect(t *testing.T) {
	h := newHub()
	go h.run()

	client := &Client{
		hub:  h,
		send: make(chan []byte, 256),
	}

	// Register client
	h.register <- client

	// Should receive welcome message
	msg := waitForMessage(t, client, "welcome")
	if msg == nil {
		return
	}

	// Cleanup
	h.unregister <- client
}

func TestHubIntegration_LobbyAndGame(t *testing.T) {
	h := newHub()
	go h.run()

	// Client 1
	c1 := &Client{hub: h, send: make(chan []byte, 256)}
	h.register <- c1

	// Client 2
	c2 := &Client{hub: h, send: make(chan []byte, 256)}
	h.register <- c2

	waitForMessage(t, c1, "welcome")
	waitForMessage(t, c2, "welcome")

	// Get users (handleConnect sets them)
	// We check for not nil to satisfy unused variable check if logic changes
	if c1.user == nil || c2.user == nil {
		t.Error("Users should be set")
		return
	}

	// Client 1 challenges Client 2
	challengeMsg := &Message{
		Type:         "challenge",
		TargetUserID: c2.user.ID,
		Rows:         10,
		Cols:         10,
	}
	sendMessage(h, c1, challengeMsg)

	// We expect C2 to receive a challenge message.
	waitForMessage(t, c2, "challenge_received")

	// Verify Challenge exists
	var challengeCount int
	var challengeID string
	runOnHub(h, func() {
		challengeCount = len(h.challenges)
		for id := range h.challenges {
			challengeID = id
		}
	})
	if challengeCount != 1 {
		t.Errorf("Expected 1 challenge, got %d", challengeCount)
	}

	// Client 2 accepts
	acceptMsg := &Message{
		Type:        "accept_challenge",
		ChallengeID: challengeID,
	}
	sendMessage(h, c2, acceptMsg)

	// Expect Game Start messages for both
	waitForMessage(t, c1, "game_start")
	waitForMessage(t, c2, "game_start")

	var gameCount int
	runOnHub(h, func() { gameCount = len(h.games) })
	if gameCount != 1 {
		t.Errorf("Expected 1 active game, got %d", gameCount)
	}
}

func TestHubIntegration_Move(t *testing.T) {
	h := newHub()
	go h.run()

	c1 := &Client{hub: h, send: make(chan []byte, 256)}
	c2 := &Client{hub: h, send: make(chan []byte, 256)}
	h.register <- c1
	h.register <- c2

	waitForMessage(t, c1, "welcome")
	waitForMessage(t, c2, "welcome")

	u1 := c1.user
	u2 := c2.user

	// Create game manually to skip challenge flow
	gameID := "test-game-move"
	rows, cols := 5, 5
	board := make(Board, rows)
	for i := range board {
		board[i] = make([]CellValue, cols)
	}
	game := &Game{
		ID:            gameID,
		Player1:       u1,
		Player2:       u2,
		Board:         board,
		Rows:          rows,
		Cols:          cols,
		CurrentPlayer: 1,
		MovesLeft:     1,
		Player1Base:   CellPos{0, 0},
		Player2Base:   CellPos{4, 4},
		// Initialize history to avoid panic
		MoveHistory:    []MoveAction{},
		LastActionTime: time.Now(),
	}
	game.Board[0][0] = NewCell(1, CellFlagBase)
	game.Board[0][1] = NewCell(1, CellFlagNormal)
	game.Board[4][4] = NewCell(2, CellFlagBase)

	runOnHub(h, func() {
		h.games[gameID] = game
		u1.InGame = true
		u1.GameID = gameID
		u2.InGame = true
		u2.GameID = gameID
	})

	// Player 1 makes a move at (1,1) (valid)
	r, c := 1, 1
	moveMsg := &Message{
		Type:   "move",
		GameID: gameID,
		Row:    &r,
		Col:    &c,
	}
	sendMessage(h, c1, moveMsg)

	// Wait for move_made message
	waitForMessage(t, c1, "move_made")
	waitForMessage(t, c2, "move_made")

	// Check if move was applied
	var movedPlayer, currentPlayer int
	runOnHub(h, func() {
		movedPlayer = game.Board[1][1].Player()
		currentPlayer = game.CurrentPlayer
	})
	if movedPlayer != 1 {
		t.Error("Board not updated after move")
	}

	// Turn should change to Player 2
	waitForMessage(t, c1, "turn_change")
	waitForMessage(t, c2, "turn_change")

	if currentPlayer != 2 {
		t.Error("Turn should switch to Player 2")
	}
}

func TestHubIntegration_Neutrals(t *testing.T) {
	h := newHub()
	go h.run()

	c1 := &Client{hub: h, send: make(chan []byte, 256)}
	c2 := &Client{hub: h, send: make(chan []byte, 256)}
	h.register <- c1
	h.register <- c2

	waitForMessage(t, c1, "welcome")
	waitForMessage(t, c2, "welcome")

	u1 := c1.user
	u2 := c2.user

	rows, cols := 5, 5
	board := make(Board, rows)
	for i := range board {
		board[i] = make([]CellValue, cols)
	}
	gameID := "test-neutrals"
	game := &Game{
		ID:             gameID,
		Player1:        u1,
		Player2:        u2,
		Board:          board,
		Rows:           rows,
		Cols:           cols,
		CurrentPlayer:  1,
		MovesLeft:      3,
		Player1Base:    CellPos{0, 0},
		Player2Base:    CellPos{4, 4},
		MoveHistory:    []MoveAction{},
		LastActionTime: time.Now(),
	}
	game.Board[0][0] = NewCell(1, CellFlagBase)
	game.Board[0][1] = NewCell(1, CellFlagNormal)
	game.Board[1][0] = NewCell(1, CellFlagNormal)
	// P2 needs a base so endTurn passes the turn instead of ending the game on a
	// player who cannot move (vs-ai2.58: neutrals now route through endTurn).
	game.Board[4][4] = NewCell(2, CellFlagBase)

	runOnHub(h, func() {
		h.games[gameID] = game
		u1.InGame = true
		u1.GameID = gameID
	})

	neutralsMsg := &Message{
		Type:   "neutrals",
		GameID: gameID,
		Cells: []CellPos{
			{0, 1},
			{1, 0},
		},
	}
	sendMessage(h, c1, neutralsMsg)

	// Should receive neutrals_placed
	waitForMessage(t, c2, "neutrals_placed")

	// Should receive turn_change
	waitForMessage(t, c1, "turn_change")

	var killed bool
	runOnHub(h, func() { killed = game.Board[0][1].IsKilled() })
	if !killed {
		t.Error("Cell (0,1) should be killed")
	}
}

func TestHubIntegration_Resign(t *testing.T) {
	h := newHub()
	go h.run()

	c1 := &Client{hub: h, send: make(chan []byte, 256)}
	c2 := &Client{hub: h, send: make(chan []byte, 256)}
	h.register <- c1
	h.register <- c2

	waitForMessage(t, c1, "welcome")
	waitForMessage(t, c2, "welcome")

	u1 := c1.user
	u2 := c2.user

	gameID := "test-resign"
	game := &Game{
		ID:            gameID,
		Player1:       u1,
		Player2:       u2,
		CurrentPlayer: 1,
		MovesLeft:     3,
	}
	runOnHub(h, func() {
		h.games[gameID] = game
		u1.InGame = true
		u1.GameID = gameID
		u2.InGame = true
		u2.GameID = gameID
	})

	resignMsg := &Message{
		Type:   "resign",
		GameID: gameID,
	}
	sendMessage(h, c1, resignMsg)

	waitForMessage(t, c2, "game_end")

	var gameOver bool
	var winner int
	runOnHub(h, func() {
		gameOver = game.GameOver
		winner = game.Winner
	})
	if !gameOver {
		t.Error("Game should be over")
	}
	if winner != 2 {
		t.Errorf("Player 2 should win, got %d", winner)
	}
}

func TestHubIntegration_MultiplayerLobby(t *testing.T) {
	h := newHub()
	go h.run()

	// Create 3 clients
	clients := make([]*Client, 3)
	for i := 0; i < 3; i++ {
		clients[i] = &Client{hub: h, send: make(chan []byte, 256)}
		h.register <- clients[i]
	}

	for i := 0; i < 3; i++ {
		waitForMessage(t, clients[i], "welcome")
	}

	// 1. Host creates lobby
	createMsg := &Message{
		Type: "create_lobby",
		Rows: 10,
		Cols: 10,
	}
	sendMessage(h, clients[0], createMsg)

	// Wait for lobby_created
	msg := waitForMessage(t, clients[0], "lobby_created")
	if msg == nil {
		return
	}
	lobbyID := msg.LobbyID

	// 2. Others join lobby
	joinMsg := &Message{
		Type:    "join_lobby",
		LobbyID: lobbyID,
	}
	sendMessage(h, clients[1], joinMsg)
	waitForMessage(t, clients[1], "lobby_joined")

	sendMessage(h, clients[2], joinMsg)
	waitForMessage(t, clients[2], "lobby_joined")

	// 3. Start Game
	startMsg := &Message{
		Type: "start_multiplayer_game",
	}
	sendMessage(h, clients[0], startMsg)

	// Wait for multiplayer_game_start
	waitForMessage(t, clients[0], "multiplayer_game_start")
	waitForMessage(t, clients[1], "multiplayer_game_start")
	waitForMessage(t, clients[2], "multiplayer_game_start")

	if len(h.games) != 1 {
		t.Errorf("Expected 1 active game, got %d", len(h.games))
	}

	// Safe to read h.games keys?
	var game *Game
	for _, g := range h.games {
		game = g
		break
	}

	if !game.IsMultiplayer {
		t.Error("Game should be multiplayer")
	}
	if game.ActivePlayers != 3 {
		t.Errorf("Expected 3 active players, got %d", game.ActivePlayers)
	}
}
