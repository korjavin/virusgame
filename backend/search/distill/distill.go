// Package distill runs a bounded, deterministic teacher-preference distillation
// pilot. It generates its own fixed-seed states, canonicalizes them by evaluator
// symmetry orbit, labels the exact bounded root candidate universe a deeper
// fixed-depth search actually evaluated (search.RootScores), fits regularized
// integer weights toward the frozen incumbent by pairwise ranking, and measures
// — with exact production integer semantics — whether the ranking signal
// transfers.
//
// Correctness contract:
//   - Student ranking and the panel use search.ChooseDepthWeighted: real
//     weighted production search with injected, immutable weights. There is no
//     hand-rolled evaluator, so agreement and panel results carry exact
//     production integer semantics. Production defaults are provably unchanged.
//   - Pairs are built only among candidates the teacher actually searched at
//     equal completed depth; incomplete roots are rejected, never mislabeled.
//   - The verdict flow is fit -> tune (calibration) -> once-only test. RunFit
//     never sees test seeds; only EvaluateTest touches a fresh, frozen test set.
//   - Structural isolation: imports only game, search, and pure stdlib; no path,
//     io.Reader, or filesystem entry point, so no strength corpus can be loaded
//     (TestStructuralIsolation).
package distill

import (
	"context"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"runtime/debug"
	"sync"
	"time"

	"virusgame/game"
	"virusgame/search"
)

// Version stamps the provenance so a checksum is only compared within one pilot
// contract.
const Version = "distill-pilot-v2"

// seedBase is deliberately distinct from corpusgen's constant so uniform-random
// rollouts here cannot retrace the directed strength-corpus trajectories.
const seedBase = 0xD1B54A32D192ED03

// scoreBound is a conservative ceiling on |feature|; any value above it means the
// evaluator contract changed (TestScoreOverflowBound).
const scoreBound = 1 << 40

// agreementDepth is the shallow weighted-search depth at which student ranking
// agreement with the deep teacher is measured (1-ply, production semantics).
const agreementDepth = 1

// maxSafeWeightDelta bounds how far a weight may move from the incumbent so the
// dot product ScoreFeatures computes cannot overflow int64: with |feature| <
// scoreBound and FeatureCount terms, |w| <= 1000+delta stays far below int64.
const maxSafeWeightDelta = 200_000

// Resource bounds on the exported API. They are finite maxima chosen so no input
// can request unbounded CPU/memory or overflow the loss accumulators. The teacher
// search already caps root candidates per state (search.rootOptionalLimit=32), so
// pairs per state are O(32^2); with maxRoots states and maxMargin per hinge term,
// the loss sum stays far below int64 (see TestScoreOverflowBound / boundary test).
const (
	maxBoardDim    = 50   // project boards are 2..50 per side; also caps rows*cols to 2500
	maxBoardCount  = 64   // distinct board sizes per config
	maxSeedCount   = 4096 // seeds per split
	maxPlyMarks    = 256  // number of checkpoint plies
	maxPlyDepth    = 1024 // rollout length (MaxPlies)
	maxMargin      = 1 << 28
	maxLambda      = 1 << 28
	maxPairsCap    = 4096 // MaxPairs per state
	maxFitPasses   = 1 << 14
	maxSearchDepth = 64 // matches search.maxDepth
	maxRoots       = 1 << 16
	// maxCandidatePairs bounds pairs contributed per state: the teacher's root
	// universe is <= 32 candidates, so ordered non-terminal pairs are < 32*32.
	maxCandidatePairs = 1024
	// maxTotalRows bounds the COMBINED dataset size (roots x pairs), so validated
	// per-knob maxima cannot multiply into a tens-of-GB allocation. At ~1.1 KiB
	// per prefRow this caps the pair dataset near ~1.1 GiB.
	maxTotalRows = 1 << 20
)

type Board struct{ Rows, Cols int }

// Config is the path-free fit input. It carries only train and tune seeds; test
// seeds are supplied separately to EvaluateTest so the fit can never observe
// them. Workers is passed to RunFit separately (execution concern, not a label
// determinant).
type Config struct {
	Boards       []Board  `json:"boards"`
	TrainSeeds   []uint64 `json:"train_seeds"`
	TuneSeeds    []uint64 `json:"tune_seeds"`
	Plies        []int    `json:"plies"`
	MaxPlies     int      `json:"max_plies"`
	TeacherDepth int      `json:"teacher_depth"`
	ShallowDepth int      `json:"shallow_depth"`
	MaxPairs     int      `json:"max_pairs"`
	Margin       int64    `json:"margin"`
	Lambda       int64    `json:"lambda"`
	MaxDelta     int64    `json:"max_delta"`
	Step         int64    `json:"step"`
	FitPasses    int      `json:"fit_passes"`
	PanelDepth   int      `json:"panel_depth"`
}

// Limits is a hard, measured ceiling honored inside generation and labeling. A
// zero field means no cap. Deadline additionally bounds wall time via context.
type Limits struct {
	MaxStates       int
	MaxTeacherNodes uint64
	Deadline        time.Time
}

// CIConfig is a fast, deterministic smoke pilot for CI (<10m).
func CIConfig() Config {
	return Config{
		Boards:       []Board{{5, 5}, {12, 12}},
		TrainSeeds:   []uint64{1, 2, 3, 4},
		TuneSeeds:    []uint64{51, 52},
		Plies:        []int{6, 9},
		MaxPlies:     12,
		TeacherDepth: 4,
		ShallowDepth: 1,
		MaxPairs:     16,
		Margin:       1000,
		Lambda:       2,
		MaxDelta:     2000,
		Step:         50,
		FitPasses:    200,
		PanelDepth:   2,
	}
}

// PilotConfig is the offline 5/12/20 fit config. It is a STARTING point whose
// size the operator scales from measured per-board throughput, keeping the full
// run within the offline budget. Do not run it before a measured allocation.
func PilotConfig() Config {
	c := CIConfig()
	c.Boards = []Board{{5, 5}, {12, 12}, {20, 20}}
	c.TrainSeeds = []uint64{1, 2, 3, 4, 5, 6, 7, 8}
	c.TuneSeeds = []uint64{51, 52, 53, 54}
	c.Plies = []int{6, 9, 12, 15}
	c.MaxPlies = 18
	c.TeacherDepth = 6
	return c
}

// validate rejects malformed configs and limits before any work. groups is the
// number of splits the caller will generate (2 for RunFit, 3 for EvaluateTest).
func validate(cfg Config, limits Limits, groups int) error {
	if len(cfg.Boards) == 0 || len(cfg.Boards) > maxBoardCount {
		return fmt.Errorf("distill: need 1..%d boards", maxBoardCount)
	}
	for _, b := range cfg.Boards {
		if b.Rows < 2 || b.Rows > maxBoardDim || b.Cols < 2 || b.Cols > maxBoardDim {
			return fmt.Errorf("distill: board %dx%d invalid (each side must be 2..%d)", b.Rows, b.Cols, maxBoardDim)
		}
	}
	if len(cfg.TrainSeeds) == 0 || len(cfg.TuneSeeds) == 0 {
		return fmt.Errorf("distill: need train and tune seeds")
	}
	if len(cfg.TrainSeeds) > maxSeedCount || len(cfg.TuneSeeds) > maxSeedCount {
		return fmt.Errorf("distill: at most %d seeds per split", maxSeedCount)
	}
	if err := uniqueSeeds(cfg.TrainSeeds); err != nil {
		return fmt.Errorf("distill: train %w", err)
	}
	if err := uniqueSeeds(cfg.TuneSeeds); err != nil {
		return fmt.Errorf("distill: tune %w", err)
	}
	train := map[uint64]bool{}
	for _, s := range cfg.TrainSeeds {
		train[s] = true
	}
	for _, s := range cfg.TuneSeeds {
		if train[s] {
			return fmt.Errorf("distill: seed %d appears in both train and tune splits", s)
		}
	}
	if cfg.TeacherDepth < 1 || cfg.TeacherDepth > maxSearchDepth || cfg.ShallowDepth < 1 || cfg.ShallowDepth > maxSearchDepth {
		return fmt.Errorf("distill: teacher and shallow depth must be in [1,%d]", maxSearchDepth)
	}
	if cfg.PanelDepth < 1 || cfg.PanelDepth > maxSearchDepth {
		return fmt.Errorf("distill: panel_depth must be in [1,%d]", maxSearchDepth)
	}
	if cfg.MaxPlies < 1 || cfg.MaxPlies > maxPlyDepth {
		return fmt.Errorf("distill: max_plies must be in [1,%d]", maxPlyDepth)
	}
	if len(cfg.Plies) == 0 || len(cfg.Plies) > maxPlyMarks {
		return fmt.Errorf("distill: need 1..%d checkpoint plies", maxPlyMarks)
	}
	for _, p := range cfg.Plies {
		if p < 1 || p > cfg.MaxPlies {
			return fmt.Errorf("distill: ply %d out of range [1,%d]", p, cfg.MaxPlies)
		}
	}
	if cfg.MaxPairs < 0 || cfg.MaxPairs > maxPairsCap {
		return fmt.Errorf("distill: max_pairs must be in [0,%d]", maxPairsCap)
	}
	if cfg.Margin < 0 || cfg.Margin > maxMargin {
		return fmt.Errorf("distill: margin must be in [0,%d]", maxMargin)
	}
	if cfg.Lambda < 0 || cfg.Lambda > maxLambda {
		return fmt.Errorf("distill: lambda must be in [0,%d]", maxLambda)
	}
	if cfg.Step < 1 || cfg.Step > maxSafeWeightDelta {
		return fmt.Errorf("distill: step must be in [1,%d]", maxSafeWeightDelta)
	}
	if cfg.FitPasses < 1 || cfg.FitPasses > maxFitPasses {
		return fmt.Errorf("distill: fit_passes must be in [1,%d]", maxFitPasses)
	}
	if cfg.MaxDelta < 0 || cfg.MaxDelta > maxSafeWeightDelta {
		return fmt.Errorf("distill: max_delta must be in [0,%d] to keep scoring overflow-safe", maxSafeWeightDelta)
	}
	if limits.MaxStates < 0 {
		return fmt.Errorf("distill: max_states must be >= 0")
	}
	if limits.MaxStates > 0 && limits.MaxStates < groups {
		return fmt.Errorf("distill: max_states %d cannot populate every one of %d splits", limits.MaxStates, groups)
	}
	return nil
}

// checkRowEnvelope bounds the COMBINED pair dataset (roots x pairs-per-state),
// so validated per-knob maxima cannot multiply into an unbounded allocation. A
// state contributes at most min(MaxPairs, maxCandidatePairs) rows (MaxPairs==0
// means "all", capped by the teacher's bounded candidate universe).
func checkRowEnvelope(roots, maxPairs int) error {
	perState := maxPairs
	if perState <= 0 || perState > maxCandidatePairs {
		perState = maxCandidatePairs
	}
	if int64(roots)*int64(perState) > maxTotalRows {
		return fmt.Errorf("distill: combined roots(%d) x pairs(%d) exceeds the %d-row memory envelope; reduce seeds/plies/boards or max_pairs", roots, perState, maxTotalRows)
	}
	return nil
}

func uniqueSeeds(seeds []uint64) error {
	seen := map[uint64]bool{}
	for _, s := range seeds {
		if seen[s] {
			return fmt.Errorf("duplicate seed %d", s)
		}
		seen[s] = true
	}
	return nil
}

func countSplit(samples []sample, split int) int {
	n := 0
	for _, s := range samples {
		if s.split == split {
			n++
		}
	}
	return n
}

type BoardThroughput struct {
	Board        Board
	States       int
	TeacherNodes uint64
	NodesPerSec  float64 // summed nodes / summed per-call teacher time (CPU-time)
}

// SplitMetrics summarizes one generated split.
type SplitMetrics struct {
	States              int
	Pairs               int
	MeanLegal           float64
	MeanCandidates      float64
	CandidateCoverage   float64 // candidates / legal: the bounded universe fraction
	ForcedWinStates     int     // teacher top is a terminal win
	ForcedLossStates    int     // teacher top is a terminal loss (no survival available)
	TerminalCandidates  int
	AgreementLearned    float64 // exact 1-ply weighted-search agreement with teacher
	AgreementIncumbent  float64
	PositionalStates    int
	PositionalLearned   float64
	PositionalIncumbent float64
	TieFractionLearned  float64
	TeacherDisagreement float64 // deep teacher vs shallow search
}

type Provenance struct {
	Version     string
	Build       string // go version + VCS revision + dirty flag
	Config      string
	Dataset     string
	Labels      string
	Weights     string
	Checksum    string
	trustworthy bool // revision present and tree clean; required to publish weights
}

// FitResult is the outcome of RunFit. Approved is true only if the fitted weights
// improve tune ranking agreement over the incumbent; otherwise the pilot reports
// a null result and the weights are a non-approved candidate.
type FitResult struct {
	Weights      search.WeightVector
	Train, Tune  SplitMetrics
	PerBoard     []BoardThroughput
	Provenance   Provenance
	WallElapsed  time.Duration
	StatesPerSec float64
	LabeledNodes uint64 // cumulative teacher nodes of the kept (within-budget) prefix
	Duplicates   int
	Incomplete   int
	Truncated    bool // a node ceiling or wall deadline made this measurement-only
	Approved     bool
	Verdict      string
	// Frozen is set only on an approved, trustworthy fit. It is the sole token
	// EvaluateTest accepts, binding the approved weights to their provenance so the
	// once-only holdout cannot be run against an arbitrary unfrozen candidate.
	Frozen *FrozenCandidate
}

// FrozenCandidate binds an approved fit's weights to its provenance (checksums +
// trustworthy source identity). Its fields are UNEXPORTED so it is unforgeable
// outside this package: only RunFit constructs a genuine one, and external code
// can neither build a non-zero token nor mutate a genuine one (it can copy the
// value, but the copy carries the original genuine fields). A zero-value
// FrozenCandidate{} an external caller can name is untrusted and rejected. Read
// access is via the copy accessors below, so reports can serialize a safe view
// without exposing a forgery path.
type FrozenCandidate struct {
	weights    search.WeightVector
	provenance Provenance
}

// Weights returns a copy of the frozen weights (WeightVector is an array, copied
// by value).
func (c FrozenCandidate) Weights() search.WeightVector { return c.weights }

// Provenance returns a copy of the frozen fit provenance (all its own fields are
// value types, so this exposes no mutable internal state).
func (c FrozenCandidate) Provenance() Provenance { return c.provenance }

// MarshalJSON emits a safe, read-only view for reports. There is deliberately no
// UnmarshalJSON: decoding cannot reconstruct a trusted token, so a serialized
// candidate can never be replayed to authorize a holdout.
func (c FrozenCandidate) MarshalJSON() ([]byte, error) {
	return json.Marshal(struct {
		Weights    search.WeightVector `json:"weights"`
		Provenance Provenance          `json:"provenance"`
	}{c.weights, c.provenance})
}

// TestResult is the once-only frozen-holdout evaluation.
type TestResult struct {
	Test          SplitMetrics
	Provenance    Provenance
	FitProvenance Provenance // the frozen fit identity this holdout was bound to
	Duplicates    int
	Incomplete    int
}

// RunFit generates the train and tune splits, fits weights on train, and reports
// tune calibration. It never accesses any test seeds.
func RunFit(ctx context.Context, cfg Config, limits Limits, workers int) (FitResult, error) {
	if err := validate(cfg, limits, 2); err != nil {
		return FitResult{}, err
	}
	raw, dups, err := generate(cfg, [][]uint64{cfg.TrainSeeds, cfg.TuneSeeds}, limits.MaxStates)
	if err != nil {
		return FitResult{}, err
	}
	if len(raw) == 0 {
		return FitResult{}, fmt.Errorf("distill: no states generated")
	}
	if len(raw) > maxRoots {
		return FitResult{}, fmt.Errorf("distill: %d generated roots exceeds the %d cap; reduce seeds/plies/boards", len(raw), maxRoots)
	}
	if err := checkRowEnvelope(len(raw), cfg.MaxPairs); err != nil {
		return FitResult{}, err
	}
	// A total node ceiling below the root count cannot be honored per root
	// (per-root budgets would sum above the ceiling); reject before any search.
	if limits.MaxTeacherNodes > 0 && limits.MaxTeacherNodes < uint64(len(raw)) {
		return FitResult{}, fmt.Errorf("distill: max_teacher_nodes %d < %d generated roots; ceiling too small to bound per-root search", limits.MaxTeacherNodes, len(raw))
	}

	inc := search.IncumbentWeights()
	perRoot := perRootBudget(limits.MaxTeacherNodes, len(raw))
	started := time.Now()
	labeled, actualNodes, budgetHit, deadlineHit := labelAll(ctx, cfg, raw, perRoot, limits.Deadline, workers)
	wall := time.Since(started)

	// A wall deadline that leaves any root incomplete makes the dataset
	// schedule-dependent: discard the whole run as measurement only.
	if deadlineHit {
		return FitResult{Weights: inc, Approved: false, Truncated: true, Duplicates: dups, WallElapsed: wall, LabeledNodes: actualNodes,
			Verdict: "DISCARDED: wall deadline made teacher completion schedule-dependent; measurement only, incumbent weights"}, nil
	}

	// Collect completed samples in generation order (worker-invariant).
	var samples []sample
	for _, s := range labeled {
		if s.cands != nil {
			samples = append(samples, s)
		}
	}
	res := FitResult{Weights: inc, Duplicates: dups, Incomplete: budgetHit, WallElapsed: wall, LabeledNodes: actualNodes}
	res.PerBoard = throughput(samples)
	if secs := wall.Seconds(); secs > 0 && len(samples) > 0 {
		res.StatesPerSec = float64(len(samples)) / secs
	}

	// A finite node ceiling that could not fully search every root makes the run
	// measurement only: return incumbent weights, do not fit, publish nothing.
	if limits.MaxTeacherNodes > 0 && budgetHit > 0 {
		res.Truncated = true
		res.Train = splitMetrics(samples, 0, nil, inc, inc)
		res.Tune = splitMetrics(samples, 1, nil, inc, inc)
		res.Provenance = provenance(cfg, samples, inc)
		res.Verdict = fmt.Sprintf("TRUNCATED: node ceiling (%d nodes, %d/root) left %d/%d roots unsearched; measurement only, incumbent weights, no fit", limits.MaxTeacherNodes, perRoot, budgetHit, len(raw))
		return res, nil
	}
	if budgetHit > 0 {
		return FitResult{}, fmt.Errorf("distill: %d roots failed to complete without a node ceiling", budgetHit)
	}

	// Both fit splits must be non-empty to produce a coherent fit.
	trainStates, tuneStates := countSplit(samples, 0), countSplit(samples, 1)
	if trainStates == 0 || tuneStates == 0 {
		return FitResult{}, fmt.Errorf("distill: empty fit split (train=%d tune=%d)", trainStates, tuneStates)
	}

	rows := pairRows(cfg, samples, 0)
	weights := fit(cfg, inc, rows)
	res.Weights = weights
	res.Train = splitMetrics(samples, 0, rows, weights, inc)
	res.Tune = splitMetrics(samples, 1, pairRows(cfg, samples, 1), weights, inc)
	res.Provenance = provenance(cfg, samples, weights)

	res.Approved = res.Tune.AgreementLearned > res.Tune.AgreementIncumbent
	if res.Approved {
		// A publishable (approved) run must have a trustworthy build identity.
		if !res.Provenance.trustworthy {
			return FitResult{}, fmt.Errorf("distill: refusing to publish approved weights from an untrusted build (%s); commit a clean tree so vcs.revision is stamped", res.Provenance.Build)
		}
		res.Frozen = &FrozenCandidate{weights: weights, provenance: res.Provenance}
		res.Verdict = fmt.Sprintf("tune improved: learned=%.3f > incumbent=%.3f (candidate weights, pending frozen test)", res.Tune.AgreementLearned, res.Tune.AgreementIncumbent)
	} else {
		res.Verdict = fmt.Sprintf("NULL: tune did not improve (learned=%.3f <= incumbent=%.3f); no candidate weights, no scale/freeze", res.Tune.AgreementLearned, res.Tune.AgreementIncumbent)
	}
	return res, nil
}

// EvaluateTest is the single, frozen-holdout evaluation of an APPROVED fit
// candidate on a fresh test seed set. It accepts only a FrozenCandidate produced
// by an approved RunFit, verifies the candidate's provenance is trustworthy and
// its weights match the frozen checksum, and requires the current build itself to
// be trustworthy — so the once-only holdout can never be evaluated from a dirty
// or revision-unavailable build, nor against an arbitrary unfrozen candidate. It
// rejects test seeds that reuse any fit seed and enforces orbit disjointness.
func EvaluateTest(ctx context.Context, cfg Config, testSeeds []uint64, candidate FrozenCandidate, limits Limits, workers int) (TestResult, error) {
	prov := candidate.provenance
	if prov.Version != Version {
		return TestResult{}, fmt.Errorf("distill: candidate provenance version %q != %q", prov.Version, Version)
	}
	if !prov.trustworthy {
		return TestResult{}, fmt.Errorf("distill: candidate is not a trusted approved fit (frozen provenance untrustworthy)")
	}
	// Verify the FULL frozen provenance is internally consistent — the combined
	// checksum must bind exactly the recorded build/config/dataset/label/weight
	// checksums — not merely the (recomputable) weight checksum.
	if prov.Checksum != combineProvenance(prov) || prov.Weights != weightChecksum(candidate.weights) {
		return TestResult{}, fmt.Errorf("distill: candidate provenance checksums are inconsistent (tampered token)")
	}
	if !currentBuildTrusted() {
		return TestResult{}, fmt.Errorf("distill: refusing to evaluate the frozen holdout from an untrusted build; commit a clean tree so vcs.revision is stamped")
	}
	if err := validate(cfg, limits, 3); err != nil {
		return TestResult{}, err
	}
	// The holdout may only be run under the EXACT config the candidate was fit on:
	// a changed config/seeds/boards yields a different hash and is rejected.
	if prov.Config != configChecksum(cfg) {
		return TestResult{}, fmt.Errorf("distill: config does not match the frozen fit identity; the holdout is bound to its fit config")
	}
	weights := candidate.weights
	if len(testSeeds) == 0 {
		return TestResult{}, fmt.Errorf("distill: test seeds required")
	}
	if err := uniqueSeeds(testSeeds); err != nil {
		return TestResult{}, fmt.Errorf("distill: test %w", err)
	}
	fitSeeds := map[uint64]bool{}
	for _, s := range append(append([]uint64{}, cfg.TrainSeeds...), cfg.TuneSeeds...) {
		fitSeeds[s] = true
	}
	for _, s := range testSeeds {
		if fitSeeds[s] {
			return TestResult{}, fmt.Errorf("distill: test seed %d reuses a fit seed; a burned seed cannot decide the verdict", s)
		}
	}
	// Generating all three groups reuses the cross-split orbit-collision guard, so
	// the test split is provably disjoint (by symmetry orbit) from train and tune.
	raw, dups, err := generate(cfg, [][]uint64{cfg.TrainSeeds, cfg.TuneSeeds, testSeeds}, limits.MaxStates)
	if err != nil {
		return TestResult{}, err
	}
	test := raw[:0]
	for _, r := range raw {
		if r.split == 2 {
			test = append(test, r)
		}
	}
	if len(test) == 0 {
		return TestResult{}, fmt.Errorf("distill: no test states generated")
	}
	if len(test) > maxRoots {
		return TestResult{}, fmt.Errorf("distill: %d test roots exceeds the %d cap", len(test), maxRoots)
	}
	if err := checkRowEnvelope(len(test), cfg.MaxPairs); err != nil {
		return TestResult{}, err
	}
	if limits.MaxTeacherNodes > 0 && limits.MaxTeacherNodes < uint64(len(test)) {
		return TestResult{}, fmt.Errorf("distill: max_teacher_nodes %d < %d test roots; ceiling too small to bound per-root search", limits.MaxTeacherNodes, len(test))
	}
	perRoot := perRootBudget(limits.MaxTeacherNodes, len(test))
	labeled, _, budgetHit, deadlineHit := labelAll(ctx, cfg, test, perRoot, limits.Deadline, workers)
	if deadlineHit {
		return TestResult{}, fmt.Errorf("distill: test run discarded; wall deadline made completion schedule-dependent")
	}
	if budgetHit > 0 {
		return TestResult{}, fmt.Errorf("distill: test run discarded; node ceiling left %d/%d roots unsearched", budgetHit, len(test))
	}
	var samples []sample
	for _, s := range labeled {
		if s.cands != nil {
			samples = append(samples, s)
		}
	}
	if len(samples) == 0 {
		return TestResult{}, fmt.Errorf("distill: all test teacher choices incomplete")
	}
	inc := search.IncumbentWeights()
	res := TestResult{Duplicates: dups, FitProvenance: candidate.provenance}
	res.Test = splitMetrics(samples, 2, pairRows(cfg, samples, 2), weights, inc)
	res.Provenance = provenance(cfg, samples, weights)
	return res, nil
}

// --- state generation with symmetry-orbit canonical dedup -------------------

type rawState struct {
	split int
	board Board
	state game.State
	key   string
}

func xorshift(x uint64) uint64 {
	x ^= x << 13
	x ^= x >> 7
	x ^= x << 17
	return x
}

// generate produces snapshot states via uniform-random legal rollouts. Every
// state of one seed's trajectory stays in that seed's split. States are
// canonicalized by evaluator symmetry orbit: an orbit seen twice in the SAME
// split is dropped (counted); an orbit seen in TWO splits is a hard error, so no
// essentially-identical position can leak between splits.
//
// maxStates caps the dataset deterministically by allocating an equal per-split
// budget (at least 1 each), so a cap never exhausts the first split and empties a
// later one. Output stays in a fixed generation order independent of workers.
func generate(cfg Config, groups [][]uint64, maxStates int) ([]rawState, int, error) {
	if maxStates > 0 && maxStates < len(groups) {
		return nil, 0, fmt.Errorf("distill: max_states %d cannot populate every one of %d splits", maxStates, len(groups))
	}
	perSplit := 0
	if maxStates > 0 {
		perSplit = maxStates / len(groups) // floor: total <= perSplit*groups <= maxStates
		if perSplit < 1 {
			perSplit = 1
		}
	}
	seen := map[string]int{}
	var out []rawState
	dups := 0
	for split, seeds := range groups {
		kept := 0
		for _, board := range cfg.Boards {
			for _, seed := range seeds {
				for _, r := range rollout(board, seed, split, cfg.Plies, cfg.MaxPlies) {
					prior, ok := seen[r.key]
					if ok {
						if prior != split {
							return nil, 0, fmt.Errorf("distill: symmetry-orbit collision across splits %d and %d (key %s)", prior, split, r.key)
						}
						dups++
						continue
					}
					seen[r.key] = split
					if perSplit > 0 && kept >= perSplit {
						continue
					}
					out = append(out, r)
					kept++
				}
			}
		}
	}
	return out, dups, nil
}

func rollout(board Board, seed uint64, split int, plies []int, maxPlies int) []rawState {
	state, err := game.New(board.Rows, board.Cols, 2)
	if err != nil {
		return nil
	}
	mark := map[int]bool{}
	for _, p := range plies {
		mark[p] = true
	}
	rng := xorshift(seedBase ^ seed ^ uint64(board.Rows*131+board.Cols*613))
	var out []rawState
	for ply := 1; ply <= maxPlies; ply++ {
		if state.GameOver() {
			break
		}
		actions := state.LegalActions()
		if len(actions) == 0 {
			break
		}
		rng = xorshift(rng)
		next, err := state.Apply(actions[int(rng%uint64(len(actions)))])
		if err != nil {
			break
		}
		state = next
		if mark[ply] && !state.GameOver() && len(state.LegalActions()) >= 2 {
			out = append(out, rawState{split: split, board: board, state: state, key: canonicalKey(state)})
		}
	}
	return out
}

// --- labeling ---------------------------------------------------------------

// candidate is one root move the fixed-depth teacher actually evaluated. score is
// the teacher's completed fixed-depth search score. For non-terminal candidates
// feats/active hold the successor's full per-player feature matrix and active
// mask, so the fit can compute each preference under the EXACT production utility
// (search.UtilityFromFeatures) rather than any surrogate. Terminal candidates
// carry no feature signal and are excluded from fitting; their forced outcomes
// are handled exactly by weighted search at agreement/panel time.
type candidate struct {
	action      game.Action
	score       int
	ordinal     int
	terminal    bool
	winForMover bool
	survives    bool
	feats       [4]search.FeatureVector
	active      [4]bool
}

type sample struct {
	split         int
	board         Board
	key           string
	mover         game.Player
	state         game.State
	legalCount    int
	candCount     int
	teacherAction game.Action
	shallowAction game.Action
	teacherNodes  uint64
	teacherWall   time.Duration
	forcedWin     bool
	forcedLoss    bool
	positional    bool
	cands         []candidate
}

// perRootBudget derives the deterministic per-root node budget from a total
// teacher-node ceiling and the fixed root count: floor(ceiling/roots), at least 1
// when a finite ceiling is set. Because the budget depends only on the ceiling
// and the (worker-independent) root count, every root's bounded search is
// identical for any worker count, and the summed actual nodes never exceed the
// ceiling.
func perRootBudget(ceiling uint64, roots int) uint64 {
	if ceiling == 0 || roots <= 0 {
		return 0
	}
	b := ceiling / uint64(roots)
	if b == 0 {
		b = 1
	}
	return b
}

// labelAll shards teacher labeling across workers, writing results by index so
// the ordered outcome is worker-count invariant. Each root searches under a fixed
// per-root node budget (a true hard compute ceiling: the search aborts at the
// budget). A root that cannot complete within its budget is left incomplete
// (cands == nil) and counts toward budgetHit. A wall deadline is honored only as
// a discard signal.
func labelAll(ctx context.Context, cfg Config, raw []rawState, perRoot uint64, deadline time.Time, workers int) (labeled []sample, actualNodes uint64, budgetHit int, deadlineHit bool) {
	if ctx == nil {
		ctx = context.Background()
	}
	if !deadline.IsZero() {
		var cancel context.CancelFunc
		ctx, cancel = context.WithDeadline(ctx, deadline)
		defer cancel()
	}
	if workers < 1 {
		workers = 1
	}
	labeled = make([]sample, len(raw))
	jobs := make(chan int)
	var wg sync.WaitGroup
	for w := 0; w < workers; w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			var fe search.FeatureExtractor
			for i := range jobs {
				labeled[i] = labelOne(ctx, cfg, raw[i], perRoot, &fe)
			}
		}()
	}
	for i := range raw {
		jobs <- i
	}
	close(jobs)
	wg.Wait()

	for _, s := range labeled {
		actualNodes += s.teacherNodes
		if s.cands == nil {
			budgetHit++
		}
	}
	deadlineHit = ctx.Err() != nil
	return labeled, actualNodes, budgetHit, deadlineHit
}

func labelOne(ctx context.Context, cfg Config, r rawState, perRoot uint64, fe *search.FeatureExtractor) sample {
	state := r.state
	mover := state.CurrentPlayer()
	s := sample{split: r.split, board: r.board, key: r.key, mover: mover, state: state}
	s.legalCount = len(state.LegalActions())

	start := time.Now()
	scored, nodes, _, ok := search.RootScoresBudget(ctx, state, cfg.TeacherDepth, perRoot)
	s.teacherWall = time.Since(start)
	s.teacherNodes = nodes // real nodes expanded, even on a budget-exhausted root
	if !ok || len(scored) == 0 {
		return s // cands stays nil -> rejected as incomplete (budget/deadline)
	}
	s.cands = candidatesFrom(state, mover, scored, fe)
	s.candCount = len(s.cands)

	top, _ := search.TopCandidate(scored)
	s.teacherAction = top.Action
	for _, c := range s.cands {
		if c.action != top.Action {
			continue
		}
		switch {
		case c.terminal && c.winForMover:
			s.forcedWin = true
		case c.terminal:
			s.forcedLoss = true
		default:
			s.positional = true
		}
	}

	shallowDepth := cfg.ShallowDepth
	if shallowDepth < 1 {
		shallowDepth = 1
	}
	if shallow, ok := search.ChooseDepth(ctx, state, shallowDepth); ok {
		s.shallowAction = shallow.Action
	}
	return s
}

func candidatesFrom(state game.State, mover game.Player, scored []search.CandidateScore, fe *search.FeatureExtractor) []candidate {
	out := make([]candidate, 0, len(scored))
	for _, cs := range scored {
		next, err := state.Apply(cs.Action)
		if err != nil {
			continue
		}
		c := candidate{action: cs.Action, score: cs.Score, ordinal: cs.Ordinal, terminal: next.GameOver(), survives: next.Active(mover)}
		if c.terminal {
			c.winForMover = next.Winner() == mover
		} else {
			c.feats = fe.Extract(next)
			for p := game.Player(1); p <= 4; p++ {
				c.active[p-1] = next.Active(p)
			}
		}
		out = append(out, c)
	}
	return out
}

// --- pairwise ranking dataset ----------------------------------------------

// prefRow is one teacher preference a >- b between two non-terminal candidates
// the teacher scored (a's fixed-depth score strictly higher). It carries each
// successor's full per-player feature matrix and active mask so the fit can
// score the preference under the EXACT production utility for any trial weights,
// never a surrogate.
type prefRow struct {
	mover            game.Player
	aFeats, bFeats   [4]search.FeatureVector
	aActive, bActive [4]bool
}

// pairRows builds the exact-utility preference dataset for one split, ONLY among
// non-terminal candidates the teacher actually evaluated. Ordered pairs (a, b)
// with strictly higher teacher score for a become rows. When a state yields more
// than MaxPairs rows, a deterministic even stride subsamples them across the
// whole ordered list, so the dataset is balanced across the score/order
// distribution rather than biased to the first candidates.
func pairRows(cfg Config, samples []sample, split int) []prefRow {
	var out []prefRow
	for _, s := range samples {
		if s.split != split {
			continue
		}
		var nt []candidate
		for _, c := range s.cands {
			if !c.terminal {
				nt = append(nt, c)
			}
		}
		var stateRows []prefRow
		for _, a := range nt {
			for _, b := range nt {
				if a.score <= b.score {
					continue
				}
				stateRows = append(stateRows, prefRow{mover: s.mover, aFeats: a.feats, bFeats: b.feats, aActive: a.active, bActive: b.active})
			}
		}
		out = append(out, subsample(stateRows, cfg.MaxPairs)...)
	}
	return out
}

// subsample keeps at most limit items with a deterministic even stride.
func subsample(items []prefRow, limit int) []prefRow {
	if limit <= 0 || len(items) <= limit {
		return items
	}
	out := make([]prefRow, 0, limit)
	for k := 0; k < limit; k++ {
		out = append(out, items[k*len(items)/limit])
	}
	return out
}

// rowPreference is the exact production utility gap of a preference row under w:
// (preferred - dispreferred), both scored with search.UtilityFromFeatures. The
// fit wants this strictly above the hinge margin.
// lossSentinel caps accumulated loss well below int64's range so the fit's loss
// arithmetic provably cannot overflow for ANY validated input, independent of
// feature magnitudes. Realistic losses are ~1e16, far below the sentinel, so
// saturation never triggers in practice and the fit comparison stays exact.
const lossSentinel = int64(1) << 61

// satAdd adds two non-negative values, saturating at lossSentinel. The plain a+b
// path only runs when a+b < lossSentinel < MaxInt64, so it cannot overflow.
func satAdd(a, b int64) int64 {
	if b >= lossSentinel-a {
		return lossSentinel
	}
	return a + b
}

func clampNonNeg(v int64) int64 {
	if v > lossSentinel {
		return lossSentinel
	}
	if v < 0 {
		return 0
	}
	return v
}

func rowPreference(r prefRow, w search.WeightVector) int64 {
	// Clamp each utility to +-lossSentinel/2 before subtracting so the difference
	// (and any downstream margin-s) cannot overflow int64 even for adversarial
	// feature magnitudes. Realistic utilities are ~1e8, far inside this range.
	a := clampHalf(int64(search.UtilityFromFeatures(r.aFeats, r.aActive, r.mover, w)))
	b := clampHalf(int64(search.UtilityFromFeatures(r.bFeats, r.bActive, r.mover, w)))
	return a - b
}

func clampHalf(x int64) int64 {
	if x > lossSentinel/2 {
		return lossSentinel / 2
	}
	if x < -lossSentinel/2 {
		return -lossSentinel / 2
	}
	return x
}

// --- deterministic integer weight fit --------------------------------------

// fit runs deterministic coordinate descent from the incumbent weights,
// minimizing hinge ranking loss under the EXACT production utility plus an L1
// pull toward the incumbent. Because the exact utility is non-linear in w
// (per-player /WeightScale truncation, integer opponent averaging), each trial
// recomputes every row's exact preference — the dataset is small and
// determinism matters more than micro-optimization. Weights stay within MaxDelta
// of the incumbent.
func fit(cfg Config, inc search.WeightVector, rows []prefRow) search.WeightVector {
	w := inc
	if len(rows) == 0 || cfg.Step == 0 {
		return w
	}
	cur := hingeLoss(cfg.Margin, rows, w) + regLoss(cfg.Lambda, w, inc)
	for pass := 0; pass < cfg.FitPasses; pass++ {
		improved := false
		for i := 0; i < search.FeatureCount; i++ {
			bestDelta, bestLoss := int64(0), cur
			for _, delta := range [2]int64{cfg.Step, -cfg.Step} {
				nw := w[i] + delta
				if nw < inc[i]-cfg.MaxDelta || nw > inc[i]+cfg.MaxDelta {
					continue
				}
				trial := w
				trial[i] = nw
				if l := hingeLoss(cfg.Margin, rows, trial) + regLoss(cfg.Lambda, trial, inc); l < bestLoss {
					bestLoss, bestDelta = l, delta
				}
			}
			if bestDelta != 0 {
				w[i] += bestDelta
				cur = bestLoss
				improved = true
			}
		}
		if !improved {
			break
		}
	}
	return w
}

func hingeLoss(margin int64, rows []prefRow, w search.WeightVector) int64 {
	var l int64
	for _, r := range rows {
		// rowPreference is clamped to +-lossSentinel and margin is validated
		// <= maxMargin, so margin-s cannot overflow before clamping/saturating.
		if s := rowPreference(r, w); margin-s > 0 {
			l = satAdd(l, clampNonNeg(margin-s))
		}
	}
	return l
}

func regLoss(lambda int64, w, inc search.WeightVector) int64 {
	var l int64
	for i := 0; i < search.FeatureCount; i++ {
		d := w[i] - inc[i]
		if d < 0 {
			d = -d
		}
		l = satAdd(l, clampNonNeg(lambda*d))
	}
	return l
}

// --- exact agreement via weighted search ------------------------------------

// agrees reports whether a 1-ply weighted search under w reproduces the teacher's
// chosen action. This is exact production integer semantics: the same search
// core, injected weights, no surrogate.
func agrees(state game.State, teacher game.Action, w search.WeightVector) bool {
	res, ok := search.ChooseDepthWeighted(context.Background(), state, agreementDepth, w)
	return ok && res.Action == teacher
}

// WeightedAgent returns a fixed-depth weighted-search agent for the offline
// panel. Both contender and incumbent play through this same production search
// core at equal depth; the incumbent-weight instance is byte-identical to the
// production engine. It never mutates production weights.
//
// It is context-aware: ChooseDepthWeighted aborts an in-progress search when ctx
// is cancelled and returns a legal preserving fallback, so a cancelled panel
// finishes the current game in fallback moves almost immediately instead of
// completing a full deep search. A fully completed search returns its real move.
func WeightedAgent(ctx context.Context, w search.WeightVector, depth int) func(game.State) (game.Action, bool) {
	if ctx == nil {
		ctx = context.Background()
	}
	if depth < 1 {
		depth = 1
	}
	return func(state game.State) (game.Action, bool) {
		res, ok := search.ChooseDepthWeighted(ctx, state, depth, w)
		if !ok && res.Action != (game.Action{}) {
			// Cancelled/incomplete: play the legal fallback so the game ends
			// legally and fast rather than stalling.
			return res.Action, true
		}
		return res.Action, ok
	}
}

// --- measurement ------------------------------------------------------------

func splitMetrics(samples []sample, split int, rows []prefRow, w, inc search.WeightVector) SplitMetrics {
	var m SplitMetrics
	m.Pairs = len(rows)
	var legalSum, candSum, disagree int
	var lrn, incb, posLrn, posIncb float64
	var ties, contested int
	for _, s := range samples {
		if s.split != split {
			continue
		}
		m.States++
		legalSum += s.legalCount
		candSum += s.candCount
		if s.teacherAction != s.shallowAction {
			disagree++
		}
		if s.forcedWin {
			m.ForcedWinStates++
		}
		if s.forcedLoss {
			m.ForcedLossStates++
		}
		for _, c := range s.cands {
			if c.terminal {
				m.TerminalCandidates++
			}
		}
		lh := agrees(s.state, s.teacherAction, w)
		ih := agrees(s.state, s.teacherAction, inc)
		lrn += b2f(lh)
		incb += b2f(ih)
		if s.positional {
			m.PositionalStates++
			posLrn += b2f(lh)
			posIncb += b2f(ih)
			if margin, tie, ok := topMargin(s.cands, s.mover, w); ok {
				contested++
				if tie {
					ties++
				}
				_ = margin
			}
		}
	}
	if m.States == 0 {
		return m
	}
	total := float64(m.States)
	m.MeanLegal = float64(legalSum) / total
	m.MeanCandidates = float64(candSum) / total
	if legalSum > 0 {
		m.CandidateCoverage = float64(candSum) / float64(legalSum)
	}
	m.TeacherDisagreement = float64(disagree) / total
	m.AgreementLearned = lrn / total
	m.AgreementIncumbent = incb / total
	if m.PositionalStates > 0 {
		m.PositionalLearned = posLrn / float64(m.PositionalStates)
		m.PositionalIncumbent = posIncb / float64(m.PositionalStates)
	}
	if contested > 0 {
		m.TieFractionLearned = float64(ties) / float64(contested)
	}
	return m
}

// topMargin returns the linear surrogate score gap between the best and
// second-best surviving non-terminal candidates and whether the top ties.
func topMargin(cands []candidate, mover game.Player, w search.WeightVector) (int64, bool, bool) {
	first, second := int64(0), int64(0)
	haveFirst, haveSecond := false, false
	for _, c := range cands {
		if c.terminal || !c.survives {
			continue
		}
		s := int64(search.UtilityFromFeatures(c.feats, c.active, mover, w))
		switch {
		case !haveFirst || s > first:
			second, haveSecond = first, haveFirst
			first, haveFirst = s, true
		case !haveSecond || s > second:
			second, haveSecond = s, true
		}
	}
	if !haveFirst {
		return 0, false, false
	}
	if !haveSecond {
		return 0, false, true
	}
	return first - second, first == second, true
}

func throughput(samples []sample) []BoardThroughput {
	perBoard := map[Board]*BoardThroughput{}
	var order []Board
	dur := map[Board]time.Duration{}
	for _, s := range samples {
		bt, ok := perBoard[s.board]
		if !ok {
			bt = &BoardThroughput{Board: s.board}
			perBoard[s.board] = bt
			order = append(order, s.board)
		}
		bt.States++
		bt.TeacherNodes += s.teacherNodes
		dur[s.board] += s.teacherWall
	}
	var out []BoardThroughput
	for _, b := range order {
		bt := perBoard[b]
		if secs := dur[b].Seconds(); secs > 0 {
			bt.NodesPerSec = float64(bt.TeacherNodes) / secs
		}
		out = append(out, *bt)
	}
	return out
}

// --- canonical evaluator-orbit key ------------------------------------------

// canonicalKey returns the orbit-invariant identity of a state under the exact
// symmetries the evaluator is byte-invariant to (vs-ai2.28.1): 180-degree
// rotation with a P1<->P2 / P3<->P4 swap, and — on square boards — transposition
// with a P3<->P4 swap, plus their composition. The min hash over the orbit is the
// canonical representative.
func canonicalKey(state game.State) string {
	snap := state.Snapshot()
	variants := []game.Snapshot{snap, rotate180Swap(snap)}
	if snap.Rows == snap.Cols {
		t := transposeSwap(snap)
		variants = append(variants, t, rotate180Swap(t))
	}
	best := ""
	for _, v := range variants {
		if h := snapHash(v); best == "" || h < best {
			best = h
		}
	}
	return best
}

func swap12and34(p game.Player) game.Player {
	switch p {
	case 1:
		return 2
	case 2:
		return 1
	case 3:
		return 4
	case 4:
		return 3
	}
	return p
}

func swap34(p game.Player) game.Player {
	switch p {
	case 3:
		return 4
	case 4:
		return 3
	}
	return p
}

func rotate180Swap(s game.Snapshot) game.Snapshot {
	out := blankLike(s, s.Rows, s.Cols)
	for r := 0; r < s.Rows; r++ {
		for c := 0; c < s.Cols; c++ {
			cell := s.Board[s.Rows-1-r][s.Cols-1-c]
			cell.Owner = swap12and34(cell.Owner)
			out.Board[r][c] = cell
		}
	}
	for i := range s.Bases {
		j := int(swap12and34(game.Player(i+1))) - 1
		out.Bases[j] = game.Pos{Row: s.Rows - 1 - s.Bases[i].Row, Col: s.Cols - 1 - s.Bases[i].Col}
		out.Active[j] = s.Active[i]
		out.NeutralUsed[j] = s.NeutralUsed[i]
	}
	out.Current = swap12and34(s.Current)
	out.Winner = swap12and34(s.Winner)
	return out
}

func transposeSwap(s game.Snapshot) game.Snapshot {
	out := blankLike(s, s.Cols, s.Rows)
	for r := 0; r < s.Rows; r++ {
		for c := 0; c < s.Cols; c++ {
			cell := s.Board[r][c]
			cell.Owner = swap34(cell.Owner)
			out.Board[c][r] = cell
		}
	}
	for i := range s.Bases {
		j := int(swap34(game.Player(i+1))) - 1
		out.Bases[j] = game.Pos{Row: s.Bases[i].Col, Col: s.Bases[i].Row}
		out.Active[j] = s.Active[i]
		out.NeutralUsed[j] = s.NeutralUsed[i]
	}
	out.Current = swap34(s.Current)
	out.Winner = swap34(s.Winner)
	return out
}

func blankLike(s game.Snapshot, rows, cols int) game.Snapshot {
	board := make([][]game.Cell, rows)
	for r := range board {
		board[r] = make([]game.Cell, cols)
	}
	return game.Snapshot{
		Rows: rows, Cols: cols, Board: board,
		Bases:       make([]game.Pos, len(s.Bases)),
		Active:      make([]bool, len(s.Active)),
		NeutralUsed: make([]bool, len(s.NeutralUsed)),
		MovesLeft:   s.MovesLeft, GameOver: s.GameOver,
	}
}

// --- hashing, provenance, helpers -------------------------------------------

func snapHash(s game.Snapshot) string {
	encoded, _ := json.Marshal(s)
	sum := sha256.Sum256(encoded)
	return hex.EncodeToString(sum[:])
}

// buildStamp returns a source-identity string (go version + VCS revision + dirty
// flag) and whether that identity is trustworthy for publishing weights: a run
// is trustworthy only when a VCS revision is stamped and the tree is clean.
func buildStamp() (string, bool) {
	info, ok := debug.ReadBuildInfo()
	if !ok {
		return "unknown", false
	}
	rev, modified := "", ""
	for _, s := range info.Settings {
		switch s.Key {
		case "vcs.revision":
			rev = s.Value
		case "vcs.modified":
			modified = s.Value
		}
	}
	dirty := modified == "true"
	stamp := info.GoVersion + " rev=" + rev
	if rev == "" {
		stamp = info.GoVersion + " rev=unavailable"
	}
	if dirty {
		stamp += " (dirty)"
	}
	return stamp, rev != "" && !dirty
}

// provenance binds versioned checksums of the build, config, dataset, and the
// FULL teacher-evaluated candidate universe (each candidate's action, score,
// ordinal, and terminal outcome) plus the fitted weights. The pair dataset is a
// deterministic function of the candidate universe and config, so binding those
// fully determines it.
// configChecksum is the canonical identity of a fit config, used both to stamp
// provenance and to bind a frozen candidate to the exact config it was fit on.
func configChecksum(cfg Config) string {
	cfgJSON, _ := json.Marshal(cfg)
	return sha256Hex(cfgJSON)
}

func provenance(cfg Config, samples []sample, w search.WeightVector) Provenance {
	configSum := configChecksum(cfg)

	dataset := sha256.New()
	labels := sha256.New()
	var scratch [8]byte
	putInt := func(h interface{ Write([]byte) (int, error) }, v int64) {
		binary.BigEndian.PutUint64(scratch[:], uint64(v))
		h.Write(scratch[:])
	}
	putAction := func(h interface{ Write([]byte) (int, error) }, a game.Action) {
		putInt(h, int64(a.Kind))
		putInt(h, int64(a.Target.Row))
		putInt(h, int64(a.Target.Col))
		for _, n := range a.Neutrals {
			putInt(h, int64(n.Row))
			putInt(h, int64(n.Col))
		}
	}
	for _, s := range samples {
		dataset.Write([]byte(s.key))
		putInt(dataset, int64(s.split))
		putInt(dataset, int64(s.mover))
		putInt(labels, int64(s.split))
		putAction(labels, s.teacherAction)
		putAction(labels, s.shallowAction)
		putInt(labels, int64(s.legalCount))
		putInt(labels, int64(len(s.cands)))
		for _, c := range s.cands { // bind the full candidate universe
			putAction(labels, c.action)
			putInt(labels, int64(c.score))
			putInt(labels, int64(c.ordinal))
			putInt(labels, b2i(c.terminal))
			putInt(labels, b2i(c.winForMover))
		}
	}
	datasetSum := hex.EncodeToString(dataset.Sum(nil))
	labelSum := hex.EncodeToString(labels.Sum(nil))

	weightSum := weightChecksum(w)
	build, trustworthy := buildStamp()
	p := Provenance{Version: Version, Build: build, Config: configSum, Dataset: datasetSum, Labels: labelSum, Weights: weightSum, trustworthy: trustworthy}
	p.Checksum = combineProvenance(p)
	return p
}

// combineProvenance is the single definition of the combined checksum, over the
// version, build identity, and every component checksum. EvaluateTest recomputes
// it to verify a frozen candidate's provenance is internally consistent.
func combineProvenance(p Provenance) string {
	return sha256Hex([]byte(p.Version + "\x00" + p.Build + "\x00" + p.Config + "\x00" + p.Dataset + "\x00" + p.Labels + "\x00" + p.Weights))
}

func weightChecksum(w search.WeightVector) string {
	buf := make([]byte, 0, search.FeatureCount*8)
	var scratch [8]byte
	for i := 0; i < search.FeatureCount; i++ {
		binary.BigEndian.PutUint64(scratch[:], uint64(w[i]))
		buf = append(buf, scratch[:]...)
	}
	return sha256Hex(buf)
}

func sha256Hex(b []byte) string {
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:])
}

// currentBuildTrusted reports whether the running binary has a trustworthy source
// identity (clean tree, stamped revision). It is a function variable so a test
// can exercise the trusted-build path deterministically; production always uses
// buildStamp.
var currentBuildTrusted = func() bool {
	_, ok := buildStamp()
	return ok
}

func b2f(b bool) float64 {
	if b {
		return 1
	}
	return 0
}

func b2i(b bool) int64 {
	if b {
		return 1
	}
	return 0
}
