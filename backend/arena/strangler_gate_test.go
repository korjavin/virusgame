package arena

import (
	"os"
	"testing"
)

// TestVsStrangler is the vs-ai2.34 primary anti-strangulation gate.
//
// A "strangler" is a trivial agent that ignores material and simply maximizes
// its own mobility (MobilityAttacker) or presses toward the opponent's base
// (BaseAttacker), steering games into no_moves strangulation losses. All 12
// recent real 12x12 production losses ended exactly that way, so the tuning
// objective is win-rate VS A STRANGLER, not vs the incumbent: an eval can
// beat the incumbent head-to-head while still losing to strangulation, and
// head-to-head self-play cannot expose that failure mode.
//
// The gate plays the candidate (live eval) and the byte-frozen incumbent
// against MobilityAttacker and BaseAttacker under a deterministic 1000-node
// search budget (no wall clock), balanced seats, and logs wins/games with
// Wilson 95% CIs per (engine, opponent) pair. The MobilityAttacker pairs feed
// the hard regression floor, so both engines play the FULL opening set from
// the SAME seeded openings — a paired sample; independently early-stopped
// point estimates (as few as 8 games each) would be far too noisy to compare
// across engines. The log-only BaseAttacker pairs run with SPRT-style early
// stopping (threshold 50%): each ends as soon as its Wilson CI clears 50%,
// else at the opening cap. It is a measurement, failing only on
// illegal/stalled/maxed games — plus one regression floor: the candidate's
// MobilityAttacker win-rate must exceed the incumbent's (measured vs-ai2.34:
// candidate ~62-70%, incumbent ~17-25%).
//
// Reproduce (full gate, ~40 openings = 80 games per pair):
//
//	VS_STRANGLER=1 go test ./arena -run TestVsStrangler -v -timeout 120m
//
// Quick wiring check:
//
//	VS_STRANGLER=1 VS_STRANGLER_OPENINGS=4 go test ./arena -run TestVsStrangler -v
func TestVsStrangler(t *testing.T) {
	if os.Getenv("VS_STRANGLER") != "1" {
		t.Skip("set VS_STRANGLER=1 to run the slow 12x12 strangler gate")
	}
	openings := stranglerOpenings(t)
	const nodes = 1000
	engines := []struct {
		name  string
		agent TelemetryAgent
	}{
		{"candidate", TelemetryNodeBudget(nodes, false)},
		{"incumbent", TelemetryNodeBudget(nodes, true)},
	}
	opponents := []struct {
		name  string
		agent TelemetryAgent
		floor bool // floor pairs play the full paired sample; log-only pairs early-stop
	}{
		{"MobilityAttacker", Instrument(MobilityAttacker), true},
		{"BaseAttacker", Instrument(BaseAttacker), false},
	}

	rates := map[string]float64{}
	for _, engine := range engines {
		for _, opponent := range opponents {
			label := engine.name + " vs " + opponent.name
			var report Report
			if opponent.floor {
				report = playBalancedOpenings(t, label, openings, engine.agent, opponent.agent)
			} else {
				report = playSequentialOpenings(t, label, openings, 50, sequentialMinGames, engine.agent, opponent.agent).Report
			}
			interval := Wilson95(report.Wins, report.Games)
			rate := 100 * float64(report.Wins) / float64(report.Games)
			rates[engine.name+"/"+opponent.name] = rate
			t.Logf("%s vs %s (nodes=%d): %d/%d=%.1f%% wilson95=[%.1f%%, %.1f%%] games-played=%d/%d",
				engine.name, opponent.name, nodes, report.Wins, report.Games, rate, interval.Low, interval.High, report.Games, 2*openings)
		}
	}
	if rates["candidate/MobilityAttacker"] <= rates["incumbent/MobilityAttacker"] {
		t.Fatalf("regression: candidate MobilityAttacker win-rate %.1f%% <= incumbent %.1f%%",
			rates["candidate/MobilityAttacker"], rates["incumbent/MobilityAttacker"])
	}
}
