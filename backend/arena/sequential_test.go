package arena

import (
	"math/rand"
	"testing"

	"virusgame/game"
)

// sequentialOrderSeed fixes the opening permutation so a sequential run is a
// pure function of (code, maxOpenings): same inputs => same game sequence,
// same stop point, same verdict.
const sequentialOrderSeed = 20260716

// wilsonDecision is the SPRT-style stop rule: stop once games >= minGames and
// the Wilson 95% interval lies entirely on one side of thresholdPct. above
// reports which side. minGames guards against a premature stop on the first
// pair.
func wilsonDecision(wins, games int, thresholdPct float64, minGames int) (stop, above bool) {
	if games < minGames {
		return false, false
	}
	interval := Wilson95(wins, games)
	switch {
	case interval.Low > thresholdPct:
		return true, true
	case interval.High < thresholdPct:
		return true, false
	}
	return false, false
}

type sequentialResult struct {
	Report
	ThresholdPct float64
	Stopped      bool
	Above        bool
}

// playSequentialOpenings is playBalancedOpenings with early stopping: it plays
// both seats of each seeded opening in a fixed-seed permutation, updating a's
// running Wilson 95% CI after each pair, and stops as soon as wilsonDecision
// resolves the matchup against thresholdPct — else runs to the maxOpenings
// cap. Opening snapshots are the same randomLegalOpening(t, idx+1) positions
// playBalancedOpenings uses. Fails on any illegal/stalled/maxed game.
func playSequentialOpenings(t *testing.T, label string, maxOpenings int, thresholdPct float64, minGames int, a, b TelemetryAgent) sequentialResult {
	t.Helper()
	result := sequentialResult{ThresholdPct: thresholdPct}
	order := rand.New(rand.NewSource(sequentialOrderSeed)).Perm(maxOpenings)
	for _, idx := range order {
		snapshot := randomLegalOpening(t, uint64(idx)+1)
		for seat := 0; seat < 2; seat++ {
			agents := []TelemetryAgent{a, b}
			if seat == 1 {
				agents[0], agents[1] = agents[1], agents[0]
			}
			played, err := Play(Match{Rows: 12, Cols: 12, Initial: &snapshot, TelemetryAgents: agents})
			if err != nil {
				t.Fatalf("%s opening %d seat %d: %v", label, idx, seat, err)
			}
			if played.Illegal != 0 || played.Stalled || played.Maxed {
				t.Fatalf("%s opening %d seat %d produced illegal/stalled/maxed game: %+v", label, idx, seat, played)
			}
			result.Add(played, game.Player(seat+1))
		}
		if stop, above := wilsonDecision(result.Wins, result.Games, thresholdPct, minGames); stop {
			result.Stopped, result.Above = true, above
			break
		}
	}
	return result
}

func TestSequentialEarlyStopDeterministic(t *testing.T) {
	const (
		maxOpenings = 6
		cap         = 2 * maxOpenings
		minGames    = 4
	)
	run := func() sequentialResult {
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
