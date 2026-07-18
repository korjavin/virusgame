package arena

import (
	"testing"
	"virusgame/nnuefeat"

	"virusgame/game"
)

// emptyBoard builds a rows×cols board of Empty cells with the two bases placed.
func emptyBoard(rows, cols int) [][]game.Cell {
	board := make([][]game.Cell, rows)
	for r := range board {
		board[r] = make([]game.Cell, cols)
	}
	board[0][0] = game.Cell{Owner: 1, Kind: game.Base}
	board[rows-1][cols-1] = game.Cell{Owner: 2, Kind: game.Base}
	return board
}

func mustState(t *testing.T, snap game.Snapshot) game.State {
	t.Helper()
	state, err := game.FromSnapshot(snap)
	if err != nil {
		t.Fatalf("FromSnapshot: %v", err)
	}
	return state
}

func TestNNUEFeaturesFreshBoard(t *testing.T) {
	state, err := game.New(5, 5, 2)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	feats := NNUEFeatures(state)

	// Player 1 on a fresh 5x5: base only, three empty base-neighbours, symmetric
	// space-race split (9 each, 5 contested anti-diagonal cells).
	want := PlayerFeatures{
		Connected: 1, Mobility: 3, BaseOpenings: 3,
		SpaceRace: 9, NeutralUnused: true, MovesLeftTempo: 3, ThreatTempo: 1,
		// vs-ai2.56: base is the lone frontier cell (3 empty neighbours), no
		// articulation (MinCutThreatDist = rows+cols sentinel), and the enemy base
		// at (4,4) is Chebyshev 4 away.
		MinCutThreatDist: 10, MinEnemyBaseDist: 4, FrontOpenness: 3, FrontWidth: 1,
	}
	if feats[0] != want {
		t.Errorf("p1 fresh:\n got %+v\nwant %+v", feats[0], want)
	}
	// Symmetric board: p2 matches p1 except it is not the mover.
	if feats[1].SpaceRace != 9 || feats[1].Connected != 1 || feats[1].MovesLeftTempo != 0 {
		t.Errorf("p2 fresh: got %+v", feats[1])
	}
	// Inactive seats stay zero.
	if feats[2] != (PlayerFeatures{}) || feats[3] != (PlayerFeatures{}) {
		t.Errorf("inactive seats not zero: %+v %+v", feats[2], feats[3])
	}
}

func TestNNUEFeaturesArticulationChain(t *testing.T) {
	// A three-cell line from p1's base: base(0,0) - normal(0,1) - normal(0,2).
	// (0,1) is the sole articulation point; cutting it strands (0,2), so its
	// cutLoss is 2 (the downstream cell plus itself).
	board := emptyBoard(5, 5)
	board[0][1] = game.Cell{Owner: 1, Kind: game.Normal}
	board[0][2] = game.Cell{Owner: 1, Kind: game.Normal}
	state := mustState(t, game.Snapshot{
		Rows: 5, Cols: 5, Board: board,
		Bases:       []game.Pos{{Row: 0, Col: 0}, {Row: 4, Col: 4}},
		Active:      []bool{true, true},
		NeutralUsed: []bool{false, false},
		Current:     1, MovesLeft: 3,
	})
	f := NNUEFeatures(state)[0]

	check := func(name string, got, want int) {
		if got != want {
			t.Errorf("%s = %d, want %d", name, got, want)
		}
	}
	check("Normal", f.Normal, 2)
	check("Fortified", f.Fortified, 0)
	check("Connected", f.Connected, 3)
	check("Disconnected", f.Disconnected, 0)
	check("Mobility", f.Mobility, 5)
	check("Captures", f.Captures, 0)
	check("BaseExits", f.BaseExits, 1)
	check("BaseOpenings", f.BaseOpenings, 2)
	check("BaseAnchors", f.BaseAnchors, 0)
	check("Articulation", f.Articulation, 1)
	check("MaxCutLoss", f.MaxCutLoss, 2)
	check("Threatened", f.Threatened, 0)
	check("ThreatenedLoss", f.ThreatenedLoss, 0)
	check("MovesLeftTempo", f.MovesLeftTempo, 3)
	if f.SealedBase {
		t.Errorf("SealedBase = true, want false")
	}
	if !f.NeutralUnused {
		t.Errorf("NeutralUnused = false, want true")
	}
	// vs-ai2.56 (d): single-cut loss normalized by own connected mass (2/3).
	if want := 2.0 / 3.0; f.SeverableFrac < want-1e-9 || f.SeverableFrac > want+1e-9 {
		t.Errorf("SeverableFrac = %v, want %v", f.SeverableFrac, want)
	}
	// vs-ai2.56 (a): no enemy stone borders the (0,1) cut, so it is not a
	// threatened cut and the nearest enemy stone is p2's base(4,4) at Chebyshev 4.
	check("ThreatenedCuts", f.ThreatenedCuts, 0)
	check("MinCutThreatDist", f.MinCutThreatDist, 4)
}

func TestNNUEFeaturesCaptureAndThreat(t *testing.T) {
	// p1 base(0,0) + normal(0,1). p2 normal at (1,1) sits adjacent to p1's
	// connected normal — a mutual capture target and a threat.
	board := emptyBoard(5, 5)
	board[0][1] = game.Cell{Owner: 1, Kind: game.Normal}
	board[1][1] = game.Cell{Owner: 2, Kind: game.Normal}
	// give p2 a connected chain back to its base so its normal is "connected"
	board[2][2] = game.Cell{Owner: 2, Kind: game.Normal}
	board[3][3] = game.Cell{Owner: 2, Kind: game.Normal}
	state := mustState(t, game.Snapshot{
		Rows: 5, Cols: 5, Board: board,
		Bases:       []game.Pos{{Row: 0, Col: 0}, {Row: 4, Col: 4}},
		Active:      []bool{true, true},
		NeutralUsed: []bool{false, false},
		Current:     1, MovesLeft: 3,
	})
	f := NNUEFeatures(state)[0]

	// p1's (0,1) normal borders p2's connected (1,1): it is a capture target and
	// p1's (0,1) is threatened by p2's connected territory.
	if f.Captures < 1 {
		t.Errorf("Captures = %d, want >= 1", f.Captures)
	}
	if f.Threatened < 1 {
		t.Errorf("Threatened = %d, want >= 1", f.Threatened)
	}
	// vs-ai2.56 (c): p2's three diagonally-linked normals (1,1)-(2,2)-(3,3) form
	// one 8-connected cluster in capture-contact with p1's territory.
	if f.ChainReach != 3 {
		t.Errorf("ChainReach = %d, want 3", f.ChainReach)
	}
	// vs-ai2.56 (b): p1's nearest cell to p2's base(4,4) is Chebyshev 4; p1's
	// front (base + (0,1)) borders empty cells, so openness is positive.
	if f.MinEnemyBaseDist != 4 {
		t.Errorf("MinEnemyBaseDist = %d, want 4", f.MinEnemyBaseDist)
	}
	if f.FrontOpenness < 1 {
		t.Errorf("FrontOpenness = %d, want >= 1", f.FrontOpenness)
	}
}

func TestFeaturesFlatVectorStable(t *testing.T) {
	f := PlayerFeatures{Normal: 3, SealedBase: true, NeutralUnused: false, MovesLeftTempo: 2}
	v := f.Features()
	if len(v) != nnuefeat.FeatureCount {
		t.Fatalf("Features len = %d, want %d", len(v), nnuefeat.FeatureCount)
	}
	if v[0] != 3 {
		t.Errorf("v[0] Normal = %v, want 3", v[0])
	}
	if v[16] != 1 { // SealedBase flag position
		t.Errorf("v[16] SealedBase = %v, want 1", v[16])
	}
	if v[18] != 2 { // MovesLeftTempo
		t.Errorf("v[18] MovesLeftTempo = %v, want 2", v[18])
	}
}
