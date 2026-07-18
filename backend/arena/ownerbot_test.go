package arena

import (
	"os"
	"testing"

	"virusgame/game"
)

// TestOwnerBotLegalAndDeterministic sanity-checks the sparring agent: on a
// family of seeded openings it must return a legal move and repeat the same
// choice, and on the crafted cut position it must attack the victim's lone
// articulation point (target it or adjoin it) like CutSeeker does.
func TestOwnerBotLegalAndDeterministic(t *testing.T) {
	for seed := uint64(1); seed <= 5; seed++ {
		snapshot := randomLegalOpening(t, seed)
		state, err := game.FromSnapshot(snapshot)
		if err != nil {
			t.Fatal(err)
		}
		action, ok := OwnerBot(state)
		if !ok {
			t.Fatalf("seed %d: returned no action", seed)
		}
		if _, err := state.Apply(action); err != nil {
			t.Fatalf("seed %d: illegal action %+v: %v", seed, action, err)
		}
		if repeat, _ := OwnerBot(state); repeat != action {
			t.Fatalf("seed %d: nondeterministic: %+v then %+v", seed, action, repeat)
		}
	}

	state := craftedCutPosition(t)
	cut := game.Pos{Row: 4, Col: 4}
	cuts := opponentArticulations(state, 2)
	action, ok := OwnerBot(state)
	if !ok || action.Kind != game.Move {
		t.Fatalf("OwnerBot returned no move: %+v ok=%v", action, ok)
	}
	if action.Target != cut && !adjacentToCut(action.Target, cuts) {
		t.Fatalf("OwnerBot should target or adjoin the cut %v, chose %+v", cut, action)
	}
}

// TestProductionEvalBeatsOwnerBot is the standing OwnerBot gate. History: the
// vs-ai2.55 original (TestOwnerBotBeatsProductionEval) asserted OwnerBot won
// >=40% — built as the strongest scripted rung against the then-current
// hand-tuned eval, which lost 62.5% to it. The vs-ai2.52 owner-targeted SPSA
// eval inverted that premise by success (OwnerBot fell to 31.2%), so the gate
// now asserts the win from the production side: from the empty 12x12 board over
// the family of opening lines, both seats, the production eval must beat
// OwnerBot in >=55% of games. Deterministic (node budget, no wall clock), so
// the verdict is load-immune.
//
//	VS_OWNERBOT=1 go test ./arena -run TestProductionEvalBeatsOwnerBot -v -timeout 60m
func TestProductionEvalBeatsOwnerBot(t *testing.T) {
	if os.Getenv("VS_OWNERBOT") != "1" {
		t.Skip("set VS_OWNERBOT=1 to run the OwnerBot validation gate")
	}
	nodes := uint64(envInt(t, "VS_OWNERBOT_NODES", 1000))
	eval := nodeBudgetPlainAgent(nodes, false)
	openings := emptyOpeningLines()
	var evalWins, games int
	for _, line := range openings {
		snapshot := buildOpening(t, line)
		for seat := 0; seat < 2; seat++ {
			// seat = OwnerBot's position; the production eval sits opposite.
			agents := []Agent{OwnerBot, eval}
			if seat == 1 {
				agents[0], agents[1] = agents[1], agents[0]
			}
			result, err := Play(Match{Rows: 12, Cols: 12, Initial: &snapshot, Agents: agents})
			if err != nil {
				t.Fatalf("%s seat %d: %v", line.name, seat, err)
			}
			if result.Illegal != 0 || result.Stalled {
				t.Fatalf("%s seat %d illegal/stalled: %+v", line.name, seat, result)
			}
			games++
			if result.Winner == game.Player(2-seat) {
				evalWins++
			}
		}
	}
	rate := 100 * float64(evalWins) / float64(games)
	t.Logf("production eval vs OwnerBot (nodes=%d): %d/%d = %.1f%%", nodes, evalWins, games, rate)
	if rate < 55 {
		t.Fatalf("production eval win-rate %.1f%% < 55%% bar (wins=%d games=%d)", rate, evalWins, games)
	}
}
