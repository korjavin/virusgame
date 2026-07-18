package main

import (
	"bufio"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"virusgame/arena"
	"virusgame/game"
)

// TestRecordRoundTrip asserts a Record survives JSON marshal/unmarshal
// unchanged. Full in-process generation + determinism lands in Task 3.
func TestRecordRoundTrip(t *testing.T) {
	want := Record{
		SchemaVersion: schemaVersion,
		Fingerprint:   "deadbeefcafef00d",
		Position: Position{
			Cells: "ABCDE", Bases: []int{0, 63}, Active: []bool{true, true},
			NeutralUsed: []bool{false, true}, MovesLeft: 2, Winner: 0,
		},
		Rows:          8,
		Cols:          8,
		CurrentPlayer: 1,
		Features: [4][]float64{
			{1, 0, 2.5, 3, 0, 0, 1, 0, 0, 0, 0, 0, 1, 0, 0, 4, 0, 1, 2},
			{0, 1, 1, 0, 0, 0, 0, 1, 0, 0, 0, 0, 1, 0, 0, 3, 1, 0, 0},
			nil,
			nil,
		},
		DeepScore: -42,
		Budget:    20000,
		Outcome:   Outcome{Winner: 1, Placement: 1},
		Source:    "selfplay",
	}
	encoded, err := json.Marshal(want)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var got Record
	if err := json.Unmarshal(encoded, &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if !reflect.DeepEqual(want, got) {
		t.Fatalf("round-trip mismatch:\n want %+v\n got  %+v", want, got)
	}
}

// TestLabelSkipsNoDeepScore asserts Label surfaces errNoDeepScore (rather than
// silently recording DeepScore 0) when ChooseNodeBudget cannot score the
// position. Guards against mislabeling decided/terminal corpus positions.
func TestLabelSkipsNoDeepScore(t *testing.T) {
	state, err := game.New(8, 8, 2)
	if err != nil {
		t.Fatal(err)
	}
	// Drive a greedy game to a terminal state — the mover then has no legal
	// move, so ChooseNodeBudget returns ok=false (the corpus-terminal case).
	agent := arena.Greedy
	for i := 0; !state.GameOver() && i < 8*8*4; i++ {
		action, ok := agent(state)
		if !ok {
			break
		}
		next, err := state.Apply(action)
		if err != nil {
			t.Fatal(err)
		}
		state = next
	}
	if !state.GameOver() {
		t.Fatal("greedy game did not reach a terminal state")
	}
	if _, err := Label(state, 2000, "corpus"); !errors.Is(err, errNoDeepScore) {
		t.Fatalf("want errNoDeepScore for terminal position, got %v", err)
	}
}

// tinyConfig is a small, corpus-free run used for fast in-process assertions.
func tinyConfig(dir string) Config {
	return Config{
		Out:       dir,
		Workers:   1,
		Positions: 20,
		Budget:    200,
		Seed:      99,
		Boards:    []arena.Board{{Rows: 8, Cols: 8}},
	}
}

func TestGenerateRoundTrips(t *testing.T) {
	dir := t.TempDir()
	total, err := Generate(tinyConfig(dir))
	if err != nil {
		t.Fatal(err)
	}
	if total == 0 {
		t.Fatal("generated no positions")
	}
	lines := readShard(t, filepath.Join(dir, "shard-000.jsonl"))
	if len(lines) != total {
		t.Fatalf("shard has %d lines, Generate reported %d", len(lines), total)
	}
	seen := map[string]bool{}
	for i, line := range lines {
		var record Record
		if err := json.Unmarshal(line, &record); err != nil {
			t.Fatalf("line %d does not parse: %v", i, err)
		}
		if record.Fingerprint == "" || record.CurrentPlayer == 0 {
			t.Fatalf("line %d missing required fields: %+v", i, record)
		}
		if record.Features[record.CurrentPlayer-1] == nil {
			t.Fatalf("line %d: mover seat %d has no features", i, record.CurrentPlayer)
		}
		if seen[record.Fingerprint] {
			t.Fatalf("line %d: duplicate fingerprint %s", i, record.Fingerprint)
		}
		seen[record.Fingerprint] = true
	}
}

func TestGenerateDeterministic(t *testing.T) {
	a, b := t.TempDir(), t.TempDir()
	if _, err := Generate(tinyConfig(a)); err != nil {
		t.Fatal(err)
	}
	if _, err := Generate(tinyConfig(b)); err != nil {
		t.Fatal(err)
	}
	dataA, err := os.ReadFile(filepath.Join(a, "shard-000.jsonl"))
	if err != nil {
		t.Fatal(err)
	}
	dataB, err := os.ReadFile(filepath.Join(b, "shard-000.jsonl"))
	if err != nil {
		t.Fatal(err)
	}
	if string(dataA) != string(dataB) {
		t.Fatal("two same-seed runs produced different shard bytes")
	}
}

func TestResumeSkipsExisting(t *testing.T) {
	dir := t.TempDir()
	if _, err := Generate(tinyConfig(dir)); err != nil {
		t.Fatal(err)
	}
	before := len(readShard(t, filepath.Join(dir, "shard-000.jsonl")))
	cfg := tinyConfig(dir)
	cfg.Resume = true
	if _, err := Generate(cfg); err != nil {
		t.Fatal(err)
	}
	after := len(readShard(t, filepath.Join(dir, "shard-000.jsonl")))
	// Same seed re-samples the same fingerprints; resume must skip them all, so
	// the shard line count is unchanged.
	if after != before {
		t.Fatalf("resume added lines: before=%d after=%d", before, after)
	}
}

// TestResumeRepairsTornRecord simulates a killed writer: the shard's final
// record is left half-written (no trailing newline). Resume must repair it,
// scan cleanly, and append the missing record on a valid line boundary rather
// than aborting on the unparseable tail or concatenating onto the garbage.
func TestResumeRepairsTornRecord(t *testing.T) {
	dir := t.TempDir()
	if _, err := Generate(tinyConfig(dir)); err != nil {
		t.Fatal(err)
	}
	shard := filepath.Join(dir, "shard-000.jsonl")
	full := len(readShard(t, shard))

	// Drop the last record's trailing newline and half its bytes.
	data, err := os.ReadFile(shard)
	if err != nil {
		t.Fatal(err)
	}
	lastNL := -1
	for i := len(data) - 2; i >= 0; i-- { // -2 skips the file's final newline
		if data[i] == '\n' {
			lastNL = i
			break
		}
	}
	torn := data[:lastNL+1+((len(data)-lastNL-1)/2)] // keep a partial final record
	if err := os.WriteFile(shard, torn, 0o644); err != nil {
		t.Fatal(err)
	}

	cfg := tinyConfig(dir)
	cfg.Resume = true
	if _, err := Generate(cfg); err != nil {
		t.Fatalf("resume after torn record: %v", err)
	}
	lines := readShard(t, shard)
	if len(lines) != full {
		t.Fatalf("resume did not restore full shard: got %d lines, want %d", len(lines), full)
	}
	seen := map[string]bool{}
	for i, line := range lines {
		var record Record
		if err := json.Unmarshal(line, &record); err != nil {
			t.Fatalf("line %d does not parse after resume: %v", i, err)
		}
		if seen[record.Fingerprint] {
			t.Fatalf("line %d: duplicate fingerprint after resume: %s", i, record.Fingerprint)
		}
		seen[record.Fingerprint] = true
	}
}

// TestPositionRecomputesFeatures is the whole point of schema v2: the stored raw
// Position must reproduce the record's Features via the public game/arena API,
// so labels stay reusable when the extractor changes.
func TestPositionRecomputesFeatures(t *testing.T) {
	dir := t.TempDir()
	if _, err := Generate(tinyConfig(dir)); err != nil {
		t.Fatal(err)
	}
	lines := readShard(t, filepath.Join(dir, "shard-000.jsonl"))
	for i, line := range lines {
		var record Record
		if err := json.Unmarshal(line, &record); err != nil {
			t.Fatalf("line %d: %v", i, err)
		}
		if record.SchemaVersion != schemaVersion {
			t.Fatalf("line %d: schemaVersion = %d, want %d", i, record.SchemaVersion, schemaVersion)
		}
		state, err := game.FromSnapshot(record.toSnapshot())
		if err != nil {
			t.Fatalf("line %d: rebuild position: %v", i, err)
		}
		feats := arena.NNUEFeatures(state)
		for seat := 0; seat < 4; seat++ {
			var recomputed []float64
			if state.Active(game.Player(seat + 1)) {
				recomputed = feats[seat].Features()
			}
			if !reflect.DeepEqual(record.Features[seat], recomputed) {
				t.Fatalf("line %d seat %d: features not reproducible from stored position\n stored %v\n recomp %v",
					i, seat, record.Features[seat], recomputed)
			}
		}
	}
}

// TestRefusesSchemaMix asserts a v2 run refuses to append onto a directory that
// already holds a v1 shard (no schemaVersion field).
func TestRefusesSchemaMix(t *testing.T) {
	dir := t.TempDir()
	v1 := []byte(`{"fingerprint":"abc","rows":8,"cols":8,"currentPlayer":1,"source":"ladder"}` + "\n")
	if err := os.WriteFile(filepath.Join(dir, "shard-000.jsonl"), v1, 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := Generate(tinyConfig(dir)); err == nil {
		t.Fatal("expected refusal to mix v1 shard with v2 output, got nil error")
	}
}

func TestSmokeFixtureParses(t *testing.T) {
	lines := readShard(t, "testdata/smoke.jsonl")
	if len(lines) == 0 {
		t.Fatal("smoke fixture is empty")
	}
	sources := map[string]int{}
	for i, line := range lines {
		var record Record
		if err := json.Unmarshal(line, &record); err != nil {
			t.Fatalf("smoke line %d does not parse: %v", i, err)
		}
		sources[record.Source]++
	}
	// The committed fixture is generated with all three sources enabled.
	for _, want := range []string{"selfplay", "ladder", "corpus"} {
		if sources[want] == 0 {
			t.Errorf("smoke fixture missing source %q (have %v)", want, sources)
		}
	}
}

func readShard(t *testing.T, path string) [][]byte {
	t.Helper()
	file, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	var lines [][]byte
	scanner := bufio.NewScanner(file)
	scanner.Buffer(make([]byte, 0, 1<<20), 1<<24)
	for scanner.Scan() {
		lines = append(lines, append([]byte(nil), scanner.Bytes()...))
	}
	if err := scanner.Err(); err != nil {
		t.Fatal(err)
	}
	return lines
}
