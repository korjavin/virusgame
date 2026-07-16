package main

import (
	"context"
	"testing"

	"virusgame/arena"
	"virusgame/game"
	"virusgame/search"
	"virusgame/search/distill"
)

// arena.PlayContext must abort a match within a single ply of cancellation, not
// play on to MaxActions. An agent cancels the context on its 3rd decision; the
// match must then stop almost immediately (Aborted, few actions), deterministically
// — no wall-clock timing. On 12x12 an un-cancelled game would run to ~1728 actions.
func TestPlayContextCancelsMidGameImmediately(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	decisions := 0
	agent := func(state game.State) (game.Action, bool) {
		decisions++
		if decisions == 3 {
			cancel() // cancel mid-game
		}
		actions := state.LegalActions()
		return actions[0], true
	}
	result, err := arena.PlayContext(ctx, arena.Match{Rows: 12, Cols: 12, Agents: []arena.Agent{agent, agent}})
	if err != nil {
		t.Fatal(err)
	}
	if !result.Aborted {
		t.Fatal("expected the match to abort on cancellation")
	}
	if result.Actions > 4 {
		t.Fatalf("cancellation did not stop the match promptly: %d actions (MaxActions would be ~1728)", result.Actions)
	}
}

// A cancelled context stops the panel at the first game of the first opponent —
// it never runs a full opponent batch, proving the bound is enforced per game
// (and, via the context-aware agent, mid-search) rather than only between
// opponents. The 12x12/depth-3 grid would be slow if it were allowed to run.
func TestRunPanelHonorsCancelledContext(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	cfg := distill.CIConfig()
	cfg.Boards = []distill.Board{{Rows: 12, Cols: 12}}
	if _, err := runPanel(ctx, cfg, search.IncumbentWeights(), 3, 8); err == nil {
		t.Fatal("expected cancellation error from a cancelled panel context")
	}
}

// With a live context the panel completes and reports every opponent, with no
// illegal or stalled decision.
func TestRunPanelCompletesLive(t *testing.T) {
	cfg := distill.CIConfig()
	cfg.Boards = []distill.Board{{Rows: 5, Cols: 5}}
	rows, err := runPanel(context.Background(), cfg, search.IncumbentWeights(), 2, 1)
	if err != nil {
		t.Fatalf("live panel: %v", err)
	}
	if len(rows) != 4 {
		t.Fatalf("expected 4 opponent rows, got %d", len(rows))
	}
	for _, r := range rows {
		if r.Illegal != 0 || r.Stalled != 0 || r.Games == 0 {
			t.Fatalf("opponent %s: games=%d illegal=%d stalled=%d", r.Opponent, r.Games, r.Illegal, r.Stalled)
		}
	}
}
