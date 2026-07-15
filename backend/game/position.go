package game

// Position is the allocation-conscious, 1v1 search-facing view of an
// authoritative State. State.Apply remains the sole rules implementation.
// Search selection is intentionally limited to 1v1: multiplayer callers get
// exact enumeration because opponent-specific tactical pruning is ambiguous.
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
	p.ForEachLegalAction(func(action Action) bool {
		actions = append(actions, action)
		return true
	})
	return actions
}

// ForEachLegalAction enumerates the authoritative action set without building
// an Action slice. Returning false stops enumeration.
func (p Position) ForEachLegalAction(yield func(Action) bool) {
	if p.state.over || !p.state.Active(p.state.current) {
		return
	}
	for _, target := range p.moves {
		if !yield(Action{Kind: Move, Target: target}) {
			return
		}
	}
	p.forEachNeutralPair(p.ownedNormals(), yield)
}

// ForEachSearchAction enumerates moves without materializing the branch list.
// In 1v1 it omits strategically interchangeable neutral pairs while retaining:
// every pair containing a base-defense, threatened, or articulation cell; and
// every adjacent width-two pair whose simultaneous removal disconnects owned
// territory.
// Multiplayer positions deliberately fall back to exact enumeration.
func (p Position) ForEachSearchAction(yield func(Action) bool) {
	if p.state.over || !p.state.Active(p.state.current) {
		return
	}
	for _, target := range p.moves {
		if !yield(Action{Kind: Move, Target: target}) {
			return
		}
	}
	owned := p.ownedNormals()
	if !p.canPlaceNeutrals() {
		return
	}
	if p.state.players != 2 {
		p.forEachNeutralPair(owned, yield)
		return
	}
	singles, pairs := p.tacticalNeutrals(owned)
	index := make([]int, len(p.state.cells))
	for i := range index {
		index[i] = -1
	}
	for i, cell := range owned {
		index[p.state.index(cell)] = i
	}
	relevant := make([]bool, len(owned))
	for _, cell := range singles {
		relevant[index[p.state.index(cell)]] = true
	}
	for i, isRelevant := range relevant {
		if !isRelevant {
			continue
		}
		for j := i + 1; j < len(owned); j++ {
			if !yield(Action{Kind: PlaceNeutrals, Neutrals: [2]Pos{owned[i], owned[j]}}) {
				return
			}
		}
		for j := 0; j < i; j++ {
			if relevant[j] {
				continue
			} // emitted by the lower relevant endpoint.
			if !yield(Action{Kind: PlaceNeutrals, Neutrals: [2]Pos{owned[j], owned[i]}}) {
				return
			}
		}
	}
	for _, pair := range pairs {
		if relevant[index[p.state.index(pair[0])]] || relevant[index[p.state.index(pair[1])]] {
			continue
		}
		if !yield(Action{Kind: PlaceNeutrals, Neutrals: pair}) {
			return
		}
	}
}

func (p Position) Apply(action Action) (Position, error) {
	next, err := p.state.Apply(action)
	if err != nil {
		return p, err
	}
	return NewPosition(next), nil
}

func (p Position) canPlaceNeutrals() bool {
	s := p.state
	return s.movesLeft == actionsPerTurn && !s.neutralUsed[s.current-1]
}

func (p Position) forEachNeutralPair(cells []Pos, yield func(Action) bool) {
	if !p.canPlaceNeutrals() {
		return
	}
	for i := range cells {
		for j := i + 1; j < len(cells); j++ {
			if !yield(Action{Kind: PlaceNeutrals, Neutrals: [2]Pos{cells[i], cells[j]}}) {
				return
			}
		}
	}
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

// tacticalNeutrals returns individually relevant cells and exact two-cell
// vertex separators. The latter catches two-wide bridges where neither cell is
// an articulation point by itself.
func (p Position) tacticalNeutrals(owned []Pos) ([]Pos, [][2]Pos) {
	s, player := p.state, p.state.current
	if !p.canPlaceNeutrals() {
		return nil, nil
	}
	connected := s.connected(player)
	scratch := newArticulationScratch(len(s.cells))
	cuts := append([]bool(nil), articulationCells(s, connected, -1, scratch)...)
	threatened := make([]bool, len(s.cells))
	opponent := Player(3 - player) // Position's selective path is explicitly 1v1.
	for _, target := range s.moveTargets(opponent) {
		cell := s.cells[s.index(target)]
		if cell.Owner == player && cell.Kind == Normal {
			threatened[s.index(target)] = true
		}
	}
	base := s.bases[player-1]
	singles := make([]Pos, 0)
	for _, pos := range owned {
		index := s.index(pos)
		if cuts[index] || threatened[index] || adjacent(pos, base) {
			singles = append(singles, pos)
		}
	}

	// A minimal tactically relevant width-two separator has adjacent endpoints.
	// A cheap local screen rejects pairs whose surrounding territory remains
	// connected; the survivors are verified against the full base component.
	// This preserves two-wide bridges without a quadratic collection of states.
	pairs := make([][2]Pos, 0)
	ordinal := make([]int, len(s.cells))
	for index := range ordinal {
		ordinal[index] = -1
	}
	for i, pos := range owned {
		ordinal[s.index(pos)] = i
	}
	for i, u := range owned {
		uIndex := s.index(u)
		if cuts[uIndex] || !connected[uIndex] {
			continue
		}
		for row := u.Row - 1; row <= u.Row+1; row++ {
			for col := u.Col - 1; col <= u.Col+1; col++ {
				pos := Pos{row, col}
				if !s.inBounds(pos) {
					continue
				}
				j := ordinal[s.index(pos)]
				if j <= i {
					continue
				}
				v := owned[j]
				vIndex := s.index(v)
				if cuts[vIndex] || !locallySeparatingPair(s, connected, uIndex, vIndex) {
					continue
				}
				if pairDisconnects(s, connected, uIndex, vIndex) {
					pairs = append(pairs, [2]Pos{u, v})
				}
			}
		}
	}
	return singles, pairs
}

func locallySeparatingPair(s State, connected []bool, first, second int) bool {
	var neighbors [16]int
	neighborCount := 0
	for _, center := range [2]int{first, second} {
		row, col := center/s.cols, center%s.cols
		for r := row - 1; r <= row+1; r++ {
			for c := col - 1; c <= col+1; c++ {
				pos := Pos{r, c}
				if !s.inBounds(pos) {
					continue
				}
				index := s.index(pos)
				if index != first && index != second && connected[index] {
					duplicate := false
					for i := 0; i < neighborCount; i++ {
						if neighbors[i] == index {
							duplicate = true
							break
						}
					}
					if !duplicate {
						neighbors[neighborCount] = index
						neighborCount++
					}
				}
			}
		}
	}
	if neighborCount < 2 {
		return false
	}
	var seen [16]bool
	var queue [16]int
	seen[0], queue[0] = true, 0
	head, tail, seenCount := 0, 1, 1
	for head < tail {
		current := neighbors[queue[head]]
		head++
		row, col := current/s.cols, current%s.cols
		for nextIndex := 0; nextIndex < neighborCount; nextIndex++ {
			if seen[nextIndex] {
				continue
			}
			next := neighbors[nextIndex]
			nr, nc := next/s.cols, next%s.cols
			if abs(row-nr) <= 1 && abs(col-nc) <= 1 {
				seen[nextIndex] = true
				seenCount++
				queue[tail] = nextIndex
				tail++
			}
		}
	}
	return seenCount != neighborCount
}

func pairDisconnects(s State, connected []bool, first, second int) bool {
	base := s.index(s.bases[s.current-1])
	if base == first || base == second {
		return false
	}
	seen := make([]bool, len(s.cells))
	seen[base] = true
	queue := []int{base}
	for len(queue) > 0 {
		current := queue[0]
		queue = queue[1:]
		row, col := current/s.cols, current%s.cols
		for r := row - 1; r <= row+1; r++ {
			for c := col - 1; c <= col+1; c++ {
				pos := Pos{r, c}
				if !s.inBounds(pos) {
					continue
				}
				next := s.index(pos)
				if next != first && next != second && connected[next] && !seen[next] {
					seen[next] = true
					queue = append(queue, next)
				}
			}
		}
	}
	for index, wasConnected := range connected {
		if wasConnected && index != first && index != second && !seen[index] {
			return true
		}
	}
	return false
}

func abs(value int) int {
	if value < 0 {
		return -value
	}
	return value
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

// articulationCells finds connected owned vertices whose removal disconnects
// the base component, optionally excluding one vertex from the graph.
type articulationScratch struct {
	discovery []int
	low       []int
	parent    []int
	cuts      []bool
}

func newArticulationScratch(size int) *articulationScratch {
	return &articulationScratch{
		discovery: make([]int, size), low: make([]int, size),
		parent: make([]int, size), cuts: make([]bool, size),
	}
}

func articulationCells(s State, connected []bool, excluded int, work *articulationScratch) []bool {
	discovery, low, parent, cuts := work.discovery, work.low, work.parent, work.cuts
	clear(discovery)
	clear(low)
	clear(cuts)
	for index := range parent {
		parent[index] = -1
	}
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
				if nextIndex == excluded || !connected[nextIndex] {
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
	if baseIndex != excluded && connected[baseIndex] {
		visit(baseIndex)
	}
	return cuts
}
