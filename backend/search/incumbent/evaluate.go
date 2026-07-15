package incumbent

import "virusgame/game"

const mateScore = 1_000_000_000

type playerMetrics struct {
	connected, disconnected    int
	normal, fortified          int
	mobility, captures         int
	baseExits, baseOpenings    int
	baseAnchors, baseThreat    int
	threatened, threatenedLoss int
	threatTempo                int
	articulation               []bool
	cutLoss                    []uint16
	connectedCells             []bool
}

type analysisScratch struct {
	targets        []bool
	discovery, low []uint16
	parent         []int16
	subtree        []uint16
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
	cells := snapshotCells(state)
	connected := allConnected(state, cells)
	scratch := newAnalysisScratch(state.Rows() * state.Cols())
	var raw [4]int
	active := 0
	for player := game.Player(1); player <= 4; player++ {
		if !state.Active(player) {
			raw[player-1] = -mateScore / 2
			continue
		}
		active++
		metrics[player-1] = analyzeWithConnectivity(state, player, cells, connected, &scratch)
		m := metrics[player-1]
		area := state.Rows() * state.Cols()
		owned := m.normal + m.fortified + 1 // include the base
		raw[player-1] = normalized(m.connected, area, 10) +
			normalized(m.normal, area, 30) + normalized(m.fortified, area, 6) +
			normalized(m.mobility, area, 1) + normalized(m.captures, area, 1) -
			normalized(m.disconnected, owned, 1) +
			180*m.baseExits + 80*m.baseOpenings + 240*m.baseAnchors -
			650*m.baseThreat*m.threatTempo -
			m.threatTempo*ratio(m.threatenedLoss, max(1, m.connected)) -
			m.threatTempo*ratio(m.threatened, max(1, m.connected))
		if m.baseExits+m.baseOpenings == 0 {
			raw[player-1] -= 5000
		}
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
		for opponent := game.Player(1); opponent <= 4; opponent++ {
			if opponent == player || !state.Active(opponent) {
				continue
			}
			for index, cut := range metrics[opponent-1].articulation {
				if cut && adjacentConnected(state, index, own.connectedCells) {
					loss := int(metrics[opponent-1].cutLoss[index])
					raw[player-1] += 150 + ratio(loss, max(1, metrics[opponent-1].connected))/2
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
	cells := snapshotCells(state)
	connected := allConnected(state, cells)
	scratch := newAnalysisScratch(size)
	return analyzeWithConnectivity(state, player, cells, connected, &scratch)
}

func snapshotCells(state game.State) []game.Cell {
	cells := make([]game.Cell, state.Rows()*state.Cols())
	for row := 0; row < state.Rows(); row++ {
		for col := 0; col < state.Cols(); col++ {
			index := row*state.Cols() + col
			cells[index], _ = state.At(game.Pos{Row: row, Col: col})
		}
	}
	return cells
}

func allConnected(state game.State, cells []game.Cell) [4][]bool {
	var connected [4][]bool
	queue := make([]int, len(cells))
	for opponent := game.Player(1); opponent <= 4; opponent++ {
		if state.Active(opponent) {
			connected[opponent-1] = connectedCells(state, cells, opponent, queue)
		}
	}
	return connected
}

func newAnalysisScratch(size int) analysisScratch {
	return analysisScratch{
		targets: make([]bool, size), discovery: make([]uint16, size),
		low: make([]uint16, size), parent: make([]int16, size), subtree: make([]uint16, size),
	}
}

func (scratch *analysisScratch) reset() {
	clear(scratch.targets)
	clear(scratch.discovery)
	clear(scratch.low)
	clear(scratch.subtree)
	for index := range scratch.parent {
		scratch.parent[index] = -1
	}
}

func analyzeWithConnectivity(state game.State, player game.Player, cells []game.Cell, connected [4][]bool, scratch *analysisScratch) playerMetrics {
	size := state.Rows() * state.Cols()
	scratch.reset()
	m := playerMetrics{
		connectedCells: connected[player-1],
		articulation:   make([]bool, size),
		cutLoss:        make([]uint16, size),
	}
	m.articulation, m.cutLoss = articulationPoints(state, player, cells, m.connectedCells, scratch)
	targets := scratch.targets
	base := basePos(state, player)
	for row := 0; row < state.Rows(); row++ {
		for col := 0; col < state.Cols(); col++ {
			pos := game.Pos{Row: row, Col: col}
			index := row*state.Cols() + col
			cell := cells[index]
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
			if m.connectedCells[index] && cell.Kind == game.Normal && threatenedByConnected(state, index, player, connected) {
				m.threatened++
				if m.articulation[index] {
					m.threatenedLoss += int(m.cutLoss[index])
				}
			}
			if !m.connectedCells[index] {
				continue
			}
			var nearby [8]game.Pos
			count := neighbors(state, pos, &nearby)
			for i := 0; i < count; i++ {
				neighbor := nearby[i]
				targetIndex := neighbor.Row*state.Cols() + neighbor.Col
				target := cells[targetIndex]
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
	var nearby [8]game.Pos
	count := neighbors(state, base, &nearby)
	for i := 0; i < count; i++ {
		pos := nearby[i]
		index := pos.Row*state.Cols() + pos.Col
		cell := cells[index]
		switch {
		case cell.Owner == player && m.connectedCells[index]:
			m.baseExits++
			if cell.Kind == game.Fortified {
				m.baseAnchors++
			}
		case cell.Kind == game.Empty:
			m.baseOpenings++
		case cell.Kind == game.Normal && cell.Owner != player:
			// An enemy normal is a legal capture from the base, but is a
			// contested opening rather than owned escape structure.
			m.baseOpenings++
			if threatenedByConnected(state, indexFor(state, base), player, connected) {
				m.baseThreat++
			}
		}
	}
	m.threatTempo = threatTempo(state, player)
	return m
}

// threatTempo makes an unresolved attack more urgent as the defender spends
// its turn, and fully urgent while an opponent still has actions available.
func threatTempo(state game.State, player game.Player) int {
	if state.CurrentPlayer() == player {
		return max(1, 4-state.MovesLeft())
	}
	return max(1, state.MovesLeft())
}

func connectedCells(state game.State, cells []game.Cell, player game.Player, queue []int) []bool {
	seen := make([]bool, state.Rows()*state.Cols())
	base := basePos(state, player)
	cell := cells[indexFor(state, base)]
	if cell.Owner != player || cell.Kind != game.Base {
		return seen
	}
	baseIndex := base.Row*state.Cols() + base.Col
	seen[baseIndex] = true
	queue[0] = baseIndex
	head, tail := 0, 1
	for head < tail {
		index := queue[head]
		head++
		pos := game.Pos{Row: index / state.Cols(), Col: index % state.Cols()}
		var nearby [8]game.Pos
		count := neighbors(state, pos, &nearby)
		for i := 0; i < count; i++ {
			next := nearby[i]
			index := next.Row*state.Cols() + next.Col
			owner := cells[index]
			if !seen[index] && owner.Owner == player {
				seen[index] = true
				queue[tail] = index
				tail++
			}
		}
	}
	return seen
}

func articulationPoints(state game.State, player game.Player, cells []game.Cell, connected []bool, scratch *analysisScratch) ([]bool, []uint16) {
	size := len(connected)
	discovery := scratch.discovery
	low := scratch.low
	parent := scratch.parent
	result := make([]bool, size)
	cutLoss := make([]uint16, size)
	subtree := scratch.subtree
	time := uint16(0)
	var visit func(int)
	visit = func(index int) {
		time++
		discovery[index], low[index] = time, time
		subtree[index] = 1
		children := 0
		pos := game.Pos{Row: index / state.Cols(), Col: index % state.Cols()}
		var nearby [8]game.Pos
		count := neighbors(state, pos, &nearby)
		for i := 0; i < count; i++ {
			next := nearby[i]
			nextIndex := next.Row*state.Cols() + next.Col
			if !connected[nextIndex] {
				continue
			}
			if discovery[nextIndex] == 0 {
				children++
				parent[nextIndex] = int16(index)
				visit(nextIndex)
				subtree[index] += subtree[nextIndex]
				if low[nextIndex] < low[index] {
					low[index] = low[nextIndex]
				}
				if parent[index] == -1 && children > 1 || parent[index] != -1 && low[nextIndex] >= discovery[index] {
					result[index] = true
					cutLoss[index] += subtree[nextIndex]
				}
			} else if nextIndex != int(parent[index]) && discovery[nextIndex] < low[index] {
				low[index] = discovery[nextIndex]
			}
		}
	}
	base := basePos(state, player)
	baseIndex := indexFor(state, base)
	if baseIndex >= 0 && baseIndex < size && connected[baseIndex] {
		visit(baseIndex)
	}
	for index, live := range connected {
		if live && discovery[index] == 0 {
			visit(index)
		}
	}
	for index, cut := range result {
		if cut {
			cell := cells[index]
			if cell.Kind != game.Normal || cell.Owner != player {
				result[index] = false
				cutLoss[index] = 0
			} else {
				// Capturing the cut cell loses the cell itself as well as every
				// downstream component separated from the base.
				cutLoss[index]++
			}
		}
	}
	return result, cutLoss
}

func threatenedByConnected(state game.State, index int, player game.Player, connected [4][]bool) bool {
	pos := game.Pos{Row: index / state.Cols(), Col: index % state.Cols()}
	for opponent := game.Player(1); opponent <= 4; opponent++ {
		if opponent == player || !state.Active(opponent) {
			continue
		}
		var nearby [8]game.Pos
		count := neighbors(state, pos, &nearby)
		for i := 0; i < count; i++ {
			neighbor := nearby[i]
			if connected[opponent-1] != nil && connected[opponent-1][indexFor(state, neighbor)] {
				return true
			}
		}
	}
	return false
}

func indexFor(state game.State, pos game.Pos) int { return pos.Row*state.Cols() + pos.Col }

func ratio(value, denominator int) int {
	if value <= 0 || denominator <= 0 {
		return 0
	}
	return value * 1000 / denominator
}

func normalized(value, denominator, weight int) int {
	if value <= 0 || denominator <= 0 || weight <= 0 {
		return 0
	}
	return value * weight * 1000 / denominator
}

func adjacentConnected(state game.State, index int, connected []bool) bool {
	pos := game.Pos{Row: index / state.Cols(), Col: index % state.Cols()}
	var nearby [8]game.Pos
	count := neighbors(state, pos, &nearby)
	for i := 0; i < count; i++ {
		neighbor := nearby[i]
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

func neighbors(state game.State, pos game.Pos, result *[8]game.Pos) int {
	count := 0
	for row := pos.Row - 1; row <= pos.Row+1; row++ {
		for col := pos.Col - 1; col <= pos.Col+1; col++ {
			if row >= 0 && row < state.Rows() && col >= 0 && col < state.Cols() && (row != pos.Row || col != pos.Col) {
				result[count] = game.Pos{Row: row, Col: col}
				count++
			}
		}
	}
	return count
}
