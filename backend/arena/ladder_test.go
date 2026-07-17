package arena

import (
	"fmt"
	"os"
	"strings"
	"testing"
)

// TestLadderReport is THE pre-merge strength report for eval/search PRs: the
// current production eval (deterministic node budget) vs the full opponent
// ladder over seeded openings, each rung measured with SPRT-style early
// stopping. Fixed rungs stop against 50%; the two hybrid qualification rungs
// stop against the actual 60% decision boundary, so the verdict comes from
// the sequential decision itself (CI clear of 60%) rather than a point
// estimate biased by optional stopping. A hybrid qualifies for a ladder slot
// only if it beats the current eval >40% (eval win% < 60); the frozen
// incumbent is measured against each hybrid too.
//
// It is a measurement, failing only on illegal/stalled/maxed games (enforced
// inside playSequentialOpenings).
//
// Reproduce (full report, ~40 openings = 80-game cap per rung):
//
//	VS_LADDER=1 go test ./arena -run TestLadderReport -v -timeout 240m
//
// Quick wiring check:
//
//	VS_LADDER=1 VS_LADDER_OPENINGS=4 go test ./arena -run TestLadderReport -v
func TestLadderReport(t *testing.T) {
	if os.Getenv("VS_LADDER") != "1" {
		t.Skip("set VS_LADDER=1 to run the slow 12x12 ladder report")
	}
	openings := envInt(t, "VS_LADDER_OPENINGS", 40)
	nodes := uint64(envInt(t, "VS_LADDER_NODES", 1000))
	eval := TelemetryNodeBudget(nodes, false)
	incumbent := TelemetryNodeBudget(nodes, true)

	rungs := []struct {
		name     string
		engine   TelemetryAgent // whose win-rate the row reports
		opponent TelemetryAgent
		hybrid   bool // qualification row: eval win% < 60 => hybrid qualifies
	}{
		{"eval vs Greedy", eval, Instrument(Greedy), false},
		{"eval vs Legacy", eval, Instrument(Legacy(1)), false},
		{"eval vs BaseAttacker", eval, Instrument(BaseAttacker), false},
		{"eval vs MobilityAttacker", eval, Instrument(MobilityAttacker), false},
		{"eval vs MobilityBaseAttacker", eval, Instrument(MobilityBaseAttacker), true},
		{"incumbent vs MobilityBaseAttacker", incumbent, Instrument(MobilityBaseAttacker), false},
		{"eval vs CutSeeker", eval, Instrument(CutSeeker), true},
		{"incumbent vs CutSeeker", incumbent, Instrument(CutSeeker), false},
		{"eval vs OwnerBot", eval, Instrument(OwnerBot), true},
		{"incumbent vs OwnerBot", incumbent, Instrument(OwnerBot), false},
		{"eval vs incumbent", eval, incumbent, false},
	}

	cap := 2 * openings
	var table strings.Builder
	fmt.Fprintf(&table, "ladder report (nodes=%d, openings cap=%d):\n", nodes, openings)
	fmt.Fprintf(&table, "%-35s | %10s | %6s | %-16s | %6s | %s\n",
		"opponent", "wins/games", "win%", "wilson95", "played", "qualifies")
	for _, rung := range rungs {
		threshold := 50.0
		if rung.hybrid {
			threshold = 60 // stop against the qualification boundary the verdict is read at
		}
		result := playSequentialOpenings(t, rung.name, openings, threshold, sequentialMinGames, rung.engine, rung.opponent)
		interval := Wilson95(result.Wins, result.Games)
		rate := 100 * float64(result.Wins) / float64(result.Games)
		qualifies := ""
		if rung.hybrid {
			qualifies = "no"
			if (result.Stopped && !result.Above) || (!result.Stopped && rate < 60) {
				qualifies = "yes"
			}
		}
		fmt.Fprintf(&table, "%-35s | %5d/%-4d | %5.1f%% | [%5.1f%%, %5.1f%%] | %3d/%-3d | %s\n",
			rung.name, result.Wins, result.Games, rate, interval.Low, interval.High, result.Games, cap, qualifies)
	}
	t.Log(table.String())
}
