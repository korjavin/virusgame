package arena

import (
	"testing"
)

// sequentialOrderSeed aliases the exported harness's ladderOrderSeed so the
// multiplayer ladder test (which builds its own permutation) shares the one
// source of truth.
const sequentialOrderSeed = ladderOrderSeed

// sequentialMinGames is the shared minimum sample the gates require before
// wilsonDecision may stop a matchup early.
const sequentialMinGames = 8

// wilsonDecision delegates to the exported WilsonDecision (single source of
// truth for the SPRT stop rule).
func wilsonDecision(wins, games int, thresholdPct float64, minGames int) (stop, above bool) {
	return WilsonDecision(wins, games, thresholdPct, minGames)
}

// playSequentialOpenings is the t-wrapper around the exported
// PlaySequentialOpenings: it runs the serial (workers=1) 12x12 ladder and
// t.Fatalf's on a returned error, preserving the historical gate behavior.
func playSequentialOpenings(t *testing.T, label string, maxOpenings int, thresholdPct float64, minGames int, a, b TelemetryAgent) SequentialResult {
	t.Helper()
	result, err := PlaySequentialOpenings(12, 12, maxOpenings, thresholdPct, minGames, a, b, 1)
	if err != nil {
		t.Fatalf("%s: %v", label, err)
	}
	return result
}

func TestSequentialEarlyStopDeterministic(t *testing.T) {
	const (
		maxOpenings = 6
		cap         = 2 * maxOpenings
		minGames    = 4
	)
	run := func() SequentialResult {
		// fresh Random each run: its closure mutates its seed across calls
		return playSequentialOpenings(t, "greedy vs random", maxOpenings, 50, minGames, Instrument(Greedy), Instrument(Random(7)))
	}
	first, second := run(), run()
	if first.Games != second.Games || first.Wins != second.Wins || first.Stopped != second.Stopped || first.Above != second.Above {
		t.Fatalf("sequential run not deterministic: first={games:%d wins:%d stopped:%v above:%v} second={games:%d wins:%d stopped:%v above:%v}",
			first.Games, first.Wins, first.Stopped, first.Above, second.Games, second.Wins, second.Stopped, second.Above)
	}
	if !first.Stopped || !first.Above {
		t.Fatalf("lopsided matchup should stop early above threshold: %+v", first.Report)
	}
	if first.Games >= cap {
		t.Fatalf("lopsided matchup played %d games, expected fewer than cap %d", first.Games, cap)
	}

	// mirror matchup: identical deterministic agent both sides => every pair
	// splits 1-1, the CI straddles 50%, and the run must hit the cap
	coin := playSequentialOpenings(t, "greedy mirror", maxOpenings, 50, minGames, Instrument(Greedy), Instrument(Greedy))
	if coin.Stopped || coin.Games != cap {
		t.Fatalf("coin-flip matchup should run to cap %d without stopping: games=%d stopped=%v", cap, coin.Games, coin.Stopped)
	}
	if coin.Wins*2 != coin.Games {
		t.Fatalf("mirror matchup should split every pair: wins=%d games=%d", coin.Wins, coin.Games)
	}
}
