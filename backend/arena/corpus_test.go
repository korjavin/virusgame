package arena

import (
	"os"
	"reflect"
	"strconv"
	"strings"
	"testing"

	"virusgame/game"
)

func TestFrozenCorpusCoverageAndChecksums(t *testing.T) {
	corpus := loadTestCorpus(t)
	wantCompetitive := map[string]bool{"5x5": true, "8x8": true, "10x10": true, "12x12": true, "15x20": true, "20x15": true, "20x20": true}
	for _, split := range []string{"train", "heldout"} {
		competitive, multiplayer, stress := map[string]bool{}, map[string]bool{}, map[string]bool{}
		competitivePlayers, competitiveMoves := map[string]map[game.Player]bool{}, map[string]map[int]bool{}
		competitivePhases, competitiveStrata, competitiveTrajectories := map[string]map[string]bool{}, map[string]map[string]bool{}, map[string]map[string]bool{}
		players, moves, phases, strata := map[game.Player]bool{}, map[int]bool{}, map[string]bool{}, map[string]bool{}
		trajectories := map[string]bool{}
		for _, testCase := range corpus.Cases {
			if testCase.Split != split {
				continue
			}
			switch testCase.Track {
			case "competitive_1v1":
				key := boardKey(testCase.State)
				competitive[key] = true
				if competitivePlayers[key] == nil {
					competitivePlayers[key], competitiveMoves[key] = map[game.Player]bool{}, map[int]bool{}
					competitivePhases[key], competitiveStrata[key], competitiveTrajectories[key] = map[string]bool{}, map[string]bool{}, map[string]bool{}
				}
				competitivePlayers[key][testCase.State.CurrentPlayer()] = true
				competitiveMoves[key][testCase.State.MovesLeft()] = true
				competitivePhases[key][testCase.Phase] = true
				competitiveTrajectories[key][testCase.Trajectory] = true
				for _, stratum := range testCase.Strata {
					competitiveStrata[key][stratum] = true
				}
			case "multiplayer":
				multiplayer[boardKey(testCase.State)+"p"+strconv.Itoa(testCase.Players)] = true
			case "stress":
				stress[boardKey(testCase.State)] = true
			}
			players[testCase.State.CurrentPlayer()] = true
			moves[testCase.State.MovesLeft()] = true
			phases[testCase.Phase] = true
			trajectories[testCase.Trajectory] = true
			for _, stratum := range testCase.Strata {
				strata[stratum] = true
			}
			if rebuilt, err := game.FromSnapshot(testCase.State.Snapshot()); err != nil || !reflect.DeepEqual(rebuilt.Snapshot(), testCase.State.Snapshot()) {
				t.Fatalf("%s failed exact snapshot validation: %v", testCase.ID, err)
			}
		}
		if !reflect.DeepEqual(competitive, wantCompetitive) || !multiplayer["12x12p3"] || !multiplayer["20x20p4"] || !multiplayer["28x28p3"] || !multiplayer["28x28p4"] || !stress["25x25"] || !stress["30x30"] || len(players) != 4 || len(moves) != 3 || !phases["opening"] || !strata["neutral_available"] || !strata["tactical"] || !strata["base_threat"] || len(trajectories) != 20 {
			t.Fatalf("split %s incomplete: competitive=%v multiplayer=%v stress=%v players=%v moves=%v phases=%v strata=%v trajectories=%d", split, competitive, multiplayer, stress, players, moves, phases, strata, len(trajectories))
		}
		for board := range wantCompetitive {
			if len(competitivePlayers[board]) != 2 || len(competitiveMoves[board]) != 3 || len(competitiveTrajectories[board]) != 2 || !competitivePhases[board]["opening"] || !competitivePhases[board]["contact_consolidation"] || !competitivePhases[board]["tactical_base_threat"] || !competitiveStrata[board]["tactical"] || !competitiveStrata[board]["base_threat"] || !competitiveStrata[board]["consolidation_candidate"] {
				t.Fatalf("split %s board %s lacks coverage: players=%v moves=%v trajectories=%v phases=%v strata=%v", split, board, competitivePlayers[board], competitiveMoves[board], competitiveTrajectories[board], competitivePhases[board], competitiveStrata[board])
			}
		}
	}
	if corpus.GroupHashes["train"] != "28c65702a6a9664a609465bd14316fe588acfc7749c47d560c0e27527e53edeb" || corpus.GroupHashes["heldout"] != "063a143c57dfbdbce0a64af60303132650dabad57f432260d8389b39aa4d5529" {
		t.Fatalf("unexpected frozen group hashes: %v", corpus.GroupHashes)
	}
}

func TestCorpusComparisonUsesIdenticalSnapshotsAndReportsBuckets(t *testing.T) {
	corpus := loadTestCorpus(t)
	corpus.Cases = corpus.Cases[:1]
	factory := func() TelemetryAgent { return Instrument(Greedy) }
	a, err := CompareCorpus(corpus, corpus.Cases[0].Split, factory, factory)
	if err != nil {
		t.Fatal(err)
	}
	b, err := CompareCorpus(corpus, corpus.Cases[0].Split, factory, factory)
	if err != nil {
		t.Fatal(err)
	}
	stripTiming := func(report *CorpusReport) {
		report.Overall.Latencies, report.Overall.Elapsed = nil, 0
		for key, bucket := range report.Buckets {
			bucket.Latencies, bucket.Elapsed = nil, 0
			report.Buckets[key] = bucket
		}
	}
	stripTiming(&a)
	stripTiming(&b)
	if !reflect.DeepEqual(a, b) || a.Overall.Games != 2 || len(a.Buckets) < 4 || a.Overall.Illegal != 0 || a.Overall.Stalled != 0 || a.Overall.Maxed != 0 {
		t.Fatalf("non-deterministic or incomplete paired report: %+v / %+v", a, b)
	}
}

func TestWilson95(t *testing.T) {
	got := Wilson95(80, 100)
	if got.Low < 71.1 || got.Low > 71.2 || got.High < 86.6 || got.High > 86.7 {
		t.Fatalf("Wilson95(80,100)=%+v", got)
	}
}

func TestCorpusRejectsTamperedCheckpoint(t *testing.T) {
	data, err := os.ReadFile("testdata/strength-corpus-v1.json")
	if err != nil {
		t.Fatal(err)
	}
	marker := `"hash": "`
	index := strings.Index(string(data), marker)
	if index < 0 {
		t.Fatal("fixture has no checkpoint hash")
	}
	start := index + len(marker)
	tampered := append([]byte(nil), data...)
	for i := start; i < start+64; i++ {
		tampered[i] = '0'
	}
	if _, err := DecodeCorpus(strings.NewReader(string(tampered))); err == nil {
		t.Fatal("accepted tampered frozen checkpoint")
	}
}

func loadTestCorpus(t *testing.T) Corpus {
	t.Helper()
	fixture, err := os.Open("testdata/strength-corpus-v1.json")
	if err != nil {
		t.Fatal(err)
	}
	defer fixture.Close()
	corpus, err := DecodeCorpus(fixture)
	if err != nil {
		t.Fatal(err)
	}
	return corpus
}

func boardKey(state game.State) string {
	return strconv.Itoa(state.Rows()) + "x" + strconv.Itoa(state.Cols())
}
