package main

import (
	"fmt"
	"log"
	"math"
	"math/rand"
	"sort"
	"sync"
	"time"
)

// AIEngine handles bot move calculations
type AIEngine struct {
	settings        *BotSettings
	transTable      *TranspositionTable
	// Time management for iterative deepening
	searchStartTime time.Time
	timeLimit       time.Duration
	searchAborted   bool
	zobristTable    [100][100][256]uint64
	zobristTurn     [5]uint64 // To hash whose turn it is
}

// TranspositionTable caches board evaluations
type TranspositionTable struct {
	table map[uint64]TranspositionEntry
	mu    sync.RWMutex
}

type TranspositionEntry struct {
	Score float64
	Depth int
	Flag  int
}

const (
	exactScore = iota
	lowerBound
	upperBound
)

// Cell value constants and helpers - imported logic from backend/types.go
// Note: We duplicate these here because they are in different packages (main vs bot-hoster)
// Ideally these should be in a shared package, but for now we keep it simple as requested
const (
    CellFlagNormal    byte = 0x00
    CellFlagBase      byte = 0x10
    CellFlagFortified byte = 0x20
    CellFlagKilled    byte = 0x30

    FlagMask   byte = 0x30
    PlayerMask byte = 0x0F
)

type CellValue byte

func NewCell(player int, flag byte) CellValue {
    return CellValue(flag | byte(player))
}

func (c CellValue) Player() int {
    return int(byte(c) & PlayerMask)
}

func (c CellValue) Flag() byte {
    return byte(c) & FlagMask
}

func (c CellValue) IsBase() bool {
    return c.Flag() == CellFlagBase
}

func (c CellValue) IsFortified() bool {
    return c.Flag() == CellFlagFortified
}

func (c CellValue) IsKilled() bool {
    return c.Flag() == CellFlagKilled
}

func (c CellValue) CanBeAttacked() bool {
    return c.Flag() == CellFlagNormal
}

func NewAIEngine(settings *BotSettings) *AIEngine {
	ai := &AIEngine{
		settings:   settings,
		transTable: NewTranspositionTable(),
	}
	ai.initZobrist()
	return ai
}

func (ai *AIEngine) initZobrist() {
	r := rand.New(rand.NewSource(time.Now().UnixNano()))
	for row := 0; row < 100; row++ {
		for col := 0; col < 100; col++ {
			for k := 0; k < 256; k++ {
				ai.zobristTable[row][col][k] = r.Uint64()
			}
		}
	}
	for i := 0; i < 5; i++ {
		ai.zobristTurn[i] = r.Uint64()
	}
}

func NewTranspositionTable() *TranspositionTable {
	return &TranspositionTable{
		table: make(map[uint64]TranspositionEntry),
	}
}

// CalculateMove returns the best move for the given game state
// Returns the move object and boolean indicating success
// Uses iterative deepening with time budget (~670ms per move, 2 seconds per turn)
func (ai *AIEngine) CalculateMove(state *GameState, player int) (*Move, bool) {
	// Initialize hash if not present
	if state.Hash == 0 {
		state.Hash = ai.computeHash(state.Board, player)
	}

	// Get all valid moves
	validMoves := ai.getAllValidMoves(state, player)
	if len(validMoves) == 0 {
		return nil, false
	}

	moveCount := len(validMoves)

	// Time budget: aim for ~670ms per move (2000ms / 3 moves)
	// Give a bit more time for early moves in case there are fewer total moves
	ai.timeLimit = 670 * time.Millisecond
	ai.searchStartTime = time.Now()
	ai.searchAborted = false

	log.Printf("[AI] Calculating move for player %d (moves: %d, time limit: %dms)",
		player, moveCount, ai.timeLimit.Milliseconds())

	// Iterative deepening: start at depth 1 and increase until time runs out
	var bestMove Move
	maxDepth := 1

	for depth := 1; depth <= 20; depth++ { // Increased max depth limit
		// Check if we have time for this depth
		elapsed := time.Since(ai.searchStartTime)
		if elapsed > ai.timeLimit * 3 / 4 && depth > 1 {
			// If we've used 75% of time and have at least depth 1, stop
			log.Printf("[AI] Time limit approaching (elapsed: %dms), stopping at depth %d",
				elapsed.Milliseconds(), depth-1)
			break
		}

		ai.searchAborted = false
		move := ai.findBestMoveWithMinimax(state, validMoves, player, depth)

		// If search completed without abort, use this result
		if !ai.searchAborted {
			bestMove = move
			maxDepth = depth
			elapsed = time.Since(ai.searchStartTime)
			log.Printf("[AI] Completed depth %d in %dms, score: %.2f",
				depth, elapsed.Milliseconds(), move.Score)
		} else {
			log.Printf("[AI] Search aborted at depth %d", depth)
			break
		}
	}

	elapsed := time.Since(ai.searchStartTime)
	log.Printf("[AI] Selected move: Type=%d at depth %d, score %.2f (time: %dms)",
		bestMove.Type, maxDepth, bestMove.Score, elapsed.Milliseconds())

	return &bestMove, true
}

// GameState represents the current state of the game
type GameState struct {
	Board        [][]CellValue
	Rows         int
	Cols         int
	PlayerBases  [4]CellPos
	Players      []GamePlayerInfo
	Hash         uint64
	NeutralsUsed bool
}

const (
	MoveTypeStandard = 0
	MoveTypeNeutral  = 1
)

type Move struct {
	Type  int // 0 = Standard, 1 = Neutral
	Row   int
	Col   int
	Cells []CellPos
	Score float64
}

// MinimaxResult represents the result of minimax search
type MinimaxResult struct {
	Score float64
	Move  *Move
}

func (ai *AIEngine) getAllValidMoves(state *GameState, player int) []Move {
	var moves []Move

	// 1. Standard moves (place/attack)
	for row := 0; row < state.Rows; row++ {
		for col := 0; col < state.Cols; col++ {
			if ai.isValidMove(state, row, col, player) {
				moves = append(moves, Move{Type: MoveTypeStandard, Row: row, Col: col})
			}
		}
	}

	// 2. Neutral moves (if not used yet)
	if !state.NeutralsUsed {
		neutralMoves := ai.getNeutralMoves(state, player)
		moves = append(moves, neutralMoves...)
	}

	return moves
}

func (ai *AIEngine) getNeutralMoves(state *GameState, player int) []Move {
	// Pruning: Only consider own cells that are "threatened" (adjacent to opponent)
	// This avoids combinatorial explosion
	var threatenedCells []CellPos

	for r := 0; r < state.Rows; r++ {
		for c := 0; c < state.Cols; c++ {
			cell := state.Board[r][c]
			if cell != 0 && cell.Player() == player {
				// Check neighbors for opponents
				hasOpponentNeighbor := false
				for i := -1; i <= 1; i++ {
					for j := -1; j <= 1; j++ {
						if i == 0 && j == 0 { continue }
						nr, nc := r+i, c+j
						if nr >= 0 && nr < state.Rows && nc >= 0 && nc < state.Cols {
							nCell := state.Board[nr][nc]
							if nCell != 0 && nCell.Player() != player && nCell.Player() != 0 {
								hasOpponentNeighbor = true
								break
							}
						}
					}
					if hasOpponentNeighbor { break }
				}

				if hasOpponentNeighbor {
					threatenedCells = append(threatenedCells, CellPos{Row: r, Col: c})
				}
			}
		}
	}

	// If no threatened cells, maybe don't use neutral move at all
	// Strategy: "usually it need to protect own position"
	if len(threatenedCells) < 1 {
		return nil
	}

	var moves []Move

	// If we have threatened cells, generate pairs from them
	// If only 1 threatened cell, pair it with any other cell (preferably close to it)
	// To keep it simple and fast:
	// 1. If >= 2 threatened cells, generate pairs from top 5 (closest to base/most vulnerable)
	// 2. If 1 threatened cell, pair with closest friendly cell

	candidates := threatenedCells

	// Sort candidates by distance to base (closest to base are more critical to defend)
	base := state.PlayerBases[player-1]
	sort.Slice(candidates, func(i, j int) bool {
		distI := abs(candidates[i].Row-base.Row) + abs(candidates[i].Col-base.Col)
		distJ := abs(candidates[j].Row-base.Row) + abs(candidates[j].Col-base.Col)
		return distI < distJ
	})

	// Limit candidates to avoid explosion
	if len(candidates) > 6 {
		candidates = candidates[:6]
	}

	// Generate pairs
	// If only 1 candidate, we need to find a partner
	if len(candidates) == 1 {
		// Find closest friendly neighbor
		bestPartner := CellPos{Row: -1}
		minDist := 999
		c1 := candidates[0]

		for r := 0; r < state.Rows; r++ {
			for c := 0; c < state.Cols; c++ {
				if r == c1.Row && c == c1.Col { continue }
				cell := state.Board[r][c]
				if cell != 0 && cell.Player() == player {
					dist := abs(r-c1.Row) + abs(c-c1.Col)
					if dist < minDist {
						minDist = dist
						bestPartner = CellPos{Row: r, Col: c}
					}
				}
			}
		}
		if bestPartner.Row != -1 {
			moves = append(moves, Move{
				Type: MoveTypeNeutral,
				Cells: []CellPos{c1, bestPartner},
			})
		}
	} else {
		// Generate pairs from candidates
		for i := 0; i < len(candidates); i++ {
			for j := i + 1; j < len(candidates); j++ {
				moves = append(moves, Move{
					Type: MoveTypeNeutral,
					Cells: []CellPos{candidates[i], candidates[j]},
				})
			}
		}
	}

	return moves
}

func (ai *AIEngine) isValidMove(state *GameState, row, col, player int) bool {
	// Check bounds
	if row < 0 || row >= state.Rows || col < 0 || col >= state.Cols {
		return false
	}

	if state.Board == nil || len(state.Board) <= row || len(state.Board[row]) <= col {
		return false
	}

	cell := state.Board[row][col]

	// Cannot move on fortified or base cells
	if cell != 0 {
        if !cell.CanBeAttacked() {
			return false
		}
	}

	// Can only attack opponent's non-fortified cells or expand to empty cells
	if cell != 0 {
		isOpponent := false

		cellPlayer := cell.Player()

		if cellPlayer != player && cellPlayer > 0 && cellPlayer <= 4 {
			isOpponent = true
		}

		if !isOpponent {
			return false
		}
	}

	// Must be adjacent to own territory and connected to base
	return ai.isAdjacentAndConnected(state, row, col, player)
}

func (ai *AIEngine) isAdjacentAndConnected(state *GameState, row, col, player int) bool {
	// Check all 8 neighbors for friendly cells that are connected to base
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
					if ai.isConnectedToBase(state, adjRow, adjCol, player) {
						return true
					}
				}
			}
		}
	}
	return false
}

func (ai *AIEngine) isConnectedToBase(state *GameState, startRow, startCol, player int) bool {
	// BFS to check if cell is connected to base
	base := state.PlayerBases[player-1]
	visited := make(map[string]bool)
	stack := []struct{ row, col int }{{startRow, startCol}}
	visited[fmt.Sprintf("%d,%d", startRow, startCol)] = true

	for len(stack) > 0 {
		curr := stack[len(stack)-1]
		stack = stack[:len(stack)-1]

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
				key := fmt.Sprintf("%d,%d", newRow, newCol)

				if newRow >= 0 && newRow < state.Rows && newCol >= 0 && newCol < state.Cols && !visited[key] {
					cell := state.Board[newRow][newCol]
					if cell != 0 && cell.Player() == player {
						visited[key] = true
						stack = append(stack, struct{ row, col int }{newRow, newCol})
					}
				}
			}
		}
	}
	return false
}

func (ai *AIEngine) findBestMoveWithMinimax(state *GameState, moves []Move, player int, depth int) Move {
	// Sort moves by heuristic for better pruning
	for i := range moves {
		moves[i].Score = ai.scoreMoveQuick(state, moves[i], player)
	}
	sort.Slice(moves, func(i, j int) bool {
		return moves[i].Score > moves[j].Score
	})

	// Adaptive move limit: reduce moves considered based on game complexity
	maxMoves := 20 // Increased from 15
	if len(moves) > 40 {
		maxMoves = 15 // Increased from 10
	}
	if len(moves) > maxMoves {
		moves = moves[:maxMoves]
	}

	bestMove := moves[0]
	bestScore := math.Inf(-1)
	alpha := math.Inf(-1)
	beta := math.Inf(1)

	for _, move := range moves {
		// Check if search was aborted
		if ai.searchAborted {
			break
		}

		newBoard := ai.copyBoard(state.Board)
		newHash := ai.applyMove(newBoard, move, player, state.Hash)

		// Update turn hash: remove current player, add next player
		if player >= 1 && player <= 4 {
			newHash ^= ai.zobristTurn[player-1]
		}

		newState := &GameState{
			Board:        newBoard,
			Rows:         state.Rows,
			Cols:         state.Cols,
			PlayerBases:  state.PlayerBases,
			Players:      state.Players,
			Hash:         0, // Set momentarily
			NeutralsUsed: state.NeutralsUsed || (move.Type == MoveTypeNeutral),
		}

		// Determine next player to correctly update hash
		nextPlayer := ai.getNextOpponent(newState, player)
		if nextPlayer >= 1 && nextPlayer <= 4 {
			newHash ^= ai.zobristTurn[nextPlayer-1]
		}
		newState.Hash = newHash

		result := ai.minimax(newState, depth-1, alpha, beta, false, player)

		// Check if search was aborted during minimax
		if ai.searchAborted {
			break
		}

		if result.Score > bestScore {
			bestScore = result.Score
			bestMove = move
			bestMove.Score = bestScore
		}

		alpha = math.Max(alpha, result.Score)
		if beta <= alpha {
			break
		}
	}

	return bestMove
}

func (ai *AIEngine) minimax(state *GameState, depth int, alpha, beta float64, isMaximizing bool, aiPlayer int) MinimaxResult {
	// Check time limit periodically (every few nodes)
	if time.Since(ai.searchStartTime) > ai.timeLimit {
		ai.searchAborted = true
		return MinimaxResult{Score: 0, Move: nil}
	}

	// Check transposition table
	if entry, exists := ai.transTable.Get(state.Hash); exists && entry.Depth >= depth {
		if entry.Flag == exactScore {
			return MinimaxResult{Score: entry.Score, Move: nil}
		} else if entry.Flag == lowerBound {
			alpha = math.Max(alpha, entry.Score)
		} else if entry.Flag == upperBound {
			beta = math.Min(beta, entry.Score)
		}
		if alpha >= beta {
			return MinimaxResult{Score: entry.Score, Move: nil}
		}
	}

	// Base case: reached max depth
	if depth == 0 {
		score := ai.evaluateBoard(state, aiPlayer)
		ai.transTable.Put(state.Hash, TranspositionEntry{
			Score: score,
			Depth: depth,
			Flag:  exactScore,
		})
		return MinimaxResult{Score: score, Move: nil}
	}

	player := aiPlayer
	if !isMaximizing {
		player = ai.getNextOpponent(state, aiPlayer)
	}

	possibleMoves := ai.getAllValidMoves(state, player)

	// Terminal state: no moves available
	if len(possibleMoves) == 0 {
		score := ai.evaluateBoard(state, aiPlayer)
		// Penalize losing positions, reward winning positions
		if isMaximizing {
			score -= 10000
		} else {
			score += 10000
		}
		ai.transTable.Put(state.Hash, TranspositionEntry{
			Score: score,
			Depth: depth,
			Flag:  exactScore,
		})
		return MinimaxResult{Score: score, Move: nil}
	}

	// Move ordering: sort by heuristic score
	for i := range possibleMoves {
		possibleMoves[i].Score = ai.scoreMoveQuick(state, possibleMoves[i], player)
	}
	if isMaximizing {
		sort.Slice(possibleMoves, func(i, j int) bool {
			return possibleMoves[i].Score > possibleMoves[j].Score
		})
	} else {
		sort.Slice(possibleMoves, func(i, j int) bool {
			return possibleMoves[i].Score < possibleMoves[j].Score
		})
	}

	// Limit number of moves to consider at deeper levels for speed
	maxMoves := 16 // Increased from 12
	if depth <= 2 {
		maxMoves = 12 // Increased from 8
	}
	if depth == 1 {
		maxMoves = 10 // Increased from 6
	}
	if len(possibleMoves) > maxMoves {
		possibleMoves = possibleMoves[:maxMoves]
	}

	originalAlpha := alpha
	if isMaximizing {
		// AI's turn: maximize score
		maxScore := math.Inf(-1)
		var bestMove *Move

		for _, move := range possibleMoves {
			// Try this move
			newBoard := ai.copyBoard(state.Board)
			newHash := ai.applyMove(newBoard, move, player, state.Hash)

			// Update turn hash: remove current player, add next player
			if player >= 1 && player <= 4 {
				newHash ^= ai.zobristTurn[player-1]
			}

			newState := &GameState{
				Board:        newBoard,
				Rows:         state.Rows,
				Cols:         state.Cols,
				PlayerBases:  state.PlayerBases,
				Players:      state.Players,
				Hash:         0,
				NeutralsUsed: state.NeutralsUsed || (move.Type == MoveTypeNeutral),
			}

			// Next is opponent (minimizing)
			nextPlayer := ai.getNextOpponent(newState, aiPlayer)
			if nextPlayer >= 1 && nextPlayer <= 4 {
				newHash ^= ai.zobristTurn[nextPlayer-1]
			}
			newState.Hash = newHash

			// Recursively evaluate
			result := ai.minimax(newState, depth-1, alpha, beta, false, aiPlayer)

			if result.Score > maxScore {
				maxScore = result.Score
				bestMove = &move
			}

			alpha = math.Max(alpha, result.Score)
			if beta <= alpha {
				break // Beta cutoff
			}
		}

		// Store in transposition table
		flag := exactScore
		if maxScore <= originalAlpha {
			flag = upperBound
		} else if maxScore >= beta {
			flag = lowerBound
		}
		ai.transTable.Put(state.Hash, TranspositionEntry{
			Score: maxScore,
			Depth: depth,
			Flag:  flag,
		})

		return MinimaxResult{Score: maxScore, Move: bestMove}

	} else {
		// Opponent's turn: minimize score
		minScore := math.Inf(1)
		var bestMove *Move

		for _, move := range possibleMoves {
			// Try this move
			newBoard := ai.copyBoard(state.Board)
			newHash := ai.applyMove(newBoard, move, player, state.Hash)

			// Update turn hash: remove current player
			if player >= 1 && player <= 4 {
				newHash ^= ai.zobristTurn[player-1]
			}

			// Next is AI (maximizing)
			if aiPlayer >= 1 && aiPlayer <= 4 {
				newHash ^= ai.zobristTurn[aiPlayer-1]
			}

			newState := &GameState{
				Board:        newBoard,
				Rows:         state.Rows,
				Cols:         state.Cols,
				PlayerBases:  state.PlayerBases,
				Players:      state.Players,
				Hash:         newHash,
				NeutralsUsed: state.NeutralsUsed || (move.Type == MoveTypeNeutral),
			}

			// Recursively evaluate
			result := ai.minimax(newState, depth-1, alpha, beta, true, aiPlayer)

			if result.Score < minScore {
				minScore = result.Score
				bestMove = &move
			}

			beta = math.Min(beta, result.Score)
			if beta <= alpha {
				break // Alpha cutoff
			}
		}

		// Store in transposition table
		flag := exactScore
		if minScore <= alpha {
			flag = lowerBound
		} else if minScore >= beta {
			flag = upperBound
		}
		ai.transTable.Put(state.Hash, TranspositionEntry{
			Score: minScore,
			Depth: depth,
			Flag:  flag,
		})

		return MinimaxResult{Score: minScore, Move: bestMove}
	}
}

func (ai *AIEngine) evaluateBoard(state *GameState, aiPlayer int) float64 {
	// PRIORITY 0: Check if any opponent is defeated in this position
	// This is the most important factor - defeating opponents wins games
	defeatedOpponents := 0
	for p := 1; p <= 4; p++ {
		if p != aiPlayer {
			isActive := false
			for _, pl := range state.Players {
				if pl.PlayerIndex+1 == p && pl.IsActive {
					isActive = true
					break
				}
			}
			if isActive {
				// Check if this opponent has any pieces left
				pieceCount := ai.countPlayerPieces(state, p)
				if pieceCount == 0 {
					defeatedOpponents++
				}
			}
		}
	}
	// Massive bonus for each defeated opponent
	if defeatedOpponents > 0 {
		return 500000.0 * float64(defeatedOpponents)
	}

	// Single pass through board to collect all metrics
	aiCells := 0
	opponentCells := 0
	aiFortified := 0
	opponentFortified := 0
	aiAttackOpportunities := 0
	opponentAttackOpportunities := 0
	aiAggression := 0.0
	opponentAggression := 0.0
	aiRedundantCells := 0 // Cells with 2+ friendly neighbors
	opponentRedundantCells := 0
	aiCohesionPenalty := 0 // Gaps in territory
	opponentCohesionPenalty := 0
	baseDanger := 0.0
	vulnerableOpponentBonus := 0.0

	// Get opponent bases for aggression calculation
	opponentBases := ai.getOpponentBases(state, aiPlayer)
	aiBase := state.PlayerBases[aiPlayer-1]

	for r := 0; r < state.Rows; r++ {
		for c := 0; c < state.Cols; c++ {
			cell := state.Board[r][c]

			if cell != 0 {
				if cell.Player() == aiPlayer {
					// AI cell
					aiCells++
					if cell.IsFortified() {
						aiFortified++
					}

					// Aggression: distance to closest opponent base
					if len(opponentBases) > 0 {
						minDist := 999999
						for _, base := range opponentBases {
							dist := abs(r-base.Row) + abs(c-base.Col)
							if dist < minDist {
								minDist = dist
							}
						}
						aiAggression += float64(state.Rows + state.Cols - minDist)
					}

					// Count opponent neighbors (cells opponent can attack)
					opponentNeighborCount := ai.countOpponentNeighborsOnBoard(state.Board, r, c, aiPlayer, state.Rows, state.Cols)
					if opponentNeighborCount > 0 {
						opponentAttackOpportunities++
					}

					// Redundancy: cells with 2+ friendly neighbors
					friendlyNeighbors := ai.countFriendlyNeighborsOnBoard(state.Board, r, c, aiPlayer, state.Rows, state.Cols)
					if friendlyNeighbors >= 2 {
						aiRedundantCells++
					}

				} else {
					// Opponent cell
					opponentCells++
					if cell.IsFortified() {
						opponentFortified++
					}

					// Base Danger: check distance of opponent cell to our base
					distToBase := abs(r-aiBase.Row) + abs(c-aiBase.Col)
					if distToBase < 4 {
						// Extremely high penalty for opponents near base
						baseDanger += float64((4 - distToBase) * 500)
					}

					// Count AI neighbors (cells AI can attack)
					aiNeighborCount := ai.countPlayerNeighborsOnBoard(state.Board, r, c, aiPlayer, state.Rows, state.Cols)
					if aiNeighborCount > 0 {
						aiAttackOpportunities++
					}

					// Opponent aggression and redundancy
					opponentPlayer := cell.Player()
					if opponentPlayer > 0 {
						// Distance to AI base
						dist := abs(r-aiBase.Row) + abs(c-aiBase.Col)
						opponentAggression += float64(state.Rows + state.Cols - dist)

						// Redundancy
						friendlyNeighbors := ai.countFriendlyNeighborsOnBoard(state.Board, r, c, opponentPlayer, state.Rows, state.Cols)
						if friendlyNeighbors >= 2 {
							opponentRedundantCells++
						}
					}
				}
			} else {
				// Empty cell - check for gaps/holes
				aiFriendlyNeighbors := ai.countFriendlyNeighborsOnBoard(state.Board, r, c, aiPlayer, state.Rows, state.Cols)
				if aiFriendlyNeighbors >= 2 {
					aiCohesionPenalty += aiFriendlyNeighbors
				}

				// Check for opponent gaps
				for p := 1; p <= 4; p++ {
					// Check if player is active
					isActive := false
					for _, player := range state.Players {
						if player.PlayerIndex+1 == p && player.IsActive {
							isActive = true
							break
						}
					}

					if p != aiPlayer && isActive {
						oppNeighbors := ai.countFriendlyNeighborsOnBoard(state.Board, r, c, p, state.Rows, state.Cols)
						if oppNeighbors >= 2 {
							opponentCohesionPenalty += oppNeighbors
						}
					}
				}
			}
		}
	}

	// 1. Material Score (cells + fortifications)
	materialScore := float64(aiCells*10+aiFortified*20) - float64(opponentCells*10+opponentFortified*20)

	// 2. Mobility Score (fast approximation using attack opportunities instead of full move generation)
	// This is much faster than calling getAllValidMoves for all players
	mobilityScore := float64(aiAttackOpportunities - opponentAttackOpportunities)

	// 3. Strategic Position Score (aggression + attack opportunities)
	positionScore := (aiAggression - opponentAggression) + float64(aiAttackOpportunities-opponentAttackOpportunities)*5.0

	// 4. Redundancy Score (network resilience)
	redundancyScore := float64(aiRedundantCells - opponentRedundantCells)

	// 5. Cohesion Score (penalize gaps/holes)
	cohesionScore := float64(opponentCohesionPenalty - aiCohesionPenalty)

	// 6. Base Safety Score (Survival)
	baseSafetyScore := -baseDanger

	// 7. Vulnerable Opponent Score (Aggression)
	// Check for vulnerable opponents (low piece count)
	for p := 1; p <= 4; p++ {
		if p != aiPlayer {
			isActive := false
			for _, pl := range state.Players {
				if pl.PlayerIndex+1 == p && pl.IsActive {
					isActive = true
					break
				}
			}
			if isActive {
				pieceCount := ai.countPlayerPieces(state, p)
				if pieceCount > 0 && pieceCount <= 5 {
					// Bonus increases as piece count decreases
					vulnerableOpponentBonus += float64((6 - pieceCount) * 1000)
				}
			}
		}
	}

	// Combine scores with weights from bot settings
	totalScore := materialScore*ai.settings.MaterialWeight +
		mobilityScore*ai.settings.MobilityWeight +
		positionScore*ai.settings.PositionWeight +
		redundancyScore*ai.settings.RedundancyWeight +
		cohesionScore*ai.settings.CohesionWeight +
		baseSafetyScore + // Add direct penalty
		vulnerableOpponentBonus // Add direct bonus

	return totalScore
}

func (ai *AIEngine) scoreMoveQuick(state *GameState, move Move, player int) float64 {
	if move.Type == MoveTypeNeutral {
		// Evaluation for Neutral Move
		// Cost: Lose 3 moves (initiative) + Lose 2 cells
		// Benefit: Block opponent

		score := -1500.0 // Base penalty for skipping turn and losing cells

		// Check value of blocking
		for _, cellPos := range move.Cells {
			// Reward based on opponent adjacency (blocking potential)
			oppNeighbors := ai.countOpponentNeighborsOnBoard(state.Board, cellPos.Row, cellPos.Col, player, state.Rows, state.Cols)
			score += float64(oppNeighbors * 1000) // High value for blocking active fronts

			// Bonus for protecting base (distance to base)
			base := state.PlayerBases[player-1]
			dist := abs(cellPos.Row-base.Row) + abs(cellPos.Col-base.Col)
			if dist < 4 {
				score += 2000.0 // Critical defense
			}
		}

		// Only use if really threatened (heuristic score must be > standard moves ~2000-3000)
		return score
	}

	cellValue := state.Board[move.Row][move.Col]
	score := 0.0

	// PRIORITY 0: Check if this move defeats any opponent (kills a player)
	// This check is very fast - just counts opponent pieces
	defeatedPlayer := ai.checkIfMoveDefeatsOpponent(state, move, player)
	if defeatedPlayer > 0 {
		// MASSIVE bonus for defeating a player - this should ALWAYS be chosen
		return 1000000.0 + score // Return immediately with overwhelming score
	}

	// 1. Capturing opponent cells (1500 points, +800 if fortified)
	isCapture := false
	if cellValue != 0 {
        cellPlayer := cellValue.Player()
		for p := 1; p <= 4; p++ {
			// Check if player is active
			isActive := false
			for _, pl := range state.Players {
				if pl.PlayerIndex+1 == p && pl.IsActive {
					isActive = true
					break
				}
			}

			if p != player && isActive && cellPlayer == p {
				isCapture = true
				score += 1500.0
				if cellValue.IsFortified() {
					score += 800.0
				}
				// Bonus for capturing cells near their base (aggressive play)
				oppBase := state.PlayerBases[p-1]
				distToTheirBase := abs(move.Row-oppBase.Row) + abs(move.Col-oppBase.Col)
				if distToTheirBase <= 3 {
					score += 500.0 // Big bonus for attacking near their base
				}

				// Aggression Bonus: Future Kill Potential
				// If opponent has very few pieces left (e.g., < 3), prioritize attacking them
				opponentPieceCount := ai.countPlayerPieces(state, p)
				if opponentPieceCount <= 3 {
					score += 2000.0 // Huge incentive to finish off weak opponents
				}

				break
			}
		}
	}

	// 2. Count neighbors with improved scoring
	friendlyNeighbors := 0
	opponentNeighbors := 0
	emptyNeighbors := 0
	fortifiedNeighbors := 0
	directions := [][]int{{-1, 0}, {1, 0}, {0, -1}, {0, 1}}

	for _, dir := range directions {
		nr := move.Row + dir[0]
		nc := move.Col + dir[1]
		if nr >= 0 && nr < state.Rows && nc >= 0 && nc < state.Cols {
			neighbor := state.Board[nr][nc]
			if neighbor != 0 {
				if neighbor.Player() == player {
					friendlyNeighbors++
					if neighbor.IsFortified() {
						fortifiedNeighbors++
					}
				} else {
					opponentNeighbors++
				}
			} else {
				emptyNeighbors++
			}
		}
	}

	// Reward connecting to existing territory
	score += float64(friendlyNeighbors * 80)
	// Bonus for being near fortified cells (defensive strength)
	score += float64(fortifiedNeighbors * 40)
	// Reward being near opponent cells (attack opportunities)
	score += float64(opponentNeighbors * 60)
	// Slight bonus for expansion potential
	score += float64(emptyNeighbors * 15)

	// 3. Strategic positioning
	opponentBase := ai.getClosestOpponentBase(state, player, move.Row, move.Col)
	if opponentBase != nil {
		distToOpponentBase := abs(move.Row-opponentBase.Row) + abs(move.Col-opponentBase.Col)
		// Encourage aggressive expansion toward opponent
		score += float64((state.Rows+state.Cols)-distToOpponentBase) * 5
	}

	// 4. Penalize overextension from own base
	ownBase := state.PlayerBases[player-1]
	distToOwnBase := abs(move.Row-ownBase.Row) + abs(move.Col-ownBase.Col)
	if distToOwnBase > 10 {
		score -= float64((distToOwnBase - 10) * 20)
	}

	// 5. Prefer moves that create multiple expansion opportunities
	if !isCapture && emptyNeighbors >= 2 {
		score += 100.0 // Bonus for creating branching points
	}

	// 6. Slight preference for center control early game
	centerRow := state.Rows / 2
	centerCol := state.Cols / 2
	distToCenter := abs(move.Row-centerRow) + abs(move.Col-centerCol)
	if ai.countPlayerPieces(state, player) < 15 {
		score += float64((state.Rows+state.Cols)-distToCenter) * 2
	}

	return score
}

func (ai *AIEngine) copyBoard(board [][]CellValue) [][]CellValue {
	newBoard := make([][]CellValue, len(board))
	for i := range board {
		newBoard[i] = make([]CellValue, len(board[i]))
		copy(newBoard[i], board[i])
	}
	return newBoard
}

// applyMoveToBoard updates the board and returns the new Zobrist hash
// Uses incremental update: XOR out old value, XOR in new value
func (ai *AIEngine) applyMoveToBoard(board [][]CellValue, row, col, player int, currentHash uint64) uint64 {
	return ai.applyMove(board, Move{Type: MoveTypeStandard, Row: row, Col: col}, player, currentHash)
}

func (ai *AIEngine) applyMove(board [][]CellValue, move Move, player int, currentHash uint64) uint64 {
	if move.Type == MoveTypeNeutral {
		for _, cell := range move.Cells {
			r, c := cell.Row, cell.Col
			oldCell := board[r][c]
			if r < 100 && c < 100 {
				currentHash ^= ai.zobristTable[r][c][oldCell]
			}

			// Convert to Killed (Neutral)
			// Killed cell has no player? The backend used NewCell(0, CellFlagKilled)
			board[r][c] = NewCell(0, CellFlagKilled)
			newCell := board[r][c]

			if r < 100 && c < 100 {
				currentHash ^= ai.zobristTable[r][c][newCell]
			}
		}
		return currentHash
	}

	// Standard Move
	row, col := move.Row, move.Col
	oldCell := board[row][col]
	if row < 100 && col < 100 {
		currentHash ^= ai.zobristTable[row][col][oldCell]
	}

	if oldCell == 0 {
		board[row][col] = NewCell(player, CellFlagNormal)
	} else {
		board[row][col] = NewCell(player, CellFlagFortified)
	}

	newCell := board[row][col]
	if row < 100 && col < 100 {
		currentHash ^= ai.zobristTable[row][col][newCell]
	}

	return currentHash
}

// computeHash computes the full Zobrist hash of a board from scratch
func (ai *AIEngine) computeHash(board [][]CellValue, player int) uint64 {
	var h uint64
	for r := range board {
		for c := range board[r] {
			val := board[r][c]
			if r < 100 && c < 100 {
				h ^= ai.zobristTable[r][c][val]
			}
		}
	}
	// Also hash the player whose turn it is
	if player > 0 && player <= 4 {
		h ^= ai.zobristTurn[player-1]
	}
	return h
}

// TranspositionTable methods
func (tt *TranspositionTable) Get(key uint64) (TranspositionEntry, bool) {
	tt.mu.RLock()
	defer tt.mu.RUnlock()
	entry, exists := tt.table[key]
	return entry, exists
}

func (tt *TranspositionTable) Put(key uint64, entry TranspositionEntry) {
	tt.mu.Lock()
	defer tt.mu.Unlock()
	tt.table[key] = entry
}

func (ai *AIEngine) getNextOpponent(state *GameState, currentPlayer int) int {
	// Find next active opponent
	for i := 1; i <= 4; i++ {
		if i != currentPlayer {
			isActive := false
			for _, p := range state.Players {
				if p.PlayerIndex+1 == i && p.IsActive {
					isActive = true
					break
				}
			}
			if isActive && ai.countPlayerPieces(state, i) > 0 {
				return i
			}
		}
	}
	return currentPlayer
}

func (ai *AIEngine) countPlayerPieces(state *GameState, player int) int {
	count := 0
	for r := 0; r < state.Rows; r++ {
		for c := 0; c < state.Cols; c++ {
			cell := state.Board[r][c]
			if cell != 0 && cell.Player() == player {
				count++
			}
		}
	}
	return count
}

func (ai *AIEngine) getOpponentBases(state *GameState, aiPlayer int) []CellPos {
	var bases []CellPos
	for i := 1; i <= 4; i++ {
		if i != aiPlayer {
			isActive := false
			for _, p := range state.Players {
				if p.PlayerIndex+1 == i && p.IsActive {
					isActive = true
					break
				}
			}
			if isActive && ai.countPlayerPieces(state, i) > 0 {
				bases = append(bases, state.PlayerBases[i-1])
			}
		}
	}
	return bases
}

func (ai *AIEngine) countFriendlyNeighborsOnBoard(board [][]CellValue, row, col, player, rows, cols int) int {
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

func (ai *AIEngine) countPlayerNeighborsOnBoard(board [][]CellValue, row, col, player, rows, cols int) int {
	return ai.countFriendlyNeighborsOnBoard(board, row, col, player, rows, cols)
}

func (ai *AIEngine) countOpponentNeighborsOnBoard(board [][]CellValue, row, col, player, rows, cols int) int {
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
				if cell != 0 && cell.Player() != player {
					count++
				}
			}
		}
	}
	return count
}

func (ai *AIEngine) getCellPlayer(cell CellValue) int {
	return cell.Player()
}

func (ai *AIEngine) getClosestOpponentBase(state *GameState, player int, fromRow, fromCol int) *CellPos {
	var closestBase *CellPos
	minDist := 999999

	for i := 1; i <= 4; i++ {
		if i != player {
			isActive := false
			for _, p := range state.Players {
				if p.PlayerIndex+1 == i && p.IsActive {
					isActive = true
					break
				}
			}
			if isActive && ai.countPlayerPieces(state, i) > 0 {
				base := state.PlayerBases[i-1]
				dist := abs(fromRow-base.Row) + abs(fromCol-base.Col)
				if dist < minDist {
					minDist = dist
					closestBase = &base
				}
			}
		}
	}
	return closestBase
}

func abs(x int) int {
	if x < 0 {
		return -x
	}
	return x
}

// checkIfMoveDefeatsOpponent checks if making this move would eliminate any opponent
// Returns the defeated player number (1-4) or 0 if no player is defeated
// This is optimized to be very fast - just simulates the move and counts pieces
func (ai *AIEngine) checkIfMoveDefeatsOpponent(state *GameState, move Move, player int) int {
	if move.Type == MoveTypeNeutral {
		return 0 // Neutral moves don't defeat opponents directly
	}

	// Get the cell we're targeting
	cellValue := state.Board[move.Row][move.Col]
	if cellValue == 0 {
		// Expanding into empty space - cannot defeat anyone
		return 0
	}

	// Get the targeted opponent player
	targetPlayer := cellValue.Player()
	if targetPlayer == 0 || targetPlayer == player {
		return 0
	}

	// Check if this opponent is active
	isActive := false
	for _, pl := range state.Players {
		if pl.PlayerIndex+1 == targetPlayer && pl.IsActive {
			isActive = true
			break
		}
	}
	if !isActive {
		return 0
	}

	// Fast check: count how many pieces this opponent has
	// If they only have 1 piece and we're taking it, they're defeated
	opponentPieceCount := 0
	for r := 0; r < state.Rows && opponentPieceCount <= 1; r++ {
		for c := 0; c < state.Cols && opponentPieceCount <= 1; c++ {
			cell := state.Board[r][c]
			if cell != 0 && cell.Player() == targetPlayer {
				opponentPieceCount++
				// Early exit if we find more than 1 piece
				if opponentPieceCount > 1 {
					return 0
				}
			}
		}
	}

	// If opponent has exactly 1 piece and it's the one we're taking, they're defeated!
	if opponentPieceCount == 1 {
		return targetPlayer
	}

	return 0
}
