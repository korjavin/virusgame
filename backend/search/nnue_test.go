package search

import (
	"testing"

	"virusgame/game"
	"virusgame/nnuefeat"
)

// TestNNUEInputLayout guards the one invariant that makes inference valid: the
// Input() vector search feeds the net must be the exact seat-order,
// zero-padded flatten the labeler recorded (arena/nnuegen writes each seat's
// Features()). If these ever drift, the net evaluates garbage.
func TestNNUEInputLayout(t *testing.T) {
	state := mustState(t, 6, 6, 2)
	got := nnuefeat.Input(state)
	if len(got) != nnuefeat.InputDim {
		t.Fatalf("Input width = %d, want %d", len(got), nnuefeat.InputDim)
	}
	feats := nnuefeat.NNUEFeatures(state)
	want := make([]float64, nnuefeat.InputDim)
	for seat := 0; seat < nnuefeat.Seats; seat++ {
		if state.Active(game.Player(seat + 1)) {
			copy(want[seat*nnuefeat.FeatureCount:], feats[seat].Features())
		}
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("Input[%d] = %v, want %v", i, got[i], want[i])
		}
	}
}

// TestNNUEEvaluateRouting proves the flag gates the leaf eval: on, it returns
// the net score (with the 2-player perspective flip); off, it is the frozen
// eval untouched.
func TestNNUEEvaluateRouting(t *testing.T) {
	state := mustState(t, 6, 6, 2)
	root := state.CurrentPlayer()

	defer func(prev bool) { nnueEnabled = prev }(nnueEnabled)

	nnueEnabled = false
	classic := evaluateWithWorkspace(state, root, &evalWorkspace{})
	if classic != evaluateAllWithWorkspace(state, &evalWorkspace{})[root-1] {
		t.Fatalf("flag-off path is not the frozen eval")
	}

	nnueEnabled = true
	if got, want := evaluateWithWorkspace(state, root, &evalWorkspace{}), nnueEvaluate(state, root); got != want {
		t.Fatalf("flag-on eval = %d, want net %d", got, want)
	}
	// Perspective: the net predicts the mover's score; the other seat negates it.
	other := game.Player(1)
	if root == 1 {
		other = 2
	}
	if got := nnueEvaluate(state, other); got != -nnueEvaluate(state, root) {
		t.Fatalf("opponent perspective = %d, want %d", got, -nnueEvaluate(state, root))
	}
}
