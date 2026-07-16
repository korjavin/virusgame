package distill

import (
	"context"
	"go/parser"
	"go/token"
	"io/fs"
	"reflect"
	"strings"
	"testing"
	"time"

	"virusgame/arena"
	"virusgame/game"
	"virusgame/search"
)

func testConfig() Config {
	c := CIConfig()
	c.Boards = []Board{{5, 5}, {12, 12}}
	c.TrainSeeds = []uint64{1, 2, 3}
	c.TuneSeeds = []uint64{51, 52}
	return c
}

// The fit must produce byte-identical labels, weights, and provenance regardless
// of worker count.
func TestReproducibleAcrossWorkers(t *testing.T) {
	cfg := testConfig()
	a, err := RunFit(context.Background(), cfg, Limits{}, 1)
	if err != nil {
		t.Fatalf("workers=1: %v", err)
	}
	b, err := RunFit(context.Background(), cfg, Limits{}, 4)
	if err != nil {
		t.Fatalf("workers=4: %v", err)
	}
	if a.Provenance != b.Provenance {
		t.Fatalf("provenance differs across workers:\n%+v\n%+v", a.Provenance, b.Provenance)
	}
	if a.Weights != b.Weights {
		t.Fatal("weights differ across workers")
	}
	if a.Train.States != b.Train.States || a.Tune.States != b.Tune.States || a.Train.Pairs != b.Train.Pairs {
		t.Fatal("metrics differ across workers")
	}
	if a.Train.States == 0 || a.Tune.States == 0 {
		t.Fatalf("expected non-empty train and tune splits: %+v", a)
	}
}

// Structural isolation: non-test source imports nothing that can read a corpus,
// and Config exposes no path/file/reader/test field. This is the whole proof the
// trainer cannot load TRAIN/heldout/production data.
func TestStructuralIsolation(t *testing.T) {
	fset := token.NewFileSet()
	pkgs, err := parser.ParseDir(fset, ".", func(fi fs.FileInfo) bool {
		return !strings.HasSuffix(fi.Name(), "_test.go")
	}, parser.ImportsOnly)
	if err != nil {
		t.Fatalf("parse package: %v", err)
	}
	allowed := map[string]bool{
		`"context"`: true, `"crypto/sha256"`: true, `"encoding/binary"`: true,
		`"encoding/hex"`: true, `"encoding/json"`: true, `"fmt"`: true,
		`"runtime/debug"`: true, `"sync"`: true, `"time"`: true,
		`"virusgame/game"`: true, `"virusgame/search"`: true,
	}
	for _, pkg := range pkgs {
		for name, file := range pkg.Files {
			for _, imp := range file.Imports {
				if !allowed[imp.Path.Value] {
					t.Fatalf("%s imports disallowed package %s (would break corpus isolation)", name, imp.Path.Value)
				}
			}
		}
	}
	ct := reflect.TypeOf(Config{})
	for i := 0; i < ct.NumField(); i++ {
		n := strings.ToLower(ct.Field(i).Name)
		for _, bad := range []string{"path", "file", "reader", "corpus", "dir", "test"} {
			if strings.Contains(n, bad) {
				t.Fatalf("Config field %q looks like a filesystem/test input the fit must not carry", ct.Field(i).Name)
			}
		}
	}
}

// Generated splits are mutually disjoint by symmetry orbit; each is non-empty.
func TestGeneratedSplitsDisjoint(t *testing.T) {
	cfg := testConfig()
	raw, _, err := generate(cfg, [][]uint64{cfg.TrainSeeds, cfg.TuneSeeds, {901, 902}}, 0)
	if err != nil {
		t.Fatalf("generate: %v", err)
	}
	bySplit := [3]map[string]bool{{}, {}, {}}
	for _, r := range raw {
		bySplit[r.split][r.key] = true
	}
	for i := 0; i < 3; i++ {
		if len(bySplit[i]) == 0 {
			t.Fatalf("split %d empty", i)
		}
		for j := i + 1; j < 3; j++ {
			for k := range bySplit[i] {
				if bySplit[j][k] {
					t.Fatalf("orbit %s in splits %d and %d", k, i, j)
				}
			}
		}
	}
}

// A canonical orbit that would fall in two splits is a hard error.
func TestCrossSplitOrbitCollisionRejected(t *testing.T) {
	cfg := testConfig()
	if _, _, err := generate(cfg, [][]uint64{{7}, {8}, {7}}, 0); err == nil {
		t.Fatal("expected cross-split orbit collision error, got nil")
	}
}

func TestCanonicalKeyOrbitInvariant(t *testing.T) {
	for _, board := range []Board{{5, 5}, {6, 6}} {
		states := rollout(board, 1234, 0, []int{6, 9}, 12)
		if len(states) == 0 {
			t.Fatalf("no rollout states for %v", board)
		}
		for _, r := range states {
			snap := r.state.Snapshot()
			for _, variant := range []game.Snapshot{rotate180Swap(snap), transposeSwap(snap), rotate180Swap(transposeSwap(snap))} {
				vs, err := game.FromSnapshot(variant)
				if err != nil {
					t.Fatalf("symmetry variant invalid: %v", err)
				}
				if got := canonicalKey(vs); got != r.key {
					t.Fatalf("canonical key not orbit-invariant: %s vs %s", got, r.key)
				}
			}
		}
	}
}

// fit optimizes the EXACT production utility: raising the weight of a feature the
// preferred successor has more of must lower the loss and increase that weight.
func TestFitBoundedAndImproving(t *testing.T) {
	cfg := testConfig()
	cfg.Margin = 6000
	inc := search.IncumbentWeights()

	var aFeats, bFeats [4]search.FeatureVector
	aFeats[0][search.FeatureConnected] = 5000 // preferred: more connected than dispreferred
	active := [4]bool{true, true, false, false}
	rows := []prefRow{{mover: 1, aFeats: aFeats, bFeats: bFeats, aActive: active, bActive: active}}

	before := hingeLoss(cfg.Margin, rows, inc) + regLoss(cfg.Lambda, inc, inc)
	w := fit(cfg, inc, rows)
	for i := 0; i < search.FeatureCount; i++ {
		if w[i] < inc[i]-cfg.MaxDelta || w[i] > inc[i]+cfg.MaxDelta {
			t.Fatalf("weight %d = %d escaped bounds", i, w[i])
		}
	}
	after := hingeLoss(cfg.Margin, rows, w) + regLoss(cfg.Lambda, w, inc)
	if after > before {
		t.Fatalf("fit increased loss: before=%d after=%d", before, after)
	}
	if w[search.FeatureConnected] <= inc[search.FeatureConnected] {
		t.Fatalf("expected fit to raise the preferred feature weight, got %d", w[search.FeatureConnected])
	}
}

// Candidate features and exact utilities stay well inside int64.
func TestScoreOverflowBound(t *testing.T) {
	cfg := testConfig()
	raw, _, err := generate(cfg, [][]uint64{cfg.TrainSeeds, cfg.TuneSeeds}, 0)
	if err != nil {
		t.Fatal(err)
	}
	labeled, _, _, _ := labelAll(context.Background(), cfg, raw, 0, time.Time{}, 2)
	var samples []sample
	for _, s := range labeled {
		if s.cands != nil {
			samples = append(samples, s)
		}
	}
	rows := pairRows(cfg, samples, 0)
	if len(rows) == 0 {
		t.Fatal("no pairs generated")
	}
	inc := search.IncumbentWeights()
	for _, s := range samples {
		for _, c := range s.cands {
			if c.terminal {
				continue
			}
			for p := 0; p < 4; p++ {
				for i := 0; i < search.FeatureCount; i++ {
					if c.feats[p][i] > scoreBound || c.feats[p][i] < -scoreBound {
						t.Fatalf("feature %d/%d = %d exceeds scoreBound", p, i, c.feats[p][i])
					}
				}
			}
		}
	}
	for _, r := range rows {
		if u := rowPreference(r, inc); u > 1<<52 || u < -(1<<52) {
			t.Fatalf("row preference %d near int64 limit", u)
		}
	}
}

// frozenIncumbent builds a genuine, internally-consistent frozen candidate bound
// to cfg, exactly as an approved trustworthy RunFit would. Only same-package code
// (this test) can populate the unexported fields.
func frozenIncumbent(cfg Config) FrozenCandidate {
	w := search.IncumbentWeights()
	p := Provenance{Version: Version, trustworthy: true, Config: configChecksum(cfg), Weights: weightChecksum(w)}
	p.Checksum = combineProvenance(p)
	return FrozenCandidate{weights: w, provenance: p}
}

func withTrustedBuild() func() {
	prev := currentBuildTrusted
	currentBuildTrusted = func() bool { return true }
	return func() { currentBuildTrusted = prev }
}

// EvaluateTest must refuse an untrusted (dirty / revision-unavailable) build even
// with a valid frozen candidate: the default build in `go test` is untrusted.
func TestEvaluateTestRejectsUntrustedBuild(t *testing.T) {
	cfg := testConfig()
	if _, err := EvaluateTest(context.Background(), cfg, []uint64{7001, 7002}, frozenIncumbent(cfg), Limits{}, 2); err == nil {
		t.Fatal("expected rejection from an untrusted build")
	}
}

// EvaluateTest must refuse an arbitrary unfrozen candidate (the only FrozenCandidate
// external/adversarial code can name is the zero value, which is untrusted).
func TestEvaluateTestRejectsUnfrozenCandidate(t *testing.T) {
	defer withTrustedBuild()()
	if _, err := EvaluateTest(context.Background(), testConfig(), []uint64{7001, 7002}, FrozenCandidate{}, Limits{}, 2); err == nil {
		t.Fatal("expected rejection of an unfrozen candidate")
	}
}

// Adversarial mutations of a genuine token must all be rejected: changed weights
// (even with a recomputed weight checksum), changed config binding, and changed
// provenance fields, all break the combined checksum or config-hash binding.
func TestEvaluateTestRejectsTamperedCandidate(t *testing.T) {
	defer withTrustedBuild()()
	cfg := testConfig()
	seeds := []uint64{7001, 7002}

	// Changed weights with a recomputed WEIGHT checksum: the combined checksum
	// (which also binds config/build) no longer matches.
	tw := frozenIncumbent(cfg)
	tw.weights[0] += 1
	tw.provenance.Weights = weightChecksum(tw.weights)
	if _, err := EvaluateTest(context.Background(), cfg, seeds, tw, Limits{}, 2); err == nil {
		t.Fatal("changed weights + recomputed weight checksum must still be rejected")
	}

	// Changed config binding (a different config than the one fit on).
	other := testConfig()
	other.TrainSeeds = []uint64{9, 10, 11}
	if _, err := EvaluateTest(context.Background(), other, seeds, frozenIncumbent(cfg), Limits{}, 2); err == nil {
		t.Fatal("mismatched config must be rejected")
	}

	// Changed provenance field breaks the combined checksum.
	tp := frozenIncumbent(cfg)
	tp.provenance.Build = "forged-clean-build"
	if _, err := EvaluateTest(context.Background(), cfg, seeds, tp, Limits{}, 2); err == nil {
		t.Fatal("tampered provenance field must be rejected")
	}
}

// EvaluateTest must reject test seeds that reuse a burned fit seed.
func TestEvaluateTestRejectsSeedReuse(t *testing.T) {
	defer withTrustedBuild()()
	cfg := testConfig()
	if _, err := EvaluateTest(context.Background(), cfg, []uint64{cfg.TrainSeeds[0]}, frozenIncumbent(cfg), Limits{}, 2); err == nil {
		t.Fatal("expected seed-reuse rejection, got nil")
	}
}

// A genuine, trusted, config-bound candidate is accepted, and the holdout binds
// the frozen fit provenance.
func TestEvaluateTestFreshSeeds(t *testing.T) {
	defer withTrustedBuild()()
	cfg := testConfig()
	c := frozenIncumbent(cfg)
	res, err := EvaluateTest(context.Background(), cfg, []uint64{7001, 7002}, c, Limits{}, 2)
	if err != nil {
		t.Fatalf("EvaluateTest: %v", err)
	}
	if res.Test.States == 0 {
		t.Fatal("no test states")
	}
	if res.FitProvenance != c.Provenance() {
		t.Fatal("holdout result must bind the frozen fit provenance")
	}
}

func TestCandidateCoverageBounded(t *testing.T) {
	res, err := RunFit(context.Background(), testConfig(), Limits{}, 2)
	if err != nil {
		t.Fatal(err)
	}
	m := res.Train
	if m.CandidateCoverage <= 0 || m.CandidateCoverage > 1 {
		t.Fatalf("candidate coverage %.3f out of (0,1]", m.CandidateCoverage)
	}
	if m.MeanCandidates > m.MeanLegal+1e-9 {
		t.Fatalf("mean candidates %.2f exceeds mean legal %.2f", m.MeanCandidates, m.MeanLegal)
	}
	if len(res.PerBoard) == 0 {
		t.Fatal("expected per-board throughput")
	}
}

// MaxStates allocates deterministically across splits, never exhausting the
// first split and emptying the second.
func TestMaxStatesSplitBalance(t *testing.T) {
	res, err := RunFit(context.Background(), testConfig(), Limits{MaxStates: 2}, 2)
	if err != nil {
		t.Fatal(err)
	}
	if res.Train.States < 1 || res.Tune.States < 1 {
		t.Fatalf("MaxStates emptied a split: train=%d tune=%d", res.Train.States, res.Tune.States)
	}
	if res.Train.States+res.Tune.States > 2 {
		t.Fatalf("MaxStates ceiling violated: %d states", res.Train.States+res.Tune.States)
	}
}

// perRootBudget divides the ceiling across the fixed root count.
func TestPerRootBudget(t *testing.T) {
	if got := perRootBudget(0, 10); got != 0 {
		t.Fatalf("no ceiling: got %d", got)
	}
	if got := perRootBudget(1000, 4); got != 250 {
		t.Fatalf("1000/4: got %d, want 250", got)
	}
	if got := perRootBudget(3, 10); got != 1 {
		t.Fatalf("tiny ceiling must floor to 1, got %d", got)
	}
}

// A finite node ceiling is a TRUE hard compute ceiling: actual teacher nodes
// searched never exceed it, and the measurement-only output is identical for any
// worker count (per-root budgets are schedule-independent).
func TestNodeCeilingHardAndReproducible(t *testing.T) {
	cfg := testConfig()
	lim := Limits{MaxTeacherNodes: 12000}
	a, err := RunFit(context.Background(), cfg, lim, 1)
	if err != nil {
		t.Fatal(err)
	}
	b, err := RunFit(context.Background(), cfg, lim, 4)
	if err != nil {
		t.Fatal(err)
	}
	if a.LabeledNodes > lim.MaxTeacherNodes || b.LabeledNodes > lim.MaxTeacherNodes {
		t.Fatalf("hard ceiling violated: a=%d b=%d > %d", a.LabeledNodes, b.LabeledNodes, lim.MaxTeacherNodes)
	}
	if !a.Truncated || !b.Truncated {
		t.Fatalf("expected truncation at 12000 nodes: a=%v b=%v", a.Truncated, b.Truncated)
	}
	if a.Provenance != b.Provenance || a.LabeledNodes != b.LabeledNodes {
		t.Fatalf("truncated run differs across workers")
	}
}

// A total node ceiling below the generated root count is rejected before any
// search; ceilings at or above the root count are accepted and never overshoot.
func TestNodeCeilingBoundaries(t *testing.T) {
	cfg := testConfig()
	raw, _, err := generate(cfg, [][]uint64{cfg.TrainSeeds, cfg.TuneSeeds}, 0)
	if err != nil {
		t.Fatal(err)
	}
	roots := uint64(len(raw))
	if roots < 2 {
		t.Fatalf("need multiple roots, got %d", roots)
	}
	// Below the root count: rejected before searching.
	for _, ceiling := range []uint64{1, roots - 1} {
		if _, err := RunFit(context.Background(), cfg, Limits{MaxTeacherNodes: ceiling}, 2); err == nil {
			t.Fatalf("ceiling %d < roots %d must be rejected", ceiling, roots)
		}
	}
	// At and above the root count: accepted, hard-bounded, worker-invariant.
	for _, ceiling := range []uint64{roots, roots + 1} {
		a, err := RunFit(context.Background(), cfg, Limits{MaxTeacherNodes: ceiling}, 1)
		if err != nil {
			t.Fatalf("ceiling %d should be accepted: %v", ceiling, err)
		}
		b, err := RunFit(context.Background(), cfg, Limits{MaxTeacherNodes: ceiling}, 4)
		if err != nil {
			t.Fatal(err)
		}
		if a.LabeledNodes > ceiling || b.LabeledNodes > ceiling {
			t.Fatalf("ceiling %d overshoot: a=%d b=%d", ceiling, a.LabeledNodes, b.LabeledNodes)
		}
		if a.LabeledNodes != b.LabeledNodes || a.Provenance != b.Provenance {
			t.Fatalf("ceiling %d not worker-invariant", ceiling)
		}
	}
}

// A truncated (measurement-only) run must not approve or publish learned weights.
func TestTruncatedCannotApprove(t *testing.T) {
	res, err := RunFit(context.Background(), testConfig(), Limits{MaxTeacherNodes: 8000}, 2)
	if err != nil {
		t.Fatal(err)
	}
	if !res.Truncated {
		t.Fatal("expected truncated run")
	}
	if res.Approved {
		t.Fatal("truncated run must not be approved")
	}
	if res.Weights != search.IncumbentWeights() {
		t.Fatal("truncated run must return the incumbent weights, never partial learned weights")
	}
}

// A past wall deadline makes completion schedule-dependent: the run is discarded
// as measurement-only with the incumbent weights, never a dataset.
func TestDeadlineDiscardsRun(t *testing.T) {
	res, err := RunFit(context.Background(), testConfig(), Limits{Deadline: time.Now().Add(-time.Second)}, 2)
	if err != nil {
		t.Fatalf("deadline discard should be measurement-only, not an error: %v", err)
	}
	if !res.Truncated || res.Approved || res.Weights != search.IncumbentWeights() {
		t.Fatalf("discarded run must be truncated, unapproved, incumbent-weighted: %+v", res)
	}
}

func TestProductionWeightsImmutable(t *testing.T) {
	before := search.IncumbentWeights()
	res, err := RunFit(context.Background(), testConfig(), Limits{}, 2)
	if err != nil {
		t.Fatal(err)
	}
	if search.IncumbentWeights() != before {
		t.Fatal("IncumbentWeights changed across RunFit")
	}
	state, _ := game.New(5, 5, 2)
	WeightedAgent(context.Background(), res.Weights, 2)(state)
	if search.IncumbentWeights() != before {
		t.Fatal("IncumbentWeights changed after WeightedAgent use")
	}
}

// The incumbent-weight panel agent is byte-identical to production ChooseDepth.
func TestWeightedAgentMatchesProduction(t *testing.T) {
	state, err := game.New(8, 8, 2)
	if err != nil {
		t.Fatal(err)
	}
	got, ok := WeightedAgent(context.Background(), search.IncumbentWeights(), 2)(state)
	want, ok2 := search.ChooseDepth(context.Background(), state, 2)
	if !ok || !ok2 || got != want.Action {
		t.Fatalf("weighted incumbent agent %v != ChooseDepth %v", got, want.Action)
	}
	if _, err := state.Apply(got); err != nil {
		t.Fatalf("agent returned illegal action: %v", err)
	}
}

// Predeclared, seat-balanced panel of equal-depth weighted search against the
// incumbent search plus baselines. arena is a pure harness; no corpus is opened.
func TestPanelSeatBalancedSmoke(t *testing.T) {
	res, err := RunFit(context.Background(), testConfig(), Limits{}, 2)
	if err != nil {
		t.Fatal(err)
	}
	contender := arena.Agent(WeightedAgent(context.Background(), res.Weights, 2))
	boards := []arena.Board{{Rows: 5, Cols: 5}}
	panel := []struct {
		name    string
		factory arena.OpponentFactory
	}{
		{"incumbent", func(uint64) arena.Agent {
			return arena.Agent(WeightedAgent(context.Background(), search.IncumbentWeights(), 2))
		}},
		{"greedy", func(uint64) arena.Agent { return arena.Greedy }},
		{"base", func(uint64) arena.Agent { return arena.BaseAttacker }},
		{"mobility", func(uint64) arena.Agent { return arena.MobilityAttacker }},
	}
	for _, opp := range panel {
		report, err := arena.Balanced(boards, 1, contender, opp.factory)
		if err != nil {
			t.Fatalf("panel %s: %v", opp.name, err)
		}
		if report.Illegal != 0 || report.Stalled != 0 {
			t.Fatalf("panel %s illegal/stall: %s", opp.name, report)
		}
		t.Logf("panel %s: %s", opp.name, report)
	}
}

// A cancelled context aborts the weighted agent's in-progress search: it returns
// a legal action (a preserving fallback) instead of completing a deep search or
// hanging. This is the mechanism that makes the panel deadline a hard bound.
func TestWeightedAgentHonorsCancel(t *testing.T) {
	state, err := game.New(12, 12, 2)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // already cancelled: the search must not run to depth
	action, ok := WeightedAgent(ctx, search.IncumbentWeights(), 6)(state)
	if !ok {
		t.Fatal("cancelled agent should still play a legal fallback")
	}
	if _, err := state.Apply(action); err != nil {
		t.Fatalf("cancelled agent returned illegal action: %v", err)
	}
	// A live context at the same depth must instead complete the real search.
	live, ok := WeightedAgent(context.Background(), search.IncumbentWeights(), 6)(state)
	if !ok {
		t.Fatal("live agent failed")
	}
	want, _ := search.ChooseDepth(context.Background(), state, 6)
	if live != want.Action {
		t.Fatalf("live weighted agent %v != production ChooseDepth %v", live, want.Action)
	}
}

// Comprehensive config/limit validation must reject malformed input.
func TestValidationRejects(t *testing.T) {
	cases := map[string]func(c *Config, l *Limits){
		"tiny board":          func(c *Config, _ *Limits) { c.Boards = []Board{{1, 5}} },
		"dup train seed":      func(c *Config, _ *Limits) { c.TrainSeeds = []uint64{1, 1} },
		"seed in both splits": func(c *Config, _ *Limits) { c.TuneSeeds = []uint64{c.TrainSeeds[0]} },
		"empty train":         func(c *Config, _ *Limits) { c.TrainSeeds = nil },
		"teacher depth 0":     func(c *Config, _ *Limits) { c.TeacherDepth = 0 },
		"shallow depth 0":     func(c *Config, _ *Limits) { c.ShallowDepth = 0 },
		"ply beyond maxplies": func(c *Config, _ *Limits) { c.Plies = []int{99} },
		"negative step":       func(c *Config, _ *Limits) { c.Step = -1 },
		"zero fit passes":     func(c *Config, _ *Limits) { c.FitPasses = 0 },
		"negative delta":      func(c *Config, _ *Limits) { c.MaxDelta = -1 },
		"overflowing delta":   func(c *Config, _ *Limits) { c.MaxDelta = 1 << 40 },
		"negative margin":     func(c *Config, _ *Limits) { c.Margin = -1 },
		"oversize board":      func(c *Config, _ *Limits) { c.Boards = []Board{{51, 5}} },
		"panel depth 0":       func(c *Config, _ *Limits) { c.PanelDepth = 0 },
		"panel depth huge":    func(c *Config, _ *Limits) { c.PanelDepth = maxSearchDepth + 1 },
		"teacher depth huge":  func(c *Config, _ *Limits) { c.TeacherDepth = maxSearchDepth + 1 },
		"negative max states": func(_ *Config, l *Limits) { l.MaxStates = -1 },
		"max states < splits": func(_ *Config, l *Limits) { l.MaxStates = 1 },
	}
	for name, mutate := range cases {
		cfg := testConfig()
		lim := Limits{}
		mutate(&cfg, &lim)
		if _, err := RunFit(context.Background(), cfg, lim, 1); err == nil {
			t.Fatalf("%s: expected validation error, got nil", name)
		}
	}
}

// Each bounded scalar is accepted at its maximum and rejected one past it. Tested
// against validate directly so it stays fast and precise.
// Loss accumulation must not overflow even under adversarial magnitudes: maxed
// features, maxed margin/lambda, and many rows. Saturation keeps the result
// finite and non-negative.
func TestLossCannotOverflow(t *testing.T) {
	var aFeats, bFeats [4]search.FeatureVector
	for p := 0; p < 4; p++ {
		for i := 0; i < search.FeatureCount; i++ {
			aFeats[p][i] = scoreBound
			bFeats[p][i] = -scoreBound
		}
	}
	active := [4]bool{true, true, true, true}
	row := prefRow{mover: 1, aFeats: bFeats, bFeats: aFeats, aActive: active, bActive: active} // dispreferred hugely better
	rows := make([]prefRow, maxPairsCap)
	for i := range rows {
		rows[i] = row
	}
	var w, inc search.WeightVector
	for i := range w {
		w[i] = 1000 + maxSafeWeightDelta
		inc[i] = 1000
	}
	h := hingeLoss(maxMargin, rows, w)
	g := regLoss(maxLambda, w, inc)
	if h < 0 || h > lossSentinel || g < 0 || g > lossSentinel {
		t.Fatalf("loss escaped [0,sentinel]: hinge=%d reg=%d", h, g)
	}
	if s := satAdd(h, g); s < 0 || s > lossSentinel {
		t.Fatalf("combined loss overflowed: %d", s)
	}
}

// The combined roots x pairs envelope rejects datasets that would allocate far
// beyond memory even when each per-knob maximum is individually valid.
func TestRowEnvelopeCombinedBound(t *testing.T) {
	// MaxPairs==0 means "all", capped at maxCandidatePairs per state.
	okRoots := maxTotalRows/maxCandidatePairs - 1
	if err := checkRowEnvelope(okRoots, 0); err != nil {
		t.Fatalf("within-envelope should pass: %v", err)
	}
	if err := checkRowEnvelope(okRoots+2, 0); err == nil {
		t.Fatal("roots x pairs over the envelope must be rejected")
	}
	// Both knobs individually valid (roots <= maxRoots, pairs <= maxPairsCap) but
	// their product blows the envelope.
	if err := checkRowEnvelope(maxRoots, maxPairsCap); err == nil {
		t.Fatal("valid-but-multiplying maxima must be rejected by the combined bound")
	}
}

func TestValidationBoundaries(t *testing.T) {
	base := func() Config {
		c := testConfig()
		c.MaxPlies = maxPlyDepth // let boundary plies fit
		return c
	}
	set := []struct {
		name        string
		atMax, over func(*Config)
	}{
		{"board dim", func(c *Config) { c.Boards = []Board{{maxBoardDim, maxBoardDim}} }, func(c *Config) { c.Boards = []Board{{maxBoardDim + 1, 2}} }},
		{"margin", func(c *Config) { c.Margin = maxMargin }, func(c *Config) { c.Margin = maxMargin + 1 }},
		{"lambda", func(c *Config) { c.Lambda = maxLambda }, func(c *Config) { c.Lambda = maxLambda + 1 }},
		{"max pairs", func(c *Config) { c.MaxPairs = maxPairsCap }, func(c *Config) { c.MaxPairs = maxPairsCap + 1 }},
		{"fit passes", func(c *Config) { c.FitPasses = maxFitPasses }, func(c *Config) { c.FitPasses = maxFitPasses + 1 }},
		{"max plies", func(c *Config) { c.MaxPlies = maxPlyDepth }, func(c *Config) { c.MaxPlies = maxPlyDepth + 1 }},
		{"panel depth", func(c *Config) { c.PanelDepth = maxSearchDepth }, func(c *Config) { c.PanelDepth = maxSearchDepth + 1 }},
		{"max delta", func(c *Config) { c.MaxDelta = maxSafeWeightDelta }, func(c *Config) { c.MaxDelta = maxSafeWeightDelta + 1 }},
	}
	for _, s := range set {
		atMax := base()
		s.atMax(&atMax)
		if err := validate(atMax, Limits{}, 2); err != nil {
			t.Fatalf("%s at max should be accepted: %v", s.name, err)
		}
		over := base()
		s.over(&over)
		if err := validate(over, Limits{}, 2); err == nil {
			t.Fatalf("%s at max+1 should be rejected", s.name)
		}
	}
}
