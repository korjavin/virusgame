package search

import (
	"os"

	"virusgame/game"
	"virusgame/nnuefeat"
	"virusgame/search/nnueweights"
)

// vs-ai2.56 Stage 3: env-gated NNUE-lite inference wired into the leaf eval.
//
// CANARY-FIRST (bd memory vs-ai2-goodhart-scripted-gates): this is a candidate
// path only. The production default (VS_NNUE unset) is byte-identical to the
// frozen hand-tuned eval — nnueEnabled short-circuits to false and
// evaluateWithWorkspace never touches this code. The net ships only via a
// canary build the owner judges; gates are sanity floors, never promotion proof.
//
// The weights in search/nnueweights are a SMOKE PLACEHOLDER (300-position train,
// spearman ~0.45) proving the plumbing end to end. Strength is irrelevant until
// the full nnuegen run + train produces a real weights file (see
// docs/nnue-canary.md).

// nnueEnabled routes the leaf eval through the int8 net when VS_NNUE is set to a
// non-empty, non-"0" value. Read once at init so the hot eval path is a single
// bool test (and the off path stays byte-identical to origin-main).
var nnueEnabled = func() bool {
	v := os.Getenv("VS_NNUE")
	return v != "" && v != "0"
}()

// nnueEvaluate returns the net's score for player at a non-terminal leaf. The
// net predicts the mover's (CurrentPlayer's) deep-search score; for the
// 2-player zero-sum game the score for any other seat is its negation.
//
// ponytail: 2-player sign flip only. A real 4-player perspective mapping is a
// concern for when the net actually ships — the smoke net's strength is
// meaningless, so exact multi-seat perspective is not worth building now.
func nnueEvaluate(state game.State, player game.Player) int {
	// Residual mode (vs-ai2.56 v5): the net predicts deepScore - staticEval,
	// so play-time eval = frozen hand-tuned eval + learned correction. The
	// combined eval degrades gracefully toward hand-tuned when the net is
	// uncertain, and the net only has to learn what the formula misses.
	resid := int(nnueweights.Predict(nnuefeat.Input(state)))
	if player != state.CurrentPlayer() {
		resid = -resid
	}
	workspace := evalWorkspace{}
	// Call the classic eval directly — evaluateWithWorkspace would route back
	// here (VS_NNUE is set) and recurse.
	return evaluateAllWithWorkspace(state, &workspace)[player-1] + resid
}

// StaticEval exposes the frozen hand-tuned evaluation for offline tooling
// (residual-target computation in tools/nnue-train). Not used in play.
func StaticEval(state game.State, player game.Player) int {
	workspace := evalWorkspace{}
	return evaluateAllWithWorkspace(state, &workspace)[player-1]
}
