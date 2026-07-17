package arena

import (
	"testing"

	"virusgame/game"
)

// firstLegalAgent is a deliberately weak but goroutine-safe, order-independent
// baseline: it always plays its first legal action. Being a pure function of
// the position (no carried RNG state) it is safe to share across the worker
// pool, so it is the honest stand-in for a "random" weak opponent in the
// parallel-vs-serial determinism check (the real Random agent carries mutable
// state and is not goroutine-safe — see the PlaySequentialOpenings doc).
func firstLegalAgent(state game.State) (game.Action, DecisionTelemetry, bool) {
	actions := state.LegalActions()
	if len(actions) == 0 {
		return game.Action{}, DecisionTelemetry{}, false
	}
	return actions[0], DecisionTelemetry{}, true
}

// TestPlaySequentialOpeningsParallelMatchesSerial is the determinism guard for
// the parallel ladder harness: for order-independent agents the worker-pool
// path must fold to the exact same {Games,Wins,Stopped,Above} verdict as the
// serial path, because results are accumulated strictly in permutation order.
//
// The lopsided rung (Greedy vs first-legal) exercises early stopping — Greedy
// dominates, so the CI clears 50% before the cap. The mirror rung (Greedy vs
// Greedy) is fully deterministic and runs to the cap. Both agents are pure, so
// the run is also race-free under -race.
func TestPlaySequentialOpeningsParallelMatchesSerial(t *testing.T) {
	const (
		maxOpenings = 6
		minGames    = 4
	)
	cases := []struct {
		name      string
		a, b      TelemetryAgent
		wantStop  bool // lopsided must stop early above threshold
		wantAtCap bool // mirror must run to the full cap
	}{
		{"lopsided greedy vs first-legal", Instrument(Greedy), firstLegalAgent, true, false},
		{"mirror greedy vs greedy", Instrument(Greedy), Instrument(Greedy), false, true},
	}
	for _, tc := range cases {
		serial, err := PlaySequentialOpenings(12, 12, maxOpenings, 50, minGames, tc.a, tc.b, 1)
		if err != nil {
			t.Fatalf("%s serial: %v", tc.name, err)
		}
		parallel, err := PlaySequentialOpenings(12, 12, maxOpenings, 50, minGames, tc.a, tc.b, 4)
		if err != nil {
			t.Fatalf("%s parallel: %v", tc.name, err)
		}
		if serial.Games != parallel.Games || serial.Wins != parallel.Wins ||
			serial.Stopped != parallel.Stopped || serial.Above != parallel.Above {
			t.Fatalf("%s: workers=1 and workers=4 disagree: serial={games:%d wins:%d stopped:%v above:%v} parallel={games:%d wins:%d stopped:%v above:%v}",
				tc.name, serial.Games, serial.Wins, serial.Stopped, serial.Above,
				parallel.Games, parallel.Wins, parallel.Stopped, parallel.Above)
		}
		if tc.wantStop && !(serial.Stopped && serial.Above) {
			t.Fatalf("%s: expected early stop above threshold, got %+v", tc.name, serial)
		}
		if tc.wantAtCap && (serial.Stopped || serial.Games != 2*maxOpenings) {
			t.Fatalf("%s: expected run to cap %d without stopping, got games=%d stopped=%v", tc.name, 2*maxOpenings, serial.Games, serial.Stopped)
		}
	}
}
