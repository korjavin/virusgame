package arena

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"virusgame/game"
)

func TestAllCapturedProductionGamesReplayWithDistinctOutcomes(t *testing.T) {
	expected := map[string]string{
		"2cccdb97-8f6b-456d-bc76-a97933cc9cd6": "no_moves",
		"02cfddec-8a18-4522-b99c-092ed8022393": "no_moves",
		"58b2b9ab-30b4-45e8-bb32-4600300cac55": "no_moves",
		"2382b58f-c790-4131-b2a2-c200b3cf5e30": "illegal_move",
		"47e5ba48-cf37-43e2-b364-efd97ce5f233": "no_moves",
		"ea94a51b-63e9-4c0a-8c36-154a4722eb01": "no_moves",
		"4d85f7c0-d314-4dce-b3f0-bb7b169c30ef": "no_moves",
		"419c231b-7e0e-4df9-9bba-7871f758f019": "resignation",
		"fb5b584f-790d-45ca-9351-a4925010b998": "no_moves",
		"0c6bf57b-a602-4e2c-87c5-a2fff5de1dff": "no_moves",
		"fd6627c8-3d46-408d-bd48-17f081e1113b": "no_moves",
		"3d739acb-0635-4784-9d62-c076788f28be": "no_moves",
		"e854f8aa-4fc2-4be7-8348-9ae72cdef4d6": "no_moves",
		"e7b2f1d4-a68f-4f4c-b581-bac7c8a0c380": "no_moves",
		"6bf1f3aa-aee6-40d8-aa52-011b07a56d07": "no_moves",
		"4558d2fe-c22f-4940-8012-8f4f43fac728": "no_moves",
		"913c33f7-1f0c-41ce-9cee-65d3d9688073": "no_moves",
		"550cfd27-6c5c-48a2-928c-c36354f9db87": "no_moves",
		"836204cc-7c0d-4d9c-ace3-8aae41fd5e8c": "illegal_move",
		// vs-ai2.30: real 12x12 no_moves bot losses imported as frozen replays.
		"323e3f1f-d1a9-4534-973c-5df2fd4302b4": "no_moves",
		"86ecfde5-f313-4bec-bd6e-9a2e79f46227": "no_moves",
		"6ad8b536-4f29-4c41-9b7e-fa5eb350f545": "no_moves",
		"c3f39595-38f2-46a0-891e-2ca7efce4655": "no_moves",
		"b2ef469b-470c-41d5-ac6a-5e3e2ded7776": "no_moves",
		"6eaa8f07-719a-4663-aa11-90f3a9465b3a": "no_moves",
		"99796a56-2238-4408-851d-4c548d3d3a44": "no_moves",
		"6c8513fe-3c07-4471-9ea6-cc3da798bcc9": "no_moves",
		// vs-ai2.40: two more real 12x12 no_moves bot losses frozen as regression anchors.
		"b543fe02-f760-4d2c-9deb-d43b66fd061b": "no_moves",
		"bbfc5e0c-bf9f-44b3-b6b8-6af57f32e7ce": "no_moves",
	}
	fixtures, err := filepath.Glob("testdata/*.json")
	if err != nil {
		t.Fatal(err)
	}
	for _, path := range fixtures {
		base := filepath.Base(path)
		if base == "production-motifs-v1.json" || !strings.HasPrefix(base, "production-") && !strings.Contains(base, "happyotter97-") {
			continue
		}
		fixture, err := os.Open(path)
		if err != nil {
			t.Fatal(err)
		}
		replay, states, decodeErr := DecodeReplay(fixture)
		fixture.Close()
		if decodeErr != nil {
			t.Fatalf("%s: %v", path, decodeErr)
		}
		want, ok := expected[replay.SourceID]
		if !ok || replay.Termination != want || len(states) != len(replay.Turns) {
			t.Fatalf("unexpected fixture %s: replay=%+v states=%d", path, replay, len(states))
		}
		delete(expected, replay.SourceID)
		last := states[len(replay.Turns)]
		if replay.SourceID == "4d85f7c0-d314-4dce-b3f0-bb7b169c30ef" && (replay.ObservedTurns != 9 || replay.OmittedMoves == 0 || len(replay.Turns) != 8) {
			t.Fatalf("post-terminal source suffix was not preserved honestly: %+v", replay)
		}
		if replay.Termination == "no_moves" && (!last.GameOver() || last.Winner() != replay.Winner) {
			t.Fatalf("%s no_moves is not an authoritative terminal", path)
		}
		if replay.Termination == "illegal_move" && last.GameOver() {
			t.Fatalf("%s fabricated a strategic terminal for illegal_move", path)
		}
	}
	if len(expected) != 0 {
		t.Fatalf("missing production fixtures: %v", expected)
	}
}

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

func TestPostFix12x12ReplayTerminalHashes(t *testing.T) {
	want := map[string]string{
		"fd6627c8-3d46-408d-bd48-17f081e1113b": "291f48c21aeb8106",
		"3d739acb-0635-4784-9d62-c076788f28be": "201eb49d281fe6bd",
		"e854f8aa-4fc2-4be7-8348-9ae72cdef4d6": "f896a3f1e15cbeaa",
		"e7b2f1d4-a68f-4f4c-b581-bac7c8a0c380": "dc6a7e13e32ae665",
		"6bf1f3aa-aee6-40d8-aa52-011b07a56d07": "65f9fb65d6b859f3",
		"4558d2fe-c22f-4940-8012-8f4f43fac728": "fd4079756008f119",
		"913c33f7-1f0c-41ce-9cee-65d3d9688073": "de785595fcd685a1",
		"550cfd27-6c5c-48a2-928c-c36354f9db87": "b3f60b083b26a10b",
		"836204cc-7c0d-4d9c-ace3-8aae41fd5e8c": "0a7e17fab1d13ae0",
	}
	paths, err := filepath.Glob("testdata/production-12x12-*.json")
	if err != nil {
		t.Fatal(err)
	}
	for _, path := range paths {
		fixture, err := os.Open(path)
		if err != nil {
			t.Fatal(err)
		}
		replay, states, decodeErr := DecodeReplay(fixture)
		fixture.Close()
		fingerprint, selected := want[replay.SourceID]
		if !selected {
			continue
		}
		if decodeErr != nil || replay.Rows != 12 || replay.Cols != 12 {
			t.Fatalf("%s: replay=%+v err=%v", path, replay, decodeErr)
		}
		if got := snapshotFingerprint(t, states[len(replay.Turns)]); got != fingerprint {
			t.Fatalf("%s final hash=%s, want %s", replay.SourceID, got, fingerprint)
		}
		delete(want, replay.SourceID)
	}
	if len(want) != 0 {
		t.Fatalf("missing post-fix fixtures: %v", want)
	}
}

func TestDecodeReplayRejectsDivergence(t *testing.T) {
	for _, fixture := range []string{
		`{"rows":5,"cols":5,"winner":1,"turns":[{"turn":1,"player":2,"actions":[]}]}`,
		`{"rows":5,"cols":5,"winner":1,"turns":[{"turn":1,"player":1,"actions":[{"kind":"move","row":4,"col":4}]}]}`,
		`{"rows":5,"cols":5,"winner":1,"turns":[]} trailing`,
		`{"rows":5,"cols":5,"winner":1,"termination":"invented","turns":[]}`,
	} {
		if _, _, err := DecodeReplay(strings.NewReader(fixture)); err == nil {
			t.Fatalf("accepted divergent replay: %s", fixture)
		}
	}
}

func TestVariableRectangularDecisionMatrix(t *testing.T) {
	competitive := []Board{
		{Rows: 5, Cols: 5}, {Rows: 5, Cols: 10}, {Rows: 10, Cols: 5},
		{Rows: 8, Cols: 8}, {Rows: 10, Cols: 10}, {Rows: 15, Cols: 20},
		{Rows: 25, Cols: 25}, {Rows: 30, Cols: 30},
	}
	stress := []Board{
		{Rows: 50, Cols: 50},
		{Rows: 5, Cols: 50}, {Rows: 50, Cols: 5},
	}
	agent := TelemetryTournament(1)
	for name, boards := range map[string][]Board{"competitive": competitive, "max_dimension_stress": stress} {
		t.Run(name, func(t *testing.T) {
			report, err := Probe(boards, agent)
			if err != nil {
				t.Fatal(err)
			}
			if report.Games != len(boards) || report.Decisions != len(boards) || report.Illegal != 0 || report.Stalled != 0 || report.Nodes == 0 {
				t.Fatalf("decision probe failed: %s", report)
			}
		})
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
