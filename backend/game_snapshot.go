package main

import "virusgame/game"

func gameSnapshot(source *Game) game.Snapshot {
	players := 2
	bases := []game.Pos{{Row: source.Player1Base.Row, Col: source.Player1Base.Col}, {Row: source.Player2Base.Row, Col: source.Player2Base.Col}}
	neutralUsed := []bool{source.Player1NeutralsUsed, source.Player2NeutralsUsed}
	if source.IsMultiplayer {
		players = 0
		for index, player := range source.Players {
			if player != nil {
				players = index + 1
			}
		}
		bases = make([]game.Pos, players)
		neutralUsed = make([]bool, players)
		for index := 0; index < players; index++ {
			bases[index] = game.Pos{Row: source.PlayerBases[index].Row, Col: source.PlayerBases[index].Col}
			neutralUsed[index] = source.NeutralsUsed[index]
		}
	}

	snapshot := game.Snapshot{
		Rows: source.Rows, Cols: source.Cols, Board: make([][]game.Cell, source.Rows),
		Bases: bases, Active: make([]bool, players), NeutralUsed: neutralUsed,
		Current: game.Player(source.CurrentPlayer), MovesLeft: source.MovesLeft,
		GameOver: source.GameOver, Winner: game.Player(source.Winner),
	}
	for row := 0; row < source.Rows; row++ {
		snapshot.Board[row] = make([]game.Cell, source.Cols)
		for col := 0; col < source.Cols; col++ {
			snapshot.Board[row][col] = protocolCell(source.Board[row][col])
			owner := snapshot.Board[row][col].Owner
			if owner > 0 && int(owner) <= players {
				snapshot.Active[owner-1] = true
			}
		}
	}
	return snapshot
}

func protocolCell(cell CellValue) game.Cell {
	kind := game.Empty
	switch cell.Flag() {
	case CellFlagNormal:
		if cell.Player() != 0 {
			kind = game.Normal
		}
	case CellFlagBase:
		kind = game.Base
	case CellFlagFortified:
		kind = game.Fortified
	case CellFlagKilled:
		kind = game.Neutral
	}
	return game.Cell{Owner: game.Player(cell.Player()), Kind: kind}
}
