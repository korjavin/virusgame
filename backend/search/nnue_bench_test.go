package search

import (
	"testing"

	"virusgame/game"
	"virusgame/nnuefeat"
	"virusgame/search/nnueweights"
)

// benchMidgame builds a representative mid-game 8x8 2-player leaf by playing a
// few engine moves (classic eval), the kind of position the leaf eval sees
// during search. Setup runs before ResetTimer, so it is excluded from timings.
func benchMidgame(tb testing.TB) game.State {
	prev := nnueEnabled
	nnueEnabled = false // reach the position with the frozen eval
	defer func() { nnueEnabled = prev }()
	state, err := game.New(8, 8, 2)
	if err != nil {
		tb.Fatal(err)
	}
	for i := 0; i < 8 && !state.GameOver(); i++ {
		res, ok := ChooseNodeBudget(state, 400)
		if !ok {
			break
		}
		next, err := state.Apply(res.Action)
		if err != nil {
			tb.Fatal(err)
		}
		state = next
	}
	return state
}

// BenchmarkClassicEval measures the frozen hand-tuned leaf eval (~46us historic
// baseline). BenchmarkNNUEEval measures the full NNUE candidate path: feature
// extraction (nnuefeat.Input, recomputed per call) + the int8 forward pass. The
// cost gate for vs-ai2.56 is NNUE within ~2x of classic.
func BenchmarkClassicEval(b *testing.B) {
	state := benchMidgame(b)
	ws := evalWorkspace{}
	root := state.CurrentPlayer()
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = evaluateAllWithWorkspace(state, &ws)[root-1]
	}
}

func BenchmarkNNUEEval(b *testing.B) {
	state := benchMidgame(b)
	root := state.CurrentPlayer()
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = nnueEvaluate(state, root)
	}
}

// BenchmarkNNUEForwardOnly isolates the int8 forward pass from feature
// extraction, so the cost breakdown (extract vs matmul) is visible.
func BenchmarkNNUEForwardOnly(b *testing.B) {
	x := nnuefeat.Input(benchMidgame(b))
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = nnueweights.Predict(x)
	}
}
