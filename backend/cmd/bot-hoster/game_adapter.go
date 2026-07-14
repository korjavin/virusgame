package main

import "virusgame/game"

// legacyGameState is a temporary adapter for the engines retired by vs-dmv.4.
// The authoritative position remains game.State.
func legacyGameState(position game.State, players []GamePlayerInfo, botPlayer int) *GameState {
	snapshot := position.Snapshot()
	state := &GameState{
		Board: make([][]CellValue, snapshot.Rows), Rows: snapshot.Rows, Cols: snapshot.Cols,
		Players:      append([]GamePlayerInfo(nil), players...),
		NeutralsUsed: position.NeutralUsed(game.Player(botPlayer)),
	}
	for index, base := range snapshot.Bases {
		state.PlayerBases[index] = CellPos{Row: base.Row, Col: base.Col}
	}
	for row := range snapshot.Board {
		state.Board[row] = make([]CellValue, snapshot.Cols)
		for col, cell := range snapshot.Board[row] {
			flag := CellFlagNormal
			switch cell.Kind {
			case game.Empty:
				continue
			case game.Base:
				flag = CellFlagBase
			case game.Fortified:
				flag = CellFlagFortified
			case game.Neutral:
				flag = CellFlagKilled
			}
			state.Board[row][col] = NewCell(int(cell.Owner), flag)
		}
	}
	return state
}
