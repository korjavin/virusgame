package arena

import (
	"math/rand"
	"os"
	"strconv"
	"testing"

	"virusgame/game"
)

// TestStrangulationEvalNodeBudget is the vs-ai2.34 secondary gate: a FAST
// deterministic incumbent-differential measurement. Fixed-DEPTH search is not
// viable on a mostly-empty 12x12 board (neutral-placement branching explodes),
// so instead each decision runs iterative-deepening capped at a fixed NODE
// budget (search.ChooseNodeBudget via TelemetryNodeBudget). That is fully
// deterministic (no wall clock) and bounded, and at a shared compute budget the
// win-rate ranks eval quality faithfully: a better evaluator wins more from the
// SAME nodes. It plays balanced-seat 12x12 games from N seeded openings,
// candidate (live eval) vs the byte-frozen incumbent, and reports the candidate
// win-rate with a Wilson 95% CI. The run stops early (SPRT-style, threshold
// 50%) once the CI clears 50%, else at the opening cap.
//
// Parity property: because both seats are played from every shared opening,
// frozen-vs-frozen reads 50% — any deviation of the candidate from 50% is
// evaluator signal, not harness bias. A wall-clock head-to-head variant used to
// live here (vs-ai2.32 TestStrangulationEvalHeadToHead) and was deleted: timing
// jitter gave it a ~7% seat/timing bias, so all tuning gates are node-budget
// deterministic.
//
// It is a measurement, not a hard strength gate: it only fails on an
// illegal/stalled decision or a maxed-out (non-terminating) game.
//
// Reproduce:
//
//	VS_STRANGLER_DIFF=1 go test ./arena \
//	    -run TestStrangulationEvalNodeBudget -v -timeout 60m
func TestStrangulationEvalNodeBudget(t *testing.T) {
	if os.Getenv("VS_STRANGLER_DIFF") != "1" {
		t.Skip("set VS_STRANGLER_DIFF=1 to run the node-budget incumbent-differential gate")
	}
	openings := stranglerOpenings(t)
	nodes := uint64(1000)
	if v := os.Getenv("VS_STRANGLER_NODES"); v != "" {
		parsed, err := strconv.ParseUint(v, 10, 64)
		if err != nil || parsed < 1 {
			t.Fatalf("VS_STRANGLER_NODES=%q must be a positive integer", v)
		}
		nodes = parsed
	}
	contender := TelemetryNodeBudget(nodes, false)
	incumbent := TelemetryNodeBudget(nodes, true)

	result := playSequentialOpenings(t, "candidate vs incumbent", openings, 50, sequentialMinGames, contender, incumbent)
	interval := Wilson95(result.Wins, result.Games)
	t.Logf("12x12 node-budget(%d) head-to-head candidate vs frozen incumbent: %s wilson95=[%.1f%%, %.1f%%] games-played=%d/%d",
		nodes, result.Report, interval.Low, interval.High, result.Games, 2*openings)
}

// stranglerOpenings returns the shared opening count for the strangler gates,
// overridable via VS_STRANGLER_OPENINGS.
func stranglerOpenings(t *testing.T) int {
	t.Helper()
	openings := 40
	if v := os.Getenv("VS_STRANGLER_OPENINGS"); v != "" {
		parsed, err := strconv.Atoi(v)
		if err != nil || parsed < 1 {
			t.Fatalf("VS_STRANGLER_OPENINGS=%q must be a positive integer", v)
		}
		openings = parsed
	}
	return openings
}

// playBalancedOpenings plays both seats of every seeded 12x12 opening between
// a and b and returns a's report. It fails on any illegal or stalled decision,
// or on a game that hits the MaxActions cap without terminating — a maxed game
// would otherwise count as a silent draw and deflate both win rates.
func playBalancedOpenings(t *testing.T, label string, openings int, a, b TelemetryAgent) Report {
	t.Helper()
	var report Report
	for i := 0; i < openings; i++ {
		snapshot := randomLegalOpening(t, uint64(i)+1)
		for seat := 0; seat < 2; seat++ {
			agents := []TelemetryAgent{a, b}
			if seat == 1 {
				agents[0], agents[1] = agents[1], agents[0]
			}
			result, err := Play(Match{Rows: 12, Cols: 12, Initial: &snapshot, TelemetryAgents: agents})
			if err != nil {
				t.Fatalf("%s opening %d seat %d: %v", label, i, seat, err)
			}
			if result.Illegal != 0 || result.Stalled || result.Maxed {
				t.Fatalf("%s opening %d seat %d produced illegal/stalled/maxed game: %+v", label, i, seat, result)
			}
			report.Add(result, game.Player(seat+1))
		}
	}
	return report
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
