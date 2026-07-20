package arena

import (
	"context"
	"os"
	"reflect"
	"testing"
	"time"

	"virusgame/game"
	"virusgame/search"
	"virusgame/search/incumbent"
)

func TestFrozenIncumbentGoldenTacticalOutputs(t *testing.T) {
	type golden struct {
		action       game.Action
		score, depth int
		nodes        uint64
	}
	want := map[string]golden{
		"6c4edd337904f0aa8d105fe0a545e551eb3b7e36d4d50f3a2bfc1642ce63d78b": {game.Action{Kind: game.Move, Target: game.Pos{Row: 1, Col: 1}}, 2096, 2, 23},
		"33426981f95a9eafb27a3e104210dc5d87e95a3cad4d353fbd5b48bc5ac2824c": {game.Action{Kind: game.Move, Target: game.Pos{Row: 4, Col: 3}}, 6336, 2, 23},
		"32af0aa62e2721a2ec3f63f99d91690e7c51a32d3d4a541a8e46e454803bab92": {game.Action{Kind: game.Move, Target: game.Pos{Row: 6, Col: 6}}, 3431, 2, 113},
		"3c3d23f5229bf260f963ed4b41898b8c2074d8b0d468a5131e381a42d3dfa788": {game.Action{Kind: game.Move, Target: game.Pos{Row: 7, Col: 6}}, 6576, 2, 50},
	}
	fixture, err := os.Open("testdata/strength-corpus-v1.json")
	if err != nil {
		t.Fatal(err)
	}
	defer fixture.Close()
	corpus, err := DecodeCorpus(fixture)
	if err != nil {
		t.Fatal(err)
	}
	found := 0
	for _, testCase := range corpus.Cases {
		expected, ok := want[testCase.Hash]
		if !ok {
			continue
		}
		result, complete := incumbent.ChooseDepth(context.Background(), testCase.State, 2)
		if !complete || result.Action != expected.action || result.Score != expected.score || result.Depth != expected.depth || result.Nodes != expected.nodes {
			t.Fatalf("hash=%s got=%+v/%v want=%+v", testCase.Hash, result, complete, expected)
		}
		found++
	}
	if found != len(want) {
		t.Fatalf("found %d of %d golden states", found, len(want))
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

func TestCurrentTelemetryMapsRootParallelCounters(t *testing.T) {
	result := search.Result{Depth: 3, Nodes: 11, Evaluations: 7, CompletedTurnDepth: 1, Workers: 4, RootLegal: 20, RootSelected: 12, RootCompleted: 12, RootLegalNeutrals: 6, RootSelectedNeutrals: 2, IterationsStarted: 2, IterationsCompleted: 1, Elapsed: time.Millisecond}
	got := currentTelemetry(result)
	if got.Nodes != 11 || got.Evaluations != 7 || got.CompletedTurnDepth != 1 || got.Workers != 4 || got.RootCompleted != 12 || got.LegalRootActions != 20 || got.SearchedRootActions != 12 || got.LegalRootNeutrals != 6 || got.SearchedRootNeutrals != 2 || got.IterationsStarted != 2 || got.IterationsCompleted != 1 || got.SearchElapsed != time.Millisecond {
		t.Fatalf("telemetry=%+v", got)
	}
	result.Depth = 0
	got = currentTelemetry(result)
	if got.SearchedRootActions != 0 || got.SearchedRootNeutrals != 0 {
		t.Fatalf("fallback published coverage: %+v", got)
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
	result, ok := incumbent.ChooseDepth(context.Background(), state, 2)
	want := incumbent.Result{Action: game.Action{Kind: game.Move, Target: game.Pos{Row: 0, Col: 1}}, Score: 1508, Depth: 2, Nodes: 18, Evaluations: 15}
	if !ok || result != want {
		t.Fatalf("got=%+v/%v want=%+v", result, ok, want)
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
	if serial.Overall.Wins != 3 || serial.Overall.Losses != 3 || serial.Overall.Draws != 0 || serial.Overall.WinRate() != 50 {
		t.Fatalf("unbalanced deterministic self comparison: %s", serial)
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
