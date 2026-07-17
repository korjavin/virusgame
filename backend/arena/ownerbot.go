package arena

import "virusgame/game"

// OwnerBot is a scripted sparring agent distilled from the owner-loss corpus
// (testdata/owner-corpus.json; VS_OWNER_CORPUS=1 shows the attack patterns).
// The owner wins by relentless attacking (18-30 captures/game) while never
// over-extending — the restraint the naive red-team cutters lacked. Three
// behaviours, in the corpus games' own order of appearance:
//
//   - width-2 advancing front with an OWN-articulation guard: every move is
//     penalised by the number of cut points it leaves in our own base-connected
//     component, so the front advances two-wide (redundant paths => no single
//     severing cut) instead of as a width-1 tendril the bot's search out-cuts;
//   - articulation-head target selection vs the victim, reusing the CutSeeker
//     machinery (opponentArticulations / adjacentToCut): sever the victim's
//     structure at its cut points;
//   - corridor-riding: a capture that extends one of our own captured corridors
//     (adjoins a fortified stone) and march toward the nearest enemy base, so
//     once the front punches into the enemy it keeps rolling to the base.
//
// It plays only Move actions — neutral placements consume the whole turn and
// convert our own stones, pure self-harm for a relentless attacker. Deterministic
// (stable board-order ties, no wall clock) and always legal.
func OwnerBot(state game.State) (game.Action, bool) {
	actions := state.LegalActions()
	if len(actions) == 0 {
		return game.Action{}, false
	}
	actor := state.CurrentPlayer()

	// Victim's cut cells, computed once per decision (target selection).
	var cuts map[game.Pos]bool
	if victim, ok := nearestVictim(state, actor); ok {
		cuts = opponentArticulations(state, victim)
	}

	best, bestScore, found := actions[0], -1<<62, false
	for _, action := range actions {
		if action.Kind != game.Move {
			continue // neutrals are self-harm for a relentless attacker
		}
		target, _ := state.At(action.Target)
		next, err := state.Apply(action)
		if err != nil {
			continue
		}

		score := 0
		if next.GameOver() && next.Winner() == actor {
			score += 1_000_000_000 // finishing the strangle dominates everything
		}
		// Relentless capture: fortifies our stone and shrinks the victim.
		if target.Kind == game.Normal && target.Owner != actor {
			score += 200_000
			if adjacentToOwnFortified(state, actor, action.Target) {
				score += 60_000 // corridor-ride: keep the captured chain rolling in
			}
		}
		// Articulation-head targeting: sever the victim at its cut points.
		switch {
		case cuts[action.Target]:
			score += 120_000
		case adjacentToCut(action.Target, cuts):
			score += 40_000
		}
		// On a completed turn, prize the reply-starving line (drive to no_moves).
		if next.CurrentPlayer() != actor {
			score -= 300 * len(next.LegalActions())
		}
		// March the front toward the nearest enemy base.
		score -= 200 * opponentBaseDistance(state, actor, action.Target)
		// Own-articulation guard (the restraint): never leave our own structure
		// one-cut-losable. Penalise by the LARGEST chunk a single enemy cut would
		// sever from our base — a two-wide advance leaves 0, a width-1 tendril
		// leaves its whole tail. Mass, not count: a deep search targets the
		// catastrophic cut, not the trivial one.
		score -= 2_000 * ownCutRisk(next, actor)
		// Small own-mobility tiebreak keeps the front flexible.
		score += immediateMobility(next, actor)

		if !found || score > bestScore {
			best, bestScore, found = action, score, true
		}
	}
	if !found {
		return actions[0], true // only neutral placements were legal
	}
	return best, true
}

// ownCutRisk returns the size of the largest chunk of actor's base-connected
// component that a single non-base articulation cut would sever from the base —
// the "one-cut-losable" mass the owner never leaves exposed. It is a Tarjan
// low-link pass with subtree sizes over the 8-connected component; 0 for a
// two-wide front. Bases are never severable (they can't be captured), so the
// root is not counted. ponytail: O(cells) per call, same class as the ladder's
// existing opponentArticulations.
func ownCutRisk(state game.State, actor game.Player) int {
	base := basePosition(state, actor)
	if cell, ok := state.At(base); !ok || cell.Owner != actor {
		return 0
	}
	component := map[game.Pos]bool{base: true}
	queue := []game.Pos{base}
	for len(queue) > 0 {
		pos := queue[0]
		queue = queue[1:]
		for _, n := range neighbors8(state, pos) {
			if c, _ := state.At(n); c.Owner == actor && !component[n] {
				component[n] = true
				queue = append(queue, n)
			}
		}
	}

	disc := make(map[game.Pos]int, len(component))
	low := make(map[game.Pos]int, len(component))
	size := make(map[game.Pos]int, len(component))
	timer, worst := 0, 0
	var dfs func(pos, parent game.Pos, isRoot bool)
	dfs = func(pos, parent game.Pos, isRoot bool) {
		timer++
		disc[pos], low[pos], size[pos] = timer, timer, 1
		for _, n := range neighbors8(state, pos) {
			if !component[n] {
				continue
			}
			if _, seen := disc[n]; !seen {
				dfs(n, pos, false)
				size[pos] += size[n]
				if low[n] < low[pos] {
					low[pos] = low[n]
				}
				if !isRoot && low[n] >= disc[pos] && size[n] > worst {
					worst = size[n] // removing pos severs n's subtree from the base
				}
			} else if n != parent && disc[n] < low[pos] {
				low[pos] = disc[n]
			}
		}
	}
	dfs(base, game.Pos{}, true)
	return worst
}

// adjacentToOwnFortified reports whether pos adjoins a stone we have already
// captured (Fortified) — evidence of an in-progress capture corridor.
func adjacentToOwnFortified(state game.State, actor game.Player, pos game.Pos) bool {
	for _, n := range neighbors8(state, pos) {
		if c, _ := state.At(n); c.Owner == actor && c.Kind == game.Fortified {
			return true
		}
	}
	return false
}
