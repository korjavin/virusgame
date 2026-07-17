package search

import (
	"context"
	"fmt"
	"testing"

	"virusgame/game"
)

// throwaway measurement harness for vs-ai2.41 (delete before PR)
func denseN(tb testing.TB, rows, cols, plies int) game.State {
	tb.Helper()
	state, err := game.New(rows, cols, 2)
	if err != nil {
		tb.Fatal(err)
	}
	for ply := 0; ply < plies; ply++ {
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

func TestMeasureNodesToDepth(t *testing.T) {
	corpus := []struct {
		name  string
		state game.State
	}{
		{"dense12x12-p60", denseN(t, 12, 12, 60)},
		{"dense10x10-p40", denseN(t, 10, 10, 40)},
		{"dense8x8-p24", denseN(t, 8, 8, 24)},
	}
	for _, c := range corpus {
		for d := 3; d <= 6; d++ {
			r, ok := ChooseDepth(context.Background(), c.state, d)
			if !ok {
				t.Fatalf("%s depth %d incomplete", c.name, d)
			}
			fmt.Printf("MEASURE fixeddepth %-16s d=%d nodes=%-8d evals=%-8d score=%d\n", c.name, d, r.Nodes, r.Evaluations, r.Score)
		}
		// iterative deepening total nodes at production node budgets
		for _, budget := range []uint64{1000, 10000, 100000} {
			r, ok := ChooseNodeBudget(c.state, budget)
			if !ok {
				t.Fatalf("%s budget %d failed", c.name, budget)
			}
			fmt.Printf("MEASURE nodebudget %-16s budget=%-7d completedDepth=%d nodes=%d\n", c.name, budget, r.Depth, r.Nodes)
		}
	}
}
