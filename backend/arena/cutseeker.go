package arena

import "virusgame/game"

// CutSeeker is a strangler baseline that drives a thin tendril toward the
// opponent and prizes cutting the opponent's mobility. Where MobilityAttacker
// only counts the opponent's immediate replies, CutSeeker weights that cut an
// order of magnitude harder and, on ties, advances whichever move gets closest
// to the opponent base — which from an empty board reproduces the width-1
// diagonal tendril (1,1/2,2/3,3 …) that the frozen production losses were beaten
// by. It is deterministic: stable board-order ties, no wall clock.
func CutSeeker(state game.State) (game.Action, bool) {
	actions := state.LegalActions()
	if len(actions) == 0 {
		return game.Action{}, false
	}
	actor := state.CurrentPlayer()
	best, bestScore := actions[0], -1<<60
	found := false
	for _, action := range actions {
		if action.Kind == game.PlaceNeutrals {
			continue
		}
		target, _ := state.At(action.Target)
		next, err := state.Apply(action)
		if err != nil {
			continue
		}
		score := 0
		// Only a completed turn (control has passed to the opponent) exposes the
		// opponent's reply count — the tighter that cut, the higher the score.
		if next.CurrentPlayer() != actor {
			score -= 1000 * len(next.LegalActions())
		}
		// Advance the tendril: the closer this cell sits to an opponent base, the
		// further the thin diagonal reaches around them.
		score -= 20 * opponentBaseDistance(state, actor, action.Target)
		if action.Kind == game.Move && target.Kind == game.Normal && target.Owner != actor {
			score += 50_000
		}
		if next.GameOver() && next.Winner() == actor {
			score += 1_000_000
		}
		if !found || score > bestScore {
			best, bestScore, found = action, score, true
		}
	}
	if !found {
		// Only neutral placements were legal; fall back to a stable choice.
		return actions[0], true
	}
	return best, true
}
