package arena

import (
	"fmt"
	"math/rand"
	"os"
	"strings"
	"testing"

	"virusgame/game"
)

// randomLegalOpeningN is randomLegalOpening generalized to N players and any
// board: it plays ~one turn per player of pseudo-random legal plies from the
// empty board and returns the resulting non-terminal snapshot. Cyclic seat
// rotation over this shared position cancels any per-base opening advantage.
func randomLegalOpeningN(t *testing.T, seed uint64, rows, cols, players int) game.Snapshot {
	t.Helper()
	plies := 3 * players
	rng := rand.New(rand.NewSource(int64(seed)))
	state, err := game.New(rows, cols, players)
	if err != nil {
		t.Fatal(err)
	}
	for ply := 0; ply < plies; ply++ {
		actions := state.LegalActions()
		if len(actions) == 0 || state.GameOver() {
			t.Fatalf("opening seed %d (%dp %dx%d) went terminal after %d plies", seed, players, rows, cols, ply)
		}
		next, err := state.Apply(actions[rng.Intn(len(actions))])
		if err != nil {
			t.Fatalf("opening seed %d: illegal random ply: %v", seed, err)
		}
		state = next
	}
	if state.GameOver() {
		t.Fatalf("opening seed %d (%dp %dx%d) is terminal", seed, players, rows, cols)
	}
	return state.Snapshot()
}

// mpResult aggregates a focus team's multiplayer performance: 1st-place win
// rate (used for early stopping), the full 1st..4th placement distribution,
// and termination breakdown. "Win" == the focus team took 1st place.
type mpResult struct {
	Games                   int
	Wins                    int    // focus team placed 1st
	Place                   [5]int // placement histogram, index 1..4
	Decisive, Maxed, Stalled, Illegal int
	Stopped, Above          bool
}

// playMultiplayerRotations plays each seeded N-player opening under all N
// cyclic seat rotations, so every base agent occupies every seat/base exactly
// once per opening. It reads the focus team's best (lowest) placement per game,
// updates the running Wilson 95% CI on the 1st-place rate, and early-stops once
// that CI clears thresholdPct (fair-share = 100*focusSeats/N). Deterministic:
// the opening permutation is fixed-seed and every agent used here is a pure
// function of state.
func playMultiplayerRotations(t *testing.T, label string, board Board, agents []TelemetryAgent, focus []bool, maxOpenings int, thresholdPct float64) mpResult {
	t.Helper()
	n := len(agents)
	if len(focus) != n {
		t.Fatalf("%s: focus mask len %d != agents %d", label, len(focus), n)
	}
	var res mpResult
	order := rand.New(rand.NewSource(sequentialOrderSeed)).Perm(maxOpenings)
	for _, idx := range order {
		snapshot := randomLegalOpeningN(t, uint64(idx)+1, board.Rows, board.Cols, n)
		for r := 0; r < n; r++ {
			rotated := make([]TelemetryAgent, n)
			for s := 0; s < n; s++ {
				rotated[s] = agents[(s+r)%n]
			}
			played, err := Play(Match{Rows: board.Rows, Cols: board.Cols, Initial: &snapshot, TelemetryAgents: rotated})
			if err != nil {
				t.Fatalf("%s opening %d rot %d: %v", label, idx, r, err)
			}
			res.Games++
			res.Illegal += played.Illegal
			switch {
			case played.Stalled:
				res.Stalled++
			case played.Maxed:
				res.Maxed++
			default:
				res.Decisive++
			}
			// focus base agent i sits at seat (i-r+n)%n under rotation r.
			best := n
			for i := 0; i < n; i++ {
				if !focus[i] {
					continue
				}
				if p := played.Placement[(i-r+n)%n]; p != 0 && p < best {
					best = p
				}
			}
			if best >= 1 && best <= 4 {
				res.Place[best]++
			}
			if best == 1 {
				res.Wins++
			}
		}
		if stop, above := wilsonDecision(res.Wins, res.Games, thresholdPct, sequentialMinGames); stop {
			res.Stopped, res.Above = true, above
			break
		}
	}
	return res
}

// TestMultiplayerLadderReport is the 3-4 player baseline: production eval (node
// budget) vs heuristic mixes and vs the frozen incumbent, over seeded openings
// with full cyclic seat rotation, deterministic node budget, and Wilson early
// stopping against each row's fair-share threshold. It is a measurement — it
// reports win rate, placement distribution (1st..4th) and terminations, and
// only fails on illegal games (a broken harness).
//
// Reproduce:
//
//	VS_MP_LADDER=1 go test ./arena -run TestMultiplayerLadderReport -v -timeout 240m
//
// Quick wiring check:
//
//	VS_MP_LADDER=1 VS_MP_LADDER_OPENINGS=2 go test ./arena -run TestMultiplayerLadderReport -v
func TestMultiplayerLadderReport(t *testing.T) {
	if os.Getenv("VS_MP_LADDER") != "1" {
		t.Skip("set VS_MP_LADDER=1 to run the slow 3-4 player ladder report")
	}
	openings := envInt(t, "VS_MP_LADDER_OPENINGS", 20)
	nodes := uint64(envInt(t, "VS_MP_LADDER_NODES", 1000))
	prod := TelemetryNodeBudget(nodes, false)
	inc := TelemetryNodeBudget(nodes, true)
	greedy, base, mob := Instrument(Greedy), Instrument(BaseAttacker), Instrument(MobilityAttacker)

	rungs := []struct {
		name   string
		board  Board
		agents []TelemetryAgent
		focus  []bool
	}{
		{"3p prod vs greedy+base", Board{12, 12}, []TelemetryAgent{prod, greedy, base}, []bool{true, false, false}},
		{"3p prod vs 2x incumbent", Board{12, 12}, []TelemetryAgent{prod, inc, inc}, []bool{true, false, false}},
		{"4p prod vs greedy+base+mob", Board{12, 12}, []TelemetryAgent{prod, greedy, base, mob}, []bool{true, false, false, false}},
		{"4p prod vs 3x incumbent", Board{12, 12}, []TelemetryAgent{prod, inc, inc, inc}, []bool{true, false, false, false}},
		{"4p 2x prod vs 2x incumbent", Board{12, 12}, []TelemetryAgent{prod, prod, inc, inc}, []bool{true, true, false, false}},
		{"4p prod vs greedy+base+mob 16x16", Board{16, 16}, []TelemetryAgent{prod, greedy, base, mob}, []bool{true, false, false, false}},
	}

	var table strings.Builder
	fmt.Fprintf(&table, "multiplayer ladder (nodes=%d, openings=%d, cyclic seat rotation):\n", nodes, openings)
	fmt.Fprintf(&table, "%-34s | %-7s | %9s | %6s | %-16s | %-19s | %-7s | %s\n",
		"rung", "players", "wins/games", "1st%", "wilson95(1st)", "place 1/2/3/4", "share%", "term D/M/S")
	for _, rung := range rungs {
		n := len(rung.agents)
		focusSeats := 0
		for _, f := range rung.focus {
			if f {
				focusSeats++
			}
		}
		fair := 100 * float64(focusSeats) / float64(n)
		res := playMultiplayerRotations(t, rung.name, rung.board, rung.agents, rung.focus, openings, fair)
		if res.Illegal != 0 {
			t.Fatalf("%s produced %d illegal games", rung.name, res.Illegal)
		}
		iv := Wilson95(res.Wins, res.Games)
		rate := 100 * float64(res.Wins) / float64(res.Games)
		fmt.Fprintf(&table, "%-34s | %5dp   | %5d/%-3d | %5.1f%% | [%5.1f%%,%5.1f%%] | %4d/%4d/%4d/%4d | %5.1f%% | %d/%d/%d\n",
			rung.name, n, res.Wins, res.Games, rate, iv.Low, iv.High,
			res.Place[1], res.Place[2], res.Place[3], res.Place[4], fair,
			res.Decisive, res.Maxed, res.Stalled)
	}
	t.Log("\n" + table.String())
}

// TestMultiplayerPlacementInvariants is the always-on correctness check for the
// placement machinery: across small deterministic 3- and 4-player games, every
// finished game's placements must be a permutation of 1..N, and the winner (if
// decisive) must hold place 1.
func TestMultiplayerPlacementInvariants(t *testing.T) {
	for _, n := range []int{3, 4} {
		for seed := uint64(1); seed <= 6; seed++ {
			snap := randomLegalOpeningN(t, seed, 8, 8, n)
			agents := make([]TelemetryAgent, n)
			for i := range agents {
				agents[i] = Instrument(Greedy) // deterministic; Greedy for all -> decisive
			}
			res, err := Play(Match{Rows: 8, Cols: 8, Initial: &snap, TelemetryAgents: agents})
			if err != nil {
				t.Fatalf("n=%d seed=%d: %v", n, seed, err)
			}
			seen := map[int]bool{}
			for s := 0; s < n; s++ {
				p := res.Placement[s]
				if p < 1 || p > n || seen[p] {
					t.Fatalf("n=%d seed=%d: placements not a 1..%d permutation: %v", n, seed, n, res.Placement[:n])
				}
				seen[p] = true
			}
			for s := n; s < 4; s++ {
				if res.Placement[s] != 0 {
					t.Fatalf("n=%d seed=%d: unseated seat %d has place %d", n, seed, s, res.Placement[s])
				}
			}
			if res.Winner != 0 && res.Placement[res.Winner-1] != 1 {
				t.Fatalf("n=%d seed=%d: winner %d has place %d, want 1", n, seed, res.Winner, res.Placement[res.Winner-1])
			}
		}
	}
}

// TestMultiplayerRotationDeterministic guards the ladder's reproducibility: the
// same rung run twice must yield identical wins, games, placement histogram,
// and stop decision.
func TestMultiplayerRotationDeterministic(t *testing.T) {
	nodes := uint64(400)
	prod := TelemetryNodeBudget(nodes, false)
	agents := []TelemetryAgent{prod, Instrument(Greedy), Instrument(BaseAttacker), Instrument(MobilityAttacker)}
	focus := []bool{true, false, false, false}
	run := func() mpResult {
		return playMultiplayerRotations(t, "det", Board{8, 8}, agents, focus, 3, 25)
	}
	a, b := run(), run()
	if a.Games != b.Games || a.Wins != b.Wins || a.Place != b.Place || a.Stopped != b.Stopped || a.Above != b.Above {
		t.Fatalf("multiplayer rotation not deterministic:\n a=%+v\n b=%+v", a, b)
	}
	if a.Games == 0 {
		t.Fatal("no games played")
	}
}
