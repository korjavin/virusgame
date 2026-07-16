package arena

import "virusgame/game"

// MobilityBaseAttacker mixes MobilityAttacker's reply-starving score with
// BaseAttacker's base pressure: strangle the opponent's mobility while
// marching on the nearest opponent base, preferring captures. Cheap, no
// search; ties resolve in stable board order.
func MobilityBaseAttacker(state game.State) (game.Action, bool) {
	actions := state.LegalActions()
	if len(actions) == 0 {
		return game.Action{}, false
	}
	actor := state.CurrentPlayer()
	best, bestScore := actions[0], -1<<60
	for _, action := range actions {
		next, err := state.Apply(action)
		if err != nil {
			continue
		}
		score := immediateMobility(next, actor)
		if next.CurrentPlayer() != actor {
			score -= 50 * len(next.LegalActions())
		}
		if action.Kind == game.Move {
			target, _ := state.At(action.Target)
			if target.Kind == game.Normal && target.Owner != actor {
				score += 10_000
			}
			score -= 25 * opponentBaseDistance(state, actor, action.Target)
		}
		if next.GameOver() && next.Winner() == actor {
			score += 1_000_000
		}
		if score > bestScore {
			best, bestScore = action, score
		}
	}
	return best, true
}

// CutSeeker attacks the articulation points of the nearest opponent's
// base-connected territory: capturing a cut cell (or landing next to one)
// scores highest, otherwise it falls back to MobilityAttacker-style
// mobility scoring. Cheap heuristic only; ties resolve in board order.
func CutSeeker(state game.State) (game.Action, bool) {
	actions := state.LegalActions()
	if len(actions) == 0 {
		return game.Action{}, false
	}
	actor := state.CurrentPlayer()
	var cuts map[game.Pos]bool
	if victim, ok := nearestVictim(state, actor); ok {
		cuts = opponentArticulations(state, victim)
	}
	best, bestScore := actions[0], -1<<60
	for _, action := range actions {
		next, err := state.Apply(action)
		if err != nil {
			continue
		}
		score := immediateMobility(next, actor)
		if next.CurrentPlayer() != actor {
			score -= 50 * len(next.LegalActions())
		}
		if action.Kind == game.Move {
			target, _ := state.At(action.Target)
			if target.Kind == game.Normal && target.Owner != actor {
				score += 10_000
			}
			if cuts[action.Target] {
				score += 100_000
			} else if adjacentToCut(action.Target, cuts) {
				score += 40_000
			}
		}
		if next.GameOver() && next.Winner() == actor {
			score += 1_000_000
		}
		if score > bestScore {
			best, bestScore = action, score
		}
	}
	return best, true
}

// nearestVictim picks the active opponent whose base is closest to the
// actor's base (Manhattan), lowest player number on ties.
func nearestVictim(state game.State, actor game.Player) (game.Player, bool) {
	own := basePosition(state, actor)
	best, bestDistance := game.Player(0), 1<<30
	for opponent := game.Player(1); opponent <= 4; opponent++ {
		if opponent == actor || !state.Active(opponent) {
			continue
		}
		base := basePosition(state, opponent)
		distance := abs(own.Row-base.Row) + abs(own.Col-base.Col)
		if distance < bestDistance {
			best, bestDistance = opponent, distance
		}
	}
	return best, best != 0
}

// opponentArticulations BFSes the victim's base-connected component over
// 8-neighbour adjacency, then runs a Tarjan low-link pass over that subgraph
// and returns its cut cells. Arena-side reimplementation: search's
// articulation code is unexported and off-limits to test code.
// ponytail: O(cells) recompute per call, memoise only if the ladder gets slow.
func opponentArticulations(state game.State, victim game.Player) map[game.Pos]bool {
	base := basePosition(state, victim)
	cell, ok := state.At(base)
	if !ok || cell.Owner != victim {
		return nil
	}
	component := map[game.Pos]bool{base: true}
	queue := []game.Pos{base}
	for len(queue) > 0 {
		pos := queue[0]
		queue = queue[1:]
		for _, next := range neighbors8(state, pos) {
			c, _ := state.At(next)
			if c.Owner == victim && !component[next] {
				component[next] = true
				queue = append(queue, next)
			}
		}
	}

	disc := make(map[game.Pos]int, len(component))
	low := make(map[game.Pos]int, len(component))
	cuts := make(map[game.Pos]bool)
	timer := 0
	var dfs func(pos, parent game.Pos, isRoot bool)
	dfs = func(pos, parent game.Pos, isRoot bool) {
		timer++
		disc[pos], low[pos] = timer, timer
		children := 0
		for _, next := range neighbors8(state, pos) {
			if !component[next] {
				continue
			}
			if _, seen := disc[next]; !seen {
				children++
				dfs(next, pos, false)
				if low[next] < low[pos] {
					low[pos] = low[next]
				}
				if !isRoot && low[next] >= disc[pos] {
					cuts[pos] = true
				}
			} else if isRoot || next != parent {
				if disc[next] < low[pos] {
					low[pos] = disc[next]
				}
			}
		}
		if isRoot && children > 1 {
			cuts[pos] = true
		}
	}
	dfs(base, game.Pos{}, true)
	return cuts
}

func adjacentToCut(pos game.Pos, cuts map[game.Pos]bool) bool {
	for row := pos.Row - 1; row <= pos.Row+1; row++ {
		for col := pos.Col - 1; col <= pos.Col+1; col++ {
			next := game.Pos{Row: row, Col: col}
			if next != pos && cuts[next] {
				return true
			}
		}
	}
	return false
}

func neighbors8(state game.State, pos game.Pos) []game.Pos {
	out := make([]game.Pos, 0, 8)
	for row := pos.Row - 1; row <= pos.Row+1; row++ {
		for col := pos.Col - 1; col <= pos.Col+1; col++ {
			next := game.Pos{Row: row, Col: col}
			if next == pos {
				continue
			}
			if _, ok := state.At(next); ok {
				out = append(out, next)
			}
		}
	}
	return out
}
