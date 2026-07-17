package arena

import (
	"os"
	"testing"

	"virusgame/search"
)

// TestStranglerLeverSweep measures each vs-ai2.35 search-side lever in
// isolation against the strangler. It plays the live-eval candidate under a
// deterministic 1000-node budget against Instrument(MobilityAttacker) over the
// balanced strangler openings, once per lever config: baseline (all-off), each
// lever alone, and all-on. Per config it logs wins/games, win-rate, and the
// Wilson 95% CI, so the operator can read which lever moves the strangler
// win-rate and set each default accordingly.
//
// It is a measurement, not a gate: env-gated and slow, it makes no pass/fail
// assertion beyond the illegal/stalled/maxed guard inside playBalancedOpenings.
// It mutates the package-global search levers via SetSearchLevers, so it
// restores the all-on default in a deferred cleanup.
//
// Reproduce (full sweep, ~40 openings = 80 games per config):
//
//	VS_LEVER_SWEEP=1 go test ./arena -run TestStranglerLeverSweep -v -timeout 120m
//
// Quick wiring check:
//
//	VS_LEVER_SWEEP=1 VS_STRANGLER_OPENINGS=4 go test ./arena -run TestStranglerLeverSweep -v
func TestStranglerLeverSweep(t *testing.T) {
	if os.Getenv("VS_LEVER_SWEEP") != "1" {
		t.Skip("set VS_LEVER_SWEEP=1 to run the slow strangler lever sweep")
	}
	defer search.SetSearchLevers(true, true, true) // restore shipped default

	openings := stranglerOpenings(t)
	const nodes = 1000
	candidate := TelemetryNodeBudget(nodes, false)
	strangler := Instrument(MobilityAttacker)

	configs := []struct {
		name                     string
		ordering, extend, safety bool
	}{
		{"baseline-all-off", false, false, false},
		{"lever1-opponent-ordering", true, false, false},
		{"lever2-threat-extend", false, true, false},
		{"lever3-root-safety", false, false, true},
		{"all-on", true, true, true},
	}

	for _, cfg := range configs {
		search.SetSearchLevers(cfg.ordering, cfg.extend, cfg.safety)
		report := playBalancedOpenings(t, cfg.name, openings, candidate, strangler)
		interval := Wilson95(report.Wins, report.Games)
		rate := 100 * float64(report.Wins) / float64(report.Games)
		t.Logf("%s (nodes=%d): %d/%d=%.1f%% wilson95=[%.1f%%, %.1f%%]",
			cfg.name, nodes, report.Wins, report.Games, rate, interval.Low, interval.High)
	}
}
