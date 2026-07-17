package arena

import (
	"testing"

	"virusgame/game"
)

// anchorExchangeReplay builds game 91ff20fd (owner LuckyEagle12 P1 beats Bot
// 4125 P2 on a 12x12 board, P2 eliminated). Every action is an engine "move"
// (the rules auto-decide place/capture/fortify). It is the grounded evidence for
// vs-ai2.47: at turns 6 and 8 the bot fortifies one enemy cell while the human's
// reply fortifies two of the bot's — a material-losing 1-for-2.
func anchorExchangeReplay() Replay {
	mv := func(cells ...[2]int) []ReplayMove {
		out := make([]ReplayMove, len(cells))
		for i, c := range cells {
			out[i] = ReplayMove{Kind: "move", Row: c[0], Col: c[1]}
		}
		return out
	}
	turns := []struct {
		player game.Player
		moves  []ReplayMove
	}{
		{1, mv([2]int{1, 1}, [2]int{2, 2}, [2]int{3, 3})},
		{2, mv([2]int{10, 10}, [2]int{9, 10}, [2]int{9, 9})},
		{1, mv([2]int{4, 4}, [2]int{3, 5}, [2]int{5, 3})},
		{2, mv([2]int{8, 8}, [2]int{7, 7}, [2]int{6, 8})},
		{1, mv([2]int{5, 5}, [2]int{6, 6}, [2]int{7, 7})},
		{2, mv([2]int{7, 9}, [2]int{5, 7}, [2]int{6, 6})},
		{1, mv([2]int{5, 6}, [2]int{5, 7}, [2]int{6, 8})},
		{2, mv([2]int{8, 7}, [2]int{7, 6}, [2]int{5, 5})},
		{1, mv([2]int{4, 5}, [2]int{7, 6}, [2]int{8, 8})},
		{2, mv([2]int{8, 10}, [2]int{9, 11}, [2]int{10, 11})},
		{1, mv([2]int{9, 9}, [2]int{10, 10}, [2]int{10, 11})},
		{2, mv([2]int{11, 10}, [2]int{10, 9}, [2]int{6, 9})},
		{1, mv([2]int{11, 10})},
	}
	replay := Replay{
		SourceID:    "91ff20fd",
		Players:     [2]string{"LuckyEagle12", "Bot 4125"},
		Rows:        12,
		Cols:        12,
		Winner:      1,
		Termination: "no_moves",
	}
	for i, t := range turns {
		replay.Turns = append(replay.Turns, ReplayTurn{Number: i + 1, Player: t.player, Actions: t.moves})
	}
	return replay
}

// TestAnchorExchangeEvidence is the vs-ai2.47 evidence gate: the real anchor
// game reconstructs exactly (fidelity: P1 wins, game over by no_moves) and the
// two 1-for-2 exchanges are present in the reconstruction. FAST — reconstruction
// only, no search. Bot is P2, human (winner) is P1.
func TestAnchorExchangeEvidence(t *testing.T) {
	replay := anchorExchangeReplay()
	positions, err := ReplayPositions(replay)
	if err != nil {
		t.Fatalf("reconstruct anchor: %v", err)
	}

	// Fidelity: the reconstruction must land on the recorded terminal result.
	final := positions[ReplayPoint{Turn: len(replay.Turns), AfterActions: len(replay.Turns[len(replay.Turns)-1].Actions)}]
	if !final.GameOver() || final.Winner() != 1 {
		t.Fatalf("anchor fidelity: got over=%v winner=%d, want over=true winner=1", final.GameOver(), final.Winner())
	}

	// Exchange pattern: over each (bot turn N + human reply N+1), the bot nets
	// +1 owned cell while the human nets +2 — the material-losing 1-for-2.
	owned := func(p ReplayPoint, player game.Player) int { return len(ownedCells(positions[p], player)) }
	for _, botTurn := range []int{6, 8} {
		before := ReplayPoint{Turn: botTurn}
		afterReply := ReplayPoint{Turn: botTurn + 1, AfterActions: len(replay.Turns[botTurn].Actions)}
		botNet := owned(afterReply, 2) - owned(before, 2)
		humanNet := owned(afterReply, 1) - owned(before, 1)
		t.Logf("turn %d exchange: bot(P2) net %+d, human(P1) net %+d", botTurn, botNet, humanNet)
		if botNet != 1 || humanNet != 2 {
			t.Errorf("turn %d: bot net %+d / human net %+d, want +1 / +2 (the 1-for-2)", botTurn, botNet, humanNet)
		}
	}
}
