package main

import (
	"context"
	"encoding/json"
	"sync/atomic"
	"testing"
	"time"

	"virusgame/game"
	gamesearch "virusgame/search"
)

func TestSameTurnSnapshotsDriveOneSequentialActionEach(t *testing.T) {
	bot := testBot(t, 1)
	var calls atomic.Int32
	bot.choose = func(_ context.Context, state game.State) (gamesearch.Result, bool) {
		calls.Add(1)
		return gamesearch.Result{Action: state.LegalActions()[0], Depth: 1}, true
	}
	start := bot.Position.Snapshot()
	bot.handleGameState(&Message{Type: "game_state", GameID: "g", Snapshot: &start})
	first := receiveAction(t, bot)
	assertStandardMessage(t, first)

	next, err := bot.Position.Apply(game.Action{Kind: game.Move, Target: game.Pos{Row: *first.Row, Col: *first.Col}})
	if err != nil {
		t.Fatal(err)
	}
	nextSnapshot := next.Snapshot()
	row, col := first.Row, first.Col
	bot.handleMoveMade(&Message{Type: "move_made", GameID: "g", Player: 1, Row: row, Col: col, Snapshot: &nextSnapshot})
	bot.handleGameState(&Message{Type: "game_state", GameID: "g", Snapshot: &nextSnapshot})
	receiveAction(t, bot)
	if calls.Load() != 2 {
		t.Fatalf("search calls = %d, want 2", calls.Load())
	}
	bot.handleGameState(&Message{Type: "game_state", GameID: "g", Snapshot: &nextSnapshot})
	assertNoAction(t, bot)
}

func TestNewSnapshotCancelsStaleSearchAndPreventsDoubleSend(t *testing.T) {
	bot := testBot(t, 1)
	started := make(chan struct{})
	release := make(chan struct{})
	bot.choose = func(ctx context.Context, state game.State) (gamesearch.Result, bool) {
		close(started)
		<-release
		return gamesearch.Result{Action: state.LegalActions()[0]}, true
	}
	bot.startSearch()
	<-started

	next, err := bot.Position.Apply(bot.Position.LegalActions()[0])
	if err != nil {
		t.Fatal(err)
	}
	snapshot := next.Snapshot()
	row, col := snapshotAction(bot.Position.LegalActions()[0])
	bot.handleMoveMade(&Message{Type: "move_made", GameID: "g", Player: 1, Row: &row, Col: &col, Snapshot: &snapshot})
	close(release)
	assertNoAction(t, bot)
}

func TestGameEndAndGameChangeInvalidateSearch(t *testing.T) {
	for _, change := range []func(*Bot){
		func(bot *Bot) { bot.handleGameEnd(&Message{GameID: "g", Winner: 2}) },
		func(bot *Bot) {
			position, _ := game.New(5, 5, 2)
			bot.startGame("new", 2, position, nil)
		},
	} {
		bot := testBot(t, 1)
		started := make(chan struct{})
		release := make(chan struct{})
		bot.choose = func(_ context.Context, state game.State) (gamesearch.Result, bool) {
			close(started)
			<-release
			return gamesearch.Result{Action: state.LegalActions()[0]}, true
		}
		bot.startSearch()
		<-started
		change(bot)
		close(release)
		assertNoAction(t, bot)
	}
}

func TestOldGameEndCannotCancelNewGame(t *testing.T) {
	bot := testBot(t, 1)
	position, _ := game.New(5, 5, 2)
	bot.startGame("new", 2, position, nil)
	bot.handleGameEnd(&Message{GameID: "g", Winner: 2})
	if bot.State != BotInGame || bot.CurrentGame != "new" {
		t.Fatalf("stale game end changed current game: state=%v game=%q", bot.State, bot.CurrentGame)
	}
}

func TestProductionSearchQueuesDeadlineResult(t *testing.T) {
	bot := testBot(t, 1)
	bot.startSearch()
	message := receiveAction(t, bot)
	assertStandardMessage(t, message)
}

func TestActionMessageConversion(t *testing.T) {
	standard := actionMessage("g", game.Action{Kind: game.Move, Target: game.Pos{Row: 2, Col: 3}})
	assertStandardMessage(t, standard)
	neutral := actionMessage("g", game.Action{Kind: game.PlaceNeutrals, Neutrals: [2]game.Pos{{Row: 1, Col: 2}, {Row: 3, Col: 4}}})
	if neutral.Type != "neutrals" || len(neutral.Cells) != 2 || neutral.Cells[0] != (CellPos{Row: 1, Col: 2}) || neutral.Cells[1] != (CellPos{Row: 3, Col: 4}) {
		t.Fatalf("neutral conversion = %+v", neutral)
	}
}

func testBot(t *testing.T, player int) *Bot {
	t.Helper()
	position, err := game.New(6, 6, 2)
	if err != nil {
		t.Fatal(err)
	}
	bot := NewBot("", nil)
	bot.ponder = false
	bot.State = BotInGame
	bot.CurrentGame = "g"
	bot.YourPlayer = player
	bot.Position = position
	bot.positionVersion = 1
	return bot
}

func receiveAction(t *testing.T, bot *Bot) *Message {
	t.Helper()
	select {
	case outbound := <-bot.send:
		if !outbound.gameAction {
			t.Fatal("queued message is not guarded as a game action")
		}
		var message Message
		if err := json.Unmarshal(outbound.data, &message); err != nil {
			t.Fatal(err)
		}
		return &message
	case <-time.After(gamesearch.ProductionBudget + 2*time.Second):
		t.Fatal("timed out waiting for action")
		return nil
	}
}

func assertNoAction(t *testing.T, bot *Bot) {
	t.Helper()
	select {
	case action := <-bot.send:
		t.Fatalf("unexpected action queued: %s", action.data)
	case <-time.After(25 * time.Millisecond):
	}
}

func assertStandardMessage(t *testing.T, message *Message) {
	t.Helper()
	if message.Type != "move" || message.GameID != "g" || message.Row == nil || message.Col == nil {
		t.Fatalf("standard conversion = %+v", message)
	}
}

func snapshotAction(action game.Action) (int, int) {
	return action.Target.Row, action.Target.Col
}
