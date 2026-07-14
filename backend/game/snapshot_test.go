package game

import "testing"

func TestSnapshotRoundTrip(t *testing.T) {
	state, err := New(5, 6, 4)
	if err != nil {
		t.Fatal(err)
	}
	got, err := FromSnapshot(state.Snapshot())
	if err != nil {
		t.Fatal(err)
	}
	if got.Rows() != 5 || got.Cols() != 6 || got.CurrentPlayer() != 1 || got.MovesLeft() != 3 {
		t.Fatalf("unexpected state: %+v", got.Snapshot())
	}
}

func TestFromSnapshotRejectsMalformedInput(t *testing.T) {
	state, _ := New(3, 3, 2)
	tests := map[string]func(*Snapshot){
		"ragged board":         func(s *Snapshot) { s.Board[1] = s.Board[1][:2] },
		"bad cell owner":       func(s *Snapshot) { s.Board[1][1] = Cell{Owner: 3, Kind: Normal} },
		"bad cell kind":        func(s *Snapshot) { s.Board[1][1] = Cell{Kind: CellKind(99)} },
		"bad base":             func(s *Snapshot) { s.Bases[0] = Pos{-1, 0} },
		"zero player":          func(s *Snapshot) { s.Current = 0 },
		"winner while running": func(s *Snapshot) { s.Winner = 1 },
		"forged active":        func(s *Snapshot) { s.Active[0] = false },
		"misplaced base":       func(s *Snapshot) { s.Board[1][1] = Cell{Owner: 1, Kind: Base} },
	}
	for name, breakSnapshot := range tests {
		t.Run(name, func(t *testing.T) {
			snapshot := state.Snapshot()
			breakSnapshot(&snapshot)
			if _, err := FromSnapshot(snapshot); err == nil {
				t.Fatal("accepted malformed snapshot")
			}
		})
	}
}
