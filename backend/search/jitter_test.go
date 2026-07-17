package search

import (
	"context"
	"testing"

	"virusgame/game"
)

// playJitteredOpening plays a short game from the empty board where player 1 is
// the jittered bot (seed) and player 2 is the deterministic engine, returning the
// bot's action sequence. It is the demo harness for vs-ai2.39: two different game
// seeds must yield different bot lines, while the same seed must reproduce.
func playJitteredOpening(t *testing.T, seed uint64, depth, maxActions int) []game.Action {
	t.Helper()
	state, err := game.New(6, 6, 2)
	if err != nil {
		t.Fatal(err)
	}
	var actions []game.Action
	for len(actions) < maxActions && !state.GameOver() {
		var res Result
		var ok bool
		if state.CurrentPlayer() == 1 {
			res, ok = ChooseDepthSeeded(context.Background(), state, depth, seed)
		} else {
			res, ok = ChooseDepth(context.Background(), state, depth) // fixed, deterministic opponent
		}
		if !ok {
			break
		}
		actions = append(actions, res.Action)
		next, err := state.Apply(res.Action)
		if err != nil {
			t.Fatalf("illegal action %+v: %v", res.Action, err)
		}
		state = next
	}
	return actions
}

func firstDiff(a, b []game.Action) int {
	n := len(a)
	if len(b) < n {
		n = len(b)
	}
	for i := 0; i < n; i++ {
		if a[i] != b[i] {
			return i
		}
	}
	if len(a) != len(b) {
		return n
	}
	return -1
}

func TestJitterDivergesAcrossSeedsButStableWithinSeed(t *testing.T) {
	const depth = 6
	const maxActions = 18 // ~6 bot turns of 3 moves each

	lineA := playJitteredOpening(t, 0x1111, depth, maxActions)
	lineB := playJitteredOpening(t, 0x9999, depth, maxActions)
	lineArepeat := playJitteredOpening(t, 0x1111, depth, maxActions)

	// Same seed reproduces exactly: a single game never flip-flops.
	if diff := firstDiff(lineA, lineArepeat); diff != -1 {
		t.Fatalf("same-seed line diverged at index %d: jitter is not reproducible within a game", diff)
	}

	// Different seeds (different game ids) diverge — the farmable bit-identical
	// replay is broken. Turn 1 (first 3 bot actions) is the fixed opening book;
	// divergence must appear soon after.
	diff := firstDiff(lineA, lineB)
	if diff == -1 {
		t.Fatalf("two seeds produced identical bot lines (%d actions): jitter had no effect", len(lineA))
	}
	if diff > 9 {
		t.Fatalf("seeds only diverged at bot action %d (>turn 3); jitter too weak", diff)
	}
	t.Logf("seeds diverge at bot action index %d (of %d); same seed reproduces", diff, len(lineA))
}
