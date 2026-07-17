package game

// Position is the allocation-conscious search-facing view of an
// authoritative State. State.Apply remains the sole rules implementation.
type Position struct {
	state       State
	moves       []Pos
	owned       []Pos
	searchPairs [][2]Pos
	analyzed    bool
}

func NewPosition(state State) Position {
	// connected(current) is the same pre-move floodfill needed by both the move
	// frontier and strategicNeutralPairs; compute it once and share it.
	connected := state.connected(state.current)
	p := Position{state: state, moves: state.moveTargetsFrom(state.current, connected), analyzed: true}
	if p.canPlaceNeutrals() {
		p.owned = p.scanOwnedNormals()
		// ForEachSearchAction only consults searchPairs above the exact-branch
		// threshold; below it the pairs are discarded, so skip the Tarjan work.
		if len(p.moves)+len(p.owned)*(len(p.owned)-1)/2 > 32 {
			p.searchPairs = p.strategicNeutralPairs(p.owned, connected)
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
// It omits strategically interchangeable neutral pairs while retaining stable
// representatives for base defense, threatened cells, articulation cells,
// robust fillers, and two-vertex separators. Threats are the union of all
// active opponents in both two-player and multiplayer games.
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
	// Keep small positions exact. Besides avoiding needless analysis, this
	// preserves authoritative action order for deterministic tie breaking.
	if len(p.moveList())+len(owned)*(len(owned)-1)/2 <= 32 {
		p.forEachNeutralPair(owned, yield)
		return
	}
	pairs := p.searchPairs
	if !p.analyzed {
		pairs = p.strategicNeutralPairs(owned, p.state.connected(p.state.current))
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
// highest-priority defensive cells with robust fillers, and retain general
// (including non-adjacent) two-vertex separators involving those cells. Pair
// classes receive reserved representation before remaining capacity is filled,
// so a large defensive class cannot starve fillers or separators.
func (p Position) strategicNeutralPairs(owned []Pos, connected []bool) [][2]Pos {
	s, player := p.state, p.state.current
	if !p.canPlaceNeutrals() {
		return nil
	}
	scratch := newArticulationScratch(len(s.cells))
	cuts := append([]bool(nil), articulationCells(s, connected, -1, scratch)...)
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
	defensive := make([]Pos, 0, 12)
	baseDefense := make([]Pos, 0, 8)
	threatDefense := make([]Pos, 0)
	cutDefense := make([]Pos, 0)
	for _, pos := range owned {
		if adjacent(pos, base) {
			baseDefense = append(baseDefense, pos)
		}
		if threatened[s.index(pos)] {
			threatDefense = append(threatDefense, pos)
		}
		if cuts[s.index(pos)] {
			cutDefense = append(cutDefense, pos)
		}
	}
	// Seed every available defensive class, then fill in survival priority.
	// Stable board order breaks ties deterministically.
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
	for _, class := range [][]Pos{baseDefense, threatDefense, cutDefense} {
		if len(class) > 0 {
			addDefensive(class[0])
		}
	}
	for _, class := range [][]Pos{baseDefense, threatDefense, cutDefense} {
		for _, pos := range class {
			addDefensive(pos)
		}
	}
	fillers := robustFillers(s, owned, cuts, threatened, defensive, 2)
	normalize := func(a, b Pos) ([2]Pos, bool) {
		if a == b {
			return [2]Pos{}, false
		}
		if s.index(a) > s.index(b) {
			a, b = b, a
		}
		return [2]Pos{a, b}, true
	}
	defensiveFiller := make([][2]Pos, 0, len(defensive)*len(fillers))
	for _, cell := range defensive {
		for _, filler := range fillers {
			if pair, ok := normalize(cell, filler); ok {
				defensiveFiller = appendUniquePairLimited(defensiveFiller, pair, 48)
			}
		}
	}
	defensivePairs := make([][2]Pos, 0, len(defensive)*len(defensive)/2)
	for i := range defensive {
		for j := i + 1; j < len(defensive); j++ {
			if pair, ok := normalize(defensive[i], defensive[j]); ok {
				defensivePairs = appendUniquePairLimited(defensivePairs, pair, 48)
			}
		}
	}
	separators := make([][2]Pos, 0)
	// Tarjan in G-u finds all partners v of a general two-vertex separator.
	// Scratch is reused and defensive is capped, bounding time and allocation.
	for _, u := range defensive {
		uIndex := s.index(u)
		if cuts[uIndex] {
			continue
		}
		partners := articulationCells(s, connected, uIndex, scratch)
		for _, v := range owned {
			if partners[s.index(v)] {
				if pair, ok := normalize(u, v); ok {
					separators = appendUniquePairLimited(separators, pair, 48)
				}
			}
		}
	}

	pairs := make([][2]Pos, 0, 48)
	add := func(pair [2]Pos) { pairs = appendUniquePairLimited(pairs, pair, 48) }
	// Reserve one true separator and one pair for each defensive cell before
	// distributing remaining slots. Fillers are only safe partners for a
	// tactical cell; a standalone filler pair would be destructive cleanup.
	if len(separators) > 0 {
		add(separators[0])
	}
	for i := range defensive {
		if len(fillers) > 0 {
			pair, _ := normalize(defensive[i], fillers[i%len(fillers)])
			add(pair)
		} else if len(defensive) > 1 {
			pair, _ := normalize(defensive[i], defensive[(i+1)%len(defensive)])
			add(pair)
		}
	}
	classes := [][][2]Pos{separators, defensiveFiller, defensivePairs}
	maximum := 0
	for _, class := range classes {
		if len(class) > maximum {
			maximum = len(class)
		}
	}
	for index := 0; index < maximum && len(pairs) < 48; index++ {
		for _, class := range classes {
			if index < len(class) {
				add(class[index])
			}
		}
	}
	return pairs
}

func appendUniquePairLimited(pairs [][2]Pos, pair [2]Pos, limit int) [][2]Pos {
	for _, existing := range pairs {
		if existing == pair {
			return pairs
		}
	}
	if len(pairs) < limit {
		return append(pairs, pair)
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
