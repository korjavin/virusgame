package search

import "virusgame/game"

const mateScore = 1_000_000_000

type playerMetrics struct {
	connected, disconnected int
	normal, fortified       int
	mobility, captures      int
	baseExits, baseAnchors  int
	articulation            []bool
	connectedCells          []bool
}

func evaluate(state game.State, player game.Player) int {
	return evaluateAll(state)[player-1]
}

func evaluateAll(state game.State) [4]int {
	var utility [4]int
	if state.GameOver() {
		for player := game.Player(1); player <= 4; player++ {
			if state.Winner() == player {
				utility[player-1] = mateScore
			} else {
				utility[player-1] = -mateScore
			}
		}
		return utility
	}

	var metrics [4]playerMetrics
	var raw [4]int
	active := 0
	for player := game.Player(1); player <= 4; player++ {
		if !state.Active(player) {
			raw[player-1] = -mateScore / 2
			continue
		}
		active++
		metrics[player-1] = analyze(state, player)
		m := metrics[player-1]
		raw[player-1] = 40*m.connected - 25*m.disconnected + 12*m.normal +
			55*m.fortified + 18*m.mobility + 35*m.captures +
			35*m.baseExits + 45*m.baseAnchors
		if !state.NeutralUsed(player) {
			raw[player-1] += 20
		}
		if state.CurrentPlayer() == player {
			raw[player-1] += state.MovesLeft() * 12
		}
	}

	for player := game.Player(1); player <= 4; player++ {
		if !state.Active(player) {
			continue
		}
		own := &metrics[player-1]
		for index, cut := range own.articulation {
			if cut && threatenedBy(state, index, player, metrics) {
				raw[player-1] -= 140
			}
		}
		for opponent := game.Player(1); opponent <= 4; opponent++ {
			if opponent == player || !state.Active(opponent) {
				continue
			}
			for index, cut := range metrics[opponent-1].articulation {
				if cut && adjacentConnected(state, index, own.connectedCells) {
					raw[player-1] += 160
				}
			}
		}
	}

	for player := game.Player(1); player <= 4; player++ {
		if !state.Active(player) {
			utility[player-1] = raw[player-1]
			continue
		}
		opponents := 0
		for other := game.Player(1); other <= 4; other++ {
			if other != player && state.Active(other) {
				opponents += raw[other-1]
			}
		}
		if active > 1 {
			utility[player-1] = raw[player-1] - opponents/(active-1)
		} else {
			utility[player-1] = raw[player-1]
		}
	}
	return utility
}

func analyze(state game.State, player game.Player) playerMetrics {
	size := state.Rows() * state.Cols()
	m := playerMetrics{
		connectedCells: connectedCells(state, player),
		articulation:   make([]bool, size),
	}
	m.articulation = articulationPoints(state, player, m.connectedCells)
	targets := make([]bool, size)
	base := basePos(state, player)
	for row := 0; row < state.Rows(); row++ {
		for col := 0; col < state.Cols(); col++ {
			pos := game.Pos{Row: row, Col: col}
			index := row*state.Cols() + col
			cell, _ := state.At(pos)
			if cell.Owner == player {
				switch cell.Kind {
				case game.Normal:
					m.normal++
				case game.Fortified:
					m.fortified++
				}
				if m.connectedCells[index] {
					m.connected++
				} else {
					m.disconnected++
				}
			}
			if !m.connectedCells[index] {
				continue
			}
			for _, neighbor := range neighbors(state, pos) {
				target, _ := state.At(neighbor)
				targetIndex := neighbor.Row*state.Cols() + neighbor.Col
				if !targets[targetIndex] && (target.Kind == game.Empty || target.Kind == game.Normal && target.Owner != player) {
					targets[targetIndex] = true
					m.mobility++
					if target.Kind == game.Normal {
						m.captures++
					}
				}
			}
		}
	}
	for _, pos := range neighbors(state, base) {
		cell, _ := state.At(pos)
		if cell.Kind == game.Empty || cell.Kind == game.Normal {
			m.baseExits++
		}
		if cell.Owner == player && cell.Kind == game.Fortified {
			m.baseAnchors++
		}
	}
	return m
}

func connectedCells(state game.State, player game.Player) []bool {
	seen := make([]bool, state.Rows()*state.Cols())
	base := basePos(state, player)
	cell, ok := state.At(base)
	if !ok || cell.Owner != player || cell.Kind != game.Base {
		return seen
	}
	seen[base.Row*state.Cols()+base.Col] = true
	queue := []game.Pos{base}
	for len(queue) > 0 {
		pos := queue[0]
		queue = queue[1:]
		for _, next := range neighbors(state, pos) {
			index := next.Row*state.Cols() + next.Col
			owner, _ := state.At(next)
			if !seen[index] && owner.Owner == player {
				seen[index] = true
				queue = append(queue, next)
			}
		}
	}
	return seen
}

func articulationPoints(state game.State, player game.Player, connected []bool) []bool {
	size := len(connected)
	discovery := make([]int, size)
	low := make([]int, size)
	parent := make([]int, size)
	result := make([]bool, size)
	for i := range parent {
		parent[i] = -1
	}
	time := 0
	var visit func(int)
	visit = func(index int) {
		time++
		discovery[index], low[index] = time, time
		children := 0
		pos := game.Pos{Row: index / state.Cols(), Col: index % state.Cols()}
		for _, next := range neighbors(state, pos) {
			nextIndex := next.Row*state.Cols() + next.Col
			if !connected[nextIndex] {
				continue
			}
			if discovery[nextIndex] == 0 {
				children++
				parent[nextIndex] = index
				visit(nextIndex)
				if low[nextIndex] < low[index] {
					low[index] = low[nextIndex]
				}
				if parent[index] == -1 && children > 1 || parent[index] != -1 && low[nextIndex] >= discovery[index] {
					result[index] = true
				}
			} else if nextIndex != parent[index] && discovery[nextIndex] < low[index] {
				low[index] = discovery[nextIndex]
			}
		}
	}
	for index, live := range connected {
		if live && discovery[index] == 0 {
			visit(index)
		}
	}
	for index, cut := range result {
		if cut {
			cell, _ := state.At(game.Pos{Row: index / state.Cols(), Col: index % state.Cols()})
			if cell.Kind != game.Normal || cell.Owner != player {
				result[index] = false
			}
		}
	}
	return result
}

func threatenedBy(state game.State, index int, player game.Player, metrics [4]playerMetrics) bool {
	pos := game.Pos{Row: index / state.Cols(), Col: index % state.Cols()}
	for opponent := game.Player(1); opponent <= 4; opponent++ {
		if opponent != player && state.Active(opponent) && adjacentConnected(state, index, metrics[opponent-1].connectedCells) {
			cell, _ := state.At(pos)
			return cell.Kind == game.Normal
		}
	}
	return false
}

func adjacentConnected(state game.State, index int, connected []bool) bool {
	pos := game.Pos{Row: index / state.Cols(), Col: index % state.Cols()}
	for _, neighbor := range neighbors(state, pos) {
		if connected[neighbor.Row*state.Cols()+neighbor.Col] {
			return true
		}
	}
	return false
}

func basePos(state game.State, player game.Player) game.Pos {
	switch player {
	case 1:
		return game.Pos{Row: 0, Col: 0}
	case 2:
		return game.Pos{Row: state.Rows() - 1, Col: state.Cols() - 1}
	case 3:
		return game.Pos{Row: 0, Col: state.Cols() - 1}
	default:
		return game.Pos{Row: state.Rows() - 1, Col: 0}
	}
}

func neighbors(state game.State, pos game.Pos) []game.Pos {
	result := make([]game.Pos, 0, 8)
	for row := pos.Row - 1; row <= pos.Row+1; row++ {
		for col := pos.Col - 1; col <= pos.Col+1; col++ {
			if row >= 0 && row < state.Rows() && col >= 0 && col < state.Cols() && (row != pos.Row || col != pos.Col) {
				result = append(result, game.Pos{Row: row, Col: col})
			}
		}
	}
	return result
}
