package arena

import (
	"os"
	"testing"

	"virusgame/game"
)

// prodLossAnchors are the two vs-ai2.40 regression anchors: real 12x12 no_moves
// losses (the human wins in seat 1; the bot in seat 2 is strangled) frozen as
// testdata via last_games. They pin the failure mode so a rules/replay change
// cannot silently move it. We deliberately do NOT assert the bot's exact moves —
// the eval will change — only that the games load, terminate as recorded, and
// that the bot's early play is a detectable width-1 diagonal tendril.
var prodLossAnchors = []string{
	"b543fe02-f760-4d2c-9deb-d43b66fd061b",
	"bbfc5e0c-bf9f-44b3-b6b8-6af57f32e7ce",
}

// anchorTerminalFingerprints pins the reconstructed terminal position of each
// anchor (position, not moves — robust to eval changes, sensitive to rules or
// replay drift).
var anchorTerminalFingerprints = map[string]string{
	"b543fe02-f760-4d2c-9deb-d43b66fd061b": "24cb6f2863ecc238",
	"bbfc5e0c-bf9f-44b3-b6b8-6af57f32e7ce": "e7a89193e50e0838",
}

// botTurnCells returns the move targets the bot (player 2) placed on the given
// turn numbers. Neutral placements are ignored.
func botTurnCells(replay Replay, turns ...int) []game.Pos {
	want := map[int]bool{}
	for _, turn := range turns {
		want[turn] = true
	}
	var cells []game.Pos
	for _, turn := range replay.Turns {
		if turn.Player != 2 || !want[turn.Number] {
			continue
		}
		for _, move := range turn.Actions {
			if move.Kind == "move" {
				cells = append(cells, game.Pos{Row: move.Row, Col: move.Col})
			}
		}
	}
	return cells
}

// longestThinDiagonal returns the length of the longest chain of the given cells
// lying on one width-1 diagonal: cells collinear on a 45-degree line (constant
// row-col or row+col) with consecutive rows and no lateral thickness. A long
// thin diagonal is the strangulation tendril the frozen losses show the bot
// building — a single-cell-wide reach the opponent cuts at one joint.
func longestThinDiagonal(cells []game.Pos) int {
	best := 0
	for _, anti := range []bool{false, true} {
		rowsByDiag := map[int]map[int]bool{}
		for _, cell := range cells {
			key := cell.Row - cell.Col
			if anti {
				key = cell.Row + cell.Col
			}
			if rowsByDiag[key] == nil {
				rowsByDiag[key] = map[int]bool{}
			}
			rowsByDiag[key][cell.Row] = true
		}
		for _, rows := range rowsByDiag {
			for start := range rows {
				run := 0
				for row := start; rows[row]; row++ {
					run++
				}
				if run > best {
					best = run
				}
			}
		}
	}
	return best
}

// TestProdLossAnchorsAreFrozenTendrils asserts each anchor loads through the
// authoritative rules, terminates as the recorded no_moves human win with the
// bot eliminated, pins a stable terminal fingerprint, and exhibits the width-1
// diagonal tendril across the bot's turn-2 and turn-4 placements.
func TestProdLossAnchorsAreFrozenTendrils(t *testing.T) {
	for _, id := range prodLossAnchors {
		path := "testdata/production-12x12-no-moves-" + id + ".json"
		fixture, err := os.Open(path)
		if err != nil {
			t.Fatalf("%s: %v", id, err)
		}
		replay, states, err := DecodeReplay(fixture)
		fixture.Close()
		if err != nil {
			t.Fatalf("%s: decode: %v", id, err)
		}
		if replay.Rows != 12 || replay.Cols != 12 || replay.Termination != "no_moves" || replay.Winner != 1 {
			t.Fatalf("%s: not a 12x12 no_moves human win: %+v", id, replay)
		}
		final := states[len(replay.Turns)]
		if !final.GameOver() || final.Winner() != 1 || final.Active(2) {
			t.Fatalf("%s: final not a bot elimination: over=%v winner=%d botActive=%v",
				id, final.GameOver(), final.Winner(), final.Active(2))
		}
		if got := snapshotFingerprint(t, final); got != anchorTerminalFingerprints[id] {
			t.Fatalf("%s terminal fingerprint=%s, want %s", id, got, anchorTerminalFingerprints[id])
		}
		if length := longestThinDiagonal(botTurnCells(replay, 2, 4)); length < 4 {
			t.Fatalf("%s: bot turn-2/turn-4 tendril not detected: longest thin diagonal = %d (<4)", id, length)
		}
	}
}

// TestProdLossAnchorReplayDiagnostic is an opt-in walkthrough of each anchor: it
// logs the bot's early tendril and terminal fingerprint for inspection without
// asserting anything eval-dependent.
//
//	VS_ANCHOR_REPLAY=1 go test ./arena -run TestProdLossAnchorReplayDiagnostic -v
func TestProdLossAnchorReplayDiagnostic(t *testing.T) {
	if os.Getenv("VS_ANCHOR_REPLAY") != "1" {
		t.Skip("set VS_ANCHOR_REPLAY=1 to run the anchor replay diagnostic")
	}
	for _, id := range prodLossAnchors {
		fixture, err := os.Open("testdata/production-12x12-no-moves-" + id + ".json")
		if err != nil {
			t.Fatalf("%s: %v", id, err)
		}
		replay, states, err := DecodeReplay(fixture)
		fixture.Close()
		if err != nil {
			t.Fatalf("%s: decode: %v", id, err)
		}
		final := states[len(replay.Turns)]
		t.Logf("%s: %v vs %v, %d turns, winner=%d termination=%s",
			id, replay.Players[0], replay.Players[1], len(replay.Turns), replay.Winner, replay.Termination)
		t.Logf("  bot turn-2/turn-4 cells=%v thin-diagonal-run=%d",
			botTurnCells(replay, 2, 4), longestThinDiagonal(botTurnCells(replay, 2, 4)))
		t.Logf("  terminal fingerprint=%s", snapshotFingerprint(t, final))
	}
}
