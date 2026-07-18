package arena

import (
	"os"
	"testing"

	"virusgame/game"
	"virusgame/search"
)

// vs-ai2.57 en-prise placement gate.
//
// SYMPTOM (owner, 2026-07-18): from a won position the bot repeatedly placed
// non-fortified cells next to the owner's base; the owner captured-and-fortified
// each one, building a fortified wall until the advantage inverted. Harvested as
// owner-corpus game 5efcac1a — turns 26..38 march cells to (0,1),(0,2),(0,3),
// (1,4),(2,5),(2,6), one Chebyshev step from the owner's base, all captured and
// fortified (see TestEnPriseMarchDiagnostic).
//
// ROOT CAUSE (not the filed hypothesis): the losing game was played by build
// 0488b07, which ran the PRE-SPSA hand-tuned vector (ThreatenedMult=1,
// BaseThreat=650). Its huge base-attack reward lures the bot to throw cells at
// the enemy base; ThreatenedMult=1 is far too small to counter it. The
// vs-ai2.52 SPSA vector now on main (BaseThreat=76, ThreatenedMult=0) does NOT
// make the blunder — it plays safe at every march turn — so restoring a
// threatened floor on top of it is a pure no-op (verified: f0==f1==f2 choices).
// The fix that mattered shipped in af1bc2d; this gate pins that it stays fixed.
//
// The gate is deterministic (equal-node budget). It asserts the production
// default eval declines the near-base en-prise gift at the anchor blunder turn,
// and that the pre-SPSA vector took it — so a future eval regression that
// re-tilts toward base-throwing fails here.

const enPriseAnchor = "5efcac1a-e467-4d97-ad90-d7f904880db1"

// enPriseBudget reaches depth ~2-4 on this mature 12x12 — the same shallow
// horizon production gets on a crowded mid-game board under its 1s deadline.
const enPriseBudget = 30_000

// preSPSAVector is the hand-tuned eval build 0488b07 ran when it threw the game
// (now kept only as evalParamsMulti for >2 players). Frozen here as the
// regression contrast, not a production path.
var preSPSAVector = search.EvalParams{
	Connected: 10, Normal: 30, Fortified: 6, Mobility: 1, Captures: 1,
	Disconnected: 1, BaseExits: 180, BaseOpenings: 80, BaseAnchors: 240,
	BaseThreat: 650, ThreatenedLossMult: 1, ThreatenedMult: 1,
	SpaceRace: 32, SealedBasePenalty: 5000, NeutralUnusedBonus: 20,
	MovesLeftTempo: 12, PredatoryCutBase: 150, PredatoryCutLossDiv: 2,
}

// enPriseGiftsNearBase reports whether the bot's chosen action places an own
// Normal within Chebyshev 3 of the owner's base that the owner can capture next
// turn — the en-prise gift the symptom is made of.
func enPriseGiftsNearBase(t *testing.T, pre game.State, p search.EvalParams) (game.Pos, bool) {
	t.Helper()
	search.SetEvalParams(p)
	res, ok := search.ChooseNodeBudget(pre, enPriseBudget)
	if !ok {
		t.Fatal("bot produced no move")
	}
	after, err := pre.Apply(res.Action)
	if err != nil {
		t.Fatalf("apply chosen action: %v", err)
	}
	base1 := basePosition(pre, 1)
	tgt := res.Action.Target
	cell, _ := after.At(tgt)
	gift := cell.Owner == 2 && cell.Kind == game.Normal && chebyshev(tgt, base1) <= 3 && capturableAt(after, tgt, 2, 1)
	return tgt, gift
}

// capturableAt reports whether victim's Normal at pos is adjacent to attacker's
// connected territory (capturable next turn) — the same signal m.threatened uses.
func capturableAt(state game.State, pos game.Pos, victim, attacker game.Player) bool {
	cell, _ := state.At(pos)
	if cell.Owner != victim || cell.Kind != game.Normal {
		return false
	}
	amask := connectedMask(state, attacker)
	rows, cols := state.Rows(), state.Cols()
	for dr := -1; dr <= 1; dr++ {
		for dc := -1; dc <= 1; dc++ {
			n := game.Pos{Row: pos.Row + dr, Col: pos.Col + dc}
			if n.Row < 0 || n.Row >= rows || n.Col < 0 || n.Col >= cols {
				continue
			}
			if amask[n.Row*cols+n.Col] {
				return true
			}
		}
	}
	return false
}

func enPriseAnchorPre(t *testing.T, turn int) game.State {
	t.Helper()
	fixture, err := os.Open("testdata/production-12x12-no-moves-" + enPriseAnchor + ".json")
	if err != nil {
		t.Fatal(err)
	}
	defer fixture.Close()
	replay, _, err := DecodeReplay(fixture)
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	positions, err := ReplayPositions(replay)
	if err != nil {
		t.Fatalf("positions: %v", err)
	}
	pre, ok := positions[ReplayPoint{Turn: turn}]
	if !ok || pre.CurrentPlayer() != 2 {
		t.Fatalf("turn %d is not a bot-to-move position", turn)
	}
	return pre
}

// TestEnPriseGate is the standing regression gate. At the anchor blunder turn
// (T32, where the recorded pre-SPSA bot played the en-prise (0,3)), the current
// production eval must decline the near-base gift, and the pre-SPSA vector must
// take it — so the gate both proves the fix and fails if a future eval regresses.
func TestEnPriseGate(t *testing.T) {
	const blunderTurn = 32
	defer search.SetEvalParams(search.DefaultEvalParams())
	pre := enPriseAnchorPre(t, blunderTurn)

	prodTgt, prodGift := enPriseGiftsNearBase(t, pre, search.DefaultEvalParams())
	if prodGift {
		t.Errorf("production eval gifts en-prise cell %v near owner base at T%d (regression: bot is base-throwing again)", prodTgt, blunderTurn)
	}

	oldTgt, oldGift := enPriseGiftsNearBase(t, pre, preSPSAVector)
	if !oldGift {
		t.Errorf("pre-SPSA vector chose %v (no near-base gift) — gate lost its regression contrast; re-anchor it", oldTgt)
	}
	t.Logf("T%d: production=%v (gift=%v), pre-SPSA=%v (gift=%v)", blunderTurn, prodTgt, prodGift, oldTgt, oldGift)
}

// TestEnPriseMarchDiagnostic dumps the recorded gifted-fortified march near the
// owner base for every bot turn of the anchor. Opt-in:
//
//	VS_ENPRISE=1 go test ./arena -run TestEnPriseMarchDiagnostic -v
func TestEnPriseMarchDiagnostic(t *testing.T) {
	if os.Getenv("VS_ENPRISE") != "1" {
		t.Skip("set VS_ENPRISE=1 to run the en-prise march diagnostic")
	}
	fixture, err := os.Open("testdata/production-12x12-no-moves-" + enPriseAnchor + ".json")
	if err != nil {
		t.Fatal(err)
	}
	replay, _, err := DecodeReplay(fixture)
	fixture.Close()
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	positions, _ := ReplayPositions(replay)
	final := positions[ReplayPoint{Turn: len(replay.Turns), AfterActions: len(replay.Turns[len(replay.Turns)-1].Actions)}]
	base1 := basePosition(final, 1)
	gifted := 0
	for _, turn := range replay.Turns {
		if turn.Player != 2 {
			continue
		}
		for ai, mv := range turn.Actions {
			if mv.Kind != "move" {
				continue
			}
			tgt := game.Pos{Row: mv.Row, Col: mv.Col}
			after := positions[ReplayPoint{Turn: turn.Number, AfterActions: ai + 1}]
			if c, _ := after.At(tgt); c.Owner != 2 {
				continue // a capture, not an own placement
			}
			fc, _ := final.At(tgt)
			if fc.Owner == 1 && fc.Kind == game.Fortified && chebyshev(tgt, base1) <= 6 {
				gifted++
				t.Logf("T%02d place %v d=%d -> owner-fortified wall", turn.Number, tgt, chebyshev(tgt, base1))
			}
		}
	}
	t.Logf("%s: %d bot placements near owner base ended as owner-fortified wall", enPriseAnchor, gifted)
}
