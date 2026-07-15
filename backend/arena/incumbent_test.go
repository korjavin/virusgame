package arena

import (
	"context"
	"reflect"
	"testing"

	"virusgame/game"
	"virusgame/search"
	"virusgame/search/incumbent"
)

func TestFrozenIncumbentMatchesCurrentSample(t *testing.T) {
	states := sampledStates(t)
	for index, state := range states {
		got, ok := search.ChooseDepth(context.Background(), state, 2)
		want, wantOK := incumbent.ChooseDepth(context.Background(), state, 2)
		if !ok || !wantOK || got.Action != want.Action || got.Score != want.Score || got.Depth != want.Depth || got.Nodes != want.Nodes {
			t.Fatalf("sample %d parity failed: current=%+v/%v frozen=%+v/%v", index, got, ok, want, wantOK)
		}
	}
}

func TestNodeBudgetExactCeilingAndTelemetry(t *testing.T) {
	state, _ := game.New(8, 8, 2)
	for _, frozen := range []bool{false, true} {
		action, telemetry, ok := TelemetryNodeBudget(500, frozen)(state)
		if !ok || telemetry.Nodes != 500 || telemetry.BudgetShortfall || telemetry.SearchedRootActions != telemetry.LegalRootActions || telemetry.SearchedRootNeutrals != telemetry.LegalRootNeutrals {
			t.Fatalf("frozen=%v telemetry=%+v ok=%v", frozen, telemetry, ok)
		}
		if _, err := state.Apply(action); err != nil {
			t.Fatalf("frozen=%v illegal: %v", frozen, err)
		}
	}
}

func TestFrozenMultiplayerParityAfterEliminations(t *testing.T) {
	state, _ := game.New(8, 8, 4)
	snapshot := state.Snapshot()
	for _, player := range []int{2, 3} {
		base := snapshot.Bases[player]
		snapshot.Board[base.Row][base.Col] = game.Cell{}
		snapshot.Active[player] = false
	}
	state, err := game.FromSnapshot(snapshot)
	if err != nil {
		t.Fatal(err)
	}
	got, ok := search.ChooseDepth(context.Background(), state, 2)
	want, wantOK := incumbent.ChooseDepth(context.Background(), state, 2)
	if !ok || !wantOK || got.Action != want.Action || got.Score != want.Score || got.Nodes != want.Nodes {
		t.Fatalf("current=%+v frozen=%+v", got, want)
	}
}

func TestSerialAndThreeWayCorpusAggregationMatch(t *testing.T) {
	cases := []CorpusCase{}
	boards := []Board{{2, 2}, {2, 3}, {3, 2}}
	for index, board := range boards {
		state, _ := game.New(board.Rows, board.Cols, 2)
		cases = append(cases, CorpusCase{ID: string(rune('a' + index)), Split: "train", Track: "competitive_1v1", Phase: "opening", Players: 2, State: state})
	}
	corpus := Corpus{Cases: cases}
	factory := func() TelemetryAgent {
		return Instrument(func(state game.State) (game.Action, bool) {
			actions := state.LegalActions()
			if len(actions) == 0 {
				return game.Action{}, false
			}
			return actions[0], true
		})
	}
	serial, err := CompareCorpusFiltered(corpus, "train", CorpusFilter{Track: "competitive_1v1"}, nil, factory, factory)
	if err != nil {
		t.Fatal(err)
	}
	parallel, err := CompareCorpusBoards(corpus, "train", "competitive_1v1", boards, 3, nil, factory, factory)
	if err != nil {
		t.Fatal(err)
	}
	stripTiming(&serial)
	stripTiming(&parallel)
	if !reflect.DeepEqual(serial, parallel) {
		t.Fatalf("serial=%+v parallel=%+v", serial, parallel)
	}
	if serial.Overall.Illegal != 0 || serial.Overall.Stalled != 0 || serial.Overall.Maxed != 0 {
		t.Fatalf("invalid self comparison: %s", serial)
	}
	repeat, err := CompareCorpusBoards(corpus, "train", "competitive_1v1", boards, 3, nil, factory, factory)
	if err != nil {
		t.Fatal(err)
	}
	stripTiming(&repeat)
	if !reflect.DeepEqual(parallel, repeat) {
		t.Fatal("parallel self comparison is not deterministic")
	}
}

func TestSuperiorityGateIsHardAndBoardSeatAware(t *testing.T) {
	report := CorpusReport{Overall: Report{Games: 10, Wins: 8}, Buckets: map[string]Report{"board=5x5/seat=1": {Games: 10, Wins: 6}}}
	if report.ValidateSuperiority() == nil {
		t.Fatal("accepted sub-70 board/seat")
	}
	report.Buckets["board=5x5/seat=1"] = Report{Games: 10, Wins: 7}
	if err := report.ValidateSuperiority(); err != nil {
		t.Fatalf("rejected threshold: %v", err)
	}
	report.Overall.Illegal = 1
	if report.ValidateSuperiority() == nil {
		t.Fatal("accepted illegal game")
	}
}

func stripTiming(report *CorpusReport) {
	report.Overall.Latencies = nil
	report.Overall.Elapsed = 0
	for key, value := range report.Buckets {
		value.Latencies = nil
		value.Elapsed = 0
		report.Buckets[key] = value
	}
}

func sampledStates(t *testing.T) []game.State {
	var states []game.State
	for _, board := range []Board{{5, 5}, {8, 8}, {12, 12}} {
		state, err := game.New(board.Rows, board.Cols, 2)
		if err != nil {
			t.Fatal(err)
		}
		states = append(states, state)
		for step := 0; step < 5; step++ {
			actions := state.LegalActions()
			var action game.Action
			found := false
			for _, candidate := range actions {
				if candidate.Kind == game.Move {
					action = candidate
					found = true
					break
				}
			}
			if !found {
				break
			}
			state, err = state.Apply(action)
			if err != nil {
				t.Fatal(err)
			}
			states = append(states, state)
		}
	}
	return states
}
