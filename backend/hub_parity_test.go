package main

import (
	"path/filepath"
	"testing"
)

// vs-ai2.58 hub rules parity: eliminated players' cells STAY on the board (owned
// and capturable per normal rules); aliveness is the Eliminated flag, never a
// piece count; a neutral placement routes through endTurn so the next player gets
// a timer and stuck players are eliminated (no freeze); the last non-eliminated
// player wins and the game is persisted.

func parityUser(id string) *User {
	return &User{ID: id, Username: id, Client: &Client{send: make(chan []byte, 256)}}
}

// buildParity3p returns a 3-player game where player 2 is stuck (base cornered by
// killed walls, plus one disconnected stray at (2,2)), player 1 can capture that
// stray, and players 1 and 3 can both move. All three players are human.
func buildParity3p() (*Game, *User, *User, *User) {
	u1, u2, u3 := parityUser("p1"), parityUser("p2"), parityUser("p3")
	board := make(Board, 5)
	for i := range board {
		board[i] = make([]CellValue, 5)
	}
	// P1: base + a connected normal adjacent to P2's stray at (2,2).
	board[0][0] = NewCell(1, CellFlagBase)
	board[1][1] = NewCell(1, CellFlagNormal)
	// P2: cornered base (only 3 neighbours, all killed) => no legal move, plus a
	// disconnected stray at (2,2) that stays owned and capturable after elimination.
	board[4][4] = NewCell(2, CellFlagBase)
	board[4][3] = NewCell(0, CellFlagKilled)
	board[3][4] = NewCell(0, CellFlagKilled)
	board[3][3] = NewCell(0, CellFlagKilled)
	board[2][2] = NewCell(2, CellFlagNormal)
	// P3: base + a connected normal so it can always move.
	board[0][4] = NewCell(3, CellFlagBase)
	board[1][3] = NewCell(3, CellFlagNormal)

	game := &Game{
		ID:    "parity-3p",
		Board: board,
		Rows:  5, Cols: 5,
		CurrentPlayer: 1,
		MovesLeft:     3,
		IsMultiplayer: true,
		Players: [4]*LobbyPlayer{
			{User: u1, Index: 0}, {User: u2, Index: 1}, {User: u3, Index: 2}, nil,
		},
		PlayerBases:   [4]CellPos{{0, 0}, {4, 4}, {0, 4}, {}},
		ActivePlayers: 3,
		MoveHistory:   []MoveAction{},
	}
	return game, u1, u2, u3
}

// assertPlayer2EliminatedButPresent checks the vs-ai2.58 invariants that must hold
// no matter which path eliminated player 2.
func assertPlayer2EliminatedButPresent(t *testing.T, h *Hub, game *Game) {
	t.Helper()
	if !game.Eliminated[1] {
		t.Error("player 2 should be marked eliminated")
	}
	if game.playerActive(2) {
		t.Error("eliminated player 2 must not be active")
	}
	// Cells stay owned: base and stray both still belong to player 2.
	if game.Board[4][4].Player() != 2 {
		t.Error("eliminated player 2 base should remain owned")
	}
	if game.Board[2][2].Player() != 2 || !game.Board[2][2].CanBeAttacked() {
		t.Error("eliminated player 2 stray should remain owned and capturable")
	}
	// Capturable per normal rules: player 1 (connected via (1,1)) may attack (2,2).
	if !h.isValidMove(game, 2, 2, 1) {
		t.Error("player 1 should be able to capture eliminated player 2's cell at (2,2)")
	}
	// Excluded from rotation: a turn ending on player 1 skips player 2 to player 3.
	game.CurrentPlayer = 1
	game.GameOver = false
	h.endTurn(game)
	if game.CurrentPlayer != 3 {
		t.Errorf("rotation should skip eliminated player 2 and land on player 3, got %d", game.CurrentPlayer)
	}
}

func TestParity_EliminationPathsKeepCellsOwned(t *testing.T) {
	t.Run("no_moves", func(t *testing.T) {
		h := newHub()
		game, _, _, _ := buildParity3p()
		h.games[game.ID] = game
		// Stuck detection after a move eliminates player 2 by flag.
		h.eliminateDisconnectedPlayers(game)
		assertPlayer2EliminatedButPresent(t, h, game)
	})

	t.Run("resign", func(t *testing.T) {
		h := newHub()
		game, _, u2, _ := buildParity3p()
		h.games[game.ID] = game
		u2.InGame, u2.GameID = true, game.ID
		h.handleResign(u2, &Message{GameID: game.ID})
		assertPlayer2EliminatedButPresent(t, h, game)
	})

	t.Run("illegal", func(t *testing.T) {
		h := newHub()
		game, _, _, _ := buildParity3p()
		h.games[game.ID] = game
		// Player 2 is not the current player, so no auto-endTurn interferes.
		h.handleIllegalMove(game, 2, "test illegal")
		assertPlayer2EliminatedButPresent(t, h, game)
	})

	t.Run("timeout", func(t *testing.T) {
		h := newHub()
		game, _, u2, _ := buildParity3p()
		h.games[game.ID] = game
		u2.InGame, u2.GameID = true, game.ID
		// Timeout only fires for the current player.
		game.CurrentPlayer = 2
		h.handleMoveTimeout(&Message{GameID: game.ID, Player: 2})
		assertPlayer2EliminatedButPresent(t, h, game)
	})
}

// TestParity_NeutralPlacementDoesNotFreeze reproduces the vs-ai2.58 P1-B freeze:
// after a neutral placement the next player was rotated to with no move timer and
// stuck players were never eliminated. Routing through endTurn fixes both.
func TestParity_NeutralPlacementDoesNotFreeze(t *testing.T) {
	h := newHub()
	game, u1, _, _ := buildParity3p()
	// Give player 1 two spare normals to sacrifice as neutrals.
	game.Board[0][1] = NewCell(1, CellFlagNormal)
	game.Board[1][0] = NewCell(1, CellFlagNormal)
	h.games[game.ID] = game
	u1.InGame, u1.GameID = true, game.ID

	h.handleNeutrals(u1, &Message{
		GameID: game.ID,
		Cells:  []CellPos{{0, 1}, {1, 0}},
	})

	// Player 2 (stuck) must have been eliminated during rotation, the turn must
	// have advanced to player 3, and a move timer must be armed for them.
	if !game.Eliminated[1] {
		t.Error("stuck player 2 should be eliminated during neutral-turn rotation")
	}
	if game.Board[4][4].Player() != 2 {
		t.Error("eliminated player 2's cells should remain owned after neutral placement")
	}
	if game.CurrentPlayer != 3 {
		t.Errorf("turn should advance to player 3, got %d", game.CurrentPlayer)
	}
	if game.MoveTimer == nil {
		t.Error("a move timer must be started for the next player (no freeze)")
	}
	if game.GameOver {
		t.Error("game should proceed, not end")
	}
}

// TestParity_LastNonEliminatedWinsAndPersists checks winner determination from the
// flag and that the terminal game reaches persistence.
func TestParity_LastNonEliminatedWinsAndPersists(t *testing.T) {
	InitDB(filepath.Join(t.TempDir(), "parity_winner.db"))
	t.Cleanup(closePersistenceTestDB)

	h := newHub()
	game, _, _, _ := buildParity3p()
	// Repair player 2's board so no path auto-eliminates anyone unexpectedly.
	h.games[game.ID] = game

	h.eliminatePlayer(game, 1)
	h.eliminatePlayer(game, 2)
	h.checkMultiplayerStatus(game)

	if !game.GameOver {
		t.Fatal("game should be over when only one player remains")
	}
	if game.Winner != 3 {
		t.Errorf("winner should be the last non-eliminated player 3, got %d", game.Winner)
	}
	if !game.persisted {
		t.Error("terminal game should be persisted")
	}
}
