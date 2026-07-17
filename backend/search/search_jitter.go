package search

import (
	"context"
	"math/rand"

	"virusgame/game"
)

// jitterEpsilon bounds which root moves count as "near-equal" to the best and
// are therefore eligible for seeded tie-break jitter (see ChooseSeeded).
//
// Score scale (evaluate.go): one owned cell is worth normalized(1, area, 30) =
// 30_000/area — >=300 on any board up to 100 cells — and base-shape terms are
// 80..240 points each. 40 sits an order of magnitude below any single-cell or
// base positional difference, so jitter only ever reshuffles genuine near-ties
// (a few points of mobility/space noise) and never trades away a real edge.
const jitterEpsilon = 40

type scoredAction struct {
	action game.Action
	score  int
}

// ChooseSeeded is the production wall-clock entry point. It behaves exactly like
// Choose but, when seed != 0, breaks ties among near-equal root moves with an RNG
// seeded from a per-game/turn value so that two different games diverge (no more
// bit-identical replayable lines) while a single game stays stable. seed == 0 is
// identical to Choose.
func ChooseSeeded(ctx context.Context, state game.State, seed uint64) (Result, bool) {
	return choose(ctx, state, seed)
}

// ChooseDepthSeeded / ChooseNodeBudgetSeeded expose the same jitter over the
// deterministic-budget paths. They exist ONLY so the strength-guard gates can run
// a jitter-forced-ON variant; production and every existing deterministic caller
// stay bit-identical (seed 0).
func ChooseDepthSeeded(ctx context.Context, state game.State, depth int, seed uint64) (Result, bool) {
	return chooseDepth(ctx, state, depth, seed)
}

func ChooseNodeBudgetSeeded(state game.State, limit uint64, seed uint64) (Result, bool) {
	return chooseNodeBudget(state, limit, seed)
}

// pickJittered chooses uniformly among the near-equal root candidates using an RNG
// seeded from the caller's per-game seed. A unique best (0 or 1 candidate) leaves
// the deterministic best untouched, so jitter never picks a materially worse move.
func pickJittered(candidates []scoredAction, seed uint64, fallback game.Action) game.Action {
	if seed == 0 || len(candidates) <= 1 {
		if len(candidates) == 1 {
			return candidates[0].action
		}
		return fallback
	}
	rng := rand.New(rand.NewSource(int64(seed)))
	return candidates[rng.Intn(len(candidates))].action
}

// atDepthCollecting is the jitter-path root search: instead of stopping at the
// single best move it records the exact score of every root move within
// jitterEpsilon of the best, so ChooseSeeded can pick uniformly among them.
// Clearly-worse moves are rejected with a cheap null-window probe (max pruning),
// so only genuine near-ties pay for an exact re-search.
func (s *searcher) atDepthCollecting(state game.State, depth int) (Result, bool) {
	key := stateHash(state)
	rootEntry, hasRoot := s.table[key]
	children, ok := s.orderedChildren(state, rootEntry.bestAction, hasRoot)
	if !ok || len(children) == 0 {
		return Result{}, ok
	}
	children = preservingChildren(children, s.root)

	scored := make([]scoredAction, 0, len(children))
	bestScore := -infScore
	var bestAction game.Action

	if s.multi {
		// maxN returns exact per-player scores (no alpha-beta window), so collect
		// directly.
		for _, child := range children {
			values, complete := s.maxN(child.state, depth-1, 1)
			if !complete {
				return Result{}, false
			}
			score := values[s.root-1]
			scored = append(scored, scoredAction{child.action, score})
			if score > bestScore {
				bestScore, bestAction = score, child.action
			}
		}
	} else {
		alpha := -infScore
		for i, child := range children {
			var score int
			var complete bool
			if i == 0 {
				score, complete = s.minimax(child.state, depth-1, -infScore, infScore, 1)
			} else {
				// Reject anything provably worse than best-eps with a null-window
				// probe (cheap, fully pruned). Survivors are near-best; re-search
				// them exactly (beta unbounded => exact for value >= thr).
				thr := alpha - jitterEpsilon
				probe, ok := s.minimax(child.state, depth-1, thr-1, thr, 1)
				if !ok {
					return Result{}, false
				}
				if probe < thr {
					continue
				}
				score, complete = s.minimax(child.state, depth-1, thr-1, infScore, 1)
			}
			if !complete {
				return Result{}, false
			}
			scored = append(scored, scoredAction{child.action, score})
			if score > bestScore {
				bestScore, bestAction = score, child.action
			}
			if score > alpha {
				alpha = score
			}
		}
	}

	// Retain only the moves within epsilon of the final best. (Rejected moves are
	// never appended above, so this only trims exact-scored siblings whose score
	// slipped below the threshold as a later move raised the best.)
	kept := scored[:0]
	for _, c := range scored {
		if c.score >= bestScore-jitterEpsilon {
			kept = append(kept, c)
		}
	}
	s.rootCandidates = append(s.rootCandidates[:0], kept...)
	s.table[key] = tableEntry{depth: depth, ply: 0, flag: flagExact, bestAction: bestAction, values: [4]int{bestScore}}
	return Result{Action: bestAction, Score: bestScore}, true
}
