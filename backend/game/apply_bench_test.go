package game

import "testing"

// denseApplyState builds a densely populated size×size two-player board: the
// top block belongs to player 1, the bottom block to player 2, with a thin
// empty frontier between them. Both players hold large connected territories,
// so Apply's connectivity floodfills (legality + per-player stuck elimination)
// traverse many cells — the value-receiver copy hot path this bead targets.
func denseApplyState(size int) State {
	state, _ := New(size, size, 2)
	half := size / 2
	for row := 0; row < half-1; row++ {
		for col := 0; col < size; col++ {
			pos := Pos{row, col}
			if pos != state.bases[0] {
				state.set(pos, Cell{Owner: 1, Kind: Normal})
			}
		}
	}
	for row := half + 1; row < size; row++ {
		for col := 0; col < size; col++ {
			pos := Pos{row, col}
			if pos != state.bases[1] {
				state.set(pos, Cell{Owner: 2, Kind: Normal})
			}
		}
	}
	return state
}

// BenchmarkApplyDense measures State.Apply on a dense 12×12 position, driving
// the full transition: legality connectivity, mutation, per-player stuck
// elimination floodfills, terminal check, and turn advance. Apply copies the
// board, so the starting state is unchanged and every iteration is identical.
func BenchmarkApplyDense(b *testing.B) {
	state := denseApplyState(12)
	actions := state.LegalActions()
	var action Action
	found := false
	for _, candidate := range actions {
		if candidate.Kind == Move {
			action, found = candidate, true
			break
		}
	}
	if !found {
		b.Fatal("dense benchmark fixture has no legal move")
	}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := state.Apply(action); err != nil {
			b.Fatalf("apply: %v", err)
		}
	}
}
