package search

import (
	"sort"
	"testing"

	"virusgame/game"
)

// playOneBookTurn drives the current player through one full turn via
// ChooseNodeBudget and returns the resulting state.
func playOneBookTurn(t *testing.T, state game.State) game.State {
	t.Helper()
	actor := state.CurrentPlayer()
	for state.CurrentPlayer() == actor && !state.GameOver() {
		result, ok := ChooseNodeBudget(state, 1000)
		if !ok {
			t.Fatalf("ChooseNodeBudget stalled at player %d", actor)
		}
		next, err := state.Apply(result.Action)
		if err != nil {
			t.Fatalf("illegal action %+v: %v", result.Action, err)
		}
		state = next
	}
	return state
}

// ownedNonBase returns the player's non-base cells in stable board order.
func ownedNonBase(state game.State, player game.Player) []game.Pos {
	var cells []game.Pos
	for row := 0; row < state.Rows(); row++ {
		for col := 0; col < state.Cols(); col++ {
			pos := game.Pos{Row: row, Col: col}
			cell, _ := state.At(pos)
			if cell.Owner == player && cell.Kind != game.Base {
				cells = append(cells, pos)
			}
		}
	}
	return cells
}

func p(r, c int) game.Pos { return game.Pos{Row: r, Col: c} }

func sortPos(cells []game.Pos) {
	sort.Slice(cells, func(i, j int) bool {
		if cells[i].Row != cells[j].Row {
			return cells[i].Row < cells[j].Row
		}
		return cells[i].Col < cells[j].Col
	})
}

func equalPos(a, b []game.Pos) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// TestOpeningBookBlockPerCorner plays each seat's opening turn on 12x12 (4
// players) and on an odd 13x13 (2 players) and asserts the book produces the
// canonical 2x2-minus-base block oriented toward center for every corner base.
func TestOpeningBookBlockPerCorner(t *testing.T) {
	// 12x12, 4 players: bases at the four corners.
	state, err := game.New(12, 12, 4)
	if err != nil {
		t.Fatal(err)
	}
	want := map[game.Player][]game.Pos{
		1: {p(0, 1), p(1, 1), p(2, 1)},       // base (0,0) — spear along col 1
		2: {p(9, 10), p(10, 10), p(11, 10)},  // base (11,11) — the vs-ai2.47 shootout winner
		3: {p(0, 10), p(1, 10), p(2, 10)},    // base (0,11)
		4: {p(9, 1), p(10, 1), p(11, 1)},     // base (11,0)
	}
	for player := game.Player(1); player <= 4; player++ {
		if state.CurrentPlayer() != player {
			t.Fatalf("expected player %d to move, got %d", player, state.CurrentPlayer())
		}
		state = playOneBookTurn(t, state)
		got := ownedNonBase(state, player)
		sortPos(got)
		expect := want[player]
		sortPos(expect)
		if !equalPos(got, expect) {
			t.Errorf("player %d book reply = %v, want %v", player, got, expect)
		}
	}

	// 13x13 (odd), 2 players.
	odd, err := game.New(13, 13, 2)
	if err != nil {
		t.Fatal(err)
	}
	oddWant := map[game.Player][]game.Pos{
		1: {p(0, 1), p(1, 1), p(2, 1)},       // base (0,0)
		2: {p(10, 11), p(11, 11), p(12, 11)}, // base (12,12)
	}
	for player := game.Player(1); player <= 2; player++ {
		odd = playOneBookTurn(t, odd)
		got := ownedNonBase(odd, player)
		sortPos(got)
		expect := oddWant[player]
		sortPos(expect)
		if !equalPos(got, expect) {
			t.Errorf("odd-board player %d book reply = %v, want %v", player, got, expect)
		}
	}
}

// TestOpeningBookFiresExactlyThreeMoves checks the book drives moves 1-3 of the
// opening turn and then stops (turn ends), and that it does not fire again on the
// player's second turn.
func TestOpeningBookFiresExactlyThreeMoves(t *testing.T) {
	state, err := game.New(12, 12, 2)
	if err != nil {
		t.Fatal(err)
	}
	// Moves 1,2,3: book fires each time.
	for move := 1; move <= 3; move++ {
		if _, ok := openingBookMove(state); !ok {
			t.Fatalf("book did not fire on opening move %d", move)
		}
		result, _ := ChooseNodeBudget(state, 1000)
		state, err = state.Apply(result.Action)
		if err != nil {
			t.Fatal(err)
		}
	}
	// Turn has advanced to player 2; play its opening turn too.
	state = playOneBookTurn(t, state)
	// Back to player 1 on its SECOND turn: block already complete, must not fire.
	if state.CurrentPlayer() != 1 {
		t.Fatalf("expected player 1's second turn, got player %d", state.CurrentPlayer())
	}
	if _, ok := openingBookMove(state); ok {
		t.Errorf("book fired on second turn (block already placed) — should defer to search")
	}
}

// TestOpeningBookDoesNotFireMidGameOrSeeded confirms the book defers to search
// whenever the current player owns any cell outside its opening block.
func TestOpeningBookDoesNotFireMidGameOrSeeded(t *testing.T) {
	state, err := game.New(12, 12, 2)
	if err != nil {
		t.Fatal(err)
	}
	// Seed a width-1 diagonal for player 1: (1,1) is in-block but (2,2)/(3,3) are
	// strays, so as soon as a stray is owned the book must stop firing.
	diagonal := []game.Pos{p(1, 1), p(2, 2), p(3, 3)}
	for _, pos := range diagonal {
		state, err = state.Apply(game.Action{Kind: game.Move, Target: pos})
		if err != nil {
			t.Fatalf("seed move %v: %v", pos, err)
		}
	}
	// Play player 2's opening turn (book fires for it — expected), returning to p1.
	state = playOneBookTurn(t, state)
	if state.CurrentPlayer() != 1 {
		t.Fatalf("expected player 1 to move, got %d", state.CurrentPlayer())
	}
	if _, ok := openingBookMove(state); ok {
		t.Errorf("book fired for a player holding stray diagonal cells — should defer to search")
	}
	// Search still returns a legal action on that non-book position.
	result, ok := ChooseNodeBudget(state, 1000)
	if !ok {
		t.Fatal("search fallback failed on non-book position")
	}
	if _, err := state.Apply(result.Action); err != nil {
		t.Errorf("search fallback returned illegal action %+v: %v", result.Action, err)
	}
}

// TestOpeningBookVoidsOnCollision confirms the book defers to search when a block
// cell is not a legal empty placement — e.g. a tiny board where the block would
// overlap another player's base.
func TestOpeningBookVoidsOnCollision(t *testing.T) {
	state, err := game.New(2, 2, 2)
	if err != nil {
		t.Fatal(err)
	}
	// Player 1 base (0,0); block cell (1,1) is player 2's base → void.
	if _, ok := openingBookMove(state); ok {
		t.Errorf("book fired on 2x2 where block collides with enemy base — should defer")
	}
	// Search must still produce a legal action.
	if result, ok := ChooseNodeBudget(state, 1000); ok {
		if _, err := state.Apply(result.Action); err != nil {
			t.Errorf("search fallback illegal on 2x2: %v", err)
		}
	}
}
