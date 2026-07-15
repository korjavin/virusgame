package search

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"reflect"
	"testing"
	"time"

	"virusgame/game"
)

func TestForcedCaptureAndBaseDefense(t *testing.T) {
	state := mustState(t, 6, 6, 2)
	state = play(t, state,
		move(1, 1), move(2, 2), move(2, 3),
		move(4, 4), move(3, 3), move(3, 2),
	)

	result := completedDepth(t, state, 1)
	target, _ := state.At(result.Action.Target)
	if result.Action.Kind != game.Move || target.Owner != 2 || target.Kind != game.Normal {
		t.Fatalf("expected defensive capture, got %+v targeting %+v", result.Action, target)
	}
}

func TestThreeActionBuildThenCapture(t *testing.T) {
	state := mustState(t, 6, 6, 2)
	state = play(t, state,
		move(0, 1), move(1, 0), move(1, 1),
		move(4, 4), move(4, 3), move(5, 3),
	)

	actor := state.CurrentPlayer()
	captured := false
	for remaining := 3; remaining > 0; remaining-- {
		result := completedDepth(t, state, remaining)
		target, _ := state.At(result.Action.Target)
		if target.Owner == 2 && target.Kind == game.Normal {
			captured = true
		}
		var err error
		state, err = state.Apply(result.Action)
		if err != nil {
			t.Fatal(err)
		}
		if remaining > 1 && state.CurrentPlayer() != actor {
			t.Fatalf("search sequence changed player early with %d actions remaining", remaining-1)
		}
	}
	if !captured {
		t.Fatal("depth-three principal sequence did not reach a capture")
	}
}

func TestCutPositionChoosesReconnect(t *testing.T) {
	state := mustState(t, 7, 7, 2)
	state = play(t, state,
		move(1, 1), move(2, 2), move(3, 2),
		move(5, 5), move(4, 4), move(3, 3),
		move(4, 2), move(5, 2), move(6, 2),
		move(3, 2), move(2, 3), move(1, 4),
	)
	before := analyze(state, 1).connected
	result := completedDepth(t, state, 2)
	next, err := state.Apply(result.Action)
	if err != nil {
		t.Fatal(err)
	}
	after := analyze(next, 1).connected
	if after <= before+1 {
		t.Fatalf("expected reconnection, connected cells %d -> %d via %+v", before, after, result.Action)
	}
}

func TestNeutralActionsAreSearchedAsWholeTurns(t *testing.T) {
	state := mustState(t, 6, 6, 2)
	state = play(t, state,
		move(0, 1), move(1, 0), move(1, 1),
		move(4, 4), move(4, 5), move(5, 4),
	)
	s := newSearcher(context.Background(), state)
	children, ok := s.orderedChildren(state)
	if !ok {
		t.Fatal("ordering canceled")
	}
	found := false
	for _, child := range children {
		if child.action.Kind == game.PlaceNeutrals {
			found = true
			if child.state.CurrentPlayer() == state.CurrentPlayer() || child.state.MovesLeft() != 3 {
				t.Fatalf("neutral did not consume turn: player=%d moves=%d", child.state.CurrentPlayer(), child.state.MovesLeft())
			}
		}
	}
	if !found {
		t.Fatal("neutral actions were omitted")
	}
	completedDepth(t, state, 2)
}

func TestBaseSafetyPenalizesBlockingBothExits(t *testing.T) {
	state := mustState(t, 6, 6, 2)
	state = play(t, state,
		move(0, 1), move(1, 0), move(1, 1),
		move(4, 4), move(4, 5), move(5, 4),
	)
	before := analyze(state, 1)
	blocked, err := state.Apply(game.Action{
		Kind:     game.PlaceNeutrals,
		Neutrals: [2]game.Pos{{Row: 0, Col: 1}, {Row: 1, Col: 0}},
	})
	if err != nil {
		t.Fatal(err)
	}
	after := analyze(blocked, 1)
	if after.baseExits >= before.baseExits || evaluate(blocked, 1) >= evaluate(state, 1) {
		t.Fatalf("blocking base exits was not penalized: exits %d -> %d", before.baseExits, after.baseExits)
	}
}

func TestMultiplayerMaxNIsLegalAndDeterministic(t *testing.T) {
	state := mustState(t, 6, 6, 4)
	a := completedDepth(t, state, 2)
	b := completedDepth(t, state, 2)
	if !reflect.DeepEqual(a.Action, b.Action) || a.Score != b.Score {
		t.Fatalf("non-deterministic results: %+v / %+v", a, b)
	}
	if _, err := state.Apply(a.Action); err != nil {
		t.Fatalf("multiplayer result is illegal: %v", err)
	}
	if !newSearcher(context.Background(), state).multi {
		t.Fatal("four-player state did not select multiplayer search")
	}
}

func TestTwoPlayerSearchScoresPlayerTwoRoot(t *testing.T) {
	state := mustState(t, 6, 6, 2)
	state = play(t, state, move(0, 1), move(1, 1), move(2, 2))
	if state.CurrentPlayer() != 2 {
		t.Fatalf("fixture current player = %d", state.CurrentPlayer())
	}
	result := completedDepth(t, state, 1)
	want := -infScore
	for _, action := range state.LegalActions() {
		next, err := state.Apply(action)
		if err != nil {
			t.Fatal(err)
		}
		if score := evaluate(next, 2); score > want {
			want = score
		}
	}
	if result.Score != want {
		t.Fatalf("player-two score = %d, want %d", result.Score, want)
	}
}

func TestSearchFindsImmediateElimination(t *testing.T) {
	state, winning, ok := findWinningMove(t)
	if !ok {
		t.Fatal("test fixture search found no immediate elimination")
	}
	result := completedDepth(t, state, 1)
	next, err := state.Apply(result.Action)
	if err != nil {
		t.Fatal(err)
	}
	if !next.GameOver() || next.Winner() != state.CurrentPlayer() {
		t.Fatalf("chose %+v instead of winning %+v", result.Action, winning)
	}
}

func TestCancellationReturnsLegalFallback(t *testing.T) {
	state := mustState(t, 10, 10, 2)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	start := time.Now()
	result, ok := Choose(ctx, state)
	if !ok {
		t.Fatal("canceled search returned no action for movable state")
	}
	if time.Since(start) > 100*time.Millisecond {
		t.Fatal("cancellation was not enforced promptly")
	}
	if result.Depth != 0 {
		t.Fatalf("partial iteration was published at depth %d", result.Depth)
	}
	if _, err := state.Apply(result.Action); err != nil {
		t.Fatalf("fallback is illegal: %v", err)
	}
}

func TestRecordedT8NeverImmediatelySelfEliminates(t *testing.T) {
	state := recorded4D85T8(t)
	if got := searchSnapshotFingerprint(t, state); got != "06fdd264ea79e519" {
		t.Fatalf("T8 fingerprint = %s", got)
	}
	losing := game.Action{Kind: game.PlaceNeutrals, Neutrals: [2]game.Pos{{Row: 7, Col: 8}, {Row: 7, Col: 9}}}
	lost, err := state.Apply(losing)
	if err != nil {
		t.Fatal(err)
	}
	if lost.Active(2) || !lost.GameOver() {
		t.Fatal("recorded neutral action does not reproduce immediate elimination")
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	for name, choose := range map[string]func() (Result, bool){
		"production": func() (Result, bool) { return Choose(ctx, state) },
		"fixed":      func() (Result, bool) { return ChooseDepth(ctx, state, 6) },
	} {
		t.Run(name, func(t *testing.T) {
			result, ok := choose()
			if name == "production" && !ok {
				t.Fatal("production fallback reported no action")
			}
			next, err := state.Apply(result.Action)
			if err != nil {
				t.Fatalf("fallback is illegal: %+v: %v", result.Action, err)
			}
			if !next.Active(2) {
				t.Fatalf("fallback immediately self-eliminates: %+v (ok=%v)", result.Action, ok)
			}
		})
	}

	result := completedDepth(t, state, 6)
	next, err := state.Apply(result.Action)
	if err != nil || !next.Active(2) {
		t.Fatalf("completed search chose immediate elimination: %+v, active=%v err=%v", result.Action, next.Active(2), err)
	}
}

func TestCanceledFallbackPreservesActorAcrossSizesAndPlayers(t *testing.T) {
	for _, fixture := range []struct {
		rows, cols, players int
	}{{5, 5, 2}, {10, 20, 2}, {20, 10, 4}, {30, 30, 2}, {31, 31, 4}, {50, 50, 2}} {
		state := mustState(t, fixture.rows, fixture.cols, fixture.players)
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		result, ok := Choose(ctx, state)
		if !ok {
			t.Fatalf("%dx%d/%dp: no fallback", fixture.rows, fixture.cols, fixture.players)
		}
		next, err := state.Apply(result.Action)
		if err != nil || !next.Active(state.CurrentPlayer()) {
			t.Fatalf("%dx%d/%dp: fallback %+v active=%v err=%v", fixture.rows, fixture.cols, fixture.players, result.Action, next.Active(state.CurrentPlayer()), err)
		}
	}
}

func TestTerminalScoresPreferFastWinsAndSlowLosses(t *testing.T) {
	winning, err := game.New(4, 4, 2)
	if err != nil {
		t.Fatal(err)
	}
	winningState, _, ok := findWinningMove(t)
	if !ok {
		t.Fatal("missing terminal fixture")
	}
	for _, action := range winningState.LegalActions() {
		next, err := winningState.Apply(action)
		if err == nil && next.GameOver() {
			winning = next
			break
		}
	}
	if !winning.GameOver() {
		t.Fatal("failed to construct terminal state")
	}
	winner, loser := winning.Winner(), game.Player(1)
	if loser == winner {
		loser = 2
	}
	if terminalScore(winning, winner, 2) <= terminalScore(winning, winner, 5) {
		t.Fatal("later win did not score below faster win")
	}
	if terminalScore(winning, loser, 5) <= terminalScore(winning, loser, 2) {
		t.Fatal("later loss did not score above immediate loss")
	}
}

func TestChooseDepthIsDeterministicAndCancelable(t *testing.T) {
	state := mustState(t, 6, 6, 2)
	a, ok := ChooseDepth(context.Background(), state, 2)
	if !ok {
		t.Fatal("fixed-depth search failed")
	}
	b, ok := ChooseDepth(context.Background(), state, 2)
	if !ok || a != b {
		t.Fatalf("fixed-depth results differ: %+v / %+v", a, b)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, ok := ChooseDepth(ctx, state, 2); ok {
		t.Fatal("canceled fixed-depth search completed")
	}
	if _, ok := ChooseDepth(context.Background(), state, 0); ok {
		t.Fatal("invalid depth accepted")
	}
}

func BenchmarkDepthThree(b *testing.B) {
	state, _ := game.New(10, 10, 2)
	for i := 0; i < b.N; i++ {
		s := newSearcher(context.Background(), state)
		if _, ok := s.atDepth(state, 3); !ok {
			b.Fatal("search canceled")
		}
	}
}

func completedDepth(t *testing.T, state game.State, depth int) Result {
	t.Helper()
	s := newSearcher(context.Background(), state)
	result, ok := s.atDepth(state, depth)
	if !ok {
		t.Fatalf("depth %d did not complete", depth)
	}
	if _, err := state.Apply(result.Action); err != nil {
		t.Fatalf("depth %d returned illegal action %+v: %v", depth, result.Action, err)
	}
	return result
}

func mustState(t *testing.T, rows, cols, players int) game.State {
	t.Helper()
	state, err := game.New(rows, cols, players)
	if err != nil {
		t.Fatal(err)
	}
	return state
}

func play(t *testing.T, state game.State, actions ...game.Action) game.State {
	t.Helper()
	for i, action := range actions {
		var err error
		state, err = state.Apply(action)
		if err != nil {
			t.Fatalf("setup action %d %+v: %v", i, action, err)
		}
	}
	return state
}

func move(row, col int) game.Action {
	return game.Action{Kind: game.Move, Target: game.Pos{Row: row, Col: col}}
}

func recorded4D85T8(t *testing.T) game.State {
	t.Helper()
	state := mustState(t, 10, 10, 2)
	return play(t, state,
		move(1, 1), move(2, 2), move(3, 3),
		move(8, 8), move(8, 9), move(7, 7),
		move(4, 2), move(5, 3), move(2, 4),
		move(6, 6), move(5, 5), move(9, 8),
		move(4, 4), move(5, 5), move(6, 6),
		move(7, 9), move(7, 8), move(8, 7),
		move(7, 7), move(8, 8), move(9, 8),
	)
}

func searchSnapshotFingerprint(t *testing.T, state game.State) string {
	t.Helper()
	encoded, err := json.Marshal(state.Snapshot())
	if err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256(encoded)
	return hex.EncodeToString(sum[:8])
}

func findWinningMove(t *testing.T) (game.State, game.Action, bool) {
	t.Helper()
	start := mustState(t, 4, 4, 2)
	frontier := []game.State{start}
	seen := map[uint64]bool{stateHash(start): true}
	for ply := 0; ply < 12; ply++ {
		var nextFrontier []game.State
		for _, state := range frontier {
			for _, action := range state.LegalActions() {
				if action.Kind != game.Move {
					continue
				}
				next, err := state.Apply(action)
				if err != nil {
					t.Fatal(err)
				}
				if next.GameOver() && next.Winner() == state.CurrentPlayer() {
					return state, action, true
				}
				hash := stateHash(next)
				if !seen[hash] {
					seen[hash] = true
					nextFrontier = append(nextFrontier, next)
				}
			}
		}
		frontier = nextFrontier
		if len(frontier) > 20_000 {
			frontier = frontier[:20_000]
		}
	}
	return game.State{}, game.Action{}, false
}
