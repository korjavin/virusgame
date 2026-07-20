package search

import (
	"testing"
	"virusgame/game"
)

// Legacy evaluation function logic before vectorization to prove equivalence.
func evaluateAllLegacy(state game.State, workspace *evalWorkspace) [4]int {
	var utility [4]int
	if state.GameOver() {
		for player := game.Player(1); player <= 4; player++ {
			if state.Winner() == player {
				utility[player-1] = mateScore
			} else {
				utility[player-1] = -mateScore
			}
		}
		return utility
	}

	var metrics [4]playerMetrics
	size := state.Rows() * state.Cols()
	workspace.ensure(size)
	cells := snapshotCellsInto(state, workspace.cells)
	connected := allConnectedInto(state, cells, workspace)
	var raw [4]int
	active := 0
	for player := game.Player(1); player <= 4; player++ {
		if !state.Active(player) {
			raw[player-1] = -mateScore / 2
			continue
		}
		active++
		index := player - 1
		metrics[index] = analyzeWithConnectivity(state, player, cells, connected, &workspace.scratch,
			workspace.articulation[index], workspace.cutLoss[index])
		m := metrics[player-1]
		area := state.Rows() * state.Cols()
		owned := m.normal + m.fortified + 1 // include the base
		raw[player-1] = normalized(m.connected, area, 10) +
			normalized(m.normal, area, 30) + normalized(m.fortified, area, 6) +
			normalized(m.mobility, area, 1) + normalized(m.captures, area, 1) -
			normalized(m.disconnected, owned, 1) +
			180*m.baseExits + 80*m.baseOpenings + 240*m.baseAnchors -
			650*m.baseThreat*m.threatTempo -
			m.threatTempo*ratio(m.threatenedLoss, max(1, m.connected)) -
			m.threatTempo*ratio(m.threatened, max(1, m.connected))
		if m.baseExits+m.baseOpenings == 0 {
			raw[player-1] -= 5000
		}
		if !state.NeutralUsed(player) {
			raw[player-1] += 20
		}
		if state.CurrentPlayer() == player {
			raw[player-1] += state.MovesLeft() * 12
		}
	}

	for player := game.Player(1); player <= 4; player++ {
		if !state.Active(player) {
			continue
		}
		own := &metrics[player-1]
		for opponent := game.Player(1); opponent <= 4; opponent++ {
			if opponent == player || !state.Active(opponent) {
				continue
			}
			for index, cut := range metrics[opponent-1].articulation {
				if cut && adjacentConnected(state, index, own.connectedCells) {
					loss := int(metrics[opponent-1].cutLoss[index])
					raw[player-1] += 150 + ratio(loss, max(1, metrics[opponent-1].connected))/2
				}
			}
		}
	}

	for player := game.Player(1); player <= 4; player++ {
		if !state.Active(player) {
			utility[player-1] = raw[player-1]
			continue
		}
		opponents := 0
		for other := game.Player(1); other <= 4; other++ {
			if other != player && state.Active(other) {
				opponents += raw[other-1]
			}
		}
		if active > 1 {
			utility[player-1] = raw[player-1] - opponents/(active-1)
		} else {
			utility[player-1] = raw[player-1]
		}
	}
	return utility
}

func TestEvaluateEquivalenceExtra(t *testing.T) {
	for _, fixture := range []struct {
		rows, cols, players int
		seed                int64
	}{
		{5, 5, 2, 10}, {5, 5, 3, 11}, {5, 5, 4, 12},
		{12, 20, 2, 13}, {12, 20, 3, 14}, {12, 20, 4, 15},
		{20, 12, 2, 16}, {20, 12, 3, 17}, {20, 12, 4, 18},
		{20, 20, 2, 19}, {20, 20, 3, 20}, {20, 20, 4, 21},
	} {
		state := randomReachableState(t, fixture.rows, fixture.cols, fixture.players, fixture.seed)
		workspace := evalWorkspace{}

		got := evaluateAllWithWorkspace(state, &workspace)
		legacy := evaluateAllLegacy(state, &workspace)

		for player := game.Player(1); player <= 4; player++ {
			if got[player-1] != legacy[player-1] {
				t.Fatalf("%dx%d/%dp seat %d vector score %d != legacy score %d", fixture.rows, fixture.cols, fixture.players, player, got[player-1], legacy[player-1])
			}
		}
	}
}

// To ensure transpose/reflection equivalence, we can test state symmetry
// but our equivalence test primarily checks that the refactored logic exactly
// matches the legacy logic under various dimensions and seeds.
