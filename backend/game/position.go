package game

// Position is the search-facing view of an authoritative State. It exposes an
// exact action set for rule-sensitive callers and a selective action set for
// search. Apply always goes through State.Apply, keeping a single rules engine.
type Position struct {
	state State
	moves []Pos
}

func NewPosition(state State) Position {
	return Position{state: state, moves: state.moveTargets(state.current)}
}

func (p Position) State() State { return p.state }

// LegalActions is exactly State.LegalActions, with cached frontier generation.
func (p Position) LegalActions() []Action {
	if p.state.over || !p.state.Active(p.state.current) {
		return nil
	}
	actions := make([]Action, 0, len(p.moves))
	for _, target := range p.moves {
		actions = append(actions, Action{Kind: Move, Target: target})
	}
	return p.appendNeutralPairs(actions, p.ownedNormals())
}

// SearchActions omits strategically interchangeable neutral pairs while
// retaining every pair involving a cell relevant to base safety, territory
// cuts, or immediate defense. The other cell may be a required neutral filler. Ordinary moves remain complete and exact.
func (p Position) SearchActions() []Action {
	if p.state.over || !p.state.Active(p.state.current) {
		return nil
	}
	actions := make([]Action, 0, len(p.moves))
	for _, target := range p.moves {
		actions = append(actions, Action{Kind: Move, Target: target})
	}
	return p.appendRelevantNeutralPairs(actions, p.ownedNormals(), p.tacticalNeutrals())
}

func (p Position) Apply(action Action) (Position, error) {
	next, err := p.state.Apply(action)
	if err != nil {
		return p, err
	}
	return NewPosition(next), nil
}

func (p Position) appendNeutralPairs(actions []Action, cells []Pos) []Action {
	s := p.state
	if s.movesLeft != actionsPerTurn || s.neutralUsed[s.current-1] {
		return actions
	}
	for i := range cells {
		for j := i + 1; j < len(cells); j++ {
			actions = append(actions, Action{Kind: PlaceNeutrals, Neutrals: [2]Pos{cells[i], cells[j]}})
		}
	}
	return actions
}

func (p Position) appendRelevantNeutralPairs(actions []Action, owned, tactical []Pos) []Action {
	s := p.state
	if s.movesLeft != actionsPerTurn || s.neutralUsed[s.current-1] || len(tactical) == 0 {
		return actions
	}
	index := make(map[Pos]int, len(owned))
	for i, cell := range owned {
		index[cell] = i
	}
	relevant := make([]bool, len(owned))
	for _, cell := range tactical {
		relevant[index[cell]] = true
	}
	for _, cell := range tactical {
		i := index[cell]
		for j := range owned {
			if i == j || (relevant[j] && j < i) {
				continue
			}
			left, right := i, j
			if left > right {
				left, right = right, left
			}
			actions = append(actions, Action{Kind: PlaceNeutrals, Neutrals: [2]Pos{owned[left], owned[right]}})
		}
	}
	return actions
}

func (p Position) ownedNormals() []Pos {
	cells := make([]Pos, 0)
	for index, cell := range p.state.cells {
		if cell.Owner == p.state.current && cell.Kind == Normal {
			cells = append(cells, Pos{Row: index / p.state.cols, Col: index % p.state.cols})
		}
	}
	return cells
}

func (p Position) tacticalNeutrals() []Pos {
	s, player := p.state, p.state.current
	if s.movesLeft != actionsPerTurn || s.neutralUsed[player-1] {
		return nil
	}
	connected := s.connected(player)
	cuts := articulationCells(s, connected)
	threatened := make([]bool, len(s.cells))
	for opponent := Player(1); int(opponent) <= s.players; opponent++ {
		if opponent == player || !s.Active(opponent) {
			continue
		}
		for _, target := range s.moveTargets(opponent) {
			cell := s.cells[s.index(target)]
			if cell.Owner == player && cell.Kind == Normal {
				threatened[s.index(target)] = true
			}
		}
	}
	base := s.bases[player-1]
	cells := make([]Pos, 0)
	for index, cell := range s.cells {
		if cell.Owner != player || cell.Kind != Normal {
			continue
		}
		pos := Pos{Row: index / s.cols, Col: index % s.cols}
		if cuts[index] || threatened[index] || adjacent(pos, base) {
			cells = append(cells, pos)
		}
	}
	return cells
}

func adjacent(a, b Pos) bool {
	row := a.Row - b.Row
	if row < 0 {
		row = -row
	}
	col := a.Col - b.Col
	if col < 0 {
		col = -col
	}
	return row <= 1 && col <= 1 && a != b
}

// articulationCells finds owned cells whose removal disconnects the player's
// base-connected territory. Tarjan's traversal is linear in board area.
func articulationCells(s State, connected []bool) []bool {
	discovery := make([]int, len(s.cells))
	low := make([]int, len(s.cells))
	parent := make([]int, len(s.cells))
	for index := range parent {
		parent[index] = -1
	}
	cuts := make([]bool, len(s.cells))
	time := 0
	var visit func(int)
	visit = func(index int) {
		time++
		discovery[index], low[index] = time, time
		children := 0
		row, col := index/s.cols, index%s.cols
		for nextRow := row - 1; nextRow <= row+1; nextRow++ {
			for nextCol := col - 1; nextCol <= col+1; nextCol++ {
				next := Pos{nextRow, nextCol}
				if !s.inBounds(next) || (nextRow == row && nextCol == col) {
					continue
				}
				nextIndex := s.index(next)
				if !connected[nextIndex] {
					continue
				}
				if discovery[nextIndex] == 0 {
					parent[nextIndex] = index
					children++
					visit(nextIndex)
					if low[nextIndex] < low[index] {
						low[index] = low[nextIndex]
					}
					if parent[index] == -1 && children > 1 {
						cuts[index] = true
					}
					if parent[index] != -1 && low[nextIndex] >= discovery[index] {
						cuts[index] = true
					}
				} else if nextIndex != parent[index] && discovery[nextIndex] < low[index] {
					low[index] = discovery[nextIndex]
				}
			}
		}
	}
	baseIndex := s.index(s.bases[s.current-1])
	if connected[baseIndex] {
		visit(baseIndex)
	}
	return cuts
}
