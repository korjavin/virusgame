package search

import (
	"context"
	"sync"
	"testing"
	"time"

	"virusgame/game"
)

// armingContext is a self-contained test context that reports "not cancelled"
// until the search has polled it armAt times — proving the search was genuinely
// in progress (it descended through armAt node checks) — then reports cancelled.
// No production instrumentation or global seam is involved: the search observes
// cancellation only through the standard context.Context interface it already
// polls.
type armingContext struct {
	mu    sync.Mutex
	polls int
	armAt int
	ch    chan struct{}
	armed bool
}

func newArmingContext(armAt int) *armingContext {
	return &armingContext{armAt: armAt, ch: make(chan struct{})}
}
func (c *armingContext) Deadline() (time.Time, bool) { return time.Time{}, false }
func (c *armingContext) Value(any) any               { return nil }
func (c *armingContext) Done() <-chan struct{} {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.polls++
	if c.polls >= c.armAt && !c.armed {
		c.armed = true
		close(c.ch)
	}
	return c.ch
}
func (c *armingContext) Err() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.armed {
		return context.Canceled
	}
	return nil
}
func (c *armingContext) pollCount() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.polls
}

var teacherFixtures = []struct {
	rows, cols, players int
	seed                int64
}{
	{5, 5, 2, 11}, {5, 5, 2, 12}, {6, 6, 2, 13}, {8, 8, 2, 14}, {12, 12, 2, 15},
	{5, 7, 2, 16}, {7, 5, 2, 17}, {8, 12, 2, 18}, // rectangles
	{6, 6, 3, 19}, {8, 8, 3, 20}, {8, 8, 4, 21}, {12, 12, 4, 22}, // 3-4 players
}

// The maximum-score, minimum-ordinal candidate from RootScores must equal the
// action ChooseDepth selects; the candidate universe must equal ChooseDepth's
// RootCompleted count; and every candidate must be a legal root move. Covers
// rectangles and 2-4 players.
func TestRootScoresMatchesChooseDepth(t *testing.T) {
	for _, f := range teacherFixtures {
		state := randomReachableState(t, f.rows, f.cols, f.players, f.seed)
		for depth := 1; depth <= 4; depth++ {
			cands, _, _, ok := RootScores(context.Background(), state, depth)
			cd, cdOK := ChooseDepth(context.Background(), state, depth)
			if ok != cdOK {
				t.Fatalf("%dx%d-%dp depth %d: ok mismatch %v/%v", f.rows, f.cols, f.players, depth, ok, cdOK)
			}
			if !ok {
				continue
			}
			top, hasTop := TopCandidate(cands)
			if !hasTop || top.Action != cd.Action {
				t.Fatalf("%dx%d-%dp seed %d depth %d: top %v != ChooseDepth %v", f.rows, f.cols, f.players, f.seed, depth, top.Action, cd.Action)
			}
			if top.Score != cd.Score {
				t.Fatalf("%dx%d-%dp depth %d: top score %d != ChooseDepth score %d", f.rows, f.cols, f.players, depth, top.Score, cd.Score)
			}
			if len(cands) != cd.RootCompleted {
				t.Fatalf("%dx%d-%dp depth %d: candidate universe %d != RootCompleted %d", f.rows, f.cols, f.players, depth, len(cands), cd.RootCompleted)
			}
			legal := map[game.Action]bool{}
			for _, a := range state.LegalActions() {
				legal[a] = true
			}
			for _, c := range cands {
				if !legal[c.Action] {
					t.Fatalf("%dx%d-%dp depth %d: candidate %v not legal", f.rows, f.cols, f.players, depth, c.Action)
				}
			}
		}
	}
}

// RootScoresBudget must never expand more nodes than its limit, and a limit too
// small to complete the root reports incomplete without exceeding the budget.
func TestRootScoresBudgetHardCeiling(t *testing.T) {
	state := randomReachableState(t, 12, 12, 2, 55)
	full, fullNodes, _, ok := RootScoresBudget(context.Background(), state, 4, 0)
	if !ok || len(full) == 0 {
		t.Fatal("unbounded search failed")
	}
	for _, limit := range []uint64{1, 50, 500, fullNodes / 2} {
		_, nodes, _, done := RootScoresBudget(context.Background(), state, 4, limit)
		if nodes > limit {
			t.Fatalf("budget %d exceeded: expanded %d nodes", limit, nodes)
		}
		if done {
			t.Fatalf("budget %d < full %d should not complete", limit, fullNodes)
		}
	}
}

// ChooseDepthWeighted must abort on a cancelled context and return a legal
// preserving fallback rather than completing a deep search.
func TestChooseDepthWeightedHonorsCancel(t *testing.T) {
	state := randomReachableState(t, 12, 12, 2, 71)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	res, ok := ChooseDepthWeighted(ctx, state, 6, IncumbentWeights())
	if ok {
		t.Fatal("cancelled search must report incomplete")
	}
	if _, err := state.Apply(res.Action); err != nil {
		t.Fatalf("cancelled search returned illegal fallback: %v", err)
	}
}

// Deterministic mid-search cancellation with NO production instrumentation: a
// deep search runs under a self-arming context that only reports cancellation
// after the search has polled it armAt times — i.e. after it has genuinely
// descended through armAt node checks. The search must then abort with an
// incomplete result and a legal fallback. No wall-clock timing, no global seam.
func TestChooseDepthWeightedCancelMidSearch(t *testing.T) {
	state := randomReachableState(t, 12, 12, 2, 73)
	const armAt = 200
	ctx := newArmingContext(armAt)

	res, ok := ChooseDepthWeighted(ctx, state, 40, IncumbentWeights())
	if ok {
		t.Fatal("mid-search cancellation must report incomplete")
	}
	if ctx.pollCount() < armAt {
		t.Fatalf("search did not reach the arming point: polled %d < %d", ctx.pollCount(), armAt)
	}
	if _, err := state.Apply(res.Action); err != nil {
		t.Fatalf("mid-search cancellation returned illegal fallback: %v", err)
	}
}

// RootScores must be deterministic run to run.
func TestRootScoresDeterministic(t *testing.T) {
	state := randomReachableState(t, 8, 8, 2, 99)
	a, _, _, ok1 := RootScores(context.Background(), state, 3)
	b, _, _, ok2 := RootScores(context.Background(), state, 3)
	if !ok1 || !ok2 || len(a) != len(b) {
		t.Fatalf("length mismatch: ok=%v/%v len=%d/%d", ok1, ok2, len(a), len(b))
	}
	for i := range a {
		if a[i] != b[i] {
			t.Fatalf("candidate %d differs: %+v vs %+v", i, a[i], b[i])
		}
	}
}

// Injecting the incumbent weights must reproduce ChooseDepth exactly: the
// weighted-search offline path leaves production decisions unchanged.
func TestChooseDepthWeightedIncumbentDigest(t *testing.T) {
	for _, f := range teacherFixtures {
		state := randomReachableState(t, f.rows, f.cols, f.players, f.seed)
		for depth := 1; depth <= 4; depth++ {
			base, ok1 := ChooseDepth(context.Background(), state, depth)
			inj, ok2 := ChooseDepthWeighted(context.Background(), state, depth, IncumbentWeights())
			if ok1 != ok2 {
				t.Fatalf("%dx%d-%dp depth %d: ok mismatch", f.rows, f.cols, f.players, depth)
			}
			if base.Action != inj.Action || base.Score != inj.Score || base.Nodes != inj.Nodes || base.Evaluations != inj.Evaluations {
				t.Fatalf("%dx%d-%dp depth %d: weighted(incumbent) diverged from ChooseDepth: %+v vs %+v", f.rows, f.cols, f.players, depth, base, inj)
			}
		}
	}
}

// UtilitiesWeighted with incumbent weights must equal the production evaluator.
func TestUtilitiesWeightedMatchesProduction(t *testing.T) {
	for _, f := range teacherFixtures {
		state := randomReachableState(t, f.rows, f.cols, f.players, f.seed)
		var ws evalWorkspace
		want := evaluateAllWithWorkspace(state, &ws)
		got := UtilitiesWeighted(state, IncumbentWeights())
		if got != want {
			t.Fatalf("%dx%d-%dp: UtilitiesWeighted(incumbent)=%v != production %v", f.rows, f.cols, f.players, got, want)
		}
		if Utilities(state) != want {
			t.Fatalf("%dx%d-%dp: Utilities != production", f.rows, f.cols, f.players)
		}
	}
}

// An out-of-range player must return a safe neutral 0, never panic.
func TestUtilityFromFeaturesInvalidPlayer(t *testing.T) {
	var feats [4]FeatureVector
	active := [4]bool{true, true, false, false}
	for _, p := range []game.Player{0, 5, 200} {
		if got := UtilityFromFeatures(feats, active, p, IncumbentWeights()); got != 0 {
			t.Fatalf("player %d: got %d, want neutral 0", p, got)
		}
	}
}

// UtilityFromFeatures must equal the production per-player utility on any
// non-terminal state, for several weight vectors, across rectangles and 2-4
// players. This is what lets the fit optimize the exact production score.
func TestUtilityFromFeaturesMatchesProduction(t *testing.T) {
	weightSets := []WeightVector{IncumbentWeights(), {}, {}}
	for i := range weightSets[1] {
		weightSets[1][i] = 500
		weightSets[2][i] = int64(1000 + 37*i - (i%3)*250)
	}
	for _, f := range teacherFixtures {
		state := randomReachableState(t, f.rows, f.cols, f.players, f.seed)
		if state.GameOver() {
			continue
		}
		var fe FeatureExtractor
		feats := fe.Extract(state)
		var active [4]bool
		for p := game.Player(1); p <= 4; p++ {
			active[p-1] = state.Active(p)
		}
		for wi, w := range weightSets {
			var ws evalWorkspace
			want := evaluateAllWithWeights(state, &ws, w)
			for p := game.Player(1); p <= 4; p++ {
				if !active[p-1] {
					continue
				}
				got := UtilityFromFeatures(feats, active, p, w)
				if got != want[p-1] {
					t.Fatalf("%dx%d-%dp weights#%d player %d: UtilityFromFeatures=%d != production %d", f.rows, f.cols, f.players, wi, p, got, want[p-1])
				}
			}
		}
	}
}
