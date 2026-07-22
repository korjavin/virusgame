package arena

// Experiment (nnue-trainer ntd.8 follow-up): does reducing GoBot's SEARCH budget
// weaken it, holding the eval constant? If a low-budget GoBot loses badly to a
// high-budget GoBot using the SAME static eval, then GoBot's strength lives in
// its search, not its static eval -- which explains why cloning only the eval
// (the distilled Java NNUE, val MSE 0.015 yet 0/7 vs GoBot) fails to reach parity.
//
// Run: go test ./backend/arena -run TestReducedSearchStrength -v -timeout 900s

import (
	"testing"

	"virusgame/game"
	"virusgame/search"
)

func nodeAgent(nodes uint64) Agent {
	return func(s game.State) (game.Action, bool) {
		r, ok := search.ChooseNodeBudget(s, nodes)
		return r.Action, ok
	}
}

func TestReducedSearchStrength(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping slow strength test in short mode")
	}
	const ref uint64 = 200_000 // strong reference ~= what production reaches in ~1s
	strong := func(uint64) Agent { return nodeAgent(ref) }
	boards := []Board{{Rows: 12, Cols: 12}}

	t.Logf("contender (varying node budget) vs reference (%d nodes), same eval, balanced colors:", ref)
	for _, nodes := range []uint64{1_000, 5_000, 20_000, 80_000, 200_000} {
		rep, err := Balanced(boards, 1, nodeAgent(nodes), strong)
		if err != nil {
			t.Fatal(err)
		}
		t.Logf("  budget=%7d : contender W-L-D = %d-%d-%d (win %.0f%%)",
			nodes, rep.Wins, rep.Losses, rep.Draws, rep.WinRate())
	}
}
