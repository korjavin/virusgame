package search

import (
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
		// The vs-ai2.34 space-race term is global by design (fraction of the
		// board first-reached varies with board shape for the same local
		// structure), so its exact contribution is subtracted: this test pins
		// only the size-normalized local base-safety terms.
		area := dims[0] * dims[1]
		workspace := &evalWorkspace{}
		workspace.ensure(area)
		cells := snapshotCellsInto(state, workspace.cells)
		space := spaceRace(state, cells, allConnectedInto(state, cells, workspace), workspace)
		global := normalized(space[0], area, spaceRaceWeight) - normalized(space[1], area, spaceRaceWeight)
		scores = append(scores, evaluate(state, 1)-global)
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
	// Self-consistency oracle for the production evaluator. Re-pinned at
	// vs-ai2.52: 2-active-player fixtures score with the SPSA-tuned 1v1 set
	// (spaceRaceWeight = 3), 3p/4p fixtures with the hand-tuned multiplayer set
	// (mode split); does not track origin/main bf74a44, whose evaluator lacked
	// the space term.
	const oracle = "1316cf0106fb38f3707e8aba8a46808e66d3872b24a12e9739147ba846c86852"
	if got := fmt.Sprintf("%x", hash.Sum(nil)); got != oracle {
		t.Fatalf("workspace evaluator digest = %s, want vs-ai2.52 oracle %s", got, oracle)
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
		// vs-ai2.52 re-pin: production eval now runs the owner-targeted SPSA
		// weights (defaultEvalParams), so these static scores shift from the
		// prior hand-tuned goldens.
		{"initial", initial, [4]int{69, -69, -500000000, -500000000}},
		{"contact-threatened-cut", contact, [4]int{69, -69, -500000000, -500000000}},
		{"neutral", neutral, [4]int{-1016, 1016, -500000000, -500000000}},
		{"terminal", terminal, [4]int{mateScore, -mateScore, -mateScore, -mateScore}},
		{"eliminated", eliminated, [4]int{-500000000, -226, 226, -500000000}},
	} {
		workspace := evalWorkspace{}
		if got := evaluateAllWithWorkspace(fixture.state, &workspace); got != fixture.want {
			t.Fatalf("%s scores = %v, want vs-ai2.52 golden %v", fixture.name, got, fixture.want)
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

func TestSpaceRacePartition(t *testing.T) {
	// 5x5, 2 players: bases at (0,0) and (4,4), everything else empty. The
	// shared BFS is a Chebyshev-distance Voronoi split: each player first-
	// reaches the 9 cells strictly nearer its base; the 5 anti-diagonal cells
	// are equidistant and contested.
	state := mustState(t, 5, 5, 2)
	size := state.Rows() * state.Cols()
	workspace := &evalWorkspace{}
	workspace.ensure(size)
	cells := snapshotCellsInto(state, workspace.cells)
	connected := allConnectedInto(state, cells, workspace)

	counts := spaceRace(state, cells, connected, workspace)
	if counts != [4]int{9, 9, 0, 0} {
		t.Fatalf("open-board counts = %v, want [9 9 0 0]", counts)
	}
	contested := map[int]bool{
		indexFor(state, game.Pos{Row: 0, Col: 4}): true,
		indexFor(state, game.Pos{Row: 1, Col: 3}): true,
		indexFor(state, game.Pos{Row: 2, Col: 2}): true,
		indexFor(state, game.Pos{Row: 3, Col: 1}): true,
		indexFor(state, game.Pos{Row: 4, Col: 0}): true,
	}
	for index := 0; index < size; index++ {
		if got := workspace.spaceOwner[index] == -2; got != contested[index] {
			t.Fatalf("cell %d contested = %v, want %v", index, got, contested[index])
		}
	}

	// Wall the whole column 3: player 2 is boxed into column 4 (4 empties),
	// player 1 takes columns 0-2 (14 empties), nothing is contested. If walls
	// did not block the BFS the counts would stay 9/9.
	for row := 0; row < state.Rows(); row++ {
		cells[indexFor(state, game.Pos{Row: row, Col: 3})] = game.Cell{Kind: game.Fortified}
	}
	counts = spaceRace(state, cells, connected, workspace)
	if counts != [4]int{14, 4, 0, 0} {
		t.Fatalf("walled-board counts = %v, want [14 4 0 0]", counts)
	}
	for index := 0; index < size; index++ {
		if workspace.spaceOwner[index] == -2 {
			t.Fatalf("cell %d contested on walled board, regions are disjoint", index)
		}
	}
}

func absInt(value int) int {
	if value < 0 {
		return -value
	}
	return value
}
