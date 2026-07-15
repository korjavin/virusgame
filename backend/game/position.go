package game

// Position is the allocation-conscious, 1v1 search-facing view of an
// authoritative State. State.Apply remains the sole rules implementation.
// Search selection is intentionally limited to 1v1: multiplayer callers get
// exact enumeration because opponent-specific tactical pruning is ambiguous.
type Position struct {
	state       State
	moves       []Pos
	owned       []Pos
	searchPairs [][2]Pos
	analyzed    bool
}

func NewPosition(state State) Position {
	p := Position{state: state, moves: state.moveTargets(state.current), analyzed: true}
	if p.canPlaceNeutrals() {
		p.owned = p.scanOwnedNormals()
		if state.players == 2 {
			p.searchPairs = p.strategicNeutralPairs(p.owned)
		}
	}
	return p
}

func (p Position) State() State { return p.state }

func (p Position) moveList() []Pos {
	if p.moves != nil {
		return p.moves
	}
	return p.state.moveTargets(p.state.current)
}

// LegalActions is exactly State.LegalActions, with cached frontier generation.
func (p Position) LegalActions() []Action {
	if p.state.over || !p.state.Active(p.state.current) {
		return nil
	}
	actions := make([]Action, 0, len(p.moveList()))
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
	for _, target := range p.moveList() {
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
	for _, target := range p.moveList() {
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
	pairs := p.searchPairs
	if !p.analyzed {
		pairs = p.strategicNeutralPairs(owned)
	}
	for _, pair := range pairs {
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

// ApplySearch applies an action produced by ForEachSearchAction without
// repeating legality/connectivity work. Callers must not pass arbitrary input.
// State.Apply remains the boundary oracle and is used by equivalence tests.
func (p Position) ApplySearch(action Action) Position {
	next := p.state.applyGenerated(action)
	return Position{state: next}
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
	if p.analyzed {
		return p.owned
	}
	return p.scanOwnedNormals()
}

func (p Position) scanOwnedNormals() []Pos {
	cells := make([]Pos, 0)
	for index, cell := range p.state.cells {
		if cell.Owner == p.state.current && cell.Kind == Normal {
			cells = append(cells, Pos{Row: index / p.state.cols, Col: index % p.state.cols})
		}
	}
	return cells
}

// strategicNeutralPairs returns a deliberately bounded defensive branch set.
// Neutralizing one important cell with every possible filler is strategically
// redundant and catastrophically expensive. We instead pair at most twelve
// highest-priority defensive cells with two robust fillers, and retain general
// (including non-adjacent) two-vertex separators involving those cells.
func (p Position) strategicNeutralPairs(owned []Pos) [][2]Pos {
	s, player := p.state, p.state.current
	if !p.canPlaceNeutrals() {
		return nil
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
	defensive := make([]Pos, 0, 12)
	// Priority order is base survival, immediate attack defense, then narrow
	// connectivity. Stable board order breaks ties deterministically.
	addDefensive := func(pos Pos) {
		for _, existing := range defensive {
			if existing == pos {
				return
			}
		}
		if len(defensive) < 12 {
			defensive = append(defensive, pos)
		}
	}
	for _, pos := range owned {
		if adjacent(pos, base) {
			addDefensive(pos)
		}
	}
	for _, pos := range owned {
		if threatened[s.index(pos)] {
			addDefensive(pos)
		}
	}
	for _, pos := range owned {
		index := s.index(pos)
		if cuts[index] {
			addDefensive(pos)
		}
	}
	fillers := robustFillers(s, owned, cuts, threatened, defensive, 2)
	pairs := make([][2]Pos, 0, 48)
	addPair := func(a, b Pos) {
		if a == b {
			return
		}
		if s.index(a) > s.index(b) {
			a, b = b, a
		}
		pair := [2]Pos{a, b}
		for _, existing := range pairs {
			if existing == pair {
				return
			}
		}
		if len(pairs) < 48 {
			pairs = append(pairs, pair)
		}
	}
	for _, cell := range defensive {
		for _, filler := range fillers {
			addPair(cell, filler)
		}
	}
	// Tarjan in G-u finds all partners v of a general two-vertex separator.
	// Scratch is reused and defensive is capped, bounding time and allocation.
	for _, u := range defensive {
		if len(pairs) >= 48 {
			break
		}
		uIndex := s.index(u)
		if cuts[uIndex] {
			continue
		}
		partners := articulationCells(s, connected, uIndex, scratch)
		for _, v := range owned {
			if partners[s.index(v)] {
				addPair(u, v)
			}
		}
	}
	return pairs
}

func robustFillers(s State, owned []Pos, cuts, threatened []bool, defensive []Pos, limit int) []Pos {
	result := make([]Pos, 0, limit)
	isDefensive := func(pos Pos) bool {
		for _, d := range defensive {
			if d == pos {
				return true
			}
		}
		return false
	}
	for len(result) < limit {
		best, bestScore, found := Pos{}, -1, false
		for _, pos := range owned {
			index := s.index(pos)
			if cuts[index] || threatened[index] || isDefensive(pos) {
				continue
			}
			used := false
			for _, chosen := range result {
				if chosen == pos {
					used = true
					break
				}
			}
			if used {
				continue
			}
			base := s.bases[s.current-1]
			score := abs(pos.Row-base.Row) + abs(pos.Col-base.Col)
			if !found || score > bestScore {
				best, bestScore, found = pos, score, true
			}
		}
		if !found {
			break
		}
		result = append(result, best)
	}
	return result
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
