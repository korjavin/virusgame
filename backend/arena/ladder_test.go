package arena

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"testing"
)

// TestLadderReport is THE pre-merge strength report for eval/search PRs: the
// current production eval (deterministic node budget) vs the full opponent
// ladder over seeded openings, each rung measured with SPRT-style early
// stopping (threshold 50%). For the two hybrid sparring opponents the frozen
// incumbent is measured too, and a hybrid qualifies for a ladder slot only if
// it beats the current eval >40% (eval win% < 60).
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
	openings := 40
	if v := os.Getenv("VS_LADDER_OPENINGS"); v != "" {
		parsed, err := strconv.Atoi(v)
		if err != nil || parsed < 1 {
			t.Fatalf("VS_LADDER_OPENINGS=%q must be a positive integer", v)
		}
		openings = parsed
	}
	nodes := uint64(1000)
	if v := os.Getenv("VS_LADDER_NODES"); v != "" {
		parsed, err := strconv.ParseUint(v, 10, 64)
		if err != nil || parsed < 1 {
			t.Fatalf("VS_LADDER_NODES=%q must be a positive integer", v)
		}
		nodes = parsed
	}
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
		{"eval vs incumbent", eval, incumbent, false},
	}

	cap := 2 * openings
	var table strings.Builder
	fmt.Fprintf(&table, "ladder report (nodes=%d, openings cap=%d):\n", nodes, openings)
	fmt.Fprintf(&table, "%-35s | %10s | %6s | %-16s | %6s | %s\n",
		"opponent", "wins/games", "win%", "wilson95", "played", "qualifies")
	for _, rung := range rungs {
		result := playSequentialOpenings(t, rung.name, openings, 50, sequentialMinGames, rung.engine, rung.opponent)
		interval := Wilson95(result.Wins, result.Games)
		rate := 100 * float64(result.Wins) / float64(result.Games)
		qualifies := ""
		if rung.hybrid {
			qualifies = "no"
			if rate < 60 {
				qualifies = "yes"
			}
		}
		fmt.Fprintf(&table, "%-35s | %5d/%-4d | %5.1f%% | [%5.1f%%, %5.1f%%] | %3d/%-3d | %s\n",
			rung.name, result.Wins, result.Games, rate, interval.Low, interval.High, result.Games, cap, qualifies)
	}
	t.Log(table.String())
}
