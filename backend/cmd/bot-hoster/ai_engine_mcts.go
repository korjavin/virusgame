package main

import (
	"fmt"
	"log"
	"math"
	"math/rand"
	"time"
)

// MCTSEngine implements Monte Carlo Tree Search algorithm for bot AI
type MCTSEngine struct {
	settings  *BotSettings
	timeLimit time.Duration
	rng       *rand.Rand
}

// MCTSNode represents a node in the search tree
type MCTSNode struct {
	state          *GameState
	move           *Move        // Move that led to this state
	parent         *MCTSNode
	children       []*MCTSNode
	visits         int
	totalScore     float64
	untriedMoves   []Move
	player         int  // Player who made the move to reach this state
	isTerminal     bool
}

const (
	explorationConstant = 1.41 // sqrt(2), standard UCB1 constant
	simulationDepth     = 15   // Max depth for simulation rollout
)

func NewMCTSEngine(settings *BotSettings) *MCTSEngine {
	return &MCTSEngine{
		settings:  settings,
		timeLimit: 670 * time.Millisecond, // Same as minimax
		rng:       rand.New(rand.NewSource(time.Now().UnixNano())),
	}
}

// CalculateMove performs MCTS search and returns the best move
func (mcts *MCTSEngine) CalculateMove(state *GameState, player int) (*Move, bool) {
	log.Printf("[MCTS] Starting search for player %d (time limit: %dms)",
		player, mcts.timeLimit.Milliseconds())

	startTime := time.Now()

	// Get all valid moves
	validMoves := mcts.getAllValidMoves(state, player)
	if len(validMoves) == 0 {
		return nil, false
	}

	// If only one move, return it immediately
	if len(validMoves) == 1 {
		log.Printf("[MCTS] Only one valid move, returning it")
		return &validMoves[0], true
	}

	// Create root node
	root := &MCTSNode{
		state:        state,
		move:         nil,
		parent:       nil,
		children:     make([]*MCTSNode, 0),
		visits:       0,
		totalScore:   0,
		untriedMoves: validMoves,
		player:       player,
		isTerminal:   false,
	}

	iterations := 0
	for time.Since(startTime) < mcts.timeLimit {
		// MCTS main loop: Selection -> Expansion -> Simulation -> Backpropagation
		node := mcts.selectNode(root, player)

		if node == nil {
			break
		}

		// Expand if not terminal and has untried moves
		if len(node.untriedMoves) > 0 && !node.isTerminal {
			node = mcts.expand(node, player)
		}

		// Simulate
		score := mcts.simulate(node, player)

		// Backpropagate
		mcts.backpropagate(node, score)

		iterations++
	}

	elapsed := time.Since(startTime)
	log.Printf("[MCTS] Completed %d iterations in %dms", iterations, elapsed.Milliseconds())

	// Select best child based on visit count (most robust)
	var bestChild *MCTSNode
	maxVisits := -1
	for _, child := range root.children {
		if child.visits > maxVisits {
			maxVisits = child.visits
			bestChild = child
		}
	}

	if bestChild == nil || bestChild.move == nil {
		log.Printf("[MCTS] No best move found, using first valid move")
		return &validMoves[0], true
	}

	avgScore := 0.0
	if bestChild.visits > 0 {
		avgScore = bestChild.totalScore / float64(bestChild.visits)
	}

	log.Printf("[MCTS] Selected move: Type=%d, visits=%d, avg_score=%.2f",
		bestChild.move.Type, bestChild.visits, avgScore)

	return bestChild.move, true
}

// selectNode traverses the tree using UCB1 to find the most promising node to expand
func (mcts *MCTSEngine) selectNode(node *MCTSNode, aiPlayer int) *MCTSNode {
	for len(node.untriedMoves) == 0 && len(node.children) > 0 {
		node = mcts.selectBestUCB(node, aiPlayer)
		if node == nil {
			return nil
		}
	}
	return node
}

// selectBestUCB selects child with highest UCB1 value
func (mcts *MCTSEngine) selectBestUCB(node *MCTSNode, aiPlayer int) *MCTSNode {
	var bestChild *MCTSNode
	bestValue := math.Inf(-1)

	for _, child := range node.children {
		if child.visits == 0 {
			// Prioritize unexplored children
			return child
		}

		// UCB1 formula: exploitation + exploration
		exploitation := child.totalScore / float64(child.visits)
		exploration := explorationConstant * math.Sqrt(math.Log(float64(node.visits))/float64(child.visits))

		// If it's opponent's turn, invert the score
		if child.player != aiPlayer {
			exploitation = -exploitation
		}

		ucbValue := exploitation + exploration

		if ucbValue > bestValue {
			bestValue = ucbValue
			bestChild = child
		}
	}

	return bestChild
}

// expand creates a new child node by applying an untried move
func (mcts *MCTSEngine) expand(node *MCTSNode, aiPlayer int) *MCTSNode {
	if len(node.untriedMoves) == 0 {
		return node
	}

	// Pick random untried move
	moveIndex := mcts.rng.Intn(len(node.untriedMoves))
	move := node.untriedMoves[moveIndex]

	// Remove from untried moves
	node.untriedMoves = append(node.untriedMoves[:moveIndex], node.untriedMoves[moveIndex+1:]...)

	// Apply move to create new state
	newBoard := mcts.copyBoard(node.state.Board)
	newHash := mcts.applyMove(newBoard, move, node.player)

	newState := &GameState{
		Board:        newBoard,
		Rows:         node.state.Rows,
		Cols:         node.state.Cols,
		PlayerBases:  node.state.PlayerBases,
		Players:      mcts.copyPlayers(node.state.Players),
		Hash:         newHash,
		NeutralsUsed: node.state.NeutralsUsed || (move.Type == MoveTypeNeutral),
	}

	// Determine next player
	nextPlayer := mcts.getNextPlayer(newState, node.player, aiPlayer)

	// Check if terminal
	isTerminal := mcts.isTerminalState(newState, aiPlayer)

	// Get valid moves for next state
	var nextMoves []Move
	if !isTerminal {
		nextMoves = mcts.getAllValidMoves(newState, nextPlayer)
		if len(nextMoves) == 0 {
			isTerminal = true
		}
	}

	// Create child node
	child := &MCTSNode{
		state:        newState,
		move:         &move,
		parent:       node,
		children:     make([]*MCTSNode, 0),
		visits:       0,
		totalScore:   0,
		untriedMoves: nextMoves,
		player:       nextPlayer,
		isTerminal:   isTerminal,
	}

	node.children = append(node.children, child)
	return child
}

// simulate performs a random playout from the given node
func (mcts *MCTSEngine) simulate(node *MCTSNode, aiPlayer int) float64 {
	if node.isTerminal {
		return mcts.evaluateTerminal(node.state, aiPlayer)
	}

	// Create simulation state
	simState := &GameState{
		Board:        mcts.copyBoard(node.state.Board),
		Rows:         node.state.Rows,
		Cols:         node.state.Cols,
		PlayerBases:  node.state.PlayerBases,
		Players:      mcts.copyPlayers(node.state.Players),
		Hash:         node.state.Hash,
		NeutralsUsed: node.state.NeutralsUsed,
	}

	currentPlayer := node.player
	depth := 0

	for depth < simulationDepth {
		moves := mcts.getAllValidMoves(simState, currentPlayer)
		if len(moves) == 0 {
			// No moves available - terminal state
			break
		}

		// Select move using simple heuristic (not purely random)
		move := mcts.selectSimulationMove(simState, moves, currentPlayer)

		// Apply move
		newHash := mcts.applyMove(simState.Board, move, currentPlayer)
		simState.Hash = newHash
		if move.Type == MoveTypeNeutral {
			simState.NeutralsUsed = true
		}

		// Switch to next player
		currentPlayer = mcts.getNextPlayer(simState, currentPlayer, aiPlayer)
		depth++
	}

	// Evaluate final position
	return mcts.evaluateState(simState, aiPlayer)
}

// selectSimulationMove chooses a move during simulation using heuristic
func (mcts *MCTSEngine) selectSimulationMove(state *GameState, moves []Move, player int) Move {
	// Score all moves quickly
	bestScore := math.Inf(-1)
	var bestMove Move

	// Limit evaluation to first 10 moves for speed
	limit := len(moves)
	if limit > 10 {
		limit = 10
	}

	for i := 0; i < limit; i++ {
		move := moves[i]
		score := mcts.quickScoreMove(state, move, player)
		if score > bestScore {
			bestScore = score
			bestMove = move
		}
	}

	// If we didn't find a good move, pick randomly
	if bestScore == math.Inf(-1) && len(moves) > 0 {
		return moves[mcts.rng.Intn(len(moves))]
	}

	return bestMove
}

// quickScoreMove provides fast heuristic evaluation for a move
func (mcts *MCTSEngine) quickScoreMove(state *GameState, move Move, player int) float64 {
	if move.Type == MoveTypeNeutral {
		return -500.0 // Generally avoid neutrals in simulation
	}

	score := 0.0

	// Check if move defeats opponent
	if mcts.checkIfMoveDefeatsOpponent(state, move, player) > 0 {
		return 10000.0
	}

	cell := state.Board[move.Row][move.Col]

	// Prefer captures
	if cell != 0 && cell.Player() != player {
		score += 100.0
	}

	// Count friendly neighbors
	neighbors := mcts.countFriendlyNeighbors(state.Board, move.Row, move.Col, player, state.Rows, state.Cols)
	score += float64(neighbors * 10)

	return score
}

// backpropagate updates node statistics up the tree
func (mcts *MCTSEngine) backpropagate(node *MCTSNode, score float64) {
	for node != nil {
		node.visits++
		node.totalScore += score
		node = node.parent
	}
}

// evaluateState evaluates a game state from AI player's perspective
func (mcts *MCTSEngine) evaluateState(state *GameState, aiPlayer int) float64 {
	// Count pieces and territory
	aiPieces := 0
	oppPieces := 0
	aiFortified := 0
	oppFortified := 0

	for r := 0; r < state.Rows; r++ {
		for c := 0; c < state.Cols; c++ {
			cell := state.Board[r][c]
			if cell != 0 {
				if cell.Player() == aiPlayer {
					aiPieces++
					if cell.IsFortified() {
						aiFortified++
					}
				} else if cell.Player() > 0 {
					oppPieces++
					if cell.IsFortified() {
						oppFortified++
					}
				}
			}
		}
	}

	// Simple material-based evaluation
	materialScore := float64(aiPieces - oppPieces)
	fortifiedScore := float64(aiFortified - oppFortified)

	return materialScore*10.0 + fortifiedScore*20.0
}

// evaluateTerminal evaluates a terminal state
func (mcts *MCTSEngine) evaluateTerminal(state *GameState, aiPlayer int) float64 {
	// Check if AI player has pieces
	aiAlive := false
	opponentsAlive := 0

	for r := 0; r < state.Rows; r++ {
		for c := 0; c < state.Cols; c++ {
			cell := state.Board[r][c]
			if cell != 0 {
				if cell.Player() == aiPlayer {
					aiAlive = true
				} else if cell.Player() > 0 {
					opponentsAlive++
				}
			}
		}
	}

	if !aiAlive {
		return -10000.0 // AI lost
	}
	if opponentsAlive == 0 {
		return 10000.0 // AI won
	}

	return mcts.evaluateState(state, aiPlayer)
}

// isTerminalState checks if the game is over
func (mcts *MCTSEngine) isTerminalState(state *GameState, aiPlayer int) bool {
	activePlayers := 0
	for p := 1; p <= 4; p++ {
		if mcts.hasActivePieces(state, p) {
			activePlayers++
		}
	}
	return activePlayers <= 1
}

// hasActivePieces checks if a player has any pieces
func (mcts *MCTSEngine) hasActivePieces(state *GameState, player int) bool {
	for r := 0; r < state.Rows; r++ {
		for c := 0; c < state.Cols; c++ {
			cell := state.Board[r][c]
			if cell != 0 && cell.Player() == player {
				return true
			}
		}
	}
	return false
}

// Helper functions (reuse logic from minimax where applicable)

func (mcts *MCTSEngine) getAllValidMoves(state *GameState, player int) []Move {
	var moves []Move

	// Standard moves
	for row := 0; row < state.Rows; row++ {
		for col := 0; col < state.Cols; col++ {
			if mcts.isValidMove(state, row, col, player) {
				moves = append(moves, Move{Type: MoveTypeStandard, Row: row, Col: col})
			}
		}
	}

	// Neutral moves (simplified - only consider if desperate)
	if !state.NeutralsUsed && len(moves) < 3 {
		neutralMoves := mcts.getNeutralMoves(state, player)
		moves = append(moves, neutralMoves...)
	}

	return moves
}

func (mcts *MCTSEngine) getNeutralMoves(state *GameState, player int) []Move {
	// Simplified version - just pick threatened cells near base
	var threatenedCells []CellPos
	base := state.PlayerBases[player-1]

	for r := 0; r < state.Rows; r++ {
		for c := 0; c < state.Cols; c++ {
			cell := state.Board[r][c]
			if cell != 0 && cell.Player() == player && !cell.IsBase() {
				// Check if close to base and has opponent neighbors
				dist := abs(r-base.Row) + abs(c-base.Col)
				if dist < 5 {
					if mcts.hasOpponentNeighbor(state.Board, r, c, player, state.Rows, state.Cols) {
						threatenedCells = append(threatenedCells, CellPos{Row: r, Col: c})
					}
				}
			}
		}
	}

	if len(threatenedCells) < 2 {
		return nil
	}

	// Generate one neutral move from top 2 threatened cells
	var moves []Move
	if len(threatenedCells) >= 2 {
		moves = append(moves, Move{
			Type:  MoveTypeNeutral,
			Cells: []CellPos{threatenedCells[0], threatenedCells[1]},
		})
	}

	return moves
}

func (mcts *MCTSEngine) isValidMove(state *GameState, row, col, player int) bool {
	if row < 0 || row >= state.Rows || col < 0 || col >= state.Cols {
		return false
	}

	cell := state.Board[row][col]

	if cell != 0 && cell.IsKilled() {
		return false
	}

	if cell != 0 && !cell.CanBeAttacked() {
		return false
	}

	if cell != 0 {
		cellPlayer := cell.Player()
		if cellPlayer == player || cellPlayer == 0 {
			return false
		}
	}

	return mcts.isAdjacentAndConnected(state, row, col, player)
}

func (mcts *MCTSEngine) isAdjacentAndConnected(state *GameState, row, col, player int) bool {
	for i := -1; i <= 1; i++ {
		for j := -1; j <= 1; j++ {
			if i == 0 && j == 0 {
				continue
			}
			adjRow := row + i
			adjCol := col + j
			if adjRow >= 0 && adjRow < state.Rows && adjCol >= 0 && adjCol < state.Cols {
				adjCell := state.Board[adjRow][adjCol]
				if adjCell != 0 && adjCell.Player() == player {
					if mcts.isConnectedToBase(state, adjRow, adjCol, player) {
						return true
					}
				}
			}
		}
	}
	return false
}

func (mcts *MCTSEngine) isConnectedToBase(state *GameState, startRow, startCol, player int) bool {
	base := state.PlayerBases[player-1]
	visited := make(map[string]bool)
	stack := []struct{ row, col int }{{startRow, startCol}}

	for len(stack) > 0 {
		curr := stack[len(stack)-1]
		stack = stack[:len(stack)-1]

		key := fmt.Sprintf("%d,%d", curr.row, curr.col)
		if visited[key] {
			continue
		}
		visited[key] = true

		if curr.row == base.Row && curr.col == base.Col {
			return true
		}

		for i := -1; i <= 1; i++ {
			for j := -1; j <= 1; j++ {
				if i == 0 && j == 0 {
					continue
				}
				newRow := curr.row + i
				newCol := curr.col + j

				if newRow >= 0 && newRow < state.Rows && newCol >= 0 && newCol < state.Cols {
					cell := state.Board[newRow][newCol]
					if cell != 0 && cell.Player() == player {
						stack = append(stack, struct{ row, col int }{newRow, newCol})
					}
				}
			}
		}
	}
	return false
}

func (mcts *MCTSEngine) copyBoard(board [][]CellValue) [][]CellValue {
	newBoard := make([][]CellValue, len(board))
	for i := range board {
		newBoard[i] = make([]CellValue, len(board[i]))
		copy(newBoard[i], board[i])
	}
	return newBoard
}

func (mcts *MCTSEngine) copyPlayers(players []GamePlayerInfo) []GamePlayerInfo {
	newPlayers := make([]GamePlayerInfo, len(players))
	copy(newPlayers, players)
	return newPlayers
}

func (mcts *MCTSEngine) applyMove(board [][]CellValue, move Move, player int) uint64 {
	if move.Type == MoveTypeNeutral {
		for _, cell := range move.Cells {
			board[cell.Row][cell.Col] = NewCell(0, CellFlagKilled)
		}
		return 0
	}

	row, col := move.Row, move.Col
	oldCell := board[row][col]

	if oldCell == 0 {
		board[row][col] = NewCell(player, CellFlagNormal)
	} else {
		board[row][col] = NewCell(player, CellFlagFortified)
	}

	return 0 // Hash not needed for MCTS
}

func (mcts *MCTSEngine) getNextPlayer(state *GameState, currentPlayer, aiPlayer int) int {
	// Find next active opponent after current player
	for i := 1; i <= 4; i++ {
		candidate := currentPlayer + i
		if candidate > 4 {
			candidate -= 4
		}

		if candidate == currentPlayer {
			continue
		}

		// Check if player is active
		for _, p := range state.Players {
			if p.PlayerIndex+1 == candidate && p.IsActive {
				if mcts.hasActivePieces(state, candidate) {
					return candidate
				}
			}
		}
	}

	return currentPlayer
}

func (mcts *MCTSEngine) checkIfMoveDefeatsOpponent(state *GameState, move Move, player int) int {
	if move.Type == MoveTypeNeutral {
		return 0
	}

	cellValue := state.Board[move.Row][move.Col]
	if cellValue == 0 {
		return 0
	}

	targetPlayer := cellValue.Player()
	if targetPlayer == 0 || targetPlayer == player {
		return 0
	}

	// Check if target player only has 1 piece
	count := 0
	for r := 0; r < state.Rows && count <= 1; r++ {
		for c := 0; c < state.Cols && count <= 1; c++ {
			if state.Board[r][c] != 0 && state.Board[r][c].Player() == targetPlayer {
				count++
			}
		}
	}

	if count == 1 {
		return targetPlayer
	}
	return 0
}

func (mcts *MCTSEngine) countFriendlyNeighbors(board [][]CellValue, row, col, player, rows, cols int) int {
	count := 0
	for i := -1; i <= 1; i++ {
		for j := -1; j <= 1; j++ {
			if i == 0 && j == 0 {
				continue
			}
			nr := row + i
			nc := col + j
			if nr >= 0 && nr < rows && nc >= 0 && nc < cols {
				cell := board[nr][nc]
				if cell != 0 && cell.Player() == player {
					count++
				}
			}
		}
	}
	return count
}

func (mcts *MCTSEngine) hasOpponentNeighbor(board [][]CellValue, row, col, player, rows, cols int) bool {
	for i := -1; i <= 1; i++ {
		for j := -1; j <= 1; j++ {
			if i == 0 && j == 0 {
				continue
			}
			nr := row + i
			nc := col + j
			if nr >= 0 && nr < rows && nc >= 0 && nc < cols {
				cell := board[nr][nc]
				if cell != 0 && cell.Player() != player && cell.Player() != 0 {
					return true
				}
			}
		}
	}
	return false
}
