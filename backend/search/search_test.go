package search

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"math/rand"
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
	children, ok := s.orderedChildren(state, game.Action{}, false)
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

// Pins exact search Results under the vs-ai2.34 space-race evaluator
// (spaceRaceWeight = 32); no longer tracks origin/main, whose evaluator
// lacked the space term. Scores changed with the re-pin; actions did not.
// vs-ai2.41 re-pin: TT best-move ordering + one searcher hoisted across the
// ID loop + fail-soft bound fix. Only the budget-1000 minimax Action moved
// (equal-Score tie now broken by the persisted TT move); Nodes/Evals/Score
// unchanged.
// vs-ai2.42 re-pin: 1v1 PVS (null-window scout + re-search) shifted the
// depth-2 minimax fixture's Nodes 216->220, Evaluations 199->202 (scouts that
// fail inside the window trigger a full-window re-search, adding a few nodes at
// this tiny fixture; the payoff is at deeper searches). Action/Score/Depth
// unchanged. The budget-1000 minimax result and both maxn fixtures are
// unchanged (maxn immediate pruning only fires when a winning child exists).
// vs-ai2.35 ships ONLY Lever 1 (opponent ordering) on by default. Task 7
// acceptance ran the full suite and caught that Lever 2 (threat extensions) and
// Lever 3 (root safety) each regress the deterministic legacy strength gate
// below its 85% floor (62.5% and 75% respectively), while Lever 1 is neutral on
// it; per the plan's own policy ("a lever that does not measurably improve any
// gate is disabled/reverted, not shipped") both regressors are defaulted false
// (code kept, re-enable via SetSearchLevers / the arena lever sweep). Lever 1
// and Lever 3 leave every fixture in this corpus byte-identical to all-off, so
// with Lever 2 off the pins revert to their pre-Lever-2 (vs-ai2.42) values:
// depth-2 minimax Nodes 220, budget-1000 minimax Evaluations 916. Actions/
// Scores/Depth unchanged; both maxn fixtures are unchanged (all levers 1v1-only).
func TestSearchMatchesOriginMainAtFixedDepthAndNodes(t *testing.T) {
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
		name      string
		state     game.State
		wantDepth Result
		wantNodes Result
	}{
		{
			name: "minimax", state: two,
			wantDepth: Result{Action: move(2, 3), Score: 26644, Depth: 2, Nodes: 220, Evaluations: 202},
			wantNodes: Result{Action: move(3, 4), Score: 26644, Depth: 2, Nodes: 1000, Evaluations: 916, BudgetExhausted: true},
		},
		{
			name: "maxn", state: three,
			wantDepth: Result{Action: move(1, 2), Score: 6242, Depth: 2, Nodes: 46, Evaluations: 40},
			wantNodes: Result{Action: move(1, 2), Score: 9425, Depth: 3, Nodes: 1000, Evaluations: 814, BudgetExhausted: true},
		},
	} {
		t.Run(fixture.name, func(t *testing.T) {
			if _, err := fixture.state.Apply(fixture.wantDepth.Action); err != nil {
				t.Fatalf("pinned depth action %+v is illegal: %v", fixture.wantDepth.Action, err)
			}
			if _, err := fixture.state.Apply(fixture.wantNodes.Action); err != nil {
				t.Fatalf("pinned node action %+v is illegal: %v", fixture.wantNodes.Action, err)
			}
			depth, ok := ChooseDepth(context.Background(), fixture.state, 2)
			if !ok || depth != fixture.wantDepth {
				t.Fatalf("fixed-depth result = %+v ok=%v, want vs-ai2.34 golden %+v", depth, ok, fixture.wantDepth)
			}
			nodes, ok := ChooseNodeBudget(fixture.state, 1000)
			if !ok || nodes != fixture.wantNodes {
				t.Fatalf("fixed-node result = %+v ok=%v, want vs-ai2.34 golden %+v", nodes, ok, fixture.wantNodes)
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

// denseState12x12 builds a reproducible dense 12x12 midgame with a
// search-independent policy (always the first legal action) so the benchmark
// fixture is bit-identical before and after the Position-path node expansion.
func denseState12x12(tb testing.TB) game.State {
	tb.Helper()
	state, err := game.New(12, 12, 2)
	if err != nil {
		tb.Fatal(err)
	}
	for ply := 0; ply < 60; ply++ {
		actions := state.LegalActions()
		if len(actions) == 0 {
			break
		}
		next, err := state.Apply(actions[0])
		if err != nil {
			break
		}
		state = next
	}
	return state
}

// BenchmarkNodeBudgetDense12x12 measures move-generation throughput on a dense
// board at a fixed node budget, reporting nodes explored and completed depth.
func BenchmarkNodeBudgetDense12x12(b *testing.B) {
	state := denseState12x12(b)
	const budget = 200_000
	var nodes uint64
	var depth int
	for i := 0; i < b.N; i++ {
		result, ok := ChooseNodeBudget(state, budget)
		if !ok {
			b.Fatal("search failed")
		}
		nodes, depth = result.Nodes, result.Depth
	}
	b.ReportMetric(float64(nodes), "nodes")
	b.ReportMetric(float64(depth), "completedDepth")
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

func TestStrangleCount(t *testing.T) {
	// 5x5 board; player 1 owns a ring of Normal cells around target (2,2), plus
	// one Base neighbor. Player 2 owns one neighbor (must not count). One
	// neighbor stays Empty. Expected root-owned (Normal|Base) neighbors: 6.
	board := make([][]game.Cell, 5)
	for r := range board {
		board[r] = make([]game.Cell, 5)
	}
	// 8 neighbors of (2,2): rows 1..3, cols 1..3 except (2,2).
	board[1][1] = game.Cell{Owner: 1, Kind: game.Base}   // counts
	board[1][2] = game.Cell{Owner: 1, Kind: game.Normal} // counts
	board[1][3] = game.Cell{Owner: 1, Kind: game.Normal} // counts
	board[2][1] = game.Cell{Owner: 1, Kind: game.Normal} // counts
	board[2][3] = game.Cell{Owner: 1, Kind: game.Fortified} // Fortified: does NOT count
	board[3][1] = game.Cell{Owner: 1, Kind: game.Normal} // counts
	board[3][2] = game.Cell{Owner: 2, Kind: game.Normal} // opponent: does NOT count
	board[3][3] = game.Cell{Owner: 1, Kind: game.Normal} // counts; (empty otherwise)
	// player 2 base off in a corner so the snapshot validates.
	board[4][4] = game.Cell{Owner: 2, Kind: game.Base}

	snapshot := game.Snapshot{
		Rows: 5, Cols: 5, Board: board,
		Bases:       []game.Pos{{Row: 1, Col: 1}, {Row: 4, Col: 4}},
		Active:      []bool{true, true},
		NeutralUsed: []bool{false, false},
		Current:     1, MovesLeft: 3,
	}
	state, err := game.FromSnapshot(snapshot)
	if err != nil {
		t.Fatalf("FromSnapshot: %v", err)
	}
	if got := strangleCount(state, 1, game.Pos{Row: 2, Col: 2}); got != 6 {
		t.Fatalf("strangleCount = %d, want 6", got)
	}
	// A corner target has only 3 in-bounds neighbors; none owned by root here.
	if got := strangleCount(state, 1, game.Pos{Row: 0, Col: 0}); got != 1 {
		t.Fatalf("corner strangleCount = %d, want 1 (only (1,1) base)", got)
	}
}

// strangleFixture builds a 5x5 1v1 board where player 2 is the given `current`
// mover. Player 1 owns a cluster (base + normals) so that player 2's empty move
// target (1,1) is surrounded by 5 root cells; no player-2 cell touches a player-1
// normal, so no capture move exists to dominate the strangle ordering term.
func strangleFixture(t *testing.T, current game.Player) game.State {
	t.Helper()
	board := make([][]game.Cell, 5)
	for r := range board {
		board[r] = make([]game.Cell, 5)
	}
	board[0][0] = game.Cell{Owner: 1, Kind: game.Base}
	board[0][1] = game.Cell{Owner: 1, Kind: game.Normal}
	board[0][2] = game.Cell{Owner: 1, Kind: game.Normal}
	board[1][2] = game.Cell{Owner: 1, Kind: game.Normal}
	board[2][2] = game.Cell{Owner: 1, Kind: game.Normal}
	board[2][0] = game.Cell{Owner: 2, Kind: game.Base}
	state, err := game.FromSnapshot(game.Snapshot{
		Rows: 5, Cols: 5, Board: board,
		Bases:       []game.Pos{{Row: 0, Col: 0}, {Row: 2, Col: 0}},
		Active:      []bool{true, true},
		NeutralUsed: []bool{false, false},
		Current:     current, MovesLeft: 3,
	})
	if err != nil {
		t.Fatalf("FromSnapshot: %v", err)
	}
	return state
}

func findChild(children []child, action game.Action) (child, bool) {
	for _, c := range children {
		if c.action == action {
			return c, true
		}
	}
	return child{}, false
}

func TestLeverOpponentStrangleOrdersSqueezeFirst(t *testing.T) {
	defer SetSearchLevers(true, false, false)
	// Root is player 1 (searcher built from a player-1-to-move state), but the
	// node being ordered has player 2 to move — an opponent node in 1v1.
	root := strangleFixture(t, 1)
	opp := strangleFixture(t, 2)
	s := newSearcher(context.Background(), root)
	if s.root != 1 || s.multi {
		t.Fatalf("bad searcher: root=%d multi=%v", s.root, s.multi)
	}
	squeeze := move(1, 1) // empty target surrounded by 5 root cells

	SetSearchLevers(true, true, true)
	children, ok := s.orderedChildren(opp, game.Action{}, false)
	if !ok {
		t.Fatal("ordering canceled")
	}
	if children[0].action != squeeze {
		t.Fatalf("lever on: first child = %+v, want %+v", children[0].action, squeeze)
	}
	c, found := findChild(children, squeeze)
	if !found {
		t.Fatal("squeeze move missing from children")
	}
	if c.order != 100+5*1000 {
		t.Fatalf("squeeze order = %d, want %d (retain-turn + 5*1000)", c.order, 100+5*1000)
	}

	// Determinism: identical repeat call.
	again, _ := s.orderedChildren(opp, game.Action{}, false)
	if !reflect.DeepEqual(children, again) {
		t.Fatal("orderedChildren not deterministic across calls")
	}
}

func TestLeverOpponentStrangleOffMatchesBaseline(t *testing.T) {
	defer SetSearchLevers(true, false, false)
	root := strangleFixture(t, 1)
	opp := strangleFixture(t, 2)
	s := newSearcher(context.Background(), root)
	squeeze := move(1, 1)

	SetSearchLevers(false, true, true)
	children, ok := s.orderedChildren(opp, game.Action{}, false)
	if !ok {
		t.Fatal("ordering canceled")
	}
	c, found := findChild(children, squeeze)
	if !found {
		t.Fatal("squeeze move missing from children")
	}
	// No strangle bonus: only the retain-turn term survives.
	if c.order != 100 {
		t.Fatalf("lever off: squeeze order = %d, want 100 (no strangle bonus)", c.order)
	}
}

// threatFixture builds a 5x5 1v1 board (player 1 to move = root) where player 2
// owns a Normal at (2,3) directly left-adjacent to player 1's Normal at (2,2),
// so player 2 has a move that captures a root-owned Normal — a Lever 2 threat
// edge that appears at every opponent-to-move node in the search.
func threatFixture(t *testing.T) game.State {
	t.Helper()
	board := make([][]game.Cell, 5)
	for r := range board {
		board[r] = make([]game.Cell, 5)
	}
	board[0][0] = game.Cell{Owner: 1, Kind: game.Base}
	board[0][1] = game.Cell{Owner: 1, Kind: game.Normal}
	board[2][2] = game.Cell{Owner: 1, Kind: game.Normal} // capture target
	board[2][4] = game.Cell{Owner: 2, Kind: game.Base}
	board[2][3] = game.Cell{Owner: 2, Kind: game.Normal} // connected to base; can move-capture (2,2)
	state, err := game.FromSnapshot(game.Snapshot{
		Rows: 5, Cols: 5, Board: board,
		Bases:       []game.Pos{{Row: 0, Col: 0}, {Row: 2, Col: 4}},
		Active:      []bool{true, true},
		NeutralUsed: []bool{false, false},
		Current:     1, MovesLeft: 1, // last move of the turn: one root move hands over to player 2
	})
	if err != nil {
		t.Fatalf("FromSnapshot: %v", err)
	}
	return state
}

func TestLeverThreatExtendFiresAndIsBounded(t *testing.T) {
	defer SetSearchLevers(true, false, false)
	state := threatFixture(t)

	// Sanity: player 2 really does have a capture of our (2,2) Normal.
	opp := play(t, state, move(1, 1)) // any root move; player 2 to move next
	if opp.CurrentPlayer() != 2 {
		t.Fatalf("fixture did not reach player-2 to move: %d", opp.CurrentPlayer())
	}
	s := newSearcher(context.Background(), state)
	children, ok := s.orderedChildren(opp, game.Action{}, false)
	if !ok {
		t.Fatal("ordering canceled")
	}
	c, found := findChild(children, move(2, 2))
	if !found || !c.threat {
		t.Fatalf("expected a threat edge capturing (2,2); found=%v threat=%v", found, found && c.threat)
	}

	// Extension on searches the threatened line deeper => strictly more nodes.
	SetSearchLevers(true, true, true)
	on, ok := ChooseDepth(context.Background(), state, 3)
	if !ok {
		t.Fatal("extend-on search did not complete")
	}
	onAgain, ok := ChooseDepth(context.Background(), state, 3)
	if !ok || on != onAgain {
		t.Fatalf("extend-on not deterministic: %+v / %+v", on, onAgain)
	}

	SetSearchLevers(true, false, true)
	off, ok := ChooseDepth(context.Background(), state, 3)
	if !ok {
		t.Fatal("extend-off search did not complete")
	}
	if on.Nodes <= off.Nodes {
		t.Fatalf("extension did not fire: on nodes %d <= off nodes %d", on.Nodes, off.Nodes)
	}

	// Bounded: a node-budgeted search with the extension on still terminates.
	SetSearchLevers(true, true, true)
	budget, ok := ChooseNodeBudget(state, 2000)
	if !ok {
		t.Fatal("budgeted extend-on search returned no action")
	}
	if budget.Nodes > 2000 {
		t.Fatalf("extension blew the node budget: %d > 2000", budget.Nodes)
	}
}

func TestLeverThreatExtendOffMatchesNonExtended(t *testing.T) {
	defer SetSearchLevers(true, false, false)
	state := threatFixture(t)

	// Hold ordering+rootSafety fixed; only the extend lever moves. Turning it off
	// must return the search to its non-extended node count, deterministically.
	SetSearchLevers(true, true, true)
	on, ok := ChooseDepth(context.Background(), state, 3)
	if !ok {
		t.Fatal("extend-on search did not complete")
	}
	SetSearchLevers(true, false, true)
	off, ok := ChooseDepth(context.Background(), state, 3)
	if !ok {
		t.Fatal("extend-off search did not complete")
	}
	offAgain, ok := ChooseDepth(context.Background(), state, 3)
	if !ok || off != offAgain {
		t.Fatalf("extend-off not deterministic: %+v / %+v", off, offAgain)
	}
	if off.Nodes >= on.Nodes {
		t.Fatalf("disabling extend did not reduce nodes: off %d >= on %d", off.Nodes, on.Nodes)
	}
}

// catastropheChild builds a 3x3 1v1 state with player 2 to move where player 2's
// board-first reply captures player 1's sole bridge Normal at (1,1). Capturing a
// Normal fortifies it (uncapturable), stranding player 1's base with no move: a
// 0-mobility strangulation. Used as the state AFTER a turn-ending root candidate.
func catastropheChild(t *testing.T) game.State {
	t.Helper()
	board := make([][]game.Cell, 3)
	for r := range board {
		board[r] = make([]game.Cell, 3)
	}
	board[0][0] = game.Cell{Owner: 1, Kind: game.Base}
	board[0][1] = game.Cell{Owner: 2, Kind: game.Fortified}
	board[0][2] = game.Cell{Owner: 2, Kind: game.Base}
	board[1][0] = game.Cell{Owner: 2, Kind: game.Fortified}
	board[1][1] = game.Cell{Owner: 1, Kind: game.Normal} // sole bridge to open space
	state, err := game.FromSnapshot(game.Snapshot{
		Rows: 3, Cols: 3, Board: board,
		Bases:       []game.Pos{{Row: 0, Col: 0}, {Row: 0, Col: 2}},
		Active:      []bool{true, true},
		NeutralUsed: []bool{false, false},
		Current:     2, MovesLeft: 1,
	})
	if err != nil {
		t.Fatalf("FromSnapshot: %v", err)
	}
	return state
}

func TestLeverRootSafetyDropsCatastrophe(t *testing.T) {
	defer SetSearchLevers(true, false, false)
	bad := catastropheChild(t)
	// Sanity: player 2's capture of (1,1) really strangles player 1 dead.
	if dead := play(t, bad, move(1, 1)); dead.Active(1) {
		t.Fatal("fixture is not catastrophic: player 1 survives the capture")
	}

	safe := strangleFixture(t, 2) // player 2 to move, player 1 stays mobile
	s := newSearcher(context.Background(), strangleFixture(t, 1))
	if s.root != 1 || s.multi {
		t.Fatalf("bad searcher: root=%d multi=%v", s.root, s.multi)
	}
	children := []child{
		{action: move(9, 9), state: bad},
		{action: move(8, 8), state: safe},
	}

	SetSearchLevers(true, true, true)
	kept := s.rootSafetyFilter(children)
	if _, found := findChild(kept, move(8, 8)); !found {
		t.Fatal("safe candidate dropped")
	}
	if _, found := findChild(kept, move(9, 9)); found {
		t.Fatalf("catastrophic candidate survived: kept %d children", len(kept))
	}

	// Determinism: identical repeat call.
	again := s.rootSafetyFilter(children)
	if !reflect.DeepEqual(kept, again) {
		t.Fatal("rootSafetyFilter not deterministic across calls")
	}

	// Fallback: when EVERY turn-ending candidate is catastrophic, the filter
	// still returns a non-empty list (the least-bad one).
	allBad := []child{
		{action: move(9, 9), state: catastropheChild(t)},
		{action: move(8, 8), state: catastropheChild(t)},
	}
	if kept := s.rootSafetyFilter(allBad); len(kept) == 0 {
		t.Fatal("filter emptied the candidate list on the all-catastrophic fixture")
	}
}

func TestLeverRootSafetyOffKeepsCatastrophe(t *testing.T) {
	defer SetSearchLevers(true, false, false)
	children := []child{
		{action: move(9, 9), state: catastropheChild(t)},
		{action: move(8, 8), state: strangleFixture(t, 2)},
	}
	s := newSearcher(context.Background(), strangleFixture(t, 1))

	SetSearchLevers(true, true, false)
	kept := s.rootSafetyFilter(children)
	if !reflect.DeepEqual(kept, children) {
		t.Fatalf("filter off changed candidates: %d -> %d", len(children), len(kept))
	}
}

// multiPlyCatastropheChild is a real turn-ending root candidate: player 2 to move
// with a FULL 3-action turn (MovesLeft: 3, as advance() always hands over). Player
// 1's base at (0,0) has three liberties (0,1)/(1,0)/(1,1); no single opponent move
// strangles it, but the opponent can fill all three over its turn. This is the
// case the old single-ply floor missed (it counted the opponent's mid-turn moves,
// not root's post-turn mobility).
func multiPlyCatastropheChild(t *testing.T) game.State {
	t.Helper()
	board := make([][]game.Cell, 3)
	for r := range board {
		board[r] = make([]game.Cell, 3)
	}
	board[0][0] = game.Cell{Owner: 1, Kind: game.Base}
	board[0][2] = game.Cell{Owner: 2, Kind: game.Base}
	board[1][2] = game.Cell{Owner: 2, Kind: game.Fortified}
	board[2][0] = game.Cell{Owner: 2, Kind: game.Fortified}
	board[2][1] = game.Cell{Owner: 2, Kind: game.Fortified}
	board[2][2] = game.Cell{Owner: 2, Kind: game.Fortified}
	state, err := game.FromSnapshot(game.Snapshot{
		Rows: 3, Cols: 3, Board: board,
		Bases:       []game.Pos{{Row: 0, Col: 0}, {Row: 0, Col: 2}},
		Active:      []bool{true, true},
		NeutralUsed: []bool{false, false},
		Current:     2, MovesLeft: 3,
	})
	if err != nil {
		t.Fatalf("FromSnapshot: %v", err)
	}
	return state
}

func TestLeverRootSafetyPlaysOutOpponentTurn(t *testing.T) {
	defer SetSearchLevers(true, false, false)
	SetSearchLevers(true, true, true)
	bad := multiPlyCatastropheChild(t)
	s := newSearcher(context.Background(), strangleFixture(t, 1))
	if s.root != 1 || s.multi {
		t.Fatalf("bad searcher: root=%d multi=%v", s.root, s.multi)
	}

	// Guard: no SINGLE opponent reply strangles root — root survives one ply, so
	// only playing the turn out to the end exposes the catastrophe.
	for _, reply := range bad.LegalActions() {
		next, err := bad.Apply(reply)
		if err == nil && !next.Active(1) {
			t.Fatalf("fixture strangles in one ply (reply %+v); not a multi-ply case", reply)
		}
	}

	// The played-out turn leaves root with zero mobility => floor 0 => catastrophe.
	if floor := s.rootSafetyFloor(bad); floor != 0 {
		t.Fatalf("multi-ply catastrophe not detected: floor = %d, want 0", floor)
	}

	safe := []child{
		{action: move(9, 9), state: bad},
		{action: move(8, 8), state: strangleFixture(t, 2)},
	}
	kept := s.rootSafetyFilter(safe)
	if _, found := findChild(kept, move(9, 9)); found {
		t.Fatal("multi-ply catastrophic candidate survived the filter")
	}
	if _, found := findChild(kept, move(8, 8)); !found {
		t.Fatal("safe candidate dropped")
	}
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
