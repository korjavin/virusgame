package search

import (
	"context"
	"crypto/sha256"
	"encoding/binary"
	"fmt"
	"math/rand"
	"slices"
	"sync"
	"testing"

	"virusgame/game"
)

func TestBaseEscapeStructureSeparatesOwnedExitsFromOpenings(t *testing.T) {
	state := mustState(t, 6, 9, 2)
	initial := analyze(state, 1)
	if initial.baseExits != 0 || initial.baseOpenings != 3 {
		t.Fatalf("initial exits/openings = %d/%d, want 0/3", initial.baseExits, initial.baseOpenings)
	}

	state = play(t, state, move(1, 1))
	got := analyze(state, 1)
	if got.baseExits != 1 || got.baseOpenings != 2 {
		t.Fatalf("owned expansion exits/openings = %d/%d, want 1/2", got.baseExits, got.baseOpenings)
	}
}

func TestThreatenedCutUsesBaseRootedComponentLoss(t *testing.T) {
	state := mustState(t, 6, 6, 2)
	state = play(t, state,
		move(1, 1), move(2, 2), move(2, 3),
		move(4, 4), move(3, 3), move(3, 2),
	)

	before := analyze(state, 1)
	if before.threatenedLoss < 2 {
		t.Fatalf("threatened component loss = %d, want at least 2", before.threatenedLoss)
	}
	defended, err := state.Apply(move(1, 2))
	if err != nil {
		t.Fatal(err)
	}
	after := analyze(defended, 1)
	if after.threatenedLoss >= before.threatenedLoss {
		t.Fatalf("defense did not reduce threatened loss: %d -> %d", before.threatenedLoss, after.threatenedLoss)
	}
}

func TestForcedDefenseRanksAboveNeutralCleanupOnBoardShapes(t *testing.T) {
	tests := []struct {
		name       string
		rows, cols int
		setup      []game.Action
	}{
		{
			name: "square", rows: 6, cols: 6,
			setup: []game.Action{
				move(1, 1), move(2, 2), move(2, 3),
				move(4, 4), move(3, 3), move(3, 2),
			},
		},
		{
			name: "rectangular", rows: 6, cols: 7,
			setup: []game.Action{
				move(1, 1), move(2, 2), move(2, 3),
				move(4, 5), move(3, 4), move(3, 3),
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			state := mustState(t, tt.rows, tt.cols, 2)
			state = play(t, state, tt.setup...)
			if analyze(state, 1).threatenedLoss == 0 {
				t.Fatal("fixture has no forced component defense")
			}
			result := completedDepth(t, state, 1)
			if result.Action.Kind == game.PlaceNeutrals {
				t.Fatalf("chose irrelevant neutral cleanup: %+v", result.Action)
			}
			target, _ := state.At(result.Action.Target)
			if target.Owner != 2 || target.Kind != game.Normal {
				t.Fatalf("chose %+v targeting %+v, want defensive capture", result.Action, target)
			}
		})
	}
}

func TestRecordedT20RanksDefenseAboveNeutralCleanup(t *testing.T) {
	state := mustState(t, 10, 10, 2)
	state = play(t, state,
		// Turns 1-6.
		move(1, 1), move(2, 2), move(3, 3),
		move(8, 8), move(7, 7), move(6, 6),
		move(4, 3), move(5, 3), move(3, 4),
		move(5, 5), move(4, 5), move(3, 4),
		move(4, 4), move(4, 5), move(5, 5),
		move(6, 5), move(5, 4), move(4, 4),
		// Turns 7-12.
		move(5, 4), move(6, 4), move(6, 5),
		move(7, 5), move(6, 4), move(5, 3),
		move(6, 3), move(7, 4), move(7, 5),
		move(7, 6), move(8, 5), move(7, 4),
		move(7, 3), move(8, 4), move(8, 5),
		move(8, 6), move(9, 5), move(8, 4),
		// Turns 13-19 from the production game ending in no_moves.
		move(8, 3), move(9, 4), move(9, 5),
		move(5, 7), move(4, 6), move(3, 5),
		move(2, 4), move(3, 5), move(4, 6),
		move(4, 8), move(5, 8), move(6, 7),
		move(5, 7), move(6, 7), move(7, 7),
		move(7, 9), move(7, 8), move(9, 8),
		move(7, 8), move(8, 8), move(7, 9),
	)
	if state.CurrentPlayer() != 2 || state.MovesLeft() != 3 {
		t.Fatalf("recorded T20 fixture at player %d with %d moves", state.CurrentPlayer(), state.MovesLeft())
	}
	cleanup := game.Action{Kind: game.PlaceNeutrals, Neutrals: [2]game.Pos{{Row: 4, Col: 8}, {Row: 5, Col: 8}}}
	cleanupState, err := state.Apply(cleanup)
	if err != nil {
		t.Fatalf("recorded T20 neutral action is not legal: %v", err)
	}

	result := completedDepth(t, state, 1)
	if result.Action.Kind == game.PlaceNeutrals {
		t.Fatalf("repeated losing T20 neutral cleanup: %+v", result.Action)
	}
	defended, err := state.Apply(result.Action)
	if err != nil {
		t.Fatal(err)
	}
	if got, bad := evaluate(defended, 2), evaluate(cleanupState, 2); got <= bad {
		t.Fatalf("defense score %d did not beat recorded cleanup %d", got, bad)
	}
}

func TestLocalBaseSafetyIsStableAcrossBoardSizes(t *testing.T) {
	var scores []int
	for _, dims := range [][2]int{{6, 6}, {6, 12}, {12, 6}} {
		state := mustState(t, dims[0], dims[1], 2)
		state = play(t, state, move(0, 1), move(1, 0), move(1, 1))
		metrics := analyze(state, 1)
		if metrics.baseExits != 3 || metrics.baseOpenings != 0 {
			t.Fatalf("%dx%d exits/openings = %d/%d", dims[0], dims[1], metrics.baseExits, metrics.baseOpenings)
		}
		scores = append(scores, evaluate(state, 1))
	}
	for i := 1; i < len(scores); i++ {
		if delta := absInt(scores[i] - scores[0]); delta > 2000 {
			t.Fatalf("size-normalized local scores diverged: %v", scores)
		}
	}
}

func TestEvaluateWorkspaceMatchesOriginMainOracle(t *testing.T) {
	hash := sha256.New()
	for _, fixture := range []struct {
		rows, cols, players int
		seed                int64
	}{
		{5, 5, 2, 1}, {5, 9, 3, 2}, {12, 20, 4, 3},
		{20, 12, 2, 4}, {28, 28, 3, 5}, {50, 50, 4, 6},
	} {
		state := randomReachableState(t, fixture.rows, fixture.cols, fixture.players, fixture.seed)
		workspace := evalWorkspace{}
		got := evaluateAllWithWorkspace(state, &workspace)
		for player := game.Player(1); player <= 4; player++ {
			if score := evaluateWithWorkspace(state, player, &workspace); score != got[player-1] {
				t.Fatalf("%dx%d/%dp seat %d score %d != vector %d", fixture.rows, fixture.cols, fixture.players, player, score, got[player-1])
			}
			var encoded [8]byte
			binary.LittleEndian.PutUint64(encoded[:], uint64(int64(got[player-1])))
			_, _ = hash.Write(encoded[:])
		}
	}
	// Generated by the pre-refactor evaluator at origin/main bf74a44.
	const oracle = "da74b56c333804ce68dd426c52a6c909a2aa8f14ebd05921287370bbf4e80a46"
	if got := fmt.Sprintf("%x", hash.Sum(nil)); got != oracle {
		t.Fatalf("workspace evaluator digest = %s, want origin/main oracle %s", got, oracle)
	}
}

func TestEvaluateWorkspaceGoldenStates(t *testing.T) {
	initial := mustState(t, 6, 6, 2)
	contact := play(t, initial,
		move(1, 1), move(2, 2), move(2, 3),
		move(4, 4), move(3, 3), move(3, 2),
	)
	neutralBase := play(t, initial,
		move(0, 1), move(1, 0), move(1, 1),
		move(5, 4), move(4, 5), move(4, 4),
	)
	neutral, err := neutralBase.Apply(game.Action{Kind: game.PlaceNeutrals, Neutrals: [2]game.Pos{{Row: 0, Col: 1}, {Row: 1, Col: 0}}})
	if err != nil {
		t.Fatal(err)
	}
	winning, action, ok := findWinningMove(t)
	if !ok {
		t.Fatal("missing terminal fixture")
	}
	terminal, err := winning.Apply(action)
	if err != nil || !terminal.GameOver() {
		t.Fatalf("terminal fixture: over=%v err=%v", terminal.GameOver(), err)
	}
	eliminated := randomEliminatedState(t)

	for _, fixture := range []struct {
		name  string
		state game.State
		want  [4]int
	}{
		{"initial", initial, [4]int{36, -36, -500000000, -500000000}},
		{"contact-threatened-cut", contact, [4]int{2036, -2036, -500000000, -500000000}},
		{"neutral", neutral, [4]int{-2639, 2639, -500000000, -500000000}},
		{"terminal", terminal, [4]int{mateScore, -mateScore, -mateScore, -mateScore}},
		{"eliminated", eliminated, [4]int{-500000000, 512, -512, -500000000}},
	} {
		workspace := evalWorkspace{}
		if got := evaluateAllWithWorkspace(fixture.state, &workspace); got != fixture.want {
			t.Fatalf("%s scores = %v, want origin/main %v", fixture.name, got, fixture.want)
		}
	}
}

func TestEvaluateWorkspaceBuffersDoNotAliasAndClearInactivePlayers(t *testing.T) {
	state := randomReachableState(t, 12, 20, 4, 3)
	workspace := evalWorkspace{}
	workspace.ensure(state.Rows() * state.Cols())
	cells := snapshotCellsInto(state, workspace.cells)
	connected := allConnectedInto(state, cells, &workspace)
	first := analyzeWithConnectivity(state, 1, cells, connected, &workspace.scratch,
		workspace.articulation[0], workspace.cutLoss[0])
	wantArticulation := append([]bool(nil), first.articulation...)
	wantCutLoss := append([]uint16(nil), first.cutLoss...)
	_ = analyzeWithConnectivity(state, 2, cells, connected, &workspace.scratch,
		workspace.articulation[1], workspace.cutLoss[1])
	if !slices.Equal(first.articulation, wantArticulation) || !slices.Equal(first.cutLoss, wantCutLoss) {
		t.Fatal("later player analysis mutated earlier player metrics")
	}

	twoPlayer := mustState(t, 5, 5, 2)
	_ = evaluateAllWithWorkspace(twoPlayer, &workspace)
	for player := 2; player < 4; player++ {
		for _, connected := range workspace.connected[player] {
			if connected {
				t.Fatalf("inactive player %d connectivity was not cleared", player+1)
			}
		}
	}
}

func TestEvaluateWorkspaceHasNoSteadyStateAllocations(t *testing.T) {
	workspace := evalWorkspace{}
	for _, dims := range [][2]int{{5, 5}, {12, 12}, {20, 20}, {50, 50}} {
		state := matureEvaluationState(t, dims[0], dims[1])
		want := evaluateAll(state)
		if got := evaluateAllWithWorkspace(state, &workspace); got != want {
			t.Fatalf("%dx%d scores = %v, want %v", dims[0], dims[1], got, want)
		}
		if allocations := testing.AllocsPerRun(100, func() { _ = evaluateAllWithWorkspace(state, &workspace) }); allocations != 0 {
			t.Fatalf("%dx%d workspace allocations = %.0f, want zero", dims[0], dims[1], allocations)
		}
	}
}

func TestEvaluateWorkspaceResizeSequence(t *testing.T) {
	workspace := evalWorkspace{}
	for _, dims := range [][2]int{{5, 5}, {50, 50}, {12, 20}, {5, 5}} {
		state := matureEvaluationState(t, dims[0], dims[1])
		if got, want := evaluateAllWithWorkspace(state, &workspace), evaluateAll(state); got != want {
			t.Fatalf("%dx%d resized scores = %v, want %v", dims[0], dims[1], got, want)
		}
	}
}

func TestIndependentEvaluateWorkspacesAreConcurrent(t *testing.T) {
	state := matureEvaluationState(t, 20, 20)
	want := evaluateAll(state)
	var wait sync.WaitGroup
	for worker := 0; worker < 2; worker++ {
		wait.Add(1)
		go func() {
			defer wait.Done()
			workspace := evalWorkspace{}
			for i := 0; i < 100; i++ {
				if got := evaluateAllWithWorkspace(state, &workspace); got != want {
					t.Errorf("concurrent scores = %v, want %v", got, want)
					return
				}
			}
		}()
	}
	wait.Wait()
}

func BenchmarkEvaluate12x12(b *testing.B) { benchmarkEvaluate(b, 12, 12) }
func BenchmarkEvaluate20x20(b *testing.B) { benchmarkEvaluate(b, 20, 20) }
func BenchmarkEvaluate50x50(b *testing.B) { benchmarkEvaluate(b, 50, 50) }

func benchmarkEvaluate(b *testing.B, rows, cols int) {
	state := matureEvaluationState(b, rows, cols)
	workspace := evalWorkspace{}
	_ = evaluateAllWithWorkspace(state, &workspace)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = evaluateAllWithWorkspace(state, &workspace)
	}
}

func randomReachableState(t *testing.T, rows, cols, players int, seed int64) game.State {
	t.Helper()
	state := mustState(t, rows, cols, players)
	rng := rand.New(rand.NewSource(seed))
	for step := 0; step < min(36, rows+cols) && !state.GameOver(); step++ {
		actions := state.LegalActions()
		moves := actions[:0]
		for _, action := range actions {
			if action.Kind == game.Move {
				moves = append(moves, action)
			}
		}
		if len(moves) == 0 {
			break
		}
		var err error
		state, err = state.Apply(moves[rng.Intn(len(moves))])
		if err != nil {
			t.Fatal(err)
		}
	}
	return state
}

func randomEliminatedState(t *testing.T) game.State {
	t.Helper()
	state := mustState(t, 5, 5, 3)
	rng := rand.New(rand.NewSource(91))
	for step := 0; step < 500 && !state.GameOver() && activeCount(state) == 3; step++ {
		actions := state.LegalActions()
		moves := actions[:0]
		for _, action := range actions {
			if action.Kind == game.Move {
				moves = append(moves, action)
			}
		}
		if len(moves) == 0 {
			break
		}
		var err error
		state, err = state.Apply(moves[rng.Intn(len(moves))])
		if err != nil {
			t.Fatal(err)
		}
	}
	if state.GameOver() || activeCount(state) == 3 {
		t.Fatal("failed to construct nonterminal eliminated-player fixture")
	}
	return state
}

type testFataler interface {
	Helper()
	Fatal(args ...any)
}

func matureEvaluationState(t testFataler, rows, cols int) game.State {
	t.Helper()
	state, err := game.New(rows, cols, 2)
	if err != nil {
		t.Fatal(err)
	}
	turns := min(7, (min(rows, cols)-2)/6)
	for turn := 0; turn < turns; turn++ {
		for step := 1; step <= 3; step++ {
			distance := turn*3 + step
			state, err = state.Apply(move(distance, distance))
			if err != nil {
				t.Fatal(err)
			}
		}
		for step := 1; step <= 3; step++ {
			distance := turn*3 + step
			state, err = state.Apply(move(rows-1-distance, cols-1-distance))
			if err != nil {
				t.Fatal(err)
			}
		}
	}
	return state
}

func absInt(value int) int {
	if value < 0 {
		return -value
	}
	return value
}

func TestEvaluatorEquivalence(t *testing.T) {
	for _, fixture := range []struct {
		rows, cols, players int
		seed                int64
	}{
		{5, 5, 2, 42}, {6, 6, 2, 43}, {8, 8, 3, 44}, {10, 10, 4, 45},
	} {
		state := randomReachableState(t, fixture.rows, fixture.cols, fixture.players, fixture.seed)
		workspace := evalWorkspace{}
		got := evaluateAllWithWorkspace(state, &workspace)
		want := oldEvaluateAllForTest(state)
		if got != want {
			t.Fatalf("Equivalence failed for %dx%d/%dp: got %v, want %v", fixture.rows, fixture.cols, fixture.players, got, want)
		}
	}
}

func TestEvaluatorCartesianEquivalence(t *testing.T) {
	sizes := [][2]int{{5, 5}, {12, 20}, {20, 12}, {20, 20}}
	playersOpts := []int{2, 3, 4}

	for _, sz := range sizes {
		for _, players := range playersOpts {
			t.Run(fmt.Sprintf("%dx%d-%dp", sz[0], sz[1], players), func(t *testing.T) {
				seed := int64(sz[0]*1000 + sz[1]*10 + players)
				state := randomReachableState(t, sz[0], sz[1], players, seed)

				workspace := evalWorkspace{}
				got := evaluateAllWithWorkspace(state, &workspace)
				want := oldEvaluateAllForTest(state)
				if got != want {
					t.Fatalf("Equivalence failed: got %v, want %v", got, want)
				}
			})
		}
	}
}

func TestEvaluatorTransformsSymmetry(t *testing.T) {
	for _, fixture := range []struct {
		rows, cols, players int
		seed                int64
	}{
		{5, 5, 2, 99}, {5, 5, 4, 100},
		{12, 12, 2, 101}, {12, 12, 4, 102},
	} {
		state := randomReachableState(t, fixture.rows, fixture.cols, fixture.players, fixture.seed)
		workspace := evalWorkspace{}
		baseScores := evaluateAllWithWorkspace(state, &workspace)

		// 1. Transposition (Rows <-> Cols, P3 <-> P4)
		tState := transposeStateWithPlayerSwaps(state)
		tScores := evaluateAllWithWorkspace(tState, &workspace)

		expectedTScores := baseScores
		if fixture.players >= 4 {
			expectedTScores[2], expectedTScores[3] = baseScores[3], baseScores[2]
		}
		if tScores != expectedTScores {
			t.Errorf("Transposed state evaluation mismatch for %dx%d-%dp: got %v, want %v",
				fixture.rows, fixture.cols, fixture.players, tScores, expectedTScores)
		}

		// 2. 180-degree rotation (P1 <-> P2, P3 <-> P4)
		rState := rotate180State(state)
		rScores := evaluateAllWithWorkspace(rState, &workspace)

		expectedRScores := baseScores
		expectedRScores[0], expectedRScores[1] = baseScores[1], baseScores[0]
		if fixture.players >= 4 {
			expectedRScores[2], expectedRScores[3] = baseScores[3], baseScores[2]
		}
		if rScores != expectedRScores {
			t.Errorf("180-degree rotated state evaluation mismatch for %dx%d-%dp: got %v, want %v",
				fixture.rows, fixture.cols, fixture.players, rScores, expectedRScores)
		}
	}
}

func TestChooseDepthChooseNodeBudgetDigestEquality(t *testing.T) {
	state := randomReachableState(t, 6, 6, 2, 777)

	// ChooseDepth
	r1, ok1 := ChooseDepth(context.Background(), state, 2)
	if !ok1 {
		t.Fatal("ChooseDepth failed")
	}
	r2, ok2 := ChooseDepth(context.Background(), state, 2)
	if !ok2 || r1.Action != r2.Action || r1.Nodes != r2.Nodes || r1.Evaluations != r2.Evaluations {
		t.Fatalf("ChooseDepth not deterministic: %+v vs %+v", r1, r2)
	}

	// ChooseNodeBudget
	rb1, okb1 := ChooseNodeBudget(state, 500)
	if !okb1 {
		t.Fatal("ChooseNodeBudget failed")
	}
	rb2, okb2 := ChooseNodeBudget(state, 500)
	if !okb2 || rb1.Action != rb2.Action || rb1.Nodes != rb2.Nodes || rb1.Evaluations != rb2.Evaluations {
		t.Fatalf("ChooseNodeBudget not deterministic: %+v vs %+v", rb1, rb2)
	}

	h := sha256.New()
	writeResult := func(r Result) {
		_ = binary.Write(h, binary.LittleEndian, uint32(r.Action.Kind))
		_ = binary.Write(h, binary.LittleEndian, int32(r.Action.Target.Row))
		_ = binary.Write(h, binary.LittleEndian, int32(r.Action.Target.Col))
		_ = binary.Write(h, binary.LittleEndian, uint64(r.Nodes))
		_ = binary.Write(h, binary.LittleEndian, uint64(r.Evaluations))
	}
	writeResult(r1)
	writeResult(rb1)
	digest := fmt.Sprintf("%x", h.Sum(nil))

	const expectedDigest = "487541197b96732ea20c575b884d4e888c97f70e36b45b63d8fc36223388867f"
	if digest != expectedDigest {
		t.Fatalf("ChooseDepth/ChooseNodeBudget digest = %s, want %s", digest, expectedDigest)
	}
}

func transposeStateWithPlayerSwaps(state game.State) game.State {
	snap := state.Snapshot()
	tSnap := game.Snapshot{
		Rows:        snap.Cols,
		Cols:        snap.Rows,
		Bases:       make([]game.Pos, len(snap.Bases)),
		Active:      make([]bool, len(snap.Active)),
		NeutralUsed: make([]bool, len(snap.NeutralUsed)),
		Current:     snap.Current,
		MovesLeft:   snap.MovesLeft,
		GameOver:    snap.GameOver,
		Winner:      snap.Winner,
		Board:       make([][]game.Cell, snap.Cols),
	}
	for col := 0; col < snap.Cols; col++ {
		tSnap.Board[col] = make([]game.Cell, snap.Rows)
		for row := 0; row < snap.Rows; row++ {
			tSnap.Board[col][row] = snap.Board[row][col]
		}
	}
	for i := range snap.Bases {
		tSnap.Bases[i] = game.Pos{Row: snap.Bases[i].Col, Col: snap.Bases[i].Row}
		tSnap.Active[i] = snap.Active[i]
		tSnap.NeutralUsed[i] = snap.NeutralUsed[i]
	}
	// Swap Player 3 and Player 4 bases/metadata under transposition
	if len(tSnap.Bases) >= 4 {
		tSnap.Bases[2], tSnap.Bases[3] = tSnap.Bases[3], tSnap.Bases[2]
		tSnap.Active[2], tSnap.Active[3] = tSnap.Active[3], tSnap.Active[2]
		tSnap.NeutralUsed[2], tSnap.NeutralUsed[3] = tSnap.NeutralUsed[3], tSnap.NeutralUsed[2]
		// Update board cell owners for Player 3 and 4
		for r := 0; r < tSnap.Rows; r++ {
			for c := 0; c < tSnap.Cols; c++ {
				if tSnap.Board[r][c].Owner == 3 {
					tSnap.Board[r][c].Owner = 4
				} else if tSnap.Board[r][c].Owner == 4 {
					tSnap.Board[r][c].Owner = 3
				}
			}
		}
		if tSnap.Current == 3 {
			tSnap.Current = 4
		} else if tSnap.Current == 4 {
			tSnap.Current = 3
		}
		if tSnap.Winner == 3 {
			tSnap.Winner = 4
		} else if tSnap.Winner == 4 {
			tSnap.Winner = 3
		}
	}
	tState, err := game.FromSnapshot(tSnap)
	if err != nil {
		panic(err)
	}
	return tState
}

func rotate180State(state game.State) game.State {
	snap := state.Snapshot()
	rSnap := game.Snapshot{
		Rows:        snap.Rows,
		Cols:        snap.Cols,
		Bases:       make([]game.Pos, len(snap.Bases)),
		Active:      make([]bool, len(snap.Active)),
		NeutralUsed: make([]bool, len(snap.NeutralUsed)),
		Current:     snap.Current,
		MovesLeft:   snap.MovesLeft,
		GameOver:    snap.GameOver,
		Winner:      snap.Winner,
		Board:       make([][]game.Cell, snap.Rows),
	}
	for r := 0; r < snap.Rows; r++ {
		rSnap.Board[r] = make([]game.Cell, snap.Cols)
		for c := 0; c < snap.Cols; c++ {
			rSnap.Board[r][c] = snap.Board[snap.Rows-1-r][snap.Cols-1-c]
		}
	}
	for i := range snap.Bases {
		rSnap.Bases[i] = game.Pos{Row: snap.Rows - 1 - snap.Bases[i].Row, Col: snap.Cols - 1 - snap.Bases[i].Col}
		rSnap.Active[i] = snap.Active[i]
		rSnap.NeutralUsed[i] = snap.NeutralUsed[i]
	}
	// Under 180-degree rotation, bases map:
	// P1 (0,0) <-> P2 (rows-1, cols-1)
	// P3 (0,cols-1) <-> P4 (rows-1, 0)
	// So we swap P1 <-> P2, and if 4 players, P3 <-> P4.
	swap := func(p1, p2 int) {
		rSnap.Bases[p1], rSnap.Bases[p2] = rSnap.Bases[p2], rSnap.Bases[p1]
		rSnap.Active[p1], rSnap.Active[p2] = rSnap.Active[p2], rSnap.Active[p1]
		rSnap.NeutralUsed[p1], rSnap.NeutralUsed[p2] = rSnap.NeutralUsed[p2], rSnap.NeutralUsed[p1]
	}
	swap(0, 1)
	if len(rSnap.Bases) >= 4 {
		swap(2, 3)
	}
	// Map owners, current, winner
	mapper := func(p game.Player) game.Player {
		switch p {
		case 1:
			return 2
		case 2:
			return 1
		case 3:
			if len(rSnap.Bases) >= 4 {
				return 4
			}
			return 3
		case 4:
			return 3
		default:
			return 0
		}
	}
	for r := 0; r < rSnap.Rows; r++ {
		for c := 0; c < rSnap.Cols; c++ {
			rSnap.Board[r][c].Owner = mapper(rSnap.Board[r][c].Owner)
		}
	}
	rSnap.Current = mapper(rSnap.Current)
	rSnap.Winner = mapper(rSnap.Winner)

	rState, err := game.FromSnapshot(rSnap)
	if err != nil {
		panic(err)
	}
	return rState
}

func oldEvaluateAllForTest(state game.State) [4]int {
	var utility [4]int
	if state.GameOver() {
		for player := game.Player(1); player <= 4; player++ {
			if state.Winner() == player {
				utility[player-1] = mateScore
			} else {
				utility[player-1] = -mateScore
			}
		}
		return utility
	}

	var metrics [4]playerMetrics
	size := state.Rows() * state.Cols()
	workspace := evalWorkspace{}
	workspace.ensure(size)
	cells := snapshotCellsInto(state, workspace.cells)
	connected := allConnectedInto(state, cells, &workspace)
	var raw [4]int
	active := 0
	for player := game.Player(1); player <= 4; player++ {
		if !state.Active(player) {
			raw[player-1] = -mateScore / 2
			continue
		}
		active++
		index := player - 1
		metrics[index] = analyzeWithConnectivity(state, player, cells, connected, &workspace.scratch,
			workspace.articulation[index], workspace.cutLoss[index])
		m := metrics[player-1]
		area := state.Rows() * state.Cols()
		owned := m.normal + m.fortified + 1 // include the base
		raw[player-1] = normalized(m.connected, area, 10) +
			normalized(m.normal, area, 30) + normalized(m.fortified, area, 6) +
			normalized(m.mobility, area, 1) + normalized(m.captures, area, 1) -
			normalized(m.disconnected, owned, 1) +
			180*m.baseExits + 80*m.baseOpenings + 240*m.baseAnchors -
			650*m.baseThreat*m.threatTempo -
			m.threatTempo*ratio(m.threatenedLoss, max(1, m.connected)) -
			m.threatTempo*ratio(m.threatened, max(1, m.connected))
		if m.baseExits+m.baseOpenings == 0 {
			raw[player-1] -= 5000
		}
		if !state.NeutralUsed(player) {
			raw[player-1] += 20
		}
		if state.CurrentPlayer() == player {
			raw[player-1] += state.MovesLeft() * 12
		}
	}

	for player := game.Player(1); player <= 4; player++ {
		if !state.Active(player) {
			continue
		}
		own := &metrics[player-1]
		for opponent := game.Player(1); opponent <= 4; opponent++ {
			if opponent == player || !state.Active(opponent) {
				continue
			}
			for index, cut := range metrics[opponent-1].articulation {
				if cut && adjacentConnected(state, index, own.connectedCells) {
					loss := int(metrics[opponent-1].cutLoss[index])
					raw[player-1] += 150 + ratio(loss, max(1, metrics[opponent-1].connected))/2
				}
			}
		}
	}

	for player := game.Player(1); player <= 4; player++ {
		if !state.Active(player) {
			utility[player-1] = raw[player-1]
			continue
		}
		opponents := 0
		for other := game.Player(1); other <= 4; other++ {
			if other != player && state.Active(other) {
				opponents += raw[other-1]
			}
		}
		if active > 1 {
			utility[player-1] = raw[player-1] - opponents/(active-1)
		} else {
			utility[player-1] = raw[player-1]
		}
	}
	return utility
}

func TestFeatureExtractorExportedAPI(t *testing.T) {
	sizes := [][2]int{{5, 5}, {6, 9}, {9, 6}, {12, 20}, {20, 12}, {20, 20}}
	playersOpts := []int{2, 3, 4}

	extractor := &FeatureExtractor{}

	for _, sz := range sizes {
		for _, players := range playersOpts {
			seed := int64(sz[0]*1000 + sz[1]*10 + players)
			state := randomReachableState(t, sz[0], sz[1], players, seed)

			features := extractor.Extract(state)
			workspace := evalWorkspace{}
			internalFeatures := extractFeatures(state, &workspace)

			if features != internalFeatures {
				t.Fatalf("FeatureExtractor mismatch for %dx%d-%dp: got %v, want %v", sz[0], sz[1], players, features, internalFeatures)
			}

			for player := game.Player(1); player <= 4; player++ {
				if !state.Active(player) {
					continue
				}
				idx := player - 1
				gotScore := ScoreFeatures(features[idx], IncumbentWeights())
				wantScore := scoreFeatures(internalFeatures[idx], IncumbentWeights())
				if gotScore != wantScore {
					t.Fatalf("ScoreFeatures mismatch: got %d, want %d", gotScore, wantScore)
				}
			}
		}
	}

	state := matureEvaluationState(t, 12, 12)
	_ = extractor.Extract(state) // warmup

	allocs := testing.AllocsPerRun(100, func() {
		_ = extractor.Extract(state)
	})
	if allocs != 0 {
		t.Fatalf("FeatureExtractor.Extract allocated %.0f times in steady state, want 0", allocs)
	}
}

func TestEvaluatorCoordinateReflectionsAndTranspositions(t *testing.T) {
	sizes := [][2]int{{5, 5}, {6, 9}}
	playersOpts := []int{2, 3, 4}

	for _, sz := range sizes {
		for _, players := range playersOpts {
			t.Run(fmt.Sprintf("%dx%d-%dp", sz[0], sz[1], players), func(t *testing.T) {
				seed := int64(sz[0]*100 + sz[1]*10 + players + 123)
				state := randomReachableState(t, sz[0], sz[1], players, seed)

				workspace := evalWorkspace{}
				baseScores := evaluateAllWithWorkspace(state, &workspace)

				// Horizontal reflection
				hState := reflectHorizontalState(state)
				hScores := evaluateAllWithWorkspace(hState, &workspace)
				if hScores != baseScores {
					t.Errorf("Horizontal reflection score mismatch: got %v, want %v", hScores, baseScores)
				}

				// Vertical reflection
				vState := reflectVerticalState(state)
				vScores := evaluateAllWithWorkspace(vState, &workspace)
				if vScores != baseScores {
					t.Errorf("Vertical reflection score mismatch: got %v, want %v", vScores, baseScores)
				}

				// Transposition
				tState := transposeState(state)
				tScores := evaluateAllWithWorkspace(tState, &workspace)
				if tScores != baseScores {
					t.Errorf("Transpose score mismatch: got %v, want %v", tScores, baseScores)
				}
			})
		}
	}
}

func transposeState(state game.State) game.State {
	snap := state.Snapshot()
	tSnap := game.Snapshot{
		Rows:        snap.Cols,
		Cols:        snap.Rows,
		Bases:       make([]game.Pos, len(snap.Bases)),
		Active:      make([]bool, len(snap.Active)),
		NeutralUsed: make([]bool, len(snap.NeutralUsed)),
		Current:     snap.Current,
		MovesLeft:   snap.MovesLeft,
		GameOver:    snap.GameOver,
		Winner:      snap.Winner,
		Board:       make([][]game.Cell, snap.Cols),
	}
	for col := 0; col < snap.Cols; col++ {
		tSnap.Board[col] = make([]game.Cell, snap.Rows)
		for row := 0; row < snap.Rows; row++ {
			tSnap.Board[col][row] = snap.Board[row][col]
		}
	}
	for i := range snap.Bases {
		tSnap.Bases[i] = game.Pos{Row: snap.Bases[i].Col, Col: snap.Bases[i].Row}
		tSnap.Active[i] = snap.Active[i]
		tSnap.NeutralUsed[i] = snap.NeutralUsed[i]
	}
	tState, err := game.FromSnapshot(tSnap)
	if err != nil {
		panic(err)
	}
	return tState
}

func reflectHorizontalState(state game.State) game.State {
	snap := state.Snapshot()
	rSnap := game.Snapshot{
		Rows:        snap.Rows,
		Cols:        snap.Cols,
		Bases:       make([]game.Pos, len(snap.Bases)),
		Active:      make([]bool, len(snap.Active)),
		NeutralUsed: make([]bool, len(snap.NeutralUsed)),
		Current:     snap.Current,
		MovesLeft:   snap.MovesLeft,
		GameOver:    snap.GameOver,
		Winner:      snap.Winner,
		Board:       make([][]game.Cell, snap.Rows),
	}
	for r := 0; r < snap.Rows; r++ {
		rSnap.Board[r] = make([]game.Cell, snap.Cols)
		for c := 0; c < snap.Cols; c++ {
			rSnap.Board[r][c] = snap.Board[r][snap.Cols-1-c]
		}
	}
	for i := range snap.Bases {
		rSnap.Bases[i] = game.Pos{Row: snap.Bases[i].Row, Col: snap.Cols - 1 - snap.Bases[i].Col}
		rSnap.Active[i] = snap.Active[i]
		rSnap.NeutralUsed[i] = snap.NeutralUsed[i]
	}
	rState, err := game.FromSnapshot(rSnap)
	if err != nil {
		panic(err)
	}
	return rState
}

func reflectVerticalState(state game.State) game.State {
	snap := state.Snapshot()
	rSnap := game.Snapshot{
		Rows:        snap.Rows,
		Cols:        snap.Cols,
		Bases:       make([]game.Pos, len(snap.Bases)),
		Active:      make([]bool, len(snap.Active)),
		NeutralUsed: make([]bool, len(snap.NeutralUsed)),
		Current:     snap.Current,
		MovesLeft:   snap.MovesLeft,
		GameOver:    snap.GameOver,
		Winner:      snap.Winner,
		Board:       make([][]game.Cell, snap.Rows),
	}
	for r := 0; r < snap.Rows; r++ {
		rSnap.Board[r] = make([]game.Cell, snap.Cols)
		for c := 0; c < snap.Cols; c++ {
			rSnap.Board[r][c] = snap.Board[snap.Rows-1-r][c]
		}
	}
	for i := range snap.Bases {
		rSnap.Bases[i] = game.Pos{Row: snap.Rows - 1 - snap.Bases[i].Row, Col: snap.Bases[i].Col}
		rSnap.Active[i] = snap.Active[i]
		rSnap.NeutralUsed[i] = snap.NeutralUsed[i]
	}
	rState, err := game.FromSnapshot(rSnap)
	if err != nil {
		panic(err)
	}
	return rState
}
