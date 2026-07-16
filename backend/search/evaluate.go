package search

import "virusgame/game"

const mateScore = 1_000_000_000

// spaceRaceWeight scales the Voronoi space-race term. Chosen by the vs-ai2.34
// sweep: peak of the 2..48 curve, 69.5% vs the MobilityAttacker strangler at
// n=200 (w95 [63,75]); 48 was already past the peak at 61.2%.
const spaceRaceWeight = 32

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

// evalWorkspace owns the mutable buffers used by one searcher. It is not
// shared: concurrent root workers must each keep their own workspace.
type evalWorkspace struct {
	cells        []game.Cell
	queue        []int
	connected    [4][]bool
	articulation [4][]bool
	cutLoss      [4][]uint16
	scratch      analysisScratch
	spaceDist    []int16
	spaceOwner   []int8
	spaceQueue   []int
}

func (w *evalWorkspace) ensure(size int) {
	w.cells = resize(w.cells, size)
	w.queue = resize(w.queue, size)
	for i := range w.connected {
		w.connected[i] = resize(w.connected[i], size)
		w.articulation[i] = resize(w.articulation[i], size)
		w.cutLoss[i] = resize(w.cutLoss[i], size)
	}
	w.scratch.targets = resize(w.scratch.targets, size)
	w.scratch.discovery = resize(w.scratch.discovery, size)
	w.scratch.low = resize(w.scratch.low, size)
	w.scratch.parent = resize(w.scratch.parent, size)
	w.scratch.subtree = resize(w.scratch.subtree, size)
	w.spaceDist = resize(w.spaceDist, size)
	w.spaceOwner = resize(w.spaceOwner, size)
	w.spaceQueue = resize(w.spaceQueue, size)
}

// spaceRace runs one shared multi-source BFS over empty cells seeded from every
// active player's connected territory, and counts how many open cells each
// player reaches strictly first (nearest-owns Voronoi partition; equidistant
// cells are contested and awarded to nobody). This is the Tron space-partition
// counter to strangulation: it sees, many plies ahead, which open region the
// opponent is walling off — something immediate mobility cannot.
//
// Tempo bias (vs-ai2.34 loss post-mortem): the raw own-minus-opp differential
// swings ~±40-50 cells with turn phase — pro-self right after our own 3
// actions, anti-self right after the opponent's reply. This term is therefore
// kept side-to-move-independent: both players' first-reach counts come from
// the SAME shared BFS on the SAME position, so at any leaf the phase shift
// moves both sides' terms together and cancels in same-ply sibling
// comparisons; no per-side tempo baseline is needed.
// ponytail: plain cell-distance partition, not ceil(dist/3) turn-distance; the
// nearest-owner is identical under that monotone rescale, only same-turn ties
// differ, so it captures the space-race signal at lower cost.
func spaceRace(state game.State, cells []game.Cell, connected [4][]bool, w *evalWorkspace) [4]int {
	size := len(cells)
	dist, owner, queue := w.spaceDist[:size], w.spaceOwner[:size], w.spaceQueue[:size]
	for i := range dist {
		dist[i] = -1
		owner[i] = -1
	}
	tail := 0
	for p := 0; p < 4; p++ {
		if connected[p] == nil {
			continue
		}
		for i := 0; i < size; i++ {
			if connected[p][i] {
				dist[i] = 0
				owner[i] = int8(p)
				queue[tail] = i
				tail++
			}
		}
	}
	for head := 0; head < tail; head++ {
		idx := queue[head]
		d, o := dist[idx], owner[idx]
		pos := game.Pos{Row: idx / state.Cols(), Col: idx % state.Cols()}
		var nearby [8]game.Pos
		count := neighbors(state, pos, &nearby)
		for i := 0; i < count; i++ {
			ni := nearby[i].Row*state.Cols() + nearby[i].Col
			if cells[ni].Kind != game.Empty {
				continue
			}
			if dist[ni] == -1 {
				dist[ni] = d + 1
				owner[ni] = o
				queue[tail] = ni
				tail++
			} else if dist[ni] == d+1 && owner[ni] != o && owner[ni] != -2 {
				owner[ni] = -2 // contested: reached at equal distance by another source
			}
		}
	}
	var counts [4]int
	for i := 0; i < size; i++ {
		if cells[i].Kind == game.Empty && owner[i] >= 0 {
			counts[owner[i]]++
		}
	}
	return counts
}

func resize[S ~[]E, E any](buffer S, size int) S {
	if cap(buffer) < size {
		return make(S, size)
	}
	return buffer[:size]
}

func evaluate(state game.State, player game.Player) int {
	return evaluateAll(state)[player-1]
}

func evaluateAll(state game.State) [4]int {
	return evaluateAllWithWorkspace(state, &evalWorkspace{})
}

func evaluateWithWorkspace(state game.State, player game.Player, workspace *evalWorkspace) int {
	return evaluateAllWithWorkspace(state, workspace)[player-1]
}

func evaluateAllWithWorkspace(state game.State, workspace *evalWorkspace) [4]int {
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
	size := state.Rows() * state.Cols()
	workspace.ensure(size)
	cells := snapshotCellsInto(state, workspace.cells)
	connected := allConnectedInto(state, cells, workspace)
	space := spaceRace(state, cells, connected, workspace)
	var raw [4]int
	active := 0
	for player := game.Player(1); player <= 4; player++ {
		if !state.Active(player) {
			raw[player-1] = -mateScore / 2
			continue
		}
		active++
		index := player - 1
		metrics[index] = analyzeWithConnectivity(state, player, cells, connected, &workspace.scratch,
			workspace.articulation[index], workspace.cutLoss[index])
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
			m.threatTempo*ratio(m.threatened, max(1, m.connected)) +
			normalized(space[player-1], area, spaceRaceWeight)
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
	workspace := evalWorkspace{}
	workspace.ensure(size)
	cells := snapshotCellsInto(state, workspace.cells)
	connected := allConnectedInto(state, cells, &workspace)
	index := player - 1
	return analyzeWithConnectivity(state, player, cells, connected, &workspace.scratch,
		workspace.articulation[index], workspace.cutLoss[index])
}

func snapshotCells(state game.State) []game.Cell {
	return snapshotCellsInto(state, make([]game.Cell, state.Rows()*state.Cols()))
}

func snapshotCellsInto(state game.State, cells []game.Cell) []game.Cell {
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

func allConnectedInto(state game.State, cells []game.Cell, workspace *evalWorkspace) [4][]bool {
	for opponent := game.Player(1); opponent <= 4; opponent++ {
		if state.Active(opponent) {
			connectedCellsInto(state, cells, opponent, workspace.queue, workspace.connected[opponent-1])
		} else {
			clear(workspace.connected[opponent-1])
		}
	}
	return workspace.connected
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

func analyzeWithConnectivity(state game.State, player game.Player, cells []game.Cell, connected [4][]bool,
	scratch *analysisScratch, articulation []bool, cutLoss []uint16) playerMetrics {
	scratch.reset()
	clear(articulation)
	clear(cutLoss)
	m := playerMetrics{
		connectedCells: connected[player-1],
		articulation:   articulation,
		cutLoss:        cutLoss,
	}
	articulationPointsInto(state, player, cells, m.connectedCells, scratch, articulation, cutLoss)
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
	connectedCellsInto(state, cells, player, queue, seen)
	return seen
}

func connectedCellsInto(state game.State, cells []game.Cell, player game.Player, queue []int, seen []bool) {
	clear(seen)
	base := basePos(state, player)
	cell := cells[indexFor(state, base)]
	if cell.Owner != player || cell.Kind != game.Base {
		return
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
}

func articulationPoints(state game.State, player game.Player, cells []game.Cell, connected []bool, scratch *analysisScratch) ([]bool, []uint16) {
	size := len(connected)
	result := make([]bool, size)
	cutLoss := make([]uint16, size)
	articulationPointsInto(state, player, cells, connected, scratch, result, cutLoss)
	return result, cutLoss
}

func articulationPointsInto(state game.State, player game.Player, cells []game.Cell, connected []bool,
	scratch *analysisScratch, result []bool, cutLoss []uint16) {
	size := len(connected)
	discovery := scratch.discovery
	low := scratch.low
	parent := scratch.parent
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
