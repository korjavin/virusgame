package game

// Snapshot is the wire representation of a complete game position. Players
// and CurrentPlayer are 1-based; slices are ordered by player number.
type Snapshot struct {
	Rows        int      `json:"rows"`
	Cols        int      `json:"cols"`
	Board       [][]Cell `json:"board"`
	Bases       []Pos    `json:"bases"`
	Active      []bool   `json:"active"`
	NeutralUsed []bool   `json:"neutralUsed"`
	Current     Player   `json:"currentPlayer"`
	MovesLeft   int      `json:"movesLeft"`
	GameOver    bool     `json:"gameOver"`
	Winner      Player   `json:"winner"`
}

// FromSnapshot validates and imports an untrusted wire snapshot.
func FromSnapshot(snapshot Snapshot) (State, error) {
	players := len(snapshot.Bases)
	if snapshot.Rows < 2 || snapshot.Cols < 2 || players < 2 || players > 4 ||
		len(snapshot.Active) != players || len(snapshot.NeutralUsed) != players ||
		len(snapshot.Board) != snapshot.Rows || snapshot.MovesLeft < 0 || snapshot.MovesLeft > actionsPerTurn {
		return State{}, ErrInvalidAction
	}
	if snapshot.Current < 1 || int(snapshot.Current) > players ||
		(snapshot.Winner != 0 && (snapshot.Winner < 1 || int(snapshot.Winner) > players)) {
		return State{}, ErrInvalidAction
	}

	state := State{
		rows: snapshot.Rows, cols: snapshot.Cols, players: players,
		current: snapshot.Current, movesLeft: snapshot.MovesLeft,
		over: snapshot.GameOver, winner: snapshot.Winner,
		cells: make([]Cell, 0, snapshot.Rows*snapshot.Cols),
	}
	for player := 0; player < players; player++ {
		base := snapshot.Bases[player]
		if !state.inBounds(base) {
			return State{}, ErrInvalidAction
		}
		for prior := 0; prior < player; prior++ {
			if snapshot.Bases[prior] == base {
				return State{}, ErrInvalidAction
			}
		}
		state.bases[player] = base
		state.active[player] = snapshot.Active[player]
		state.neutralUsed[player] = snapshot.NeutralUsed[player]
	}

	var hasPieces [4]bool
	for rowIndex, row := range snapshot.Board {
		if len(row) != snapshot.Cols {
			return State{}, ErrInvalidAction
		}
		for colIndex, cell := range row {
			if !validSnapshotCell(cell, players) {
				return State{}, ErrInvalidAction
			}
			if cell.Owner > 0 {
				hasPieces[cell.Owner-1] = true
			}
			if cell.Kind == Base && snapshot.Bases[cell.Owner-1] != (Pos{Row: rowIndex, Col: colIndex}) {
				return State{}, ErrInvalidAction
			}
			state.cells = append(state.cells, cell)
		}
	}
	for player := Player(1); int(player) <= players; player++ {
		// vs-ai2.45: eliminated players keep their cells on the board, so an
		// inactive player MAY still own pieces. Only require the forward
		// direction: an active player must have pieces (and an intact base).
		if state.Active(player) && !hasPieces[player-1] {
			return State{}, ErrInvalidAction
		}
		cell, _ := state.At(state.bases[player-1])
		if state.Active(player) && (cell.Kind != Base || cell.Owner != player) {
			return State{}, ErrInvalidAction
		}
	}
	// The side to move is never an eliminated player.
	if !state.over && !state.Active(state.current) {
		return State{}, ErrInvalidAction
	}
	if !state.over && state.winner != 0 {
		return State{}, ErrInvalidAction
	}
	return state, nil
}

func validSnapshotCell(cell Cell, players int) bool {
	if cell.Kind > Neutral || int(cell.Owner) > players {
		return false
	}
	switch cell.Kind {
	case Empty, Neutral:
		return cell.Owner == 0
	case Normal, Base, Fortified:
		return cell.Owner >= 1
	default:
		return false
	}
}

// Snapshot returns a detached wire snapshot of the state.
func (s State) Snapshot() Snapshot {
	snapshot := Snapshot{
		Rows: s.rows, Cols: s.cols, Board: make([][]Cell, s.rows),
		Bases: make([]Pos, s.players), Active: make([]bool, s.players),
		NeutralUsed: make([]bool, s.players), Current: s.current,
		MovesLeft: s.movesLeft, GameOver: s.over, Winner: s.winner,
	}
	for row := 0; row < s.rows; row++ {
		snapshot.Board[row] = append([]Cell(nil), s.cells[row*s.cols:(row+1)*s.cols]...)
	}
	for player := 0; player < s.players; player++ {
		snapshot.Bases[player] = s.bases[player]
		snapshot.Active[player] = s.active[player]
		snapshot.NeutralUsed[player] = s.neutralUsed[player]
	}
	return snapshot
}
