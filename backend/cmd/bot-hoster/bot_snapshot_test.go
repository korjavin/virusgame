package main

import (
	"testing"

	"virusgame/game"
)

func TestDecodeSnapshotValidatesPlayerIndexes(t *testing.T) {
	position, _ := game.New(5, 5, 3)
	snapshot := position.Snapshot()
	message := Message{
		Type: "multiplayer_game_start", GameID: "g", YourPlayer: 2, Snapshot: &snapshot,
		GamePlayers: []GamePlayerInfo{{PlayerIndex: 1}, {PlayerIndex: 2}, {PlayerIndex: 3}},
	}
	if _, err := decodeSnapshot(&message); err != nil {
		t.Fatalf("valid 1-based indexes rejected: %v", err)
	}
	message.GamePlayers[0].PlayerIndex = 0
	if _, err := decodeSnapshot(&message); err == nil {
		t.Fatal("accepted legacy 0-based playerIndex")
	}
}

func TestDecodeSnapshotRejectsMalformedTrustBoundary(t *testing.T) {
	position, _ := game.New(4, 4, 2)
	snapshot := position.Snapshot()
	snapshot.Board[1] = snapshot.Board[1][:3]
	if _, err := decodeSnapshot(&Message{Type: "game_state", Snapshot: &snapshot}); err == nil {
		t.Fatal("accepted ragged snapshot board")
	}
	if _, err := decodeSnapshot(&Message{Type: "game_state"}); err == nil {
		t.Fatal("accepted missing snapshot")
	}
}

func TestBotStoresAuthoritativeSnapshotAndRejectsOtherGame(t *testing.T) {
	position, _ := game.New(4, 4, 4)
	snapshot := position.Snapshot()
	bot := &Bot{
		CurrentGame: "g", YourPlayer: 1,
		GamePlayers: []GamePlayerInfo{{PlayerIndex: 1}, {PlayerIndex: 2}, {PlayerIndex: 3}, {PlayerIndex: 4}},
	}
	if err := bot.updatePosition(&Message{Type: "game_state", GameID: "g", Snapshot: &snapshot}); err != nil {
		t.Fatal(err)
	}
	if bot.Position.Rows() != 4 || bot.Position.Cols() != 4 || bot.Position.MovesLeft() != 3 {
		t.Fatalf("snapshot not stored: %+v", bot.Position.Snapshot())
	}

	changed := snapshot
	changed.MovesLeft = 2
	if err := bot.updatePosition(&Message{Type: "game_state", GameID: "old", Snapshot: &changed}); err == nil {
		t.Fatal("accepted snapshot for another game")
	}
	if bot.Position.MovesLeft() != 3 {
		t.Fatal("rejected snapshot corrupted stored position")
	}
}

func TestLegacyAdapterCarriesCompleteDecisionState(t *testing.T) {
	position, _ := game.New(4, 4, 3)
	snapshot := position.Snapshot()
	snapshot.NeutralUsed[1] = true
	snapshot.Current = 2
	snapshot.MovesLeft = 1
	position, err := game.FromSnapshot(snapshot)
	if err != nil {
		t.Fatal(err)
	}
	state := legacyGameState(position, []GamePlayerInfo{{PlayerIndex: 1}, {PlayerIndex: 2}, {PlayerIndex: 3}}, 2)
	if position.CurrentPlayer() != 2 || position.MovesLeft() != 1 || !position.NeutralUsed(2) ||
		!state.NeutralsUsed {
		t.Fatalf("decision state incomplete: %+v", position.Snapshot())
	}
	if state.PlayerBases[2] != (CellPos{Row: 0, Col: 3}) {
		t.Fatalf("base mapping is not 1-based: %+v", state.PlayerBases)
	}
}
