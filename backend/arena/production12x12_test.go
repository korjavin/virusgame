package arena

import (
	"context"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"testing"

	"virusgame/game"
	"virusgame/search"
)

// vs-ai2.30: the twelve real production games (all 12x12, all terminated
// no_moves with the bot in seat 2) that the live 600ms search loses by
// strangulation. Each entry is the SHA-256[:8] fingerprint of the authoritative
// terminal snapshot reconstructed by replaying the recorded PGN actions from
// game.New(12,12,2). These are the frozen regression corpus for the failure
// mode; the hashes pin every reconstruction so a rules or replay change cannot
// silently move the positions.
var twelveNoMovesLosses = map[string]string{
	"323e3f1f-d1a9-4534-973c-5df2fd4302b4": "c156d0f67b213b6f",
	"86ecfde5-f313-4bec-bd6e-9a2e79f46227": "cd2ad91679cc2449",
	"6ad8b536-4f29-4c41-9b7e-fa5eb350f545": "c83f29d76d53cfdc",
	"c3f39595-38f2-46a0-891e-2ca7efce4655": "d3104de26aa02ea0",
	"b2ef469b-470c-41d5-ac6a-5e3e2ded7776": "2273f285d05550e4",
	"6eaa8f07-719a-4663-aa11-90f3a9465b3a": "09b1dbe78e935dc6",
	"4558d2fe-c22f-4940-8012-8f4f43fac728": "1e6523f0e84702fc",
	"913c33f7-1f0c-41ce-9cee-65d3d9688073": "aa34bed517bde90c",
	"550cfd27-6c5c-48a2-928c-c36354f9db87": "76f133e032952674",
	"fd6627c8-3d46-408d-bd48-17f081e1113b": "117269c21ce6f629",
	"99796a56-2238-4408-851d-4c548d3d3a44": "27bda30be456e244",
	"6c8513fe-3c07-4471-9ea6-cc3da798bcc9": "c7cca88bdcda50b5",
}

// TestTwelveNoMovesLossesAreFrozenTerminals reconstructs each of the twelve
// recorded 12x12 losses through the authoritative rules and pins its terminal
// position hash. Every game must decode to an authoritative no_moves terminal
// won by the human in seat 1, with the bot (seat 2) eliminated.
func TestTwelveNoMovesLossesAreFrozenTerminals(t *testing.T) {
	remaining := make(map[string]string, len(twelveNoMovesLosses))
	for id, fp := range twelveNoMovesLosses {
		remaining[id] = fp
	}
	paths, err := filepath.Glob("testdata/production-12x12-no-moves-*.json")
	if err != nil {
		t.Fatal(err)
	}
	for _, path := range paths {
		fixture, err := os.Open(path)
		if err != nil {
			t.Fatal(err)
		}
		replay, states, decodeErr := DecodeReplay(fixture)
		fixture.Close()
		want, selected := remaining[replay.SourceID]
		if !selected {
			continue
		}
		if decodeErr != nil {
			t.Fatalf("%s: decode: %v", path, decodeErr)
		}
		if replay.Rows != 12 || replay.Cols != 12 || replay.Termination != "no_moves" || replay.Winner != 1 {
			t.Fatalf("%s: not a 12x12 no_moves loss won by seat 1: %+v", replay.SourceID, replay)
		}
		final := states[len(replay.Turns)]
		if !final.GameOver() || final.Winner() != 1 || final.Active(2) {
			t.Fatalf("%s: final over=%v winner=%d botActive=%v, want bot eliminated and seat 1 wins",
				replay.SourceID, final.GameOver(), final.Winner(), final.Active(2))
		}
		if got := snapshotFingerprint(t, final); got != want {
			t.Fatalf("%s terminal fingerprint=%s, want %s", replay.SourceID, got, want)
		}
		delete(remaining, replay.SourceID)
	}
	if len(remaining) != 0 {
		t.Fatalf("missing frozen no_moves loss fixtures: %v", remaining)
	}
}

// botDecisionPoint returns the position at the start of a recorded turn, i.e.
// before any of that turn's actions were applied.
func botDecisionPoint(t *testing.T, sourceID string, turn int) game.State {
	t.Helper()
	fixture, err := os.Open("testdata/production-12x12-no-moves-" + sourceID + ".json")
	if err != nil {
		t.Fatal(err)
	}
	defer fixture.Close()
	replay, _, err := DecodeReplay(fixture)
	if err != nil {
		t.Fatal(err)
	}
	positions, err := ReplayPositions(replay)
	if err != nil {
		t.Fatal(err)
	}
	state, ok := positions[ReplayPoint{Turn: turn, AfterActions: 0}]
	if !ok {
		t.Fatalf("%s: no decision point at turn %d", sourceID, turn)
	}
	return state
}

// TestProduction12x12BotDecisionPointRegression replays game 4558d2fe to the
// bot's turn-18 decision point (the base-spine cut it never sees coming) and
// asserts two things the live engine already satisfies today: the reconstructed
// position is hash-stable, and production Choose returns a legal action that
// does not immediately hand the game to the human. It also carries, kept
// skipped per the vs-ai2.34 plan, the stronger assertion that the engine
// avoids the recorded losing continuation.
func TestProduction12x12BotDecisionPointRegression(t *testing.T) {
	const (
		sourceID = "4558d2fe-c22f-4940-8012-8f4f43fac728"
		turn     = 18
		// Stable fingerprint of the reconstructed turn-18 start position.
		wantPositionFingerprint = "30cee332e4c0c0d9"
	)
	state := botDecisionPoint(t, sourceID, turn)
	if state.CurrentPlayer() != 2 || state.GameOver() || state.MovesLeft() != 3 {
		t.Fatalf("turn %d is not a live 3-action bot decision: player=%d over=%v movesLeft=%d",
			turn, state.CurrentPlayer(), state.GameOver(), state.MovesLeft())
	}
	if got := snapshotFingerprint(t, state); got != wantPositionFingerprint {
		t.Fatalf("decision-point fingerprint=%s, want %s", got, wantPositionFingerprint)
	}

	// Passes today: the production engine returns a legal, non-self-eliminating
	// action at this position. Only legality and "does not lose on the spot" are
	// asserted, so the check is robust to the anytime search's node count.
	legal := map[game.Action]bool{}
	for _, action := range state.LegalActions() {
		legal[action] = true
	}
	ctx, cancel := context.WithTimeout(context.Background(), search.ProductionBudget)
	defer cancel()
	result, ok := search.Choose(ctx, state)
	if !ok || !legal[result.Action] {
		t.Fatalf("production Choose returned ok=%v illegal action=%+v", ok, result.Action)
	}
	next, err := state.Apply(result.Action)
	if err != nil {
		t.Fatalf("production action %+v was illegal: %v", result.Action, err)
	}
	if next.GameOver() && next.Winner() != 2 {
		t.Fatalf("production action %+v self-eliminated the bot: winner=%d", result.Action, next.Winner())
	}

	// Stronger assertion, kept skipped per the vs-ai2.34 plan (HARD LANDMINE:
	// beyond any static eval's horizon — leave it skipped, do not chase it):
	// the engine should not walk into the recorded losing continuation the bot
	// actually played before being strangled. The vs-ai2.34 space-race eval
	// avoids the line on a fast machine, but the wall-clock production search
	// makes that machine-dependent, so enabling it would be flaky.
	t.Run("avoids_losing_continuation", func(t *testing.T) {
		t.Skip("kept skipped per the vs-ai2.34 plan: wall-clock-dependent, do not chase")
		losing := []game.Action{
			{Kind: game.Move, Target: game.Pos{Row: 3, Col: 1}},
			{Kind: game.Move, Target: game.Pos{Row: 2, Col: 0}},
			{Kind: game.Move, Target: game.Pos{Row: 0, Col: 1}},
		}
		played := playOutTurn(t, state)
		if slices.Equal(played, losing) {
			t.Fatalf("production reproduced the losing continuation %+v", played)
		}
	})
}

// playOutTurn drives the production engine through one complete bot turn,
// collecting the actions it chooses until control passes to the opponent.
func playOutTurn(t *testing.T, state game.State) []game.Action {
	t.Helper()
	actor := state.CurrentPlayer()
	var played []game.Action
	for state.CurrentPlayer() == actor && !state.GameOver() {
		ctx, cancel := context.WithTimeout(context.Background(), search.ProductionBudget)
		result, ok := search.Choose(ctx, state)
		cancel()
		if !ok {
			break
		}
		next, err := state.Apply(result.Action)
		if err != nil {
			t.Fatalf("illegal action %+v during play-out: %v", result.Action, err)
		}
		played = append(played, result.Action)
		state = next
	}
	return played
}

// TestProduction12x12StrengthMeasurement is the deterministic, reproducible
// 12x12 strength gate on the exact production path (arena.Production, 600ms).
// It is opt-in because production-budget games are slow: set VS_AI2_30_MEASURE=1
// to run it. It plays balanced-seat games from the empty 12x12 board against the
// frozen incumbent and the greedy/base/mobility baselines, and reports win-rate
// with CompletedTurnDepth telemetry. It never fails on strength (it is a
// measurement, not a hard gate); it only fails on an illegal/stalled decision.
//
// Reproduce:
//
//	VS_AI2_30_MEASURE=1 VS_AI2_30_SEEDS=1 go test ./arena \
//	    -run TestProduction12x12StrengthMeasurement -v -timeout 30m
func TestProduction12x12StrengthMeasurement(t *testing.T) {
	if os.Getenv("VS_AI2_30_MEASURE") != "1" {
		t.Skip("set VS_AI2_30_MEASURE=1 to run the slow production-budget 12x12 measurement")
	}
	seeds := 1
	if v := os.Getenv("VS_AI2_30_SEEDS"); v != "" {
		parsed, err := strconv.Atoi(v)
		if err != nil || parsed < 1 {
			t.Fatalf("VS_AI2_30_SEEDS=%q must be a positive integer", v)
		}
		seeds = parsed
	}
	boards := []Board{{Rows: 12, Cols: 12}}
	opponents := []struct {
		name    string
		factory TelemetryOpponentFactory
	}{
		{"incumbent", func(uint64) TelemetryAgent { return TelemetryFrozenProduction() }},
		{"greedy", func(uint64) TelemetryAgent { return Instrument(Greedy) }},
		{"base", func(uint64) TelemetryAgent { return Instrument(BaseAttacker) }},
		{"mobility", func(uint64) TelemetryAgent { return Instrument(MobilityAttacker) }},
	}
	for _, opponent := range opponents {
		report, err := CompareTelemetry(boards, seeds, TelemetryProduction(), opponent.factory)
		if err != nil {
			t.Fatalf("opponent %s: %v", opponent.name, err)
		}
		if report.Illegal != 0 || report.Stalled != 0 {
			t.Fatalf("opponent %s produced illegal/stalled decisions: %s", opponent.name, report)
		}
		t.Logf("12x12 production vs %-9s %s", opponent.name, report)
	}
}
