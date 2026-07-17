package search

import (
	"context"
	"testing"

	"virusgame/game"
)

// nodeLimitedSearch runs a deterministic, node-budgeted iterative-deepening
// search reusing the supplied table (cap == 0 → uncapped plain writes). It is a
// test-only mirror of chooseNodeBudget with an injectable table, so a ponder can
// warm the table the next search reuses. No production surface.
func nodeLimitedSearch(table map[uint64]tableEntry, cap int, state game.State, limit uint64) Result {
	fallback, ok := preservingFallback(state)
	if !ok {
		return Result{}
	}
	best := Result{Action: fallback}
	s := newSearcher(context.Background(), state)
	s.table = table
	s.tableCap = cap
	s.nodeLimit = limit
	for depth := 1; depth <= maxDepth && s.nodes < limit; depth++ {
		result, complete := s.atDepth(state, depth)
		if !complete {
			break
		}
		best = result
		best.Depth = depth
	}
	best.Nodes, best.Evaluations = s.nodes, s.evaluations
	return best
}

// advancePreserving applies up to n preserving plies to reach a midgame.
func advancePreserving(state game.State, n int) game.State {
	for i := 0; i < n && !state.GameOver(); i++ {
		action, ok := preservingFallback(state)
		if !ok {
			break
		}
		next, err := state.Apply(action)
		if err != nil {
			break
		}
		state = next
	}
	return state
}

// advanceUntilPlayer applies preserving plies until it is player's move (or the
// game ends / no legal progress is possible).
func advanceUntilPlayer(state game.State, player game.Player) game.State {
	for i := 0; i < 200 && !state.GameOver() && state.CurrentPlayer() != player; i++ {
		action, ok := preservingFallback(state)
		if !ok {
			break
		}
		next, err := state.Apply(action)
		if err != nil {
			break
		}
		state = next
	}
	return state
}

// TestPonderWarmStartDoesNotRegress records the HONEST warm-vs-cold outcome for
// permanent-brain pondering and guards the only property that actually holds: a
// ponder-warmed search never searches SHALLOWER than a cold one on the same node
// budget.
//
// Measured finding (vs-ai2.53, do NOT re-label as a "warm benefit proof"):
// permanent-brain pondering yields ~zero warm benefit. A 600k-node ponder of the
// opponent-to-move position P fills a ~120k-entry shared table, yet a 150k-node
// search of the realized descendant Q reaches the SAME depth with ~0.1% fewer
// evals (70,987 -> 70,916 on this 6x6; equally flat on a 12x12 corpus midgame,
// and no better at 30x ponder budget). Two structural causes, neither fixable
// without move prediction (explicitly out of scope for this bead):
//   1. Dilution: the ponder spreads its budget over ALL opponent replies, but
//      only the single realized reply leads to Q, so <5% of ponder nodes land in
//      Q's subtree (measured: warm adds only ~1.4k probe hits over cold's ~46k).
//   2. Depth handicap: rooted 2 plies above Q, the ponder must complete depth
//      >= T_q+2 for its entries to satisfy Q's deep-iteration depth gate, which
//      its budget cannot reach at Q's frame.
// Relaxing the entry.ply gate (candidate fix) was measured and REGRESSES depth
// (10 -> 9): fail-soft bounds and ply-relative mate scores are not soundly
// reusable across plies by a blanket relaxation, so the gate stays.
func TestPonderWarmStartDoesNotRegress(t *testing.T) {
	state, err := game.New(6, 6, 2)
	if err != nil {
		t.Fatal(err)
	}
	state = advancePreserving(state, 10)
	P := advanceUntilPlayer(state, 2)
	if P.GameOver() || P.CurrentPlayer() != 2 {
		t.Skip("could not reach an opponent-to-move midgame")
	}
	Q := advanceUntilPlayer(P, 1)
	if Q.GameOver() || Q.CurrentPlayer() != 1 {
		t.Skip("could not reach an our-to-move descendant of P")
	}

	const qBudget = 150_000
	const ponderBudget = 600_000

	cold := nodeLimitedSearch(make(map[uint64]tableEntry), 0, Q, qBudget)

	shared := make(map[uint64]tableEntry)
	nodeLimitedSearch(shared, maxSessionEntries, P, ponderBudget) // ponder fills the table
	warm := nodeLimitedSearch(shared, maxSessionEntries, Q, qBudget)

	t.Logf("cold: depth=%d nodes=%d evals=%d ; warm: depth=%d nodes=%d evals=%d (honest: ~zero warm benefit, see comment)",
		cold.Depth, cold.Nodes, cold.Evaluations, warm.Depth, warm.Nodes, warm.Evaluations)

	if warm.Depth < cold.Depth {
		t.Fatalf("warm-start depth %d < cold depth %d (pondering must not lose depth)", warm.Depth, cold.Depth)
	}
}
