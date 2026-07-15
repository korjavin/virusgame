package arena

import (
	"context"
	"reflect"
	"testing"
	"time"

	"virusgame/game"
	"virusgame/search"
)

func TestProductionPathDeadlineAndLegality(t *testing.T) {
	state, err := game.New(5, 5, 2)
	if err != nil {
		t.Fatal(err)
	}
	started := time.Now()
	result, ok := search.Choose(context.Background(), state)
	elapsed := time.Since(started)
	if !ok {
		t.Fatal("production search returned no action")
	}
	if _, err := state.Apply(result.Action); err != nil {
		t.Fatalf("production search returned illegal action: %v", err)
	}
	if elapsed > search.ProductionBudget+250*time.Millisecond {
		t.Fatalf("production search exceeded bounded deadline: %s", elapsed)
	}
}

func TestPlayRejectsSnapshotAgentCountMismatch(t *testing.T) {
	state, err := game.New(5, 5, 3)
	if err != nil {
		t.Fatal(err)
	}
	snapshot := state.Snapshot()
	if _, err := Play(Match{Rows: 5, Cols: 5, Initial: &snapshot, Agents: []Agent{Greedy, Greedy}}); err == nil {
		t.Fatal("accepted three-player snapshot with two agents")
	}
}

func TestStrengthGate(t *testing.T) {
	boards := []Board{{Rows: 5, Cols: 5}, {Rows: 6, Cols: 6}}
	contender := Tournament(3)
	for _, test := range []struct {
		name      string
		opponent  OpponentFactory
		threshold float64
	}{
		{name: "legacy", opponent: Legacy, threshold: 85},
		{name: "greedy", opponent: func(uint64) Agent { return Greedy }, threshold: 75},
	} {
		t.Run(test.name, func(t *testing.T) {
			report, err := Balanced(boards, 2, contender, test.opponent)
			if err != nil {
				t.Fatal(err)
			}
			t.Log(report)
			if report.Illegal != 0 || report.Maxed != 0 || report.Stalled != 0 {
				t.Fatalf("legality/completion gate failed: %s", report)
			}
			if report.Percentile(95) > 600*time.Millisecond {
				t.Fatalf("latency gate p95=%s > 600ms: %s", report.Percentile(95), report)
			}
			if report.WinRate() < test.threshold {
				t.Fatalf("strength gate %.1f%% < %.1f%%: %s", report.WinRate(), test.threshold, report)
			}
		})
	}
}

func TestDeterministicOutcomesAndSeatBalance(t *testing.T) {
	boards := []Board{{Rows: 5, Cols: 5}}
	a, err := Balanced(boards, 2, Tournament(2), Legacy)
	if err != nil {
		t.Fatal(err)
	}
	b, err := Balanced(boards, 2, Tournament(2), Legacy)
	if err != nil {
		t.Fatal(err)
	}
	a.Latencies, b.Latencies = nil, nil
	a.Elapsed, b.Elapsed = 0, 0
	if !reflect.DeepEqual(a, b) || a.Games != 4 {
		t.Fatalf("fixed tournament outcomes differ: %+v / %+v", a, b)
	}
}

func TestMultiplayerSmoke(t *testing.T) {
	for players := 3; players <= 4; players++ {
		agents := make([]Agent, players)
		agents[0] = Tournament(2)
		for index := 1; index < players; index++ {
			agents[index] = Greedy
		}
		result, err := Play(Match{Rows: 5, Cols: 5, Agents: agents})
		if err != nil {
			t.Fatal(err)
		}
		if result.Illegal != 0 || result.Maxed || result.Stalled || result.Winner == 0 || result.Eliminations != players-1 {
			t.Fatalf("%d-player smoke failed: %+v", players, result)
		}
	}
}

func TestDetectsIllegalStallAndMaxLength(t *testing.T) {
	noAction := func(game.State) (game.Action, bool) { return game.Action{}, false }
	stalled, err := Play(Match{Rows: 5, Cols: 5, Agents: []Agent{noAction, Greedy}})
	if err != nil {
		t.Fatal(err)
	}
	if !stalled.Stalled || stalled.Illegal != 1 || stalled.Decisions != 1 {
		t.Fatalf("stall not detected: %+v", stalled)
	}
	maxed, err := Play(Match{Rows: 5, Cols: 5, Agents: []Agent{Random(1), Random(2)}, MaxActions: 1})
	if err != nil {
		t.Fatal(err)
	}
	if !maxed.Maxed || maxed.Actions != 1 || maxed.Illegal != 0 {
		t.Fatalf("max length not detected: %+v", maxed)
	}
}

func TestIllegalActionIsCounted(t *testing.T) {
	illegal := func(game.State) (game.Action, bool) {
		return game.Action{Kind: game.Move, Target: game.Pos{Row: -1, Col: -1}}, true
	}
	result, err := Play(Match{Rows: 5, Cols: 5, Agents: []Agent{illegal, Greedy}})
	if err != nil {
		t.Fatal(err)
	}
	if result.Illegal != 1 || result.Decisions != 1 || result.Actions != 0 {
		t.Fatalf("illegal action not counted: %+v", result)
	}
}
