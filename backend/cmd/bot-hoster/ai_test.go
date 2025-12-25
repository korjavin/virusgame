package main

import (
	"testing"
)

func TestMinimaxEngine(t *testing.T) {
	settings := &BotSettings{
		MaterialWeight:   30.0,
		MobilityWeight:   150.0,
		PositionWeight:   130.0,
		RedundancyWeight: 40.0,
		CohesionWeight:   40.0,
		SearchDepth:      3,
	}

	engine := NewAIEngine(settings)

	// Create simple test state
	state := createTestGameState()

	move, ok := engine.CalculateMove(state, 1)
	if !ok {
		t.Fatal("Minimax engine failed to find a move")
	}
	if move == nil {
		t.Fatal("Minimax engine returned nil move")
	}

	t.Logf("Minimax found move: Type=%d, Row=%d, Col=%d", move.Type, move.Row, move.Col)
}

func TestMCTSEngine(t *testing.T) {
	settings := &BotSettings{
		MaterialWeight:   30.0,
		MobilityWeight:   150.0,
		PositionWeight:   130.0,
		RedundancyWeight: 40.0,
		CohesionWeight:   40.0,
		SearchDepth:      3,
	}

	engine := NewMCTSEngine(settings)

	// Create simple test state
	state := createTestGameState()

	move, ok := engine.CalculateMove(state, 1)
	if !ok {
		t.Fatal("MCTS engine failed to find a move")
	}
	if move == nil {
		t.Fatal("MCTS engine returned nil move")
	}

	t.Logf("MCTS found move: Type=%d, Row=%d, Col=%d", move.Type, move.Row, move.Col)
}

func TestRandomAIEngineSelection(t *testing.T) {
	settings := &BotSettings{
		MaterialWeight:   30.0,
		MobilityWeight:   150.0,
		PositionWeight:   130.0,
		RedundancyWeight: 40.0,
		CohesionWeight:   40.0,
		SearchDepth:      3,
	}

	// Run multiple times to ensure both engines can be created
	minimaxCount := 0
	mctsCount := 0

	for i := 0; i < 20; i++ {
		engine, aiType := createRandomAIEngine(settings)
		if engine == nil {
			t.Fatal("createRandomAIEngine returned nil")
		}
		if aiType == "minimax" {
			minimaxCount++
		} else if aiType == "mcts" {
			mctsCount++
		} else {
			t.Fatalf("Unknown AI type: %s", aiType)
		}
	}

	t.Logf("Random selection: Minimax=%d, MCTS=%d", minimaxCount, mctsCount)

	// Check that both types were created at least once (statistically very likely)
	if minimaxCount == 0 {
		t.Log("Warning: Minimax was never selected in 20 tries (unlikely but possible)")
	}
	if mctsCount == 0 {
		t.Log("Warning: MCTS was never selected in 20 tries (unlikely but possible)")
	}
}

// Helper function to create a simple test game state
func createTestGameState() *GameState {
	rows := 10
	cols := 10

	board := make([][]CellValue, rows)
	for i := range board {
		board[i] = make([]CellValue, cols)
	}

	// Set up bases
	board[0][0] = NewCell(1, CellFlagBase)
	board[rows-1][cols-1] = NewCell(2, CellFlagBase)

	// Add some initial territory for player 1
	board[0][1] = NewCell(1, CellFlagNormal)
	board[1][0] = NewCell(1, CellFlagNormal)
	board[1][1] = NewCell(1, CellFlagNormal)

	// Add some territory for player 2
	board[rows-1][cols-2] = NewCell(2, CellFlagNormal)
	board[rows-2][cols-1] = NewCell(2, CellFlagNormal)
	board[rows-2][cols-2] = NewCell(2, CellFlagNormal)

	playerBases := [4]CellPos{
		{Row: 0, Col: 0},
		{Row: rows - 1, Col: cols - 1},
		{Row: 0, Col: cols - 1},
		{Row: rows - 1, Col: 0},
	}

	players := []GamePlayerInfo{
		{PlayerIndex: 0, Username: "Player1", IsBot: false, IsActive: true},
		{PlayerIndex: 1, Username: "Player2", IsBot: false, IsActive: true},
	}

	return &GameState{
		Board:        board,
		Rows:         rows,
		Cols:         cols,
		PlayerBases:  playerBases,
		Players:      players,
		Hash:         0,
		NeutralsUsed: false,
	}
}
