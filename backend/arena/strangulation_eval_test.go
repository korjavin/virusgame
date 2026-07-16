package arena

import (
	"math/rand"
	"os"
	"strconv"
	"testing"

	"virusgame/game"
)

// vs-ai2.33: opt-in, balanced-seat, seeded-opening head-to-head that isolates
// the EVALUATOR difference between the live engine (arena.TelemetryProduction,
// new anti-strangulation eval) and the frozen incumbent (search/incumbent, the
// byte-frozen baseline). The existing 2-game production gate is too noisy to
// tune an eval term against; this plays N distinct random-but-legal 12x12
// openings and, for each, runs BOTH seats so the opening bias cancels and the
// aggregate win-rate reflects only the engine (evaluator) difference.
//
// It is a measurement, not a hard strength gate: it only fails on an
// illegal/stalled decision. Production-budget games are ~20-30s each, so it is
// gated behind VS_AI2_32_MEASURE=1.
//
// Reproduce (tuning, 16 games):
//
//	VS_AI2_32_MEASURE=1 VS_AI2_32_OPENINGS=8 go test ./arena \
//	    -run TestStrangulationEvalHeadToHead -v -timeout 60m
//
// Reproduce (final confirmation, >=30 games):
//
//	VS_AI2_32_MEASURE=1 VS_AI2_32_OPENINGS=15 go test ./arena \
//	    -run TestStrangulationEvalHeadToHead -v -timeout 90m
//
// Sanity baseline (frozen-vs-frozen must be ~50%):
//
//	VS_AI2_32_MEASURE=1 VS_AI2_32_BASELINE=1 VS_AI2_32_OPENINGS=8 \
//	    go test ./arena -run TestStrangulationEvalHeadToHead -v -timeout 60m
func TestStrangulationEvalHeadToHead(t *testing.T) {
	if os.Getenv("VS_AI2_32_MEASURE") != "1" {
		t.Skip("set VS_AI2_32_MEASURE=1 to run the slow production-budget 12x12 eval head-to-head")
	}
	openings := 8
	if v := os.Getenv("VS_AI2_32_OPENINGS"); v != "" {
		parsed, err := strconv.Atoi(v)
		if err != nil || parsed < 1 {
			t.Fatalf("VS_AI2_32_OPENINGS=%q must be a positive integer", v)
		}
		openings = parsed
	}
	// The contender is the live (new-eval) production engine; the opponent is the
	// frozen incumbent. VS_AI2_32_BASELINE=1 replaces the contender with the
	// frozen engine too, so the run measures the ~50% no-difference baseline and
	// proves the harness is unbiased.
	contender := TelemetryProduction()
	if os.Getenv("VS_AI2_32_BASELINE") == "1" {
		contender = TelemetryFrozenProduction()
	}
	incumbent := TelemetryFrozenProduction()

	var report Report
	for i := 0; i < openings; i++ {
		snapshot := randomLegalOpening(t, uint64(i)+1)
		for seat := 0; seat < 2; seat++ {
			agents := []TelemetryAgent{contender, incumbent}
			if seat == 1 {
				agents[0], agents[1] = agents[1], agents[0]
			}
			result, err := Play(Match{Rows: 12, Cols: 12, Initial: &snapshot, TelemetryAgents: agents})
			if err != nil {
				t.Fatalf("opening %d seat %d: %v", i, seat, err)
			}
			if result.Illegal != 0 || result.Stalled {
				t.Fatalf("opening %d seat %d produced illegal/stalled decision: %+v", i, seat, result)
			}
			report.Add(result, game.Player(seat+1))
		}
	}
	interval := Wilson95(report.Wins, report.Games)
	t.Logf("12x12 seeded-opening head-to-head (contender=new-eval, opponent=frozen incumbent): %s wilson95=[%.1f%%, %.1f%%]",
		report, interval.Low, interval.High)
}

// TestStrangulationEvalNodeBudget is the FAST deterministic tuning proxy for the
// eval coefficient sweep. Fixed-DEPTH search is not viable on a mostly-empty
// 12x12 board (neutral-placement branching explodes), so instead each decision
// runs iterative-deepening capped at a fixed NODE budget (search.ChooseNodeBudget
// via TelemetryNodeBudget). That is fully deterministic (no wall clock) and
// bounded, and at a shared compute budget the win-rate ranks eval quality
// faithfully: a better evaluator wins more from the SAME nodes. It plays
// balanced-seat 12x12 games from N seeded openings, new eval vs frozen incumbent,
// and reports the new-eval win-rate.
//
//	VS_AI2_32_NODEGATE=1 VS_AI2_32_OPENINGS=30 VS_AI2_32_NODES=40000 \
//	    go test ./arena -run TestStrangulationEvalNodeBudget -v -timeout 30m
func TestStrangulationEvalNodeBudget(t *testing.T) {
	if os.Getenv("VS_AI2_32_NODEGATE") != "1" {
		t.Skip("set VS_AI2_32_NODEGATE=1 to run the fast node-budget eval sweep")
	}
	openings := 30
	if v := os.Getenv("VS_AI2_32_OPENINGS"); v != "" {
		parsed, err := strconv.Atoi(v)
		if err != nil || parsed < 1 {
			t.Fatalf("VS_AI2_32_OPENINGS=%q must be a positive integer", v)
		}
		openings = parsed
	}
	nodes := uint64(40000)
	if v := os.Getenv("VS_AI2_32_NODES"); v != "" {
		parsed, err := strconv.ParseUint(v, 10, 64)
		if err != nil || parsed < 1 {
			t.Fatalf("VS_AI2_32_NODES=%q must be a positive integer", v)
		}
		nodes = parsed
	}
	contender := TelemetryNodeBudget(nodes, false)
	incumbent := TelemetryNodeBudget(nodes, true)

	var report Report
	for i := 0; i < openings; i++ {
		snapshot := randomLegalOpening(t, uint64(i)+1)
		for seat := 0; seat < 2; seat++ {
			agents := []TelemetryAgent{contender, incumbent}
			if seat == 1 {
				agents[0], agents[1] = agents[1], agents[0]
			}
			result, err := Play(Match{Rows: 12, Cols: 12, Initial: &snapshot, TelemetryAgents: agents})
			if err != nil {
				t.Fatalf("opening %d seat %d: %v", i, seat, err)
			}
			if result.Illegal != 0 {
				t.Fatalf("opening %d seat %d illegal: %+v", i, seat, result)
			}
			report.Add(result, game.Player(seat+1))
		}
	}
	interval := Wilson95(report.Wins, report.Games)
	t.Logf("12x12 node-budget(%d) head-to-head new-eval vs frozen incumbent: %s wilson95=[%.1f%%, %.1f%%]",
		nodes, report, interval.Low, interval.High)
}

// randomLegalOpening plays ~8 pseudo-random legal plies from the empty 12x12
// two-player board and returns the resulting non-terminal snapshot. The seed
// makes each opening distinct but reproducible. Balancing both seats over this
// shared position cancels any opening advantage.
func randomLegalOpening(t *testing.T, seed uint64) game.Snapshot {
	t.Helper()
	const plies = 8
	rng := rand.New(rand.NewSource(int64(seed)))
	state, err := game.New(12, 12, 2)
	if err != nil {
		t.Fatal(err)
	}
	for ply := 0; ply < plies; ply++ {
		actions := state.LegalActions()
		if len(actions) == 0 || state.GameOver() {
			t.Fatalf("opening seed %d went terminal after %d plies", seed, ply)
		}
		next, err := state.Apply(actions[rng.Intn(len(actions))])
		if err != nil {
			t.Fatalf("opening seed %d: illegal random ply: %v", seed, err)
		}
		state = next
	}
	if state.GameOver() {
		t.Fatalf("opening seed %d is terminal", seed)
	}
	return state.Snapshot()
}
