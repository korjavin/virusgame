package main

// Read-only board snapshot attached to game messages for bot clients
// (nnue-trainer JavaBot's GameLoopHandler requires it to pick moves).
// Recreates the lost local patch the July 2026 eval ran with. No game logic
// reads any of this; it is serialize-only.
//
// Kind values match the JavaBot's CellKind enum:
// 0 empty, 1 normal, 2 base, 3 fortified, 4 neutral(killed).

type GameSnapshot struct {
	Rows          int          `json:"rows"`
	Cols          int          `json:"cols"`
	CurrentPlayer int          `json:"currentPlayer"`
	MovesLeft     int          `json:"movesLeft"`
	GameOver      bool         `json:"gameOver"`
	Winner        int          `json:"winner"`
	NeutralUsed   []bool       `json:"neutralUsed"`
	Board         [][]SnapCell `json:"board"`
}

type SnapCell struct {
	Owner int `json:"owner"`
	Kind  int `json:"kind"`
}

func snapshotOf(game *Game) *GameSnapshot {
	board := make([][]SnapCell, len(game.Board))
	for r, row := range game.Board {
		board[r] = make([]SnapCell, len(row))
		for c, cell := range row {
			kind := 0
			switch {
			case cell.IsBase():
				kind = 2
			case cell.IsFortified():
				kind = 3
			case cell.IsKilled():
				kind = 4
			case cell.Player() != 0:
				kind = 1
			}
			board[r][c] = SnapCell{Owner: cell.Player(), Kind: kind}
		}
	}
	var neutralUsed []bool
	if game.IsMultiplayer {
		neutralUsed = game.NeutralsUsed[:]
	} else {
		neutralUsed = []bool{game.Player1NeutralsUsed, game.Player2NeutralsUsed}
	}
	return &GameSnapshot{
		Rows:          game.Rows,
		Cols:          game.Cols,
		CurrentPlayer: game.CurrentPlayer,
		MovesLeft:     game.MovesLeft,
		GameOver:      game.GameOver,
		Winner:        game.Winner,
		NeutralUsed:   neutralUsed,
		Board:         board,
	}
}
