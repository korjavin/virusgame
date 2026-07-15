package arena

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"strings"
	"testing"

	"virusgame/game"
)

func TestHappyOtterReplayAndCriticalTurns(t *testing.T) {
	fixture, err := os.Open("testdata/happyotter97-vs-bot1090.json")
	if err != nil {
		t.Fatal(err)
	}
	defer fixture.Close()

	replay, states, err := DecodeReplay(fixture)
	if err != nil {
		t.Fatal(err)
	}
	if replay.SourceID != "2cccdb97-8f6b-456d-bc76-a97933cc9cd6" || replay.Players != [2]string{"HappyOtter97", "Bot 1090"} ||
		replay.Rows != 10 || replay.Cols != 10 || len(replay.Turns) != 22 || replay.Winner != 1 || replay.Termination != "no_moves" {
		t.Fatalf("unexpected replay metadata: %+v", replay)
	}
	want := map[int]string{
		14: "12a990342e992e1c",
		16: "bacda15ebb256a4c",
		18: "70f1c3232b3edeb4",
		20: "03f9a5dfbd14c92e",
	}
	for turn, fingerprint := range want {
		state, ok := states[turn]
		if !ok {
			t.Fatalf("critical turn %d is not addressable", turn)
		}
		got := snapshotFingerprint(t, state)
		if got != fingerprint {
			t.Fatalf("turn %d fingerprint=%s, want %s", turn, got, fingerprint)
		}
	}
	final := states[22]
	if !final.GameOver() || final.Winner() != 1 {
		t.Fatalf("final state over=%v winner=%d", final.GameOver(), final.Winner())
	}
}

func TestDecodeReplayRejectsDivergence(t *testing.T) {
	for _, fixture := range []string{
		`{"rows":5,"cols":5,"winner":1,"turns":[{"turn":1,"player":2,"actions":[]}]}`,
		`{"rows":5,"cols":5,"winner":1,"turns":[{"turn":1,"player":1,"actions":[{"kind":"move","row":4,"col":4}]}]}`,
		`{"rows":5,"cols":5,"winner":1,"turns":[]} trailing`,
	} {
		if _, _, err := DecodeReplay(strings.NewReader(fixture)); err == nil {
			t.Fatalf("accepted divergent replay: %s", fixture)
		}
	}
}

func TestVariableRectangularDecisionMatrix(t *testing.T) {
	boards := []Board{
		{Rows: 5, Cols: 5}, {Rows: 5, Cols: 10}, {Rows: 10, Cols: 5},
		{Rows: 8, Cols: 8}, {Rows: 10, Cols: 10}, {Rows: 15, Cols: 20},
		{Rows: 25, Cols: 25}, {Rows: 50, Cols: 50},
		{Rows: 5, Cols: 50}, {Rows: 50, Cols: 5},
	}
	agent := TelemetryTournament(1)
	for _, board := range boards {
		state, err := game.New(board.Rows, board.Cols, 2)
		if err != nil {
			t.Fatalf("%dx%d: %v", board.Rows, board.Cols, err)
		}
		action, telemetry, ok := agent(state)
		if !ok {
			t.Fatalf("%dx%d: no decision", board.Rows, board.Cols)
		}
		if _, err := state.Apply(action); err != nil {
			t.Fatalf("%dx%d: illegal decision: %v", board.Rows, board.Cols, err)
		}
		if telemetry.Nodes == 0 {
			t.Fatalf("%dx%d: no node telemetry", board.Rows, board.Cols)
		}
	}
}

func TestTelemetryIncumbentComparison(t *testing.T) {
	report, err := CompareTelemetry(
		[]Board{{Rows: 5, Cols: 5}}, 1, TelemetryTournament(2),
		func(uint64) TelemetryAgent { return Instrument(BaseAttacker) },
	)
	if err != nil {
		t.Fatal(err)
	}
	if report.Games != 2 || report.Illegal != 0 || report.Stalled != 0 || report.Maxed != 0 {
		t.Fatalf("incomplete comparison: %s", report)
	}
	if report.Nodes == 0 || report.CompletedTurnDepth == 0 || len(report.Latencies) == 0 {
		t.Fatalf("missing telemetry: %s", report)
	}
}

func snapshotFingerprint(t *testing.T, state game.State) string {
	t.Helper()
	encoded, err := json.Marshal(state.Snapshot())
	if err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256(encoded)
	return hex.EncodeToString(sum[:8])
}
