package search

import (

	"virusgame/game"
)

// openingBookResult wraps openingBookMove as a completed search Result so every
// entry point (Choose, ChooseNodeBudget, ChooseDepth) can short-circuit its
// iterative deepening on the opening turn with a single guard.
func openingBookResult(state game.State) (Result, bool) {
	action, ok := openingBookMove(state)
	if !ok {
		return Result{}, false
	}
	return Result{Action: action, SearchComplete: true}, true
}

// openingBookMove returns the canonical thick first-turn placement for the
// current player while it is on its opening turn from a fresh game, or false to
// defer to search.
//
// The plain search reply to an empty board is a width-1 diagonal tendril: every
// cell is an articulation point, so a single enemy cut forfeits the whole distal
// chain (prod losses b543fe02/bbfc5e0c/82d29155). Sweeps proved this shape is
// genuinely eval-optimal from empty, so no eval term dislodges it at a safe
// weight — so we place the reply by fiat. The line is the "spear" (two anchors +
// forward probe along the inward column), the winner of the vs-ai2.47 opening
// shootout. Deterministic, no data files, no tuning.
//
// Fires only while the current player owns exactly its base plus a prefix of that
// block (the opening turn, spread over its three per-move Choose calls). Any own
// cell outside the block (mid-game, seeded position) or a block cell that is not a
// legal empty placement (tiny board where the block collides with another base)
// voids the book and search runs unchanged.
func openingBookMove(state game.State) (game.Action, bool) {
	if state.GameOver() {
		return game.Action{}, false
	}
	player := state.CurrentPlayer()
	if !state.Active(player) {
		return game.Action{}, false
	}

	base, ok := findBase(state, player)
	if !ok {
		return game.Action{}, false
	}

	// Orient inward toward the board center. Starting bases are corners, so each
	// delta resolves to +1 or -1; the comparison keeps it correct for odd sizes.
	dr, dc := -1, -1
	if base.Row*2 < state.Rows()-1 {
		dr = 1
	}
	if base.Col*2 < state.Cols()-1 {
		dc = 1
	}
	// The "spear": two robust anchors (no articulation at the base junction) plus
	// one forward probe — width-2 AND advancing. The vs-ai2.47 shootout measured it
	// as the only line that decisively beats the base-hugging block (22-8, Wilson95
	// [55.6%,85.8%]) while halving the block's base halo (2 capturable base-adjacent
	// cells vs 3 — captured cells become enemy fortified footholds, the owner's
	// "no gratuitous own-base halo" motif). Order is load-bearing: the probe is only
	// connected via the diagonal anchor, so cells must be placed in array order.
	block := [3]game.Pos{
		{Row: base.Row, Col: base.Col + dc},          // base-adjacent anchor
		{Row: base.Row + dr, Col: base.Col + dc},     // diagonal anchor
		{Row: base.Row + 2*dr, Col: base.Col + dc},   // forward probe
	}
	inBlock := func(p game.Pos) bool {
		return p == block[0] || p == block[1] || p == block[2]
	}

	// Any own non-base cell outside the block means this is not a fresh opening
	// turn (mid-game or a seeded position) — defer to search.
	for row := 0; row < state.Rows(); row++ {
		for col := 0; col < state.Cols(); col++ {
			pos := game.Pos{Row: row, Col: col}
			cell, _ := state.At(pos)
			if cell.Owner == player && cell.Kind != game.Base && !inBlock(pos) {
				return game.Action{}, false
			}
		}
	}

	// Every spear cell must be reachable: already ours from an earlier book move,
	// or a legal empty placement (the anchors touch the base; the probe touches the
	// diagonal anchor, which array order guarantees is placed first). A collision —
	// out of bounds, or another player's cell/base on a tiny board — voids the
	// book. The first still-empty cell in array order is the next move.
	next := game.Pos{Row: -1}
	placed := 0
	for _, b := range block {
		cell, ok := state.At(b)
		if !ok {
			return game.Action{}, false
		}
		switch {
		case cell.Kind == game.Empty:
			if next.Row < 0 {
				next = b
			}
		case cell.Owner == player: // placed by an earlier book move this turn
			placed++
		default:
			return game.Action{}, false
		}
	}
	if next.Row < 0 {
		return game.Action{}, false // block already complete — opening over
	}
	// Only fire on the player's genuine first turn. Across that turn's three
	// per-move Choose calls placed+movesLeft is invariant at 3 (0+3, 1+2, 2+1);
	// any later turn (or a player captured down to a block-cell prefix mid-game,
	// as some fixed-depth goldens are) fails this and defers to search.
	if placed+state.MovesLeft() != 3 {
		return game.Action{}, false
	}
	return game.Action{Kind: game.Move, Target: next}, true
}

// findBase locates the current player's Base cell.
func findBase(state game.State, player game.Player) (game.Pos, bool) {
	for row := 0; row < state.Rows(); row++ {
		for col := 0; col < state.Cols(); col++ {
			pos := game.Pos{Row: row, Col: col}
			cell, _ := state.At(pos)
			if cell.Owner == player && cell.Kind == game.Base {
				return pos, true
			}
		}
	}
	return game.Pos{}, false
}
