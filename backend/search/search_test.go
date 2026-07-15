package search

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"math/rand"
	"reflect"
	"runtime"
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
	children, _, _, ok := s.orderedChildren(game.NewPosition(state), true)
	if !ok {
		t.Fatal("ordering canceled")
	}
	found := false
	for _, child := range children {
		if child.action.Kind == game.PlaceNeutrals {
			found = true
			next := child.position.State()
			if next.CurrentPlayer() == state.CurrentPlayer() || next.MovesLeft() != 3 {
				t.Fatalf("neutral did not consume turn: player=%d moves=%d", next.CurrentPlayer(), next.MovesLeft())
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

func TestFallbackFindsSolePreserverOmittedByBoundedCandidates(t *testing.T) {
	const fixture = `{"rows":8,"cols":8,"board":[[{"Owner":1,"Kind":2},{"Owner":2,"Kind":1},{"Owner":2,"Kind":3},{"Owner":1,"Kind":3},{"Owner":1,"Kind":1},{"Owner":1,"Kind":1},{"Owner":2,"Kind":3},{"Owner":1,"Kind":1}],[{"Owner":0,"Kind":4},{"Owner":0,"Kind":4},{"Owner":2,"Kind":3},{"Owner":1,"Kind":1},{"Owner":2,"Kind":1},{"Owner":2,"Kind":1},{"Owner":1,"Kind":1},{"Owner":2,"Kind":1}],[{"Owner":2,"Kind":1},{"Owner":2,"Kind":3},{"Owner":0,"Kind":4},{"Owner":1,"Kind":1},{"Owner":0,"Kind":4},{"Owner":1,"Kind":1},{"Owner":1,"Kind":3},{"Owner":1,"Kind":3}],[{"Owner":1,"Kind":3},{"Owner":0,"Kind":0},{"Owner":0,"Kind":0},{"Owner":1,"Kind":1},{"Owner":2,"Kind":1},{"Owner":2,"Kind":3},{"Owner":1,"Kind":3},{"Owner":2,"Kind":1}],[{"Owner":2,"Kind":3},{"Owner":1,"Kind":3},{"Owner":1,"Kind":3},{"Owner":0,"Kind":0},{"Owner":1,"Kind":1},{"Owner":2,"Kind":1},{"Owner":0,"Kind":4},{"Owner":1,"Kind":1}],[{"Owner":1,"Kind":3},{"Owner":1,"Kind":1},{"Owner":0,"Kind":4},{"Owner":1,"Kind":1},{"Owner":1,"Kind":1},{"Owner":2,"Kind":1},{"Owner":1,"Kind":1},{"Owner":1,"Kind":1}],[{"Owner":1,"Kind":1},{"Owner":2,"Kind":3},{"Owner":0,"Kind":0},{"Owner":1,"Kind":1},{"Owner":1,"Kind":1},{"Owner":0,"Kind":0},{"Owner":1,"Kind":3},{"Owner":0,"Kind":0}],[{"Owner":2,"Kind":3},{"Owner":2,"Kind":3},{"Owner":2,"Kind":1},{"Owner":1,"Kind":1},{"Owner":2,"Kind":1},{"Owner":0,"Kind":4},{"Owner":2,"Kind":1},{"Owner":2,"Kind":2}]],"bases":[{"Row":0,"Col":0},{"Row":7,"Col":7}],"active":[true,true],"neutralUsed":[false,false],"currentPlayer":1,"movesLeft":3,"gameOver":false,"winner":0}`
	var snapshot game.Snapshot
	if err := json.Unmarshal([]byte(fixture), &snapshot); err != nil {
		t.Fatal(err)
	}
	state, err := game.FromSnapshot(snapshot)
	if err != nil {
		t.Fatal(err)
	}
	position := game.NewPosition(state)
	boundedSurvivor := false
	position.ForEachSearchAction(func(a game.Action) bool {
		if position.ApplySearch(a).State().Active(1) {
			boundedSurvivor = true
		}
		return true
	})
	if boundedSurvivor {
		t.Fatal("fixture no longer proves bounded omission")
	}
	action, ok := preservingFallback(state)
	if !ok {
		t.Fatal("missing fallback")
	}
	next, err := state.Apply(action)
	if err != nil || !next.Active(1) || action.Kind != game.PlaceNeutrals {
		t.Fatalf("fallback=%+v active=%v err=%v", action, next.Active(1), err)
	}
}

func TestShortDeadlinePublishesFallbackWithoutIteration(t *testing.T) {
	state := syntheticContact(t, 12, 2)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
	defer cancel()
	result, ok := Choose(ctx, state)
	if !ok {
		t.Fatal("no fallback")
	}
	if result.IterationsStarted != 0 || result.Depth != 0 || result.Elapsed >= 25*time.Millisecond {
		t.Fatalf("short deadline result=%+v", result)
	}
	if _, err := state.Apply(result.Action); err != nil {
		t.Fatal(err)
	}
}

func TestCompletedSearchPreservesActorOnReachableStates(t *testing.T) {
	var covered [2][3]bool
	for seed := int64(0); seed < 12; seed++ {
		rng := rand.New(rand.NewSource(seed))
		state := mustState(t, 5+int(seed%2), 5, 2)
		for step := 0; step < 60 && !state.GameOver(); step++ {
			actor := state.CurrentPlayer()
			if actor <= 2 && state.MovesLeft() >= 1 && state.MovesLeft() <= 3 {
				covered[actor-1][state.MovesLeft()-1] = true
			}
			if hasPreservingSuccessor(state) {
				result, ok := ChooseDepth(context.Background(), state, 1)
				if !ok {
					t.Fatalf("seed %d step %d: depth-one search did not complete", seed, step)
				}
				next, err := state.Apply(result.Action)
				if err != nil || !next.Active(actor) {
					t.Fatalf("seed %d step %d player %d moves %d: chose %+v active=%v err=%v", seed, step, actor, state.MovesLeft(), result.Action, next.Active(actor), err)
				}
			}

			actions := state.LegalActions()
			moveCount := 0
			for moveCount < len(actions) && actions[moveCount].Kind == game.Move {
				moveCount++
			}
			if moveCount == 0 {
				break
			}
			var err error
			state, err = state.Apply(actions[rng.Intn(moveCount)])
			if err != nil {
				t.Fatal(err)
			}
		}
	}
	for player := range covered {
		for moves := range covered[player] {
			if !covered[player][moves] {
				t.Fatalf("missing reachable coverage for player %d with %d moves left", player+1, moves+1)
			}
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
	vector := terminalScores(winning, 4)
	for player := game.Player(1); player <= 4; player++ {
		want := -mateScore + 4
		if player == winner {
			want = mateScore - 4
		}
		if vector[player-1] != want {
			t.Fatalf("terminal vector player %d = %d, want %d", player, vector[player-1], want)
		}
	}
}

func TestChooseDepthPrefersShortestForcedWin(t *testing.T) {
	state := play(t, mustState(t, 3, 3, 2),
		move(0, 1), move(1, 0), move(1, 1),
	)
	result, ok := ChooseDepth(context.Background(), state, 8)
	if !ok {
		t.Fatal("forced-win search did not complete")
	}
	want := move(1, 1)
	if result.Action != want || result.Score != mateScore-3 {
		t.Fatalf("forced win = %+v score %d, want %+v score %d", result.Action, result.Score, want, mateScore-3)
	}
}

func TestChooseDepthPrefersLongestForcedLoss(t *testing.T) {
	state := play(t, mustState(t, 3, 3, 2),
		move(0, 1), move(1, 0),
	)
	result, ok := ChooseDepth(context.Background(), state, 8)
	if !ok {
		t.Fatal("forced-loss search did not complete")
	}
	if result.Score != -mateScore+8 {
		t.Fatalf("forced loss score = %d, want %d via %+v", result.Score, -mateScore+8, result.Action)
	}
	if result.Action == move(1, 1) {
		t.Fatalf("selected faster loss %+v", result.Action)
	}
	again, ok := ChooseDepth(context.Background(), state, 8)
	if !ok || again != result {
		t.Fatalf("forced-loss tie was non-deterministic: %+v / %+v", result, again)
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

func TestParallelRootEqualsSequentialReference(t *testing.T) {
	for players := 2; players <= 4; players++ {
		for seed := int64(1); seed <= 4; seed++ {
			state := mustState(t, 5+int(seed%2), 6, players)
			rng := rand.New(rand.NewSource(seed*100 + int64(players)))
			for ply := 0; ply < 5 && !state.GameOver(); ply++ {
				actions := state.LegalActions()
				moves := 0
				for moves < len(actions) && actions[moves].Kind == game.Move {
					moves++
				}
				if moves == 0 {
					break
				}
				var err error
				state, err = state.Apply(actions[rng.Intn(moves)])
				if err != nil {
					t.Fatal(err)
				}
			}
			if state.GameOver() {
				continue
			}
			depth := min(2, state.MovesLeft())
			sequentialSearcher := newSearcher(context.Background(), state)
			want, ok := sequentialSearcher.atDepth(state, depth)
			if !ok {
				t.Fatal("sequential incomplete")
			}
			fallback, _ := preservingFallback(state)
			for workers := 1; workers <= 4; workers++ {
				for repeat := 0; repeat < 3; repeat++ {
					s := newSearcher(context.Background(), state)
					got, complete := s.atDepthParallel(state, depth, workers, fallback)
					if !complete || got.Action != want.Action || got.Score != want.Score {
						t.Fatalf("%dp seed%d workers%d repeat%d: got %+v complete=%v want %+v", players, seed, workers, repeat, got, complete, want)
					}
				}
			}
		}
	}
}

func TestPVSEqualsFullWindowReference(t *testing.T) {
	for players := 2; players <= 2; players++ {
		for seed := int64(10); seed < 14; seed++ {
			state := mustState(t, 5+int(seed%2), 6, players)
			rng := rand.New(rand.NewSource(seed + int64(players)*1000))
			for ply := 0; ply < 6 && !state.GameOver(); ply++ {
				actions := state.LegalActions()
				moves := 0
				for moves < len(actions) && actions[moves].Kind == game.Move {
					moves++
				}
				if moves == 0 {
					break
				}
				var err error
				state, err = state.Apply(actions[rng.Intn(moves)])
				if err != nil {
					t.Fatal(err)
				}
			}
			if state.GameOver() {
				continue
			}
			// Cross the turn boundary and search multiple minimizing replies; a
			// current-turn-only oracle cannot exercise PVS null windows or bounds.
			depth := state.MovesLeft() + 2
			oracle := newSearcher(context.Background(), state)
			oracle.pvs = false
			want, ok := oracle.atDepth(state, depth)
			if !ok {
				t.Fatal("oracle incomplete")
			}
			candidate := newSearcher(context.Background(), state)
			got, ok := candidate.atDepth(state, depth)
			if !ok || got.Action != want.Action || got.Score != want.Score {
				t.Fatalf("%dp seed%d got=%+v ok=%v want=%+v", players, seed, got, ok, want)
			}
		}
	}
}

func TestCandidatesAreStableLegalUniqueAndRetainForcing(t *testing.T) {
	for _, fixture := range []struct {
		name    string
		players int
		root    bool
		limit   int
	}{{"root-2p", 2, true, rootOptionalLimit}, {"interior-2p", 2, false, interiorOptionalLimit}, {"interior-3p", 3, false, multiOptionalLimit}, {"interior-4p", 4, false, multiOptionalLimit}} {
		t.Run(fixture.name, func(t *testing.T) {
			assertCandidateSet(t, syntheticContact(t, 20, fixture.players), fixture.root, fixture.limit)
		})
	}
}

func assertCandidateSet(t *testing.T, state game.State, root bool, limit int) {
	t.Helper()
	s := newSearcher(context.Background(), state)
	first, legal, _, ok := s.orderedChildren(game.NewPosition(state), root)
	second, legal2, _, ok2 := s.orderedChildren(game.NewPosition(state), root)
	if !ok || !ok2 || legal != legal2 || !reflect.DeepEqual(first, second) {
		t.Fatal("candidate generation is not stable")
	}
	seen := map[game.Action]bool{}
	selected := map[game.Action]bool{}
	quiet := 0
	before := activeCount(state)
	actor := state.CurrentPlayer()
	for _, c := range first {
		if seen[c.action] {
			t.Fatalf("duplicate %+v", c.action)
		}
		seen[c.action] = true
		selected[c.action] = true
		if _, err := state.Apply(c.action); err != nil {
			t.Fatalf("illegal %+v: %v", c.action, err)
		}
		cell, _ := state.At(c.action.Target)
		next := c.position.State()
		forcing := next.Active(actor) && (next.GameOver() && next.Winner() == actor || activeCount(next) < before || c.action.Kind == game.Move && cell.Kind == game.Normal && cell.Owner != actor)
		if !forcing {
			quiet++
		}
	}
	if quiet > limit {
		t.Fatalf("quiet candidates=%d limit=%d", quiet, limit)
	}
	position := game.NewPosition(state)
	availableQuiet := 0
	position.ForEachSearchAction(func(a game.Action) bool {
		next := position.ApplySearch(a).State()
		cell, _ := state.At(a.Target)
		forcing := next.Active(actor) && (next.GameOver() && next.Winner() == actor || activeCount(next) < before || a.Kind == game.Move && cell.Kind == game.Normal && cell.Owner != actor)
		if forcing && !selected[a] {
			t.Fatalf("forcing action omitted: %+v", a)
		}
		if !forcing && next.Active(actor) {
			availableQuiet++
		}
		return true
	})
	if want := min(limit, availableQuiet); quiet != want {
		t.Fatalf("quiet candidates=%d want exact cap %d from %d", quiet, want, availableQuiet)
	}
}

func TestIncompleteParallelIterationIsNotPublished(t *testing.T) {
	state := syntheticContact(t, 12, 2)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	result, ok := Choose(ctx, state)
	if !ok {
		t.Fatal("missing preserving fallback")
	}
	if result.Depth != 0 || result.IterationsCompleted != 0 || result.RootCompleted != 0 || result.RootSelected != 0 {
		t.Fatalf("partial iteration leaked into result: %+v", result)
	}
	if _, err := state.Apply(result.Action); err != nil {
		t.Fatalf("fallback illegal: %v", err)
	}
}

func TestParallelTelemetryIsTruthful(t *testing.T) {
	state := syntheticContact(t, 12, 2)
	fallback, _ := preservingFallback(state)
	s := newSearcher(context.Background(), state)
	result, ok := s.atDepthParallel(state, 2, 3, fallback)
	if !ok {
		t.Fatal("parallel search incomplete")
	}
	if result.Workers != 3 || result.RootLegal < result.RootSelected || result.RootCompleted != result.RootSelected {
		t.Fatalf("telemetry=%+v", result)
	}
}

func TestParallelWorkersReportActualPeakForOneAndTwoRoots(t *testing.T) {
	two := mustState(t, 2, 2, 2)
	snapshot := two.Snapshot()
	snapshot.Board[0][1] = game.Cell{Kind: game.Neutral}
	one, err := game.FromSnapshot(snapshot)
	if err != nil {
		t.Fatal(err)
	}
	for _, fixture := range []struct {
		name  string
		state game.State
		roots int
	}{{"one", one, 1}, {"two", two, 2}} {
		t.Run(fixture.name, func(t *testing.T) {
			fallback, _ := preservingFallback(fixture.state)
			s := newSearcher(context.Background(), fixture.state)
			result, ok := s.atDepthParallel(fixture.state, 1, 8, fallback)
			if !ok {
				t.Fatal("incomplete")
			}
			if result.RootSelected != fixture.roots || result.Workers != 1 {
				t.Fatalf("result=%+v", result)
			}
		})
	}
}

func TestCanceledParallelSearchReturnsWithoutWorkerLeaks(t *testing.T) {
	state := syntheticContact(t, 12, 2)
	fallback, _ := preservingFallback(state)
	runtime.GC()
	baseline := runtime.NumGoroutine()
	for i := 0; i < 8; i++ {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Millisecond)
		s := newSearcher(ctx, state)
		_, complete := s.atDepthParallel(state, 9, 4, fallback)
		cancel()
		if complete {
			t.Fatal("deadline search unexpectedly completed")
		}
	}
	runtime.GC()
	time.Sleep(10 * time.Millisecond)
	if got := runtime.NumGoroutine(); got > baseline+2 {
		t.Fatalf("goroutines grew from %d to %d", baseline, got)
	}
}

func TestProductionLargeBoardSmoke(t *testing.T) {
	for _, fixture := range []struct{ size, players int }{{28, 4}, {50, 2}} {
		state := syntheticContact(t, fixture.size, fixture.players)
		ctx, cancel := context.WithTimeout(context.Background(), 650*time.Millisecond)
		started := time.Now()
		result, ok := Choose(ctx, state)
		elapsed := time.Since(started)
		cancel()
		if !ok {
			t.Fatalf("%dx%d/%dp: no result", fixture.size, fixture.size, fixture.players)
		}
		if _, err := state.Apply(result.Action); err != nil {
			t.Fatalf("%dx%d/%dp illegal: %v", fixture.size, fixture.size, fixture.players, err)
		}
		if elapsed > 750*time.Millisecond {
			t.Fatalf("%dx%d/%dp took %s", fixture.size, fixture.size, fixture.players, elapsed)
		}
	}
}

func TestTurnAlignedSearchIsDeterministicAndNodeBudgetExact(t *testing.T) {
	two := play(t, mustState(t, 5, 5, 2),
		move(1, 1), move(2, 2), move(3, 3),
		move(3, 4), move(2, 3), move(1, 2),
	)
	three := play(t, mustState(t, 5, 5, 3),
		move(1, 1), move(2, 2), move(3, 3),
		move(3, 3), move(2, 3), move(1, 2),
		move(1, 3), move(2, 2), move(3, 1),
	)
	for _, fixture := range []struct {
		name  string
		state game.State
		want  Result
	}{
		{"minimax", two, Result{Action: move(0, 1), Nodes: 1000, Evaluations: 913, BudgetExhausted: true, Workers: 1, IterationsStarted: 1}},
		{"maxn", three, Result{Action: move(0, 1), Score: 3946, Depth: 3, Nodes: 1000, Evaluations: 772, BudgetExhausted: true, CompletedTurnDepth: 1, Workers: 1, RootLegal: 6, RootSelected: 6, RootCompleted: 6, IterationsStarted: 2, IterationsCompleted: 1}},
	} {
		t.Run(fixture.name, func(t *testing.T) {
			a, ok := ChooseNodeBudget(fixture.state, 1000)
			b, ok2 := ChooseNodeBudget(fixture.state, 1000)
			if !ok || !ok2 || a != b {
				t.Fatalf("node search differs: %+v / %+v", a, b)
			}
			if a != fixture.want {
				t.Fatalf("golden changed: got %+v want %+v", a, fixture.want)
			}
			if a.Nodes != 1000 || !a.BudgetExhausted {
				t.Fatalf("budget telemetry = %+v", a)
			}
			if a.Depth > 0 && (a.Depth-fixture.state.MovesLeft())%3 != 0 {
				t.Fatalf("depth %d is not turn aligned", a.Depth)
			}
			if _, err := fixture.state.Apply(a.Action); err != nil {
				t.Fatalf("illegal result: %v", err)
			}
		})
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

func BenchmarkCandidateGeneration(b *testing.B) {
	for _, size := range []int{5, 12, 20} {
		state := syntheticContact(b, size, 2)
		b.Run(fmt.Sprintf("%dx%d", size, size), func(b *testing.B) {
			b.ReportAllocs()
			for i := 0; i < b.N; i++ {
				s := newSearcher(context.Background(), state)
				if _, _, _, ok := s.orderedChildren(game.NewPosition(state), true); !ok {
					b.Fatal("canceled")
				}
			}
		})
	}
}

func BenchmarkProductionContact(b *testing.B) {
	for _, size := range []int{5, 12, 20} {
		state := syntheticContact(b, size, 2)
		b.Run(fmt.Sprintf("%dx%d", size, size), func(b *testing.B) {
			b.ReportAllocs()
			for i := 0; i < b.N; i++ {
				result, ok := Choose(context.Background(), state)
				if !ok {
					b.Fatal("no result")
				}
				b.ReportMetric(float64(result.Depth), "depth")
				b.ReportMetric(float64(result.CompletedTurnDepth), "turns")
				b.ReportMetric(float64(result.Nodes), "nodes")
				b.ReportMetric(float64(result.IterationsStarted), "started")
				b.ReportMetric(float64(result.IterationsCompleted), "completed")
			}
		})
	}
}

func BenchmarkProductionSmoke(b *testing.B) {
	for _, fixture := range []struct{ size, players int }{{28, 4}, {50, 2}} {
		state := syntheticContact(b, fixture.size, fixture.players)
		b.Run(fmt.Sprintf("%dx%d-%dp", fixture.size, fixture.size, fixture.players), func(b *testing.B) {
			b.ReportAllocs()
			for i := 0; i < b.N; i++ {
				result, ok := Choose(context.Background(), state)
				if !ok {
					b.Fatal("no result")
				}
				if _, err := state.Apply(result.Action); err != nil {
					b.Fatal(err)
				}
				b.ReportMetric(float64(result.Depth), "depth")
			}
		})
	}
}

func BenchmarkFixedContact(b *testing.B) {
	state := syntheticContact(b, 20, 2)
	for _, depth := range []int{3, 6} {
		b.Run(fmt.Sprintf("depth%d", depth), func(b *testing.B) {
			b.ReportAllocs()
			for i := 0; i < b.N; i++ {
				if _, ok := ChooseDepth(context.Background(), state, depth); !ok {
					b.Fatal("incomplete")
				}
			}
		})
	}
}

func BenchmarkFixedParallel5(b *testing.B) {
	state := syntheticContact(b, 5, 2)
	fallback, _ := preservingFallback(state)
	for _, depth := range []int{3, 6, 9} {
		b.Run(fmt.Sprintf("depth%d", depth), func(b *testing.B) {
			b.ReportAllocs()
			for i := 0; i < b.N; i++ {
				s := newSearcher(context.Background(), state)
				_, ok := s.atDepthParallel(state, depth, 6, fallback)
				b.ReportMetric(float64(s.nodes), "nodes")
				if !ok {
					b.Fatal("incomplete")
				}
			}
		})
	}
}

func syntheticContact(tb testing.TB, size, players int) game.State {
	tb.Helper()
	state, err := game.New(size, size, players)
	if err != nil {
		tb.Fatal(err)
	}
	snap := state.Snapshot()
	for i := 1; i < size-1; i++ {
		snap.Board[i][i] = game.Cell{Owner: 1, Kind: game.Normal}
		snap.Board[size-1-i][size-1-i] = game.Cell{Owner: 2, Kind: game.Normal}
	}
	state, err = game.FromSnapshot(snap)
	if err != nil {
		tb.Fatal(err)
	}
	return state
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

func hasPreservingSuccessor(state game.State) bool {
	actor := state.CurrentPlayer()
	for _, action := range state.LegalActions() {
		next, err := state.Apply(action)
		if err == nil && next.Active(actor) {
			return true
		}
	}
	return false
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

func makeStateFromLayout(rows, cols int, activePlayer game.Player, movesLeft int, layout []string) game.State {
	snap := game.Snapshot{
		Rows: rows, Cols: cols,
		Board:       make([][]game.Cell, rows),
		Bases:       []game.Pos{{Row: 0, Col: 0}, {Row: rows - 1, Col: cols - 1}},
		Active:      []bool{true, true},
		NeutralUsed: []bool{false, false},
		Current:     activePlayer,
		MovesLeft:   movesLeft,
	}
	for r := 0; r < rows; r++ {
		snap.Board[r] = make([]game.Cell, cols)
		for c := 0; c < cols; c++ {
			char := layout[r][c]
			cell := game.Cell{}
			switch char {
			case '1':
				cell = game.Cell{Owner: 1, Kind: game.Normal}
			case 'b': // player 1 base
				cell = game.Cell{Owner: 1, Kind: game.Base}
			case '2':
				cell = game.Cell{Owner: 2, Kind: game.Normal}
			case 'B': // player 2 base
				cell = game.Cell{Owner: 2, Kind: game.Base}
			case 'N': // Neutral
				cell = game.Cell{Kind: game.Neutral}
			case '.':
				cell = game.Cell{Kind: game.Empty}
			}
			snap.Board[r][c] = cell
		}
	}
	state, err := game.FromSnapshot(snap)
	if err != nil {
		panic(err)
	}
	return state
}

func TestHaloDominancePolicy(t *testing.T) {
	// 1. Production motif avoids redundant halo (authoritative outward Move exists)
	t.Run("production motif avoids redundant halo", func(t *testing.T) {
		state := makeStateFromLayout(5, 5, 1, 2, []string{
			"b....",
			".1...",
			".....",
			".....",
			"....B",
		})
		s := newSearcher(context.Background(), state)
		children, _, _, ok := s.orderedChildren(game.NewPosition(state), true)
		if !ok {
			t.Fatal("orderedChildren failed")
		}

		foundHalo := false
		foundOutward := false
		for _, child := range children {
			if child.action.Kind == game.Move {
				tgt := child.action.Target
				if (tgt.Row == 0 && tgt.Col == 1) || (tgt.Row == 1 && tgt.Col == 0) {
					foundHalo = true
				}
				if tgt.Row == 2 && tgt.Col == 2 {
					foundOutward = true
				}
			}
		}
		if foundHalo {
			t.Error("unforced halo move was not suppressed when preserving outward move exists")
		}
		if !foundOutward {
			t.Error("preserving outward move was missing or suppressed")
		}
	})

	// 2. Forced first when PlaceNeutrals may be legal
	t.Run("forced first when PlaceNeutrals may be legal", func(t *testing.T) {
		// Player 1 has base at (0,0) and Normal at (1,1).
		// PlaceNeutrals could be legal (movesLeft = 3), but there are no outward Moves.
		// Layout has all outward Move cells blocked by Neutrals, leaving only halo cells empty.
		state := makeStateFromLayout(5, 5, 1, 3, []string{
			"b.N1N",
			".1NNN",
			"NNNNN",
			"NNNNN",
			"NNNNB",
		})
		s := newSearcher(context.Background(), state)
		children, _, _, ok := s.orderedChildren(game.NewPosition(state), true)
		if !ok {
			t.Fatal("orderedChildren failed")
		}

		hasHaloMove := false
		for _, child := range children {
			if child.action.Kind == game.Move {
				tgt := child.action.Target
				if (tgt.Row == 0 && tgt.Col == 1) || (tgt.Row == 1 && tgt.Col == 0) {
					hasHaloMove = true
				}
			}
		}
		if !hasHaloMove {
			t.Error("forced first halo moves were suppressed when PlaceNeutrals is legal but no outward Move exists")
		}
	})

	// 3. Contact disables
	t.Run("contact disables halo suppression", func(t *testing.T) {
		// Normal at (1,1) is adjacent to opponent normal at (1,2) -> contact!
		state := makeStateFromLayout(5, 5, 1, 2, []string{
			"b....",
			".12..",
			".....",
			".....",
			"....B",
		})
		s := newSearcher(context.Background(), state)
		children, _, _, ok := s.orderedChildren(game.NewPosition(state), true)
		if !ok {
			t.Fatal("orderedChildren failed")
		}

		hasHaloMove := false
		for _, child := range children {
			if child.action.Kind == game.Move {
				tgt := child.action.Target
				if (tgt.Row == 0 && tgt.Col == 1) || (tgt.Row == 1 && tgt.Col == 0) {
					hasHaloMove = true
				}
			}
		}
		if !hasHaloMove {
			t.Error("halo moves were suppressed under contact")
		}
	})

	// 4. Candidate set preserves capture and neutrals by construction
	t.Run("candidate set preserves capture and neutrals by construction", func(t *testing.T) {
		// Captures and neutrals are not suppressed because they are not dominated empty-halo moves.
		// Player 1 has base at (0,0), normal at (1,1).
		// (0,1) is opponent cell owned by P2 (so Move to (0,1) is a capture).
		// (1,0) is empty (halo).
		// (2,2) is empty (outward).
		state := makeStateFromLayout(5, 5, 1, 3, []string{
			"b2...",
			".1...",
			"..1..",
			".....",
			"....B",
		})
		s := newSearcher(context.Background(), state)
		children, _, _, ok := s.orderedChildren(game.NewPosition(state), true)
		if !ok {
			t.Fatal("orderedChildren failed")
		}

		hasCapture := false
		hasNeutrals := false
		for _, child := range children {
			if child.action.Kind == game.Move && child.action.Target.Row == 0 && child.action.Target.Col == 1 {
				hasCapture = true
			}
			if child.action.Kind == game.PlaceNeutrals {
				hasNeutrals = true
			}
		}
		if !hasCapture {
			t.Error("capture targeting the halo was incorrectly suppressed")
		}
		if !hasNeutrals {
			t.Error("neutrals placement was incorrectly suppressed")
		}
	})

	// 5. Sole preserving fallback
	t.Run("sole preserving fallback allowed", func(t *testing.T) {
		// Only a halo move is legal/preserving. It must be allowed.
		state := makeStateFromLayout(5, 5, 1, 2, []string{
			"b....",
			"NNNNN",
			"NNNNN",
			"NNNNN",
			"NNNNB",
		})
		s := newSearcher(context.Background(), state)
		children, _, _, ok := s.orderedChildren(game.NewPosition(state), true)
		if !ok {
			t.Fatal("orderedChildren failed")
		}

		hasHalo := false
		for _, child := range children {
			if child.action.Kind == game.Move {
				tgt := child.action.Target
				if (tgt.Row == 0 && tgt.Col == 1) || (tgt.Row == 1 && tgt.Col == 0) {
					hasHalo = true
				}
			}
		}
		if !hasHalo {
			t.Error("sole preserving halo move was suppressed")
		}
	})

	// 6. Serial/parallel equality
	t.Run("serial and parallel search identical", func(t *testing.T) {
		state := makeStateFromLayout(5, 5, 1, 2, []string{
			"b....",
			".1...",
			".....",
			".....",
			"....B",
		})
		sSerial := newSearcher(context.Background(), state)
		resSerial, okSerial := sSerial.atDepth(state, 3)
		if !okSerial {
			t.Fatal("serial search failed")
		}

		sParallel := newSearcher(context.Background(), state)
		resParallel, okParallel := sParallel.atDepthParallel(state, 3, 2, resSerial.Action)
		if !okParallel {
			t.Fatal("parallel search failed")
		}

		if resSerial.Action != resParallel.Action || resSerial.Score != resParallel.Score {
			t.Fatalf("mismatch: serial=%+v (score %d), parallel=%+v (score %d)",
				resSerial.Action, resSerial.Score, resParallel.Action, resParallel.Score)
		}
	})

	// 7. Rectangle/all seats
	t.Run("rectangle and all seats applicable", func(t *testing.T) {
		// Rectangle 5x6, player 2 (currentPlayer = 2) at base (4,5)
		state := makeStateFromLayout(5, 6, 2, 2, []string{
			"b.....",
			"......",
			"......",
			"....2.",
			".....B",
		})
		s := newSearcher(context.Background(), state)
		children, _, _, ok := s.orderedChildren(game.NewPosition(state), true)
		if !ok {
			t.Fatal("orderedChildren failed")
		}

		foundHalo := false
		for _, child := range children {
			if child.action.Kind == game.Move {
				tgt := child.action.Target
				// Seat 2 halo adjacent to (4,5): (3,4), (3,5), (4,4)
				if (tgt.Row == 4 && tgt.Col == 4) || (tgt.Row == 3 && tgt.Col == 5) {
					foundHalo = true
				}
			}
		}
		if foundHalo {
			t.Error("unforced halo move for seat 2 on rectangle was not suppressed")
		}
	})
}

func TestOrderedChildrenNoBoardScaledAllocation(t *testing.T) {
	get_allocs := func(size int) float64 {
		layout := make([]string, size)
		layout[0] = "b" + recruitDots(size-1)
		for i := 1; i < size-1; i++ {
			layout[i] = recruitDots(size)
		}
		layout[size-1] = recruitDots(size-1) + "B"
		state := makeStateFromLayout(size, size, 1, 2, layout)
		s := newSearcher(context.Background(), state)
		// Warm up
		_, _, _, _ = s.orderedChildren(game.NewPosition(state), true)
		return testing.AllocsPerRun(100, func() {
			_, _, _, _ = s.orderedChildren(game.NewPosition(state), true)
		})
	}

	// This asserts allocations do NOT grow with board size, not that they are
	// zero: orderedChildren keeps a fixed nonzero baseline (~35 allocs/op) driven
	// by the children slice and eval workspace, independent of rows*cols. The
	// contact scan reuses the searcher's preallocated buffers, so a larger board
	// must not add allocations.
	allocs10 := get_allocs(10)
	allocs20 := get_allocs(20)
	allocs30 := get_allocs(30)

	if allocs10 != allocs20 || allocs20 != allocs30 {
		t.Fatalf("orderedChildren allocations scaled with board size: 10x10=%f, 20x20=%f, 30x30=%f",
			allocs10, allocs20, allocs30)
	}
}

func TestOrderedChildren50x50Smoke(t *testing.T) {
	layout := make([]string, 50)
	layout[0] = "b" + recruitDots(49)
	for i := 1; i < 49; i++ {
		layout[i] = recruitDots(50)
	}
	layout[49] = recruitDots(49) + "B"

	state := makeStateFromLayout(50, 50, 1, 2, layout)
	s := newSearcher(context.Background(), state)

	start := time.Now()
	_, _, _, ok := s.orderedChildren(game.NewPosition(state), true)
	if !ok {
		t.Fatal("orderedChildren failed")
	}
	duration := time.Since(start)
	if duration > 15*time.Millisecond {
		t.Fatalf("orderedChildren took too long: %v, want < 15ms", duration)
	}
}

func recruitDots(n int) string {
	res := ""
	for i := 0; i < n; i++ {
		res = res + "."
	}
	return res
}

func BenchmarkOrderedChildrenRootPolicy(b *testing.B) {
	state := makeStateFromLayout(20, 20, 1, 2, []string{
		"b...................",
		".1..................",
		"....................",
		"....................",
		"....................",
		"....................",
		"....................",
		"....................",
		"....................",
		"....................",
		"....................",
		"....................",
		"....................",
		"....................",
		"....................",
		"....................",
		"....................",
		"....................",
		"....................",
		"...................B",
	})
	s := newSearcher(context.Background(), state)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _, _, ok := s.orderedChildren(game.NewPosition(state), true)
		if !ok {
			b.Fatal("orderedChildren failed")
		}
	}
}
