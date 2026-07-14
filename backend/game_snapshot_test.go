package main

import (
	"encoding/json"
	"testing"

	"virusgame/game"
)

func TestGameSnapshot1v1StateChanges(t *testing.T) {
	source := &Game{
		Board: Board{
			{NewCell(1, CellFlagBase), NewCell(1, CellFlagNormal), 0},
			{0, NewCell(0, CellFlagKilled), 0},
			{0, 0, NewCell(2, CellFlagBase)},
		},
		Rows: 3, Cols: 3, CurrentPlayer: 1, MovesLeft: 2,
		Player1Base: CellPos{0, 0}, Player2Base: CellPos{2, 2},
		Player1NeutralsUsed: true,
	}
	snapshot := gameSnapshot(source)
	state, err := game.FromSnapshot(snapshot)
	if err != nil {
		t.Fatal(err)
	}
	if state.CurrentPlayer() != 1 || state.MovesLeft() != 2 || !state.NeutralUsed(1) || state.NeutralUsed(2) {
		t.Fatalf("unexpected turn or neutral state: %+v", snapshot)
	}
	if cell, _ := state.At(game.Pos{Row: 0, Col: 1}); cell.Owner != 1 || cell.Kind != game.Normal {
		t.Fatalf("move not represented: %+v", cell)
	}
	if cell, _ := state.At(game.Pos{Row: 1, Col: 1}); cell.Kind != game.Neutral {
		t.Fatalf("neutral not represented: %+v", cell)
	}

	source.Board[2][2] = 0
	snapshot = gameSnapshot(source)
	if snapshot.Active[1] {
		t.Fatal("eliminated player remained active")
	}
	if _, err := game.FromSnapshot(snapshot); err != nil {
		t.Fatalf("elimination snapshot rejected: %v", err)
	}
}

func TestGameSnapshotMultiplayer(t *testing.T) {
	for _, players := range []int{3, 4} {
		t.Run(string(rune('0'+players))+" players", func(t *testing.T) {
			source := multiplayerSnapshotGame(players)
			source.NeutralsUsed[players-1] = true
			snapshot := gameSnapshot(source)
			state, err := game.FromSnapshot(snapshot)
			if err != nil {
				t.Fatal(err)
			}
			if len(snapshot.Bases) != players || !state.NeutralUsed(game.Player(players)) {
				t.Fatalf("incomplete %d-player snapshot: %+v", players, snapshot)
			}
			for player := 1; player <= players; player++ {
				if !state.Active(game.Player(player)) {
					t.Fatalf("player %d not active", player)
				}
			}
		})
	}
}

func TestMessageSnapshotIsBackwardCompatibleAddition(t *testing.T) {
	source := multiplayerSnapshotGame(3)
	snapshot := gameSnapshot(source)
	payload, err := json.Marshal(Message{Type: "turn_change", GameID: "g", Player: 2, MovesLeft: 3, Snapshot: &snapshot})
	if err != nil {
		t.Fatal(err)
	}
	var legacy struct {
		Type      string `json:"type"`
		GameID    string `json:"gameId"`
		Player    int    `json:"player"`
		MovesLeft int    `json:"movesLeft"`
	}
	if err := json.Unmarshal(payload, &legacy); err != nil {
		t.Fatal(err)
	}
	if legacy.Type != "turn_change" || legacy.GameID != "g" || legacy.Player != 2 || legacy.MovesLeft != 3 {
		t.Fatalf("legacy fields changed: %+v", legacy)
	}
}

func multiplayerSnapshotGame(players int) *Game {
	source := &Game{
		Board: make(Board, 4), Rows: 4, Cols: 4, CurrentPlayer: 1, MovesLeft: 3,
		IsMultiplayer: true, PlayerBases: [4]CellPos{{0, 0}, {3, 3}, {0, 3}, {3, 0}},
		ActivePlayers: players,
	}
	for row := range source.Board {
		source.Board[row] = make([]CellValue, 4)
	}
	for index := 0; index < players; index++ {
		source.Players[index] = &LobbyPlayer{Index: index}
		base := source.PlayerBases[index]
		source.Board[base.Row][base.Col] = NewCell(index+1, CellFlagBase)
	}
	return source
}
