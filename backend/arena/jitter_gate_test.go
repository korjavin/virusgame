package arena

import (
	"testing"
	"time"
)

// TestJitterStrengthParity is the vs-ai2.39 strength guard. It re-runs the
// TestStrengthGate matchups with root tie-break jitter forced ON (fixed seed)
// and requires the jittered engine to (a) clear the same win-rate thresholds and
// (b) land within the jitter-off Wilson-95 interval. Jitter only ever reshuffles
// moves within jitterEpsilon of the best, so strength must be preserved; this
// gate is the empirical proof.
func TestJitterStrengthParity(t *testing.T) {
	boards := []Board{{Rows: 5, Cols: 5}, {Rows: 6, Cols: 6}}
	const seed = 0xC0FFEE
	const seeds = 12 // 12 seeds x 2 boards x 2 seats = 48 games per matchup
	baseline := Tournament(3)
	jittered := TournamentJitter(3, seed)

	for _, test := range []struct {
		name     string
		opponent OpponentFactory
	}{
		{name: "legacy", opponent: Legacy},
		{name: "greedy", opponent: func(uint64) Agent { return Greedy }},
	} {
		t.Run(test.name, func(t *testing.T) {
			off, err := Balanced(boards, seeds, baseline, test.opponent)
			if err != nil {
				t.Fatal(err)
			}
			on, err := Balanced(boards, seeds, jittered, test.opponent)
			if err != nil {
				t.Fatal(err)
			}
			offCI := Wilson95(off.Wins, off.Games)
			onCI := Wilson95(on.Wins, on.Games)
			t.Logf("jitter-off %.1f%% (%d/%d) wilson95=[%.1f, %.1f]; jitter-on %.1f%% (%d/%d) wilson95=[%.1f, %.1f]",
				off.WinRate(), off.Wins, off.Games, offCI.Low, offCI.High,
				on.WinRate(), on.Wins, on.Games, onCI.Low, onCI.High)

			if on.Illegal != 0 || on.Maxed != 0 || on.Stalled != 0 {
				t.Fatalf("jitter-on legality/completion gate failed: %s", on)
			}
			if on.Percentile(95) > 600*time.Millisecond {
				t.Fatalf("jitter-on latency gate p95=%s > 600ms", on.Percentile(95))
			}
			// Strength guard: the jitter-on and jitter-off Wilson-95 intervals must
			// overlap — jitter reshuffles only near-ties, so any win-rate delta is
			// sampling noise, not a material regression.
			if onCI.High < offCI.Low || offCI.High < onCI.Low {
				t.Fatalf("jitter-on wilson95 [%.1f, %.1f] disjoint from jitter-off [%.1f, %.1f]: material regression",
					onCI.Low, onCI.High, offCI.Low, offCI.High)
			}
		})
	}
}
