package search

import (
	"context"
	"testing"

	"virusgame/game"
)

// denseStateNP builds a reproducible dense 12x12 midgame for N players via the
// same search-independent policy as denseState12x12 (always first legal action).
func denseStateNP(tb testing.TB, players int) game.State {
	tb.Helper()
	state, err := game.New(12, 12, players)
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

// TestMeasureNodesToDepth is a throwaway measurement (deleted before PR): it
// records s.nodes at fixed depths for 1v1, 3-player, and 4-player dense states
// on the current post-PVS/pruning code, for the PR before/after table.
func TestMeasureNodesToDepth(t *testing.T) {
	fixtures := []struct {
		name  string
		state game.State
	}{
		{"1v1", denseState12x12(t)},
		{"3p", denseStateNP(t, 3)},
		{"4p", denseStateNP(t, 4)},
	}
	for _, f := range fixtures {
		for _, depth := range []int{3, 4, 5} {
			s := newSearcher(context.Background(), f.state)
			if _, ok := s.atDepth(f.state, depth); !ok {
				t.Fatalf("%s depth %d: search canceled", f.name, depth)
			}
			t.Logf("%s depth=%d nodes=%d evaluations=%d", f.name, depth, s.nodes, s.evaluations)
		}
	}
}
