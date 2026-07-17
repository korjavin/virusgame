package arena

import (
	"os"
	"path/filepath"
	"testing"
)

// TestOwnerLossCorpusAnchors asserts every harvested human win still loads
// through the authoritative rules, terminates as the recorded no_moves win with
// the bot (seat 2) eliminated, and pins its terminal fingerprint. Each entry is
// a proven bot hole; a rules or replay change that moves any of them fails here.
func TestOwnerLossCorpusAnchors(t *testing.T) {
	corpus, err := LoadOwnerCorpus(OwnerCorpusManifest)
	if err != nil {
		t.Fatal(err)
	}
	if len(corpus) == 0 {
		t.Fatal("owner-loss corpus is empty; run `go run ./arena/cmd/replayimport -fetch 20`")
	}
	for _, entry := range corpus {
		fixture, err := os.Open(filepath.Join("testdata", entry.FixtureName()))
		if err != nil {
			t.Fatalf("%s: %v", entry.SourceID, err)
		}
		replay, states, err := DecodeReplay(fixture)
		fixture.Close()
		if err != nil {
			t.Fatalf("%s: decode: %v", entry.SourceID, err)
		}
		if replay.Winner != 1 || replay.Termination != entry.Termination {
			t.Fatalf("%s: not the recorded human win: winner=%d termination=%s", entry.SourceID, replay.Winner, replay.Termination)
		}
		final := states[len(replay.Turns)]
		if entry.Termination == "no_moves" && (!final.GameOver() || final.Winner() != 1 || final.Active(2)) {
			t.Fatalf("%s: final not a bot elimination: over=%v winner=%d botActive=%v",
				entry.SourceID, final.GameOver(), final.Winner(), final.Active(2))
		}
		got, err := StateFingerprint(final)
		if err != nil {
			t.Fatal(err)
		}
		if got != entry.TerminalFingerprint {
			t.Fatalf("%s terminal fingerprint=%s, want %s", entry.SourceID, got, entry.TerminalFingerprint)
		}
	}
}

// TestOwnerLossCorpusDiagnostic is the standing owner-loss dashboard: it replays
// the corpus and prints, per game, how many attacking moves the bot made before
// it was strangled (the passivity metric) alongside the termination. Opt-in:
//
//	VS_OWNER_CORPUS=1 go test ./arena -run TestOwnerLossCorpusDiagnostic -v
func TestOwnerLossCorpusDiagnostic(t *testing.T) {
	if os.Getenv("VS_OWNER_CORPUS") != "1" {
		t.Skip("set VS_OWNER_CORPUS=1 to run the owner-loss corpus diagnostic")
	}
	corpus, err := LoadOwnerCorpus(OwnerCorpusManifest)
	if err != nil {
		t.Fatal(err)
	}
	totalAttacks, totalTurns := 0, 0
	for _, entry := range corpus {
		fixture, err := os.Open(filepath.Join("testdata", entry.FixtureName()))
		if err != nil {
			t.Fatalf("%s: %v", entry.SourceID, err)
		}
		replay, _, err := DecodeReplay(fixture)
		fixture.Close()
		if err != nil {
			t.Fatalf("%s: decode: %v", entry.SourceID, err)
		}
		attacks := botAttackMoves(replay)
		totalAttacks += attacks
		totalTurns += len(replay.Turns)
		t.Logf("%s: %s beat %s | %s | %d turns | bot attack-moves=%d",
			entry.SourceID, replay.Players[0], replay.Players[1], replay.Termination, len(replay.Turns), attacks)
	}
	t.Logf("corpus: %d games, %d bot attack-moves over %d turns (mean %.2f attacks/game)",
		len(corpus), totalAttacks, totalTurns, float64(totalAttacks)/float64(len(corpus)))
}

// botAttackMoves counts the bot's (seat 2) attacking placements across the game.
func botAttackMoves(replay Replay) int {
	count := 0
	for _, turn := range replay.Turns {
		if turn.Player != 2 {
			continue
		}
		for _, move := range turn.Actions {
			if move.Kind == "move" {
				count++
			}
		}
	}
	return count
}
