package arena

import (
	"testing"

	"virusgame/game"
)

// craftedCutPosition builds a 6x6 two-player position where victim (player 2)
// holds a base(5,5)—(4,4)—(3,5) chain: (4,4) is the only articulation point
// and player 1's diagonal chain ending at (3,3) can capture it.
func craftedCutPosition(t *testing.T) game.State {
	t.Helper()
	const size = 6
	board := make([][]game.Cell, size)
	for row := range board {
		board[row] = make([]game.Cell, size)
	}
	set := func(row, col int, owner game.Player, kind game.CellKind) {
		board[row][col] = game.Cell{Owner: owner, Kind: kind}
	}
	set(0, 0, 1, game.Base)
	set(1, 1, 1, game.Normal)
	set(2, 2, 1, game.Normal)
	set(3, 3, 1, game.Normal)
	set(5, 5, 2, game.Base)
	set(4, 4, 2, game.Normal)
	set(3, 5, 2, game.Normal)
	state, err := game.FromSnapshot(game.Snapshot{
		Rows: size, Cols: size, Board: board,
		Bases:       []game.Pos{{Row: 0, Col: 0}, {Row: size - 1, Col: size - 1}},
		Active:      []bool{true, true},
		NeutralUsed: []bool{true, true},
		Current:     1, MovesLeft: 3,
	})
	if err != nil {
		t.Fatal(err)
	}
	return state
}

func TestSparringAgentsLegalAndDeterministic(t *testing.T) {
	agents := map[string]Agent{"MobilityBaseAttacker": MobilityBaseAttacker, "CutSeeker": CutSeeker}
	for seed := uint64(1); seed <= 5; seed++ {
		snapshot := randomLegalOpening(t, seed)
		state, err := game.FromSnapshot(snapshot)
		if err != nil {
			t.Fatal(err)
		}
		for name, agent := range agents {
			action, ok := agent(state)
			if !ok {
				t.Fatalf("%s seed %d: returned no action", name, seed)
			}
			if _, err := state.Apply(action); err != nil {
				t.Fatalf("%s seed %d: illegal action %+v: %v", name, seed, action, err)
			}
			if repeat, _ := agent(state); repeat != action {
				t.Fatalf("%s seed %d: nondeterministic: %+v then %+v", name, seed, action, repeat)
			}
		}
	}

	state := craftedCutPosition(t)
	cut := game.Pos{Row: 4, Col: 4}
	cuts := opponentArticulations(state, 2)
	if len(cuts) != 1 || !cuts[cut] {
		t.Fatalf("expected exactly {%v} as player 2 articulation points, got %v", cut, cuts)
	}
	action, ok := CutSeeker(state)
	if !ok || action.Kind != game.Move {
		t.Fatalf("CutSeeker returned no move: %+v ok=%v", action, ok)
	}
	if action.Target != cut && !adjacentToCut(action.Target, cuts) {
		t.Fatalf("CutSeeker should target or adjoin the cut %v, chose %+v", cut, action)
	}
}
