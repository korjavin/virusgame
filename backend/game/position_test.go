package game

import (
	"math/rand"
	"reflect"
	"testing"
)

func TestPositionExactActionsAndApplyMatchAuthoritativeOracle(t *testing.T) {
	rng := rand.New(rand.NewSource(20260715))
	boards := [][2]int{{5, 5}, {5, 50}, {17, 31}, {50, 5}, {50, 50}}
	for sample := 0; sample < 32; sample++ {
		boards = append(boards, [2]int{5 + rng.Intn(46), 5 + rng.Intn(46)})
	}
	for _, board := range boards {
		state, err := New(board[0], board[1], 2)
		if err != nil {
			t.Fatal(err)
		}
		for ply := 0; ply < 10 && !state.GameOver(); ply++ {
			for player := Player(1); player <= 2; player++ {
				wantTargets := slowMoveTargets(state, player)
				if gotTargets := state.moveTargets(player); !reflect.DeepEqual(gotTargets, wantTargets) {
					t.Fatalf("%dx%d ply %d player %d frontier divergence", board[0], board[1], ply, player)
				}
			}
			want := slowLegalActions(state)
			position := NewPosition(state)
			if got := position.LegalActions(); !reflect.DeepEqual(got, want) {
				t.Fatalf("%dx%d ply %d action divergence: got %d, want %d", board[0], board[1], ply, len(got), len(want))
			}
			if len(want) == 0 {
				break
			}
			action := want[rng.Intn(len(want))]
			wantNext, wantErr := state.Apply(action)
			gotNext, gotErr := position.Apply(action)
			if gotErr != wantErr || !reflect.DeepEqual(gotNext.State().Snapshot(), wantNext.Snapshot()) {
				t.Fatalf("%dx%d ply %d Apply divergence for %+v: got %v, want %v", board[0], board[1], ply, action, gotErr, wantErr)
			}
			fastNext := position.ApplySearch(action)
			if !reflect.DeepEqual(fastNext.State().Snapshot(), wantNext.Snapshot()) {
				t.Fatalf("%dx%d ply %d ApplySearch divergence for %+v", board[0], board[1], ply, action)
			}
			state = wantNext
		}
	}
}

func TestGeneratedSearchSuccessorsMatchAuthoritativeOracle(t *testing.T) {
	rng := rand.New(rand.NewSource(220260715))
	for _, board := range [][2]int{{5, 5}, {5, 50}, {23, 31}, {50, 5}, {50, 50}} {
		state, err := New(board[0], board[1], 2)
		if err != nil {
			t.Fatal(err)
		}
		for ply := 0; ply < 48 && !state.GameOver(); ply++ {
			position := NewPosition(state)
			checked := 0
			position.ForEachSearchAction(func(action Action) bool {
				want, applyErr := state.Apply(action)
				if applyErr != nil {
					t.Fatalf("generated illegal action %+v: %v", action, applyErr)
				}
				got := position.ApplySearch(action).State()
				if !reflect.DeepEqual(got.Snapshot(), want.Snapshot()) {
					t.Fatalf("%dx%d ply %d generated successor divergence for %+v", board[0], board[1], ply, action)
				}
				checked++
				return checked < 32
			})
			moves := slowMoveTargets(state, state.CurrentPlayer())
			if len(moves) == 0 {
				break
			}
			state, err = state.Apply(Action{Kind: Move, Target: moves[rng.Intn(len(moves))]})
			if err != nil {
				t.Fatal(err)
			}
		}
	}
}

func TestPositionMatureSeeded1v1FrontierEquivalence(t *testing.T) {
	rng := rand.New(rand.NewSource(550020260715))
	boards := [][2]int{{5, 50}, {50, 5}, {19, 37}, {50, 50}}
	for _, board := range boards {
		for game := 0; game < 4; game++ {
			state, err := New(board[0], board[1], 2)
			if err != nil {
				t.Fatal(err)
			}
			for ply := 0; ply < 120 && !state.GameOver(); ply++ {
				want := slowMoveTargets(state, state.CurrentPlayer())
				position := NewPosition(state)
				if !reflect.DeepEqual(position.moves, want) {
					t.Fatalf("%dx%d game %d ply %d mature frontier divergence", board[0], board[1], game, ply)
				}
				if len(want) == 0 {
					break
				}
				action := Action{Kind: Move, Target: want[rng.Intn(len(want))]}
				oracle, oracleErr := state.Apply(action)
				got, gotErr := position.Apply(action)
				if oracleErr != gotErr || !reflect.DeepEqual(got.State().Snapshot(), oracle.Snapshot()) {
					t.Fatalf("%dx%d game %d ply %d successor divergence", board[0], board[1], game, ply)
				}
				state = oracle
			}
		}
	}
}

func TestSearchActionsRetainTacticalNeutralPairs(t *testing.T) {
	state, err := New(5, 5, 2)
	if err != nil {
		t.Fatal(err)
	}
	// Player 1 has a base-connected cut chain. Its endpoint is immediately
	// threatened by player 2's base-connected chain.
	state.set(Pos{1, 1}, Cell{Owner: 1, Kind: Normal})
	state.set(Pos{0, 1}, Cell{Owner: 1, Kind: Normal})
	state.set(Pos{1, 0}, Cell{Owner: 1, Kind: Normal})
	state.set(Pos{1, 2}, Cell{Owner: 1, Kind: Normal})
	state.set(Pos{2, 1}, Cell{Owner: 1, Kind: Normal})
	state.set(Pos{2, 2}, Cell{Owner: 1, Kind: Normal})
	state.set(Pos{3, 3}, Cell{Owner: 2, Kind: Normal})

	position := NewPosition(state)
	got := make(map[[2]Pos]bool)
	position.ForEachSearchAction(func(action Action) bool {
		if action.Kind == PlaceNeutrals {
			got[action.Neutrals] = true
		}
		if _, err := state.Apply(action); err != nil {
			t.Fatalf("selective action is illegal: %+v: %v", action, err)
		}
		return true
	})
	// The base-adjacent and immediately threatened cells each receive at
	// least one robust filler; exhaustive filler permutations are intentionally
	// omitted by the bounded policy.
	for _, tactical := range []Pos{{1, 1}, {2, 2}} {
		found := false
		for pair := range got {
			if pair[0] == tactical || pair[1] == tactical {
				found = true
			}
		}
		if !found {
			t.Fatalf("missing bounded defensive pair for %v", tactical)
		}
	}
}

func TestSearchActionsReduceMatureBoardNeutralBranching(t *testing.T) {
	state := maturePosition50()
	position := NewPosition(state)
	normalCount := len(position.ownedNormals())
	exhaustivePairs := normalCount * (normalCount - 1) / 2
	neutralCount := 0
	position.ForEachSearchAction(func(action Action) bool {
		if action.Kind == PlaceNeutrals {
			neutralCount++
		}
		if _, err := state.Apply(action); err != nil {
			t.Fatalf("search action is illegal: %+v: %v", action, err)
		}
		return true
	})
	if exhaustivePairs < 1_000_000 {
		t.Fatalf("fixture is not mature: only %d exhaustive pairs", exhaustivePairs)
	}
	if neutralCount > 48 {
		t.Fatalf("neutral branching exceeds bound: got %d of %d pairs", neutralCount, exhaustivePairs)
	}
}

func TestMatureSearchEnumerationAllocationBudget(t *testing.T) {
	position := NewPosition(maturePosition50())
	allocations := testing.AllocsPerRun(10, func() {
		position.ForEachSearchAction(func(Action) bool { return true })
	})
	if allocations > 4 {
		t.Fatalf("search enumeration allocates %.0f objects, budget is 4", allocations)
	}
}

func TestMatureSuccessorAllocationBudget(t *testing.T) {
	position := NewPosition(maturePosition50())
	allocations := testing.AllocsPerRun(5, func() {
		expanded := 0
		for expanded < 128 {
			position.ForEachSearchAction(func(action Action) bool {
				_ = position.ApplySearch(action)
				expanded++
				return expanded < 128
			})
		}
	})
	if allocations > 512 {
		t.Fatalf("128 successors allocate %.0f objects, budget is 512", allocations)
	}
}

func TestSearchActionsRetainIndependentTwoCellSeparator(t *testing.T) {
	state, err := New(9, 9, 2)
	if err != nil {
		t.Fatal(err)
	}
	// Two spatially separate routes connect the left territory to (4,7).
	// One route passes through u and the other through v; only removing both
	// disconnects the far territory. u is immediately threatened, making this
	// general non-adjacent separator tactically relevant.
	want := [2]Pos{{2, 4}, {5, 4}}
	owned := []Pos{{0, 1}, {1, 1}, {2, 2}, {2, 3}, want[0], {2, 5}, {3, 6},
		{3, 2}, {4, 3}, want[1], {5, 5}, {4, 6}, {4, 7}}
	for _, pos := range owned {
		state.set(pos, Cell{Owner: 1, Kind: Normal})
	}
	state.set(state.bases[1], Cell{})
	state.bases[1] = Pos{1, 4}
	state.set(state.bases[1], Cell{Owner: 2, Kind: Base})
	if fixtureDisconnected(state, want[:1]) || fixtureDisconnected(state, want[1:]) {
		t.Fatal("fixture endpoints must not be single articulation points")
	}
	if !fixtureDisconnected(state, want[:]) {
		t.Fatal("fixture pair does not independently disconnect territory")
	}
	position := NewPosition(state)
	found := false
	position.ForEachSearchAction(func(action Action) bool {
		if action.Kind == PlaceNeutrals && action.Neutrals == want {
			found = true
		}
		return true
	})
	if !found {
		t.Fatalf("two-cell separator %v was pruned", want)
	}
}

// fixtureDisconnected is a deliberately simple test oracle. It operates on
// board coordinates and does not call the production articulation/pair code.
func fixtureDisconnected(state State, removed []Pos) bool {
	blocked := make(map[Pos]bool, len(removed))
	for _, pos := range removed {
		blocked[pos] = true
	}
	base := state.bases[0]
	seen := map[Pos]bool{base: true}
	queue := []Pos{base}
	for len(queue) > 0 {
		current := queue[0]
		queue = queue[1:]
		for row := current.Row - 1; row <= current.Row+1; row++ {
			for col := current.Col - 1; col <= current.Col+1; col++ {
				next := Pos{row, col}
				cell, ok := state.At(next)
				if ok && !blocked[next] && !seen[next] && cell.Owner == 1 {
					seen[next] = true
					queue = append(queue, next)
				}
			}
		}
	}
	for index, cell := range state.cells {
		pos := Pos{index / state.cols, index % state.cols}
		if cell.Owner == 1 && !blocked[pos] && !seen[pos] {
			return true
		}
	}
	return false
}

func containsPos(cells []Pos, want Pos) bool {
	for _, cell := range cells {
		if cell == want {
			return true
		}
	}
	return false
}

func BenchmarkMature50x50SearchActions(b *testing.B) {
	position := NewPosition(maturePosition50())
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		position.ForEachSearchAction(func(Action) bool { return true })
	}
}

func BenchmarkMature50x50PositionAndSearch(b *testing.B) {
	state := maturePosition50()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		position := NewPosition(state)
		position.ForEachSearchAction(func(Action) bool { return true })
	}
}

func BenchmarkMature50x50Successors(b *testing.B) {
	position := NewPosition(maturePosition50())
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		expanded := 0
		for expanded < 128 {
			position.ForEachSearchAction(func(action Action) bool {
				_ = position.ApplySearch(action)
				expanded++
				return expanded < 128
			})
		}
		b.ReportMetric(float64(expanded), "successors/op")
	}
}

func slowLegalActions(s State) []Action {
	if s.over || !s.Active(s.current) {
		return nil
	}
	targets := slowMoveTargets(s, s.current)
	actions := make([]Action, 0, len(targets))
	for _, target := range targets {
		actions = append(actions, Action{Kind: Move, Target: target})
	}
	if s.movesLeft == actionsPerTurn && !s.neutralUsed[s.current-1] {
		cells := make([]Pos, 0)
		for index, cell := range s.cells {
			if cell.Owner == s.current && cell.Kind == Normal {
				cells = append(cells, Pos{Row: index / s.cols, Col: index % s.cols})
			}
		}
		for i := range cells {
			for j := i + 1; j < len(cells); j++ {
				actions = append(actions, Action{Kind: PlaceNeutrals, Neutrals: [2]Pos{cells[i], cells[j]}})
			}
		}
	}
	return actions
}

func slowMoveTargets(s State, player Player) []Pos {
	targets := make([]Pos, 0)
	for row := 0; row < s.rows; row++ {
		for col := 0; col < s.cols; col++ {
			pos := Pos{row, col}
			if s.legalMove(player, pos) {
				targets = append(targets, pos)
			}
		}
	}
	return targets
}

func maturePosition50() State {
	state, _ := New(50, 50, 2)
	for row := 0; row < 35; row++ {
		for col := 0; col < 50; col++ {
			pos := Pos{row, col}
			if pos != state.bases[0] {
				state.set(pos, Cell{Owner: 1, Kind: Normal})
			}
		}
	}
	return state
}
