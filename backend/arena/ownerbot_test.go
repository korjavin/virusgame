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

// TestOwnerBotBeatsProductionEval is the vs-ai2.55 validation gate. From the
// empty 12x12 board over the family of opening lines, both seats, OwnerBot must
// beat the CURRENT production eval (deterministic node budget) in >=40% of
// games — making it the strongest scripted rung. Deterministic (node budget, no
// wall clock), so the verdict is load-immune.
//
//	VS_OWNERBOT=1 go test ./arena -run TestOwnerBotBeatsProductionEval -v -timeout 60m
func TestOwnerBotBeatsProductionEval(t *testing.T) {
	if os.Getenv("VS_OWNERBOT") != "1" {
		t.Skip("set VS_OWNERBOT=1 to run the OwnerBot validation gate")
	}
	nodes := uint64(envInt(t, "VS_OWNERBOT_NODES", 1000))
	eval := nodeBudgetPlainAgent(nodes, false)
	openings := emptyOpeningLines()
	var wins, games int
	for _, line := range openings {
		snapshot := buildOpening(t, line)
		for seat := 0; seat < 2; seat++ {
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
			if result.Winner == game.Player(seat+1) {
				wins++
			}
		}
	}
	rate := 100 * float64(wins) / float64(games)
	t.Logf("OwnerBot vs production eval (nodes=%d): %d/%d = %.1f%%", nodes, wins, games, rate)
	if rate < 40 {
		t.Fatalf("OwnerBot win-rate %.1f%% < 40%% bar (wins=%d games=%d)", rate, wins, games)
	}
}
