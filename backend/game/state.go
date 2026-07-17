// Package game implements the rules of Virus without server or search concerns.
package game

import "errors"

const actionsPerTurn = 3

type Player uint8

type CellKind uint8

const (
	Empty CellKind = iota
	Normal
	Base
	Fortified
	Neutral
)

type Cell struct {
	Owner Player
	Kind  CellKind
}

type Pos struct {
	Row int
	Col int
}

type ActionKind uint8

const (
	Move ActionKind = iota
	PlaceNeutrals
)

type Action struct {
	Kind     ActionKind
	Target   Pos
	Neutrals [2]Pos
}

var (
	ErrGameOver      = errors.New("game is over")
	ErrInvalidAction = errors.New("invalid action")
)

// State is a value-style game position. Apply always copies its board before
// making a change, so prior states remain safe to retain in a search tree.
type State struct {
	rows, cols  int
	players     int
	cells       []Cell
	bases       [4]Pos
	active      [4]bool
	neutralUsed [4]bool
	current     Player
	movesLeft   int
	winner      Player
	over        bool
}

// New creates a game with players based at top-left, bottom-right, top-right,
// then bottom-left.
func New(rows, cols, players int) (State, error) {
	if rows < 2 || cols < 2 || players < 2 || players > 4 {
		return State{}, ErrInvalidAction
	}
	s := State{
		rows: rows, cols: cols, players: players,
		cells: make([]Cell, rows*cols), current: 1, movesLeft: actionsPerTurn,
		bases: [4]Pos{{0, 0}, {rows - 1, cols - 1}, {0, cols - 1}, {rows - 1, 0}},
	}
	for i := 0; i < players; i++ {
		s.active[i] = true
		s.set(s.bases[i], Cell{Owner: Player(i + 1), Kind: Base})
	}
	return s, nil
}

func (s *State) Rows() int             { return s.rows }
func (s *State) Cols() int             { return s.cols }
func (s *State) CurrentPlayer() Player { return s.current }
func (s *State) MovesLeft() int        { return s.movesLeft }
func (s *State) GameOver() bool        { return s.over }
func (s *State) Winner() Player        { return s.winner }

func (s *State) Active(player Player) bool {
	return s.validPlayer(player) && s.active[player-1]
}

func (s *State) NeutralUsed(player Player) bool {
	return s.validPlayer(player) && s.neutralUsed[player-1]
}

func (s *State) At(pos Pos) (Cell, bool) {
	if !s.inBounds(pos) {
		return Cell{}, false
	}
	return s.cells[s.index(pos)], true
}

// LegalActions returns all legal actions for the current player in stable
// board order. Neutral pairs are only present at the start of a turn.
func (s *State) LegalActions() []Action {
	if s.over || !s.Active(s.current) {
		return nil
	}
	targets := s.moveTargets(s.current)
	actions := make([]Action, 0, len(targets))
	for _, pos := range targets {
		actions = append(actions, Action{Kind: Move, Target: pos})
	}
	if s.movesLeft == actionsPerTurn && !s.neutralUsed[s.current-1] {
		var cells []Pos
		for row := 0; row < s.rows; row++ {
			for col := 0; col < s.cols; col++ {
				pos := Pos{row, col}
				cell, _ := s.At(pos)
				if cell.Owner == s.current && cell.Kind == Normal {
					cells = append(cells, pos)
				}
			}
		}
		for i := range cells {
			for j := i + 1; j < len(cells); j++ {
				actions = append(actions, Action{Kind: PlaceNeutrals, Neutrals: [2]Pos{cells[i], cells[j]}})
			}
		}
	}
	return actions
}

// Apply returns the successor state. An error leaves the input state unchanged.
func (s *State) Apply(action Action) (State, error) {
	if s.over {
		return *s, ErrGameOver
	}
	if !s.legalAction(action) {
		return *s, ErrInvalidAction
	}

	next := *s
	next.cells = append([]Cell(nil), s.cells...)
	player := s.current
	if action.Kind == PlaceNeutrals {
		next.set(action.Neutrals[0], Cell{Kind: Neutral})
		next.set(action.Neutrals[1], Cell{Kind: Neutral})
		next.neutralUsed[player-1] = true
		next.movesLeft = 0
	} else {
		target, _ := s.At(action.Target)
		kind := Normal
		if target.Kind == Normal {
			kind = Fortified
		}
		next.set(action.Target, Cell{Owner: player, Kind: kind})
		next.movesLeft--
	}

	next.eliminateStuckPlayers()
	if next.finishIfTerminal() {
		return next, nil
	}
	if !next.Active(player) || next.movesLeft == 0 {
		next.advance(player)
	}
	return next, nil
}

func (s *State) eliminateStuckPlayersGenerated() {
	seen := make([]bool, len(s.cells))
	queue := make([]int32, len(s.cells))
	for player := Player(1); int(player) <= s.players; player++ {
		if s.Active(player) && !s.hasMoveScratch(player, seen, queue) {
			s.active[player-1] = false
			for i := range s.cells {
				if s.cells[i].Owner == player {
					s.cells[i] = Cell{}
				}
			}
		}
	}
}

func (s *State) hasMoveScratch(player Player, seen []bool, queue []int32) bool {
	clear(seen)
	base := s.bases[player-1]
	cell, ok := s.At(base)
	if !ok || cell.Owner != player || cell.Kind != Base {
		return false
	}
	baseIndex := s.index(base)
	seen[baseIndex], queue[0] = true, int32(baseIndex)
	head, tail := 0, 1
	for head < tail {
		current := int(queue[head])
		head++
		row, col := current/s.cols, current%s.cols
		for r := row - 1; r <= row+1; r++ {
			for c := col - 1; c <= col+1; c++ {
				pos := Pos{r, c}
				if !s.inBounds(pos) {
					continue
				}
				index := s.index(pos)
				candidate := s.cells[index]
				if candidate.Kind == Empty || (candidate.Kind == Normal && candidate.Owner != player) {
					return true
				}
				if !seen[index] && candidate.Owner == player {
					seen[index], queue[tail] = true, int32(index)
					tail++
				}
			}
		}
	}
	return false
}

// applyGenerated is the search hot-path transition for an action already
// emitted by Position. It skips redundant legality traversal but shares the
// mutation, elimination, terminal, and turn-advance semantics with Apply.
func (s *State) applyGenerated(action Action) State {
	next := *s
	next.cells = append([]Cell(nil), s.cells...)
	player := s.current
	if action.Kind == PlaceNeutrals {
		next.set(action.Neutrals[0], Cell{Kind: Neutral})
		next.set(action.Neutrals[1], Cell{Kind: Neutral})
		next.neutralUsed[player-1] = true
		next.movesLeft = 0
	} else {
		target := s.cells[s.index(action.Target)]
		kind := Normal
		if target.Kind == Normal {
			kind = Fortified
		}
		next.set(action.Target, Cell{Owner: player, Kind: kind})
		next.movesLeft--
	}
	next.eliminateStuckPlayersGenerated()
	if next.finishIfTerminal() {
		return next
	}
	if !next.Active(player) || next.movesLeft == 0 {
		next.advance(player)
	}
	return next
}

func (s *State) legalAction(action Action) bool {
	if !s.Active(s.current) {
		return false
	}
	switch action.Kind {
	case Move:
		return s.legalMove(s.current, action.Target)
	case PlaceNeutrals:
		if s.movesLeft != actionsPerTurn || s.neutralUsed[s.current-1] || action.Neutrals[0] == action.Neutrals[1] {
			return false
		}
		for _, pos := range action.Neutrals {
			cell, ok := s.At(pos)
			if !ok || cell.Owner != s.current || cell.Kind != Normal {
				return false
			}
		}
		return true
	default:
		return false
	}
}

func (s *State) legalMove(player Player, target Pos) bool {
	cell, ok := s.At(target)
	if !ok || (cell.Kind != Empty && (cell.Kind != Normal || cell.Owner == player)) {
		return false
	}
	connected := s.connected(player)
	for row := target.Row - 1; row <= target.Row+1; row++ {
		for col := target.Col - 1; col <= target.Col+1; col++ {
			pos := Pos{row, col}
			if pos != target && s.inBounds(pos) && connected[s.index(pos)] {
				return true
			}
		}
	}
	return false
}

func (s *State) connected(player Player) []bool {
	seen := make([]bool, len(s.cells))
	if !s.validPlayer(player) {
		return seen
	}
	base := s.bases[player-1]
	cell, ok := s.At(base)
	if !ok || cell.Owner != player || cell.Kind != Base {
		return seen
	}
	seen[s.index(base)] = true
	queue := make([]int32, len(s.cells))
	queue[0] = int32(s.index(base))
	head, tail := 0, 1
	for head < tail {
		current := int(queue[head])
		head++
		pos := Pos{current / s.cols, current % s.cols}
		for row := pos.Row - 1; row <= pos.Row+1; row++ {
			for col := pos.Col - 1; col <= pos.Col+1; col++ {
				next := Pos{row, col}
				if next == pos || !s.inBounds(next) || seen[s.index(next)] {
					continue
				}
				owner, _ := s.At(next)
				if owner.Owner == player {
					seen[s.index(next)] = true
					queue[tail] = int32(s.index(next))
					tail++
				}
			}
		}
	}
	return seen
}

func (s *State) eliminateStuckPlayers() {
	for player := Player(1); int(player) <= s.players; player++ {
		if s.Active(player) && !s.hasMove(player) {
			s.active[player-1] = false
			for i := range s.cells {
				if s.cells[i].Owner == player {
					s.cells[i] = Cell{}
				}
			}
		}
	}
}

func (s *State) hasMove(player Player) bool {
	connected := s.connected(player)
	for index, yes := range connected {
		if !yes {
			continue
		}
		row, col := index/s.cols, index%s.cols
		for nextRow := row - 1; nextRow <= row+1; nextRow++ {
			for nextCol := col - 1; nextCol <= col+1; nextCol++ {
				pos := Pos{nextRow, nextCol}
				if !s.inBounds(pos) {
					continue
				}
				cell := s.cells[s.index(pos)]
				if cell.Kind == Empty || (cell.Kind == Normal && cell.Owner != player) {
					return true
				}
			}
		}
	}
	return false
}

// moveTargets computes a player's connected territory once, then derives its
// legal frontier in stable board order. This is equivalent to legalMove over
// every cell without repeating a full connectivity traversal for each cell.
func (s *State) moveTargets(player Player) []Pos {
	return s.moveTargetsFrom(player, s.connected(player))
}

// moveTargetsFrom derives a player's legal frontier from a precomputed
// connectivity mask, so callers holding that mask (the search Position) do not
// repeat the floodfill.
func (s *State) moveTargetsFrom(player Player, connected []bool) []Pos {
	frontier := make([]bool, len(s.cells))
	for index, isConnected := range connected {
		if !isConnected {
			continue
		}
		row, col := index/s.cols, index%s.cols
		for nextRow := row - 1; nextRow <= row+1; nextRow++ {
			for nextCol := col - 1; nextCol <= col+1; nextCol++ {
				pos := Pos{nextRow, nextCol}
				if !s.inBounds(pos) {
					continue
				}
				cell := s.cells[s.index(pos)]
				if cell.Kind == Empty || (cell.Kind == Normal && cell.Owner != player) {
					frontier[s.index(pos)] = true
				}
			}
		}
	}
	targets := make([]Pos, 0)
	for index, legal := range frontier {
		if legal {
			targets = append(targets, Pos{Row: index / s.cols, Col: index % s.cols})
		}
	}
	return targets
}

func (s *State) finishIfTerminal() bool {
	active, winner := 0, Player(0)
	for player := Player(1); int(player) <= s.players; player++ {
		if s.Active(player) {
			active++
			winner = player
		}
	}
	if active > 1 {
		return false
	}
	s.over = true
	s.winner = winner
	s.movesLeft = 0
	return true
}

func (s *State) advance(after Player) {
	for offset := 1; offset <= s.players; offset++ {
		player := Player((int(after)-1+offset)%s.players + 1)
		if s.Active(player) {
			s.current = player
			s.movesLeft = actionsPerTurn
			return
		}
	}
}

func (s *State) validPlayer(player Player) bool {
	return player >= 1 && int(player) <= s.players
}

func (s *State) inBounds(pos Pos) bool {
	return pos.Row >= 0 && pos.Row < s.rows && pos.Col >= 0 && pos.Col < s.cols
}

func (s *State) index(pos Pos) int       { return pos.Row*s.cols + pos.Col }
func (s *State) set(pos Pos, cell Cell) { s.cells[s.index(pos)] = cell }
