package main

import (
	"encoding/json"
	"testing"
	"time"
)

// Helper to drain specific message type or return nil
func waitForMessageTimeout(t *testing.T, c *Client, msgType string, timeout time.Duration) *Message {
	timeoutChan := time.After(timeout)
	for {
		select {
		case msgBytes := <-c.send:
			var msg Message
			// We ignore unmarshal errors here for simplicity as we look for specific type
			// But robust test should handle it
			_ = json.Unmarshal(msgBytes, &msg)
			if msg.Type == msgType {
				return &msg
			}
		case <-timeoutChan:
			return nil
		}
	}
}

func TestHub_Logic_IllegalMove(t *testing.T) {
	h := newHub()
	go h.run()

	c1 := &Client{hub: h, send: make(chan []byte, 256)}
	c2 := &Client{hub: h, send: make(chan []byte, 256)}
	h.register <- c1
	h.register <- c2
	drainWelcome(c1)
	drainWelcome(c2)

	// We need to manually create game since createTestGame is in another file (hub_game_test.go).
	// In Go tests in same package share scope, so it should be available?
	// It says undefined.
	// Ah, createTestGame is in hub_game_test.go which is package main.
	// It should be available if we run `go test ./...` in backend dir.
	// But `go tool cover` might be compiling files differently?
	// No, normally all *_test.go files in same package are compiled together.
	// Maybe createTestGame is not exported? It starts with lowercase. But same package is fine.

	// I will just redefine a helper here to be safe and avoid "redeclaration" errors if I export it later.

	game := &Game{
		ID:            "test-illegal-move-1v1",
		Player1:       c1.user,
		Player2:       c2.user,
		CurrentPlayer: 1,
		MovesLeft:     1,
		Rows:          5, Cols: 5,
		Board: make(Board, 5),
	}
	for i := range game.Board {
		game.Board[i] = make([]CellValue, 5)
	}
	runOnHub(h, func() {
		h.games[game.ID] = game
		c1.user.InGame = true
		c1.user.GameID = game.ID
		c2.user.InGame = true
		c2.user.GameID = game.ID
	})

	// Call handleIllegalMove directly or trigger it
	// Triggering via handleMove with invalid move covers path

	// Try to move to existing cell (invalid target, but not out of bounds)
	// game.Board[0][0] is P1 Base. P1 tries to move there? No, P1 owns it.
	// P1 tries to move to P2 base?

	// Create scenario where P1 makes illegal move that isn't just "invalid move" error but triggers defeat?
	// The prompt said: "server enforces a 'Defeat on Illegal Move' rule where any player ... attempting an invalid move ... is immediately eliminated"
	// handleMove calls isValidMove. If false, it calls handleIllegalMove.

	// Let's verify handleIllegalMove specifically for Multiplayer elimination vs 1v1 game end

	// 1v1 Case
	runOnHub(h, func() { h.handleIllegalMove(game, 1, "test reason") })

	// Should receive game_end
	msg := waitForMessage(t, c2, "game_end")
	if msg == nil {
		t.Error("Expected game_end after illegal move")
	}
	if msg.Winner != 2 {
		t.Errorf("Expected winner 2, got %d", msg.Winner)
	}

	// Multiplayer Case
	// Create 3 player game
	c3 := &Client{hub: h, send: make(chan []byte, 256)}
	h.register <- c3
	drainWelcome(c3)

	// Manually setup multiplayer game
	mpGameID := "mp-illegal-test"
	mpGame := &Game{
		ID:            mpGameID,
		IsMultiplayer: true,
		ActivePlayers: 3,
		Rows:          5, Cols: 5,
		CurrentPlayer: 1,
		MovesLeft:     3,
		Board:         make(Board, 5),
	}
	for i := range mpGame.Board {
		mpGame.Board[i] = make([]CellValue, 5)
	}

	// Setup players
	mpGame.Players[0] = &LobbyPlayer{User: c1.user, Index: 0}
	mpGame.Players[1] = &LobbyPlayer{User: c2.user, Index: 1}
	mpGame.Players[2] = &LobbyPlayer{User: c3.user, Index: 2}

	// Give pieces
	mpGame.Board[0][0] = NewCell(1, CellFlagBase)
	mpGame.Board[4][4] = NewCell(2, CellFlagBase)
	mpGame.Board[0][4] = NewCell(3, CellFlagBase)

	runOnHub(h, func() {
		h.games[mpGameID] = mpGame

		// Player 1 makes illegal move
		h.handleIllegalMove(mpGame, 1, "bad move")
	})

	// Should receive player_eliminated
	msg = waitForMessage(t, c2, "player_eliminated")
	if msg == nil {
		t.Error("Expected player_eliminated")
	}
	if msg.EliminatedPlayer != 1 {
		t.Errorf("Expected eliminated player 1, got %d", msg.EliminatedPlayer)
	}

	// Verify pieces removed
	var eliminatedPiece CellValue
	runOnHub(h, func() { eliminatedPiece = mpGame.Board[0][0] })
	if eliminatedPiece != 0 {
		t.Error("Eliminated player pieces should be removed")
	}
}

func TestHub_Logic_DisconnectCleanup(t *testing.T) {
	h := newHub()
	go h.run()

	c1 := &Client{hub: h, send: make(chan []byte, 256)}
	h.register <- c1
	drainWelcome(c1)

	// Setup user in lobby
	lobbyID := "test-lobby"
	lobby := &Lobby{
		ID:         lobbyID,
		MaxPlayers: 4,
		Host:       c1.user,
	}
	lobby.Players[0] = &LobbyPlayer{User: c1.user, Index: 0}
	runOnHub(h, func() {
		h.lobbies[lobbyID] = lobby
		c1.user.InLobby = true
		c1.user.LobbyID = lobbyID

		// Disconnect
		h.handleDisconnect(c1)
	})

	// Lobby should be closed (host left)
	var lobbyExists bool
	runOnHub(h, func() { _, lobbyExists = h.lobbies[lobbyID] })
	if lobbyExists {
		t.Error("Lobby should be closed when host disconnects")
	}
}

func TestHub_Logic_WinCondition(t *testing.T) {
	h := newHub()

	game := &Game{
		ID:   "win-check",
		Rows: 3, Cols: 3,
		Board:   make(Board, 3),
		Player1: &User{ID: "p1"},
		Player2: &User{ID: "p2"},
	}
	for i := range game.Board {
		game.Board[i] = make([]CellValue, 3)
	}

	// P1 has pieces, P2 has none
	game.Board[0][0] = NewCell(1, CellFlagBase)

	h.checkWinCondition(game)

	if !game.GameOver {
		t.Error("Game should be over (P2 has no pieces)")
	}
	if game.Winner != 1 {
		t.Errorf("Expected Winner 1, got %d", game.Winner)
	}
}

func TestHub_Logic_EliminateDisconnected(t *testing.T) {
	h := newHub()

	// Multiplayer Game
	game := &Game{
		ID:            "elim-check",
		IsMultiplayer: true,
		Rows:          5, Cols: 5,
		Board:         make(Board, 5),
		ActivePlayers: 2,
	}
	for i := range game.Board {
		game.Board[i] = make([]CellValue, 5)
	}

	// P1 Base at (0,0)
	game.Board[0][0] = NewCell(1, CellFlagBase)
	// P1 Piece at (0,1) connected
	game.Board[0][1] = NewCell(1, CellFlagNormal)

	// P2 Base at (4,4)
	game.Board[4][4] = NewCell(2, CellFlagBase)
	// P2 Piece at (2,2) DISCONNECTED
	game.Board[2][2] = NewCell(2, CellFlagNormal)

	// Setup players so they are not nil
	game.Players[0] = &LobbyPlayer{Index: 0}
	game.Players[1] = &LobbyPlayer{Index: 1}

	// Setup PlayerBases
	game.PlayerBases[0] = CellPos{0, 0}
	game.PlayerBases[1] = CellPos{4, 4}

	// P2 has pieces but (2,2) is disconnected from base.
	// Can P2 make a move?
	// isValidMove checks connectivity to base.
	// Adjacent to (2,2) is (2,3), (3,2) etc.
	// Is (2,2) connected to base? NO.
	// So any move adjacent to (2,2) is invalid.
	// Adjacent to Base (4,4)? Yes, (4,3) etc.
	// So P2 CAN make a move adjacent to base.

	// We need to completely block P2.
	// Surround P2 base with P1 pieces or Neutral.
	game.Board[4][3] = NewCell(0, CellFlagKilled)
	game.Board[3][4] = NewCell(0, CellFlagKilled)
	game.Board[3][3] = NewCell(0, CellFlagKilled)

	// Now P2 Base has no valid expansion.
	// P2 Piece at (2,2) is disconnected, so cannot expand from there.
	// So P2 has pieces but 0 valid moves.

	h.eliminateDisconnectedPlayers(game)

	// P2 pieces should be removed
	if game.Board[2][2] != 0 {
		t.Error("P2 disconnected pieces should be removed")
	}
	// Base should also be removed (all pieces)
	if game.Board[4][4] != 0 {
		t.Error("P2 base should be removed")
	}
}
