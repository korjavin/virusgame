package arena

import (
	"os"
	"strconv"
	"testing"

	"virusgame/game"
)

// Sweep harness (throwaway for tuning): candidate eval vs MobilityAttacker.
// VS_STRANGLER_OPENINGS controls n. VS_STRANGLER_NEWONLY=1 skips the incumbent
// side (constant baseline) to halve sweep cost.
func TestVsStrangler(t *testing.T) {
	if os.Getenv("VS_STRANGLER") != "1" {
		t.Skip("set VS_STRANGLER=1")
	}
	openings := 40
	if v := os.Getenv("VS_STRANGLER_OPENINGS"); v != "" {
		openings, _ = strconv.Atoi(v)
	}
	newOnly := os.Getenv("VS_STRANGLER_NEWONLY") == "1"
	nodes := uint64(1000)
	opponent := MobilityAttacker
	if os.Getenv("VS_STRANGLER_OPP") == "base" {
		opponent = BaseAttacker
	}
	strangler := Instrument(opponent)
	newEval := TelemetryNodeBudget(nodes, false)
	inc := TelemetryNodeBudget(nodes, true)
	var newRep, incRep Report
	for i := 0; i < openings; i++ {
		snap := randomLegalOpening(t, uint64(i)+1)
		for seat := 0; seat < 2; seat++ {
			a := []TelemetryAgent{newEval, strangler}
			if seat == 1 {
				a[0], a[1] = a[1], a[0]
			}
			r1, err := Play(Match{Rows: 12, Cols: 12, Initial: &snap, TelemetryAgents: a})
			if err != nil {
				t.Fatal(err)
			}
			newRep.Add(r1, game.Player(seat+1))
			if !newOnly {
				b := []TelemetryAgent{inc, strangler}
				if seat == 1 {
					b[0], b[1] = b[1], b[0]
				}
				r2, err := Play(Match{Rows: 12, Cols: 12, Initial: &snap, TelemetryAgents: b})
				if err != nil {
					t.Fatal(err)
				}
				incRep.Add(r2, game.Player(seat+1))
			}
		}
	}
	ni := Wilson95(newRep.Wins, newRep.Games)
	t.Logf("NEW frag=%s mobw=%s danger=%s space=%s vs strangler n=%d: %d/%d=%.1f%% w95[%.0f,%.0f]",
		os.Getenv("VS_AI2_33_FRAG"), os.Getenv("VS_AI2_32_MOBW"), os.Getenv("VS_AI2_32_DANGER"), os.Getenv("VS_AI2_34_SPACE"),
		newRep.Games, newRep.Wins, newRep.Games, 100*float64(newRep.Wins)/float64(newRep.Games), ni.Low, ni.High)
	if !newOnly {
		ii := Wilson95(incRep.Wins, incRep.Games)
		t.Logf("INCUMBENT vs strangler n=%d: %d/%d=%.1f%% w95[%.0f,%.0f]",
			incRep.Games, incRep.Wins, incRep.Games, 100*float64(incRep.Wins)/float64(incRep.Games), ii.Low, ii.High)
	}
}
