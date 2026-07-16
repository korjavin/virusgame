package search

import (
	"context"

	"virusgame/game"
)

// This file is the narrow, offline/test-scoped teacher-ranking and weighted-
// evaluation surface used by the distillation pilot. Nothing here is on the
// production move path: Choose and ChooseDepth keep their exact behavior (proven
// by the unchanged decision digest in TestChooseDepthChooseNodeBudgetDigestEquality
// and TestChooseDepthWeightedIncumbentDigest). Injected weights are passed by
// value, so the incumbent weights are never mutated.

// CandidateScore is one root candidate a fixed-depth search actually evaluated:
// its action, its completed full-window search score, and its stable root
// ordinal (the legal-generation index used as the deterministic tie-break).
type CandidateScore struct {
	Action  game.Action
	Score   int
	Ordinal int
}

// RootScores runs one deterministic, fully completed fixed-depth incumbent-weight
// search and returns the exact bounded root candidate universe it evaluated — the
// same selected, preservation-filtered children the ChooseDepth path scores via
// the shared scoreChild routine — each with its completed score and stable
// ordinal, in search order. ok is false when the root is incomplete or has no
// legal move. The maximum-score, minimum-ordinal candidate equals ChooseDepth's
// action (TestRootScoresMatchesChooseDepth).
func RootScores(ctx context.Context, state game.State, depth int) (candidates []CandidateScore, nodes, evaluations uint64, ok bool) {
	return RootScoresBudget(ctx, state, depth, 0)
}

// RootScoresBudget is RootScores with a hard per-root node ceiling: the search
// aborts once it has expanded nodeLimit nodes and reports incomplete (ok=false),
// so the actual nodes searched never exceed nodeLimit. nodeLimit == 0 means no
// limit. nodes always reflects the real nodes expanded, even on an incomplete
// (budget-exhausted) root, which lets callers account for a true compute ceiling.
// Offline/test only.
func RootScoresBudget(ctx context.Context, state game.State, depth int, nodeLimit uint64) (candidates []CandidateScore, nodes, evaluations uint64, ok bool) {
	if depth < 1 || depth > maxDepth {
		return nil, 0, 0, false
	}
	if _, ok := preservingFallback(state); !ok {
		return nil, 0, 0, false
	}
	if ctx == nil {
		ctx = context.Background()
	}
	s := newSearcher(ctx, state)
	s.nodeLimit = nodeLimit
	cands, complete := s.rankAtDepth(state, depth)
	if !complete {
		return nil, s.nodes, s.evaluations, false
	}
	return cands, s.nodes, s.evaluations, true
}

func (s *searcher) rankAtDepth(state game.State, depth int) ([]CandidateScore, bool) {
	children, _, _, ok := s.orderedChildren(game.NewPosition(state), true)
	if !ok || len(children) == 0 {
		return nil, false
	}
	children = preservingChildren(children, s.root)
	out := make([]CandidateScore, 0, len(children))
	for _, child := range children {
		score, complete := s.scoreChild(child, depth)
		if !complete {
			return nil, false
		}
		out = append(out, CandidateScore{Action: child.action, Score: score, Ordinal: child.ordinal})
	}
	return out, true
}

// TopCandidate returns the maximum-score, minimum-ordinal candidate — the action
// ChooseDepth would select from the same evaluated set.
func TopCandidate(candidates []CandidateScore) (CandidateScore, bool) {
	if len(candidates) == 0 {
		return CandidateScore{}, false
	}
	best := candidates[0]
	for _, c := range candidates[1:] {
		if c.Score > best.Score || (c.Score == best.Score && c.Ordinal < best.Ordinal) {
			best = c
		}
	}
	return best, true
}

// ChooseDepthWeighted is ChooseDepth with an injected, immutable weight vector.
// Offline/test only. ChooseDepthWeighted(ctx, state, depth, IncumbentWeights())
// is byte-identical to ChooseDepth (TestChooseDepthWeightedIncumbentDigest), so
// production defaults are provably unchanged.
func ChooseDepthWeighted(ctx context.Context, state game.State, depth int, weights WeightVector) (Result, bool) {
	if depth < 1 || depth > maxDepth {
		return Result{}, false
	}
	fallback, ok := preservingFallback(state)
	if !ok {
		return Result{}, false
	}
	if ctx == nil {
		ctx = context.Background()
	}
	s := newSearcher(ctx, state)
	s.weights = weights
	result, complete := s.atDepth(state, depth)
	if !complete {
		return Result{Action: fallback}, false
	}
	result.Depth = depth
	result.CompletedTurnDepth = completedTurns(state.MovesLeft(), depth)
	result.Workers = 1
	result.IterationsStarted = 1
	result.IterationsCompleted = 1
	result.Nodes = s.nodes
	result.Evaluations = s.evaluations
	return result, true
}

// Utilities returns the production per-player evaluation of a state (incumbent
// weights). Offline/test only; it is the exact quantity distillation must match.
func Utilities(state game.State) [4]int {
	return UtilitiesWeighted(state, IncumbentWeights())
}

// UtilitiesWeighted returns the per-player evaluation under injected weights,
// using the exact production scoring core (per-player dot/WeightScale then the
// integer opponent-average combine). Offline/test only.
func UtilitiesWeighted(state game.State, weights WeightVector) [4]int {
	var workspace evalWorkspace
	return evaluateAllWithWeights(state, &workspace, weights)
}

// UtilityFromFeatures computes one player's exact production utility directly
// from a per-player feature matrix and active mask, replicating the non-terminal
// scoring core: ScoreFeatures per active player (dot with /WeightScale integer
// truncation), then the integer opponent-score-average combine. player is
// 1-based; an out-of-range player is not a seat and returns a neutral 0. This is
// the exact quantity the distillation fit must optimize; it equals
// UtilitiesWeighted / evaluateAllWithWeights on any non-terminal state
// (TestUtilityFromFeaturesMatchesProduction). Offline/test only.
func UtilityFromFeatures(features [4]FeatureVector, active [4]bool, player game.Player, weights WeightVector) int {
	if player < 1 || player > 4 {
		return 0
	}
	var raw [4]int64
	count := 0
	for p := 0; p < 4; p++ {
		if active[p] {
			raw[p] = scoreFeatures(features[p], weights)
			count++
		} else {
			raw[p] = -mateScore / 2
		}
	}
	index := int(player - 1)
	if !active[index] {
		return safeInt(raw[index])
	}
	var opponents int64
	for p := 0; p < 4; p++ {
		if p != index && active[p] {
			opponents += raw[p]
		}
	}
	if count > 1 {
		return safeInt(raw[index] - opponents/int64(count-1))
	}
	return safeInt(raw[index])
}
