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
			state = wantNext
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
	state.set(Pos{2, 2}, Cell{Owner: 1, Kind: Normal})
	state.set(Pos{3, 3}, Cell{Owner: 2, Kind: Normal})

	position := NewPosition(state)
	tactical := position.tacticalNeutrals()
	if len(tactical) < 2 {
		t.Fatalf("expected multiple tactical cells, got %v", tactical)
	}
	got := make(map[[2]Pos]bool)
	for _, action := range position.SearchActions() {
		if action.Kind == PlaceNeutrals {
			got[action.Neutrals] = true
		}
		if _, err := state.Apply(action); err != nil {
			t.Fatalf("selective action is illegal: %+v: %v", action, err)
		}
	}
	owned := position.ownedNormals()
	for i := range owned {
		for j := i + 1; j < len(owned); j++ {
			if containsPos(tactical, owned[i]) || containsPos(tactical, owned[j]) {
				pair := [2]Pos{owned[i], owned[j]}
				if !got[pair] {
					t.Fatalf("missing tactical neutral pair %v", pair)
				}
			}
		}
	}
}

func TestSearchActionsReduceMatureBoardNeutralBranching(t *testing.T) {
	state := maturePosition50()
	position := NewPosition(state)
	actions := position.SearchActions()
	normalCount := len(position.ownedNormals())
	exhaustivePairs := normalCount * (normalCount - 1) / 2
	neutralCount := 0
	for _, action := range actions {
		if action.Kind == PlaceNeutrals {
			neutralCount++
		}
		if _, err := state.Apply(action); err != nil {
			t.Fatalf("search action is illegal: %+v: %v", action, err)
		}
	}
	if exhaustivePairs < 1_000_000 {
		t.Fatalf("fixture is not mature: only %d exhaustive pairs", exhaustivePairs)
	}
	if neutralCount >= 10_000 {
		t.Fatalf("neutral branching not selective: got %d of %d pairs", neutralCount, exhaustivePairs)
	}
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
		_ = position.SearchActions()
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
