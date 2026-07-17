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

// TestPonderWarmStartReachesDeeperPerNode proves the ponder mechanism: a
// node-budgeted ponder of the opponent-to-move position P fills the shared table
// with best-action move ordering for the our-to-move descendant Q, so a
// node-budgeted search of Q that reuses the table reaches at least as deep as a
// cold search on the same node budget.
func TestPonderWarmStartReachesDeeperPerNode(t *testing.T) {
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

	t.Logf("cold: depth=%d nodes=%d evals=%d ; warm: depth=%d nodes=%d evals=%d",
		cold.Depth, cold.Nodes, cold.Evaluations, warm.Depth, warm.Nodes, warm.Evaluations)

	if warm.Depth < cold.Depth {
		t.Fatalf("warm-start depth %d < cold depth %d (pondering must not lose depth)", warm.Depth, cold.Depth)
	}
}
