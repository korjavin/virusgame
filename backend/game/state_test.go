package game

import (
	"errors"
	"reflect"
	"testing"
)

func TestNew(t *testing.T) {
	for _, tc := range []struct{ rows, cols, players int }{
		{1, 10, 2}, {10, 1, 2}, {10, 10, 1}, {10, 10, 5},
	} {
		if _, err := New(tc.rows, tc.cols, tc.players); !errors.Is(err, ErrInvalidAction) {
			t.Fatalf("New(%d, %d, %d) error = %v", tc.rows, tc.cols, tc.players, err)
		}
	}

	s, err := New(5, 6, 4)
	if err != nil {
		t.Fatal(err)
	}
	want := []Pos{{0, 0}, {4, 5}, {0, 5}, {4, 0}}
	for i, pos := range want {
		cell, ok := s.At(pos)
		if !ok || cell != (Cell{Owner: Player(i + 1), Kind: Base}) {
			t.Fatalf("base %d at %v = %+v, %v", i+1, pos, cell, ok)
		}
	}
	if s.CurrentPlayer() != 1 || s.MovesLeft() != 3 || s.GameOver() {
		t.Fatalf("unexpected initial turn state: player=%d moves=%d over=%v", s.CurrentPlayer(), s.MovesLeft(), s.GameOver())
	}
}

func TestLegalActionsUseEightWayBaseConnectivity(t *testing.T) {
	s := testState(5, 5, 2)
	s.set(Pos{1, 1}, Cell{Owner: 1, Kind: Normal}) // diagonal from base
	s.set(Pos{3, 3}, Cell{Owner: 1, Kind: Normal}) // disconnected
	s.set(Pos{1, 2}, Cell{Owner: 2, Kind: Normal})
	s.set(Pos{2, 2}, Cell{Owner: 2, Kind: Fortified})
	s.set(Pos{2, 1}, Cell{Kind: Neutral})

	assertMoveLegal(t, s, Pos{0, 1}, true)
	assertMoveLegal(t, s, Pos{1, 2}, true)
	assertMoveLegal(t, s, Pos{2, 3}, false) // only beside disconnected ownership
	assertMoveLegal(t, s, Pos{2, 2}, false)
	assertMoveLegal(t, s, Pos{2, 1}, false)
	assertMoveLegal(t, s, Pos{0, 0}, false)
	assertMoveLegal(t, s, Pos{4, 4}, false)
}

func TestApplyKeepsThreeActionsInOneTurn(t *testing.T) {
	s := testState(6, 6, 2)
	before := append([]Cell(nil), s.cells...)
	for i, pos := range []Pos{{0, 1}, {0, 2}, {0, 3}} {
		var err error
		s, err = s.Apply(Action{Kind: Move, Target: pos})
		if err != nil {
			t.Fatalf("move %d: %v", i+1, err)
		}
		wantPlayer, wantMoves := Player(1), 2-i
		if i == 2 {
			wantPlayer, wantMoves = 2, 3
		}
		if s.CurrentPlayer() != wantPlayer || s.MovesLeft() != wantMoves {
			t.Fatalf("after move %d: player=%d moves=%d", i+1, s.CurrentPlayer(), s.MovesLeft())
		}
	}
	if before[1].Kind != Empty {
		t.Fatal("retained source board changed")
	}
}

func TestCaptureCreatesImmutableFortification(t *testing.T) {
	s := testState(5, 5, 2)
	s.set(Pos{0, 1}, Cell{Owner: 2, Kind: Normal})

	next, err := s.Apply(Action{Kind: Move, Target: Pos{0, 1}})
	if err != nil {
		t.Fatal(err)
	}
	cell, _ := next.At(Pos{0, 1})
	if cell != (Cell{Owner: 1, Kind: Fortified}) {
		t.Fatalf("captured cell = %+v", cell)
	}
	if next.legalMove(2, Pos{0, 1}) {
		t.Fatal("fortified cell can be recaptured")
	}
	if next.legalMove(1, next.bases[1]) {
		t.Fatal("base can be captured")
	}
}

func TestNeutralActionRulesAndTurnConsumption(t *testing.T) {
	s := testState(6, 6, 2)
	s.set(Pos{0, 1}, Cell{Owner: 1, Kind: Normal})
	s.set(Pos{1, 0}, Cell{Owner: 1, Kind: Normal})
	s.set(Pos{1, 1}, Cell{Owner: 1, Kind: Fortified})
	action := Action{Kind: PlaceNeutrals, Neutrals: [2]Pos{{0, 1}, {1, 0}}}

	next, err := s.Apply(action)
	if err != nil {
		t.Fatal(err)
	}
	if next.CurrentPlayer() != 2 || next.MovesLeft() != 3 || !next.NeutralUsed(1) {
		t.Fatalf("neutral turn result: player=%d moves=%d used=%v", next.CurrentPlayer(), next.MovesLeft(), next.NeutralUsed(1))
	}
	for _, pos := range action.Neutrals {
		cell, _ := next.At(pos)
		if cell != (Cell{Kind: Neutral}) {
			t.Fatalf("neutral at %v = %+v", pos, cell)
		}
	}

	bad := []Action{
		{Kind: PlaceNeutrals, Neutrals: [2]Pos{{0, 1}, {0, 1}}},
		{Kind: PlaceNeutrals, Neutrals: [2]Pos{{0, 0}, {1, 0}}},
		{Kind: PlaceNeutrals, Neutrals: [2]Pos{{1, 1}, {1, 0}}},
	}
	for _, action := range bad {
		if _, err := s.Apply(action); !errors.Is(err, ErrInvalidAction) {
			t.Fatalf("Apply(%+v) error = %v", action, err)
		}
	}
	midTurn, err := s.Apply(Action{Kind: Move, Target: Pos{0, 2}})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := midTurn.Apply(action); !errors.Is(err, ErrInvalidAction) {
		t.Fatalf("mid-turn neutral error = %v", err)
	}
}

func TestEliminationAndTerminalResult(t *testing.T) {
	s := testState(5, 5, 2)
	s.set(Pos{1, 1}, Cell{Owner: 1, Kind: Normal})
	s.set(Pos{2, 2}, Cell{Owner: 1, Kind: Normal})
	s.set(Pos{3, 3}, Cell{Owner: 2, Kind: Normal})
	for _, pos := range []Pos{{3, 4}, {4, 3}} {
		s.set(pos, Cell{Kind: Neutral})
	}

	next, err := s.Apply(Action{Kind: Move, Target: Pos{3, 3}})
	if err != nil {
		t.Fatal(err)
	}
	if !next.GameOver() || next.Winner() != 1 || next.Active(2) || next.MovesLeft() != 0 {
		t.Fatalf("terminal state: over=%v winner=%d p2=%v moves=%d", next.GameOver(), next.Winner(), next.Active(2), next.MovesLeft())
	}
	// vs-ai2.45: eliminated player's cells stay on the board (owned by the
	// dead player), they just never move again.
	if base, _ := next.At(Pos{4, 4}); base.Owner != 2 || base.Kind != Base {
		t.Fatalf("eliminated player's base was wiped: %+v", base)
	}
	if _, err := next.Apply(Action{Kind: Move, Target: Pos{0, 1}}); !errors.Is(err, ErrGameOver) {
		t.Fatalf("post-game Apply error = %v", err)
	}
}

// vs-ai2.45: in a multiplayer game, eliminating one player must leave their
// cells on the board (owned by the dead player, capturable per normal rules),
// keep them permanently inactive, and let the remaining players play on.
func TestMultiplayerEliminatedCellsStayCapturable(t *testing.T) {
	s := testState(5, 5, 3)
	// Player 1 territory reaching toward the doomed player's stray cell.
	s.set(Pos{1, 1}, Cell{Owner: 1, Kind: Normal})
	s.set(Pos{2, 1}, Cell{Owner: 1, Kind: Normal})
	// Player 2's base (4,4) is walled by neutrals -> no move -> eliminated.
	// A stray Normal at (2,2) is disconnected from the base, so it neither
	// rescues player 2 nor vanishes on elimination.
	s.set(Pos{2, 2}, Cell{Owner: 2, Kind: Normal})
	for _, pos := range []Pos{{3, 3}, {3, 4}, {4, 3}} {
		s.set(pos, Cell{Kind: Neutral})
	}

	next, err := s.Apply(Action{Kind: Move, Target: Pos{0, 1}})
	if err != nil {
		t.Fatal(err)
	}
	if next.GameOver() {
		t.Fatal("game ended with two players still active")
	}
	if next.Active(2) {
		t.Fatal("stuck player 2 was not eliminated")
	}
	if !next.Active(1) || !next.Active(3) {
		t.Fatal("a surviving player was wrongly eliminated")
	}
	if base, _ := next.At(Pos{4, 4}); base.Owner != 2 || base.Kind != Base {
		t.Fatalf("eliminated player's base was wiped: %+v", base)
	}
	stray, _ := next.At(Pos{2, 2})
	if stray.Owner != 2 || stray.Kind != Normal {
		t.Fatalf("eliminated player's stray cell was wiped: %+v", stray)
	}

	// The dead cell is capturable per normal rules: player 1 (still to move,
	// connected via (2,1)) can take it, and it fortifies as any Normal capture.
	captured, err := next.Apply(Action{Kind: Move, Target: Pos{2, 2}})
	if err != nil {
		t.Fatalf("could not capture eliminated player's cell: %v", err)
	}
	if got, _ := captured.At(Pos{2, 2}); got.Owner != 1 || got.Kind != Fortified {
		t.Fatalf("capture of dead cell = %+v, want owner 1 fortified", got)
	}
	// Elimination is permanent: player 2 never reactivates.
	if captured.Active(2) {
		t.Fatal("eliminated player 2 reactivated")
	}

	// The state with a dead player round-trips through the wire snapshot.
	if _, err := FromSnapshot(next.Snapshot()); err != nil {
		t.Fatalf("snapshot round-trip rejected state with eliminated player: %v", err)
	}
}

func TestLegalActionsAreDeterministicAndApplicable(t *testing.T) {
	s := testState(5, 5, 2)
	s.set(Pos{0, 1}, Cell{Owner: 1, Kind: Normal})
	s.set(Pos{1, 0}, Cell{Owner: 1, Kind: Normal})
	a := s.LegalActions()
	b := s.LegalActions()
	if !reflect.DeepEqual(a, b) || len(a) == 0 {
		t.Fatalf("legal generation is not stable: %v / %v", a, b)
	}
	seen := make(map[Action]bool, len(a))
	for _, action := range a {
		if seen[action] {
			t.Fatalf("duplicate action: %+v", action)
		}
		seen[action] = true
		if _, err := s.Apply(action); err != nil {
			t.Fatalf("generated action %+v is invalid: %v", action, err)
		}
	}
}

func TestIllegalApplyDoesNotMutateState(t *testing.T) {
	s := testState(5, 5, 2)
	want := s
	want.cells = append([]Cell(nil), s.cells...)
	got, err := s.Apply(Action{Kind: Move, Target: Pos{-1, 0}})
	if !errors.Is(err, ErrInvalidAction) {
		t.Fatalf("error = %v", err)
	}
	if !reflect.DeepEqual(s, want) || !reflect.DeepEqual(got, want) {
		t.Fatal("illegal Apply mutated or replaced the input state")
	}
}

func testState(rows, cols, players int) State {
	s, err := New(rows, cols, players)
	if err != nil {
		panic(err)
	}
	return s
}

func assertMoveLegal(t *testing.T, s State, pos Pos, want bool) {
	t.Helper()
	if got := s.legalMove(s.current, pos); got != want {
		t.Fatalf("legalMove(%v) = %v, want %v", pos, got, want)
	}
}
