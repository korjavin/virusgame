package main

import (
	"fmt"
	"log"
	"math"
	"sort"
	"strings"
	"sync"
)

const (
	// Default search depth - increased due to optimizations
	defaultBotDepth = 4

	// Transposition table entry types
	exactScore = iota
	lowerBound
	upperBound
)

// TranspositionEntry stores cached board evaluations
type TranspositionEntry struct {
	Score float64
	Depth int
	Flag  int
}

// TranspositionTable caches board positions to avoid re-evaluation
type TranspositionTable struct {
	table map[string]TranspositionEntry
	mu    sync.RWMutex
}

// NewTranspositionTable creates a new transposition table
func NewTranspositionTable() *TranspositionTable {
	return &TranspositionTable{
		table: make(map[string]TranspositionEntry),
	}
}

// Get retrieves an entry from the table
func (tt *TranspositionTable) Get(key string) (TranspositionEntry, bool) {
	tt.mu.RLock()
	defer tt.mu.RUnlock()
	entry, exists := tt.table[key]
	return entry, exists
}

// Put stores an entry in the table
func (tt *TranspositionTable) Put(key string, entry TranspositionEntry) {
	tt.mu.Lock()
	defer tt.mu.Unlock()
	tt.table[key] = entry
}

// Clear clears the transposition table
func (tt *TranspositionTable) Clear() {
	tt.mu.Lock()
	defer tt.mu.Unlock()
	tt.table = make(map[string]TranspositionEntry)
}

// BotMove represents a potential move for the bot
type BotMove struct {
	Row   int
	Col   int
	Score float64
}

// MinimaxResult represents the result of minimax search
type MinimaxResult struct {
	Score float64
	Move  *BotMove
}

// makeBotMove makes a move for a bot player using minimax search
func (h *Hub) makeBotMove(game *Game, botPlayer int) {
	// Get bot settings
	botSettings := h.getBotSettings(game, botPlayer)
	depth := botSettings.SearchDepth
	if depth <= 0 {
		depth = defaultBotDepth
	}

	log.Printf("Bot player %d making move in game %s (using minimax depth %d)", botPlayer, game.ID, depth)

	// Get all valid moves
	validMoves := h.getAllBotMoves(game, botPlayer)

	if len(validMoves) == 0 {
		log.Printf("Bot player %d has no valid moves", botPlayer)
		return
	}

	// Create transposition table for this search
	transTable := NewTranspositionTable()

	// Use minimax to find best move
	bestMove := h.findBestMoveWithMinimax(game, validMoves, botPlayer, botSettings, depth, transTable)

	log.Printf("Bot player %d selected move [%d,%d] with score %.2f (TT size: %d)",
		botPlayer, bestMove.Row, bestMove.Col, bestMove.Score, len(transTable.table))

	// Apply the move
	h.applyBotMove(game, bestMove.Row, bestMove.Col, botPlayer)
}

// hashBoard creates a hash key for the board state
func (h *Hub) hashBoard(board [][]interface{}, player int) string {
	var key strings.Builder
	key.WriteString(fmt.Sprintf("P%d:", player))
	for r := range board {
		for c := range board[r] {
			if board[r][c] == nil {
				key.WriteString("_")
			} else {
				key.WriteString(fmt.Sprintf("%v", board[r][c]))
			}
			key.WriteString(",")
		}
	}
	return key.String()
}

// getBotSettings retrieves bot settings for a player
func (h *Hub) getBotSettings(game *Game, player int) *BotSettings {
	if game.IsMultiplayer && player >= 1 && player <= 4 {
		lobbyPlayer := game.Players[player-1]
		if lobbyPlayer != nil && lobbyPlayer.IsBot && lobbyPlayer.BotSettings != nil {
			return lobbyPlayer.BotSettings
		}
	}
	// Return default settings
	return &BotSettings{
		MaterialWeight:   100.0,
		MobilityWeight:   50.0,
		PositionWeight:   30.0,
		RedundancyWeight: 40.0,
		CohesionWeight:   25.0,
		SearchDepth:      5,
	}
}

// findBestMoveWithMinimax uses minimax algorithm to find the best move
func (h *Hub) findBestMoveWithMinimax(game *Game, moves []BotMove, player int, botSettings *BotSettings, depth int, transTable *TranspositionTable) BotMove {
	// Sort moves by heuristic score for better alpha-beta pruning (move ordering)
	for i := range moves {
		moves[i].Score = h.scoreMoveQuick(game, moves[i], player)
	}
	sort.Slice(moves, func(i, j int) bool {
		return moves[i].Score > moves[j].Score
	})

	// Only search top N moves if there are too many (pruning weak moves)
	maxMovesToConsider := 20
	if len(moves) > maxMovesToConsider {
		moves = moves[:maxMovesToConsider]
	}

	bestMove := moves[0]
	bestScore := math.Inf(-1)
	alpha := math.Inf(-1)
	beta := math.Inf(1)

	for _, move := range moves {
		// Apply move to a copy of the board
		newBoard := h.copyBoard(game.Board)
		h.applyMoveToBoard(newBoard, move.Row, move.Col, player)

		// Recursively evaluate this position
		result := h.minimax(game, newBoard, depth-1, alpha, beta, false, player, botSettings, transTable)

		if result.Score > bestScore {
			bestScore = result.Score
			bestMove = move
			bestMove.Score = bestScore
		}

		alpha = math.Max(alpha, result.Score)
		if beta <= alpha {
			break // Beta cutoff
		}
	}

	return bestMove
}

// minimax implements the minimax algorithm with alpha-beta pruning and transposition table
func (h *Hub) minimax(game *Game, board [][]interface{}, depth int, alpha, beta float64, isMaximizing bool, aiPlayer int, botSettings *BotSettings, transTable *TranspositionTable) MinimaxResult {
	// Check transposition table
	boardHash := h.hashBoard(board, aiPlayer)
	if entry, exists := transTable.Get(boardHash); exists && entry.Depth >= depth {
		// Use cached result if depth is sufficient
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
		score := h.evaluateBoard(game, board, aiPlayer, botSettings)
		transTable.Put(boardHash, TranspositionEntry{
			Score: score,
			Depth: depth,
			Flag:  exactScore,
		})
		return MinimaxResult{Score: score, Move: nil}
	}

	player := aiPlayer
	if !isMaximizing {
		// Get next opponent
		player = h.getNextOpponent(game, aiPlayer)
	}

	possibleMoves := h.getAllValidMovesOnBoard(game, board, player)

	// Terminal state: no moves available
	if len(possibleMoves) == 0 {
		score := h.evaluateBoard(game, board, aiPlayer, botSettings)
		// Penalize losing positions, reward winning positions
		if isMaximizing {
			score -= 10000
		} else {
			score += 10000
		}
		transTable.Put(boardHash, TranspositionEntry{
			Score: score,
			Depth: depth,
			Flag:  exactScore,
		})
		return MinimaxResult{Score: score, Move: nil}
	}

	// Move ordering: sort by heuristic score
	for i := range possibleMoves {
		possibleMoves[i].Score = h.scoreMoveQuick(game, possibleMoves[i], player)
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
	maxMoves := 15
	if depth <= 2 {
		maxMoves = 10
	}
	if len(possibleMoves) > maxMoves {
		possibleMoves = possibleMoves[:maxMoves]
	}

	originalAlpha := alpha
	if isMaximizing {
		// AI's turn: maximize score
		maxScore := math.Inf(-1)
		var bestMove *BotMove

		for _, move := range possibleMoves {
			// Try this move
			newBoard := h.copyBoard(board)
			h.applyMoveToBoard(newBoard, move.Row, move.Col, player)

			// Recursively evaluate
			result := h.minimax(game, newBoard, depth-1, alpha, beta, false, aiPlayer, botSettings, transTable)

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
		transTable.Put(boardHash, TranspositionEntry{
			Score: maxScore,
			Depth: depth,
			Flag:  flag,
		})

		return MinimaxResult{Score: maxScore, Move: bestMove}

	} else {
		// Opponent's turn: minimize score
		minScore := math.Inf(1)
		var bestMove *BotMove

		for _, move := range possibleMoves {
			// Try this move
			newBoard := h.copyBoard(board)
			h.applyMoveToBoard(newBoard, move.Row, move.Col, player)

			// Recursively evaluate
			result := h.minimax(game, newBoard, depth-1, alpha, beta, true, aiPlayer, botSettings, transTable)

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
		transTable.Put(boardHash, TranspositionEntry{
			Score: minScore,
			Depth: depth,
			Flag:  flag,
		})

		return MinimaxResult{Score: minScore, Move: bestMove}
	}
}

// evaluateBoard evaluates the board position from AI's perspective
// Matches ai.js evaluateBoard function (lines 464-570)
func (h *Hub) evaluateBoard(game *Game, board [][]interface{}, aiPlayer int, botSettings *BotSettings) float64 {
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

	// Get opponent bases for aggression calculation
	opponentBases := h.getOpponentBases(game, aiPlayer)

	for r := 0; r < game.Rows; r++ {
		for c := 0; c < game.Cols; c++ {
			cell := board[r][c]
			cellStr := fmt.Sprintf("%v", cell)

			if cell != nil && len(cellStr) > 0 {
				if cellStr[0] == byte('0'+aiPlayer) {
					// AI cell
					aiCells++
					if strings.HasSuffix(cellStr, "-fortified") {
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
						aiAggression += float64(game.Rows + game.Cols - minDist)
					}

					// Count opponent neighbors (cells opponent can attack)
					opponentNeighborCount := h.countOpponentNeighborsOnBoard(board, r, c, aiPlayer, game.Rows, game.Cols)
					if opponentNeighborCount > 0 {
						opponentAttackOpportunities++
					}

					// Redundancy: cells with 2+ friendly neighbors
					friendlyNeighbors := h.countFriendlyNeighborsOnBoard(board, r, c, aiPlayer, game.Rows, game.Cols)
					if friendlyNeighbors >= 2 {
						aiRedundantCells++
					}

				} else {
					// Opponent cell
					opponentCells++
					if strings.HasSuffix(cellStr, "-fortified") {
						opponentFortified++
					}

					// Count AI neighbors (cells AI can attack)
					aiNeighborCount := h.countPlayerNeighborsOnBoard(board, r, c, aiPlayer, game.Rows, game.Cols)
					if aiNeighborCount > 0 {
						aiAttackOpportunities++
					}

					// Opponent aggression and redundancy
					opponentPlayer := h.getCellPlayer(cellStr)
					if opponentPlayer > 0 {
						// Distance to AI base
						aiBase := game.PlayerBases[aiPlayer-1]
						dist := abs(r-aiBase.Row) + abs(c-aiBase.Col)
						opponentAggression += float64(game.Rows + game.Cols - dist)

						// Redundancy
						friendlyNeighbors := h.countFriendlyNeighborsOnBoard(board, r, c, opponentPlayer, game.Rows, game.Cols)
						if friendlyNeighbors >= 2 {
							opponentRedundantCells++
						}
					}
				}
			} else {
				// Empty cell - check for gaps/holes
				aiFriendlyNeighbors := h.countFriendlyNeighborsOnBoard(board, r, c, aiPlayer, game.Rows, game.Cols)
				if aiFriendlyNeighbors >= 2 {
					aiCohesionPenalty += aiFriendlyNeighbors
				}

				// Check for opponent gaps
				for p := 1; p <= 4; p++ {
					if p != aiPlayer && game.Players[p-1] != nil {
						oppNeighbors := h.countFriendlyNeighborsOnBoard(board, r, c, p, game.Rows, game.Cols)
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

	// 2. Mobility Score (available moves)
	aiMoves := len(h.getAllValidMovesOnBoard(game, board, aiPlayer))
	opponentMoves := 0
	for p := 1; p <= 4; p++ {
		if p != aiPlayer && game.Players[p-1] != nil {
			opponentMoves += len(h.getAllValidMovesOnBoard(game, board, p))
		}
	}
	mobilityScore := float64(aiMoves - opponentMoves)

	// 3. Strategic Position Score (aggression + attack opportunities)
	positionScore := (aiAggression - opponentAggression) + float64(aiAttackOpportunities-opponentAttackOpportunities)*5.0

	// 4. Redundancy Score (network resilience)
	redundancyScore := float64(aiRedundantCells - opponentRedundantCells)

	// 5. Cohesion Score (penalize gaps/holes)
	cohesionScore := float64(opponentCohesionPenalty - aiCohesionPenalty)

	// Combine scores with weights from bot settings
	totalScore := materialScore*botSettings.MaterialWeight +
		mobilityScore*botSettings.MobilityWeight +
		positionScore*botSettings.PositionWeight +
		redundancyScore*botSettings.RedundancyWeight +
		cohesionScore*botSettings.CohesionWeight

	return totalScore
}

// scoreMoveQuick scores a move for move ordering - improved heuristics
func (h *Hub) scoreMoveQuick(game *Game, move BotMove, player int) float64 {
	cellValue := game.Board[move.Row][move.Col]
	cellStr := fmt.Sprintf("%v", cellValue)
	score := 0.0

	// 1. Capturing opponent cells (1500 points, +800 if fortified)
	isCapture := false
	if cellValue != nil && len(cellStr) > 0 {
		for p := 1; p <= 4; p++ {
			if p != player && game.Players[p-1] != nil && cellStr[0] == byte('0'+p) {
				isCapture = true
				score += 1500.0
				if strings.HasSuffix(cellStr, "-fortified") {
					score += 800.0
				}
				// Bonus for capturing cells near their base (aggressive play)
				oppBase := game.PlayerBases[p-1]
				distToTheirBase := abs(move.Row-oppBase.Row) + abs(move.Col-oppBase.Col)
				if distToTheirBase <= 3 {
					score += 500.0 // Big bonus for attacking near their base
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
		if nr >= 0 && nr < game.Rows && nc >= 0 && nc < game.Cols {
			neighbor := game.Board[nr][nc]
			neighborStr := fmt.Sprintf("%v", neighbor)
			if neighbor != nil && len(neighborStr) > 0 {
				if neighborStr[0] == byte('0'+player) {
					friendlyNeighbors++
					if strings.HasSuffix(neighborStr, "-fortified") {
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
	opponentBase := h.getClosestOpponentBase(game, player, move.Row, move.Col)
	if opponentBase != nil {
		distToOpponentBase := abs(move.Row-opponentBase.Row) + abs(move.Col-opponentBase.Col)
		// Encourage aggressive expansion toward opponent
		score += float64((game.Rows+game.Cols)-distToOpponentBase) * 5
	}

	// 4. Penalize overextension from own base
	ownBase := game.PlayerBases[player-1]
	distToOwnBase := abs(move.Row-ownBase.Row) + abs(move.Col-ownBase.Col)
	if distToOwnBase > 10 {
		score -= float64((distToOwnBase - 10) * 20)
	}

	// 5. Prefer moves that create multiple expansion opportunities
	if !isCapture && emptyNeighbors >= 2 {
		score += 100.0 // Bonus for creating branching points
	}

	// 6. Slight preference for center control early game
	centerRow := game.Rows / 2
	centerCol := game.Cols / 2
	distToCenter := abs(move.Row-centerRow) + abs(move.Col-centerCol)
	if h.countPlayerPieces(game, player) < 15 {
		score += float64((game.Rows+game.Cols)-distToCenter) * 2
	}

	return score
}

// Helper functions

func (h *Hub) getAllBotMoves(game *Game, player int) []BotMove {
	var moves []BotMove
	for row := 0; row < game.Rows; row++ {
		for col := 0; col < game.Cols; col++ {
			if h.isValidMove(game, row, col, player) {
				moves = append(moves, BotMove{Row: row, Col: col})
			}
		}
	}
	return moves
}

func (h *Hub) getAllValidMovesOnBoard(game *Game, board [][]interface{}, player int) []BotMove {
	var moves []BotMove
	for row := 0; row < game.Rows; row++ {
		for col := 0; col < game.Cols; col++ {
			if h.isValidMoveOnBoard(game, board, row, col, player) {
				moves = append(moves, BotMove{Row: row, Col: col})
			}
		}
	}
	return moves
}

func (h *Hub) isValidMoveOnBoard(game *Game, board [][]interface{}, row, col, player int) bool {
	cell := board[row][col]
	cellStr := fmt.Sprintf("%v", cell)

	// Cannot move on fortified or base cells
	if cell != nil {
		if strings.HasSuffix(cellStr, "-fortified") || strings.HasSuffix(cellStr, "-base") {
			return false
		}
	}

	// Can only attack opponent's non-fortified cells or expand to empty cells
	if cell != nil {
		isOpponent := false
		for p := 1; p <= 4; p++ {
			if p != player && len(cellStr) > 0 && cellStr[0] == byte('0'+p) {
				isOpponent = true
				break
			}
		}
		if !isOpponent {
			return false
		}
	}

	// Must be adjacent to own territory and connected to base
	return h.isAdjacentAndConnectedOnBoard(game, board, row, col, player)
}

func (h *Hub) isAdjacentAndConnectedOnBoard(game *Game, board [][]interface{}, row, col, player int) bool {
	// Check all 8 neighbors
	for i := -1; i <= 1; i++ {
		for j := -1; j <= 1; j++ {
			if i == 0 && j == 0 {
				continue
			}
			adjRow := row + i
			adjCol := col + j
			if adjRow >= 0 && adjRow < game.Rows && adjCol >= 0 && adjCol < game.Cols {
				adjCell := board[adjRow][adjCol]
				adjStr := fmt.Sprintf("%v", adjCell)
				if adjCell != nil && len(adjStr) > 0 && adjStr[0] == byte('0'+player) {
					// Check if this adjacent cell is connected to base
					if h.isConnectedToBaseOnBoard(game, board, adjRow, adjCol, player) {
						return true
					}
				}
			}
		}
	}
	return false
}

func (h *Hub) isConnectedToBaseOnBoard(game *Game, board [][]interface{}, startRow, startCol, player int) bool {
	base := game.PlayerBases[player-1]
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

				if newRow >= 0 && newRow < game.Rows && newCol >= 0 && newCol < game.Cols && !visited[key] {
					cell := board[newRow][newCol]
					cellStr := fmt.Sprintf("%v", cell)
					if cell != nil && len(cellStr) > 0 && cellStr[0] == byte('0'+player) {
						visited[key] = true
						stack = append(stack, struct{ row, col int }{newRow, newCol})
					}
				}
			}
		}
	}
	return false
}

func (h *Hub) copyBoard(board [][]interface{}) [][]interface{} {
	newBoard := make([][]interface{}, len(board))
	for i := range board {
		newBoard[i] = make([]interface{}, len(board[i]))
		copy(newBoard[i], board[i])
	}
	return newBoard
}

func (h *Hub) applyMoveToBoard(board [][]interface{}, row, col, player int) {
	cell := board[row][col]
	if cell == nil {
		board[row][col] = player
	} else {
		board[row][col] = fmt.Sprintf("%d-fortified", player)
	}
}

func (h *Hub) countFriendlyNeighborsOnBoard(board [][]interface{}, row, col, player, rows, cols int) int {
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
				cellStr := fmt.Sprintf("%v", cell)
				if cell != nil && len(cellStr) > 0 && cellStr[0] == byte('0'+player) {
					count++
				}
			}
		}
	}
	return count
}

func (h *Hub) countPlayerNeighborsOnBoard(board [][]interface{}, row, col, player, rows, cols int) int {
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
				cellStr := fmt.Sprintf("%v", cell)
				if cell != nil && len(cellStr) > 0 && cellStr[0] == byte('0'+player) {
					count++
				}
			}
		}
	}
	return count
}

func (h *Hub) countOpponentNeighborsOnBoard(board [][]interface{}, row, col, player, rows, cols int) int {
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
				cellStr := fmt.Sprintf("%v", cell)
				if cell != nil && len(cellStr) > 0 && cellStr[0] != byte('0'+player) {
					count++
				}
			}
		}
	}
	return count
}

func (h *Hub) getCellPlayer(cellStr string) int {
	if len(cellStr) > 0 {
		playerChar := cellStr[0]
		if playerChar >= '1' && playerChar <= '4' {
			return int(playerChar - '0')
		}
	}
	return 0
}

func (h *Hub) getOpponentBases(game *Game, aiPlayer int) []CellPos {
	var bases []CellPos
	for i := 1; i <= 4; i++ {
		if i != aiPlayer && game.Players[i-1] != nil {
			if h.countPlayerPieces(game, i) > 0 {
				bases = append(bases, game.PlayerBases[i-1])
			}
		}
	}
	return bases
}

func (h *Hub) getNextOpponent(game *Game, currentPlayer int) int {
	// Find next active opponent
	for i := 1; i <= 4; i++ {
		if i != currentPlayer && game.Players[i-1] != nil {
			if h.countPlayerPieces(game, i) > 0 {
				return i
			}
		}
	}
	return currentPlayer
}

func (h *Hub) getClosestOpponentBase(game *Game, player int, fromRow, fromCol int) *CellPos {
	var closestBase *CellPos
	minDist := 999999

	for i := 1; i <= 4; i++ {
		if i != player && game.Players[i-1] != nil {
			if h.countPlayerPieces(game, i) > 0 {
				base := game.PlayerBases[i-1]
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

// applyBotMove applies a bot's move to the game
func (h *Hub) applyBotMove(game *Game, row, col, player int) {
	cellValue := game.Board[row][col]

	// Apply move
	if cellValue == nil {
		game.Board[row][col] = player
	} else {
		game.Board[row][col] = fmt.Sprintf("%d-fortified", player)
	}

	game.MovesLeft--

	// Broadcast move to all players
	moveMsg := Message{
		Type:      "move_made",
		GameID:    game.ID,
		Row:       &row,
		Col:       &col,
		Player:    player,
		MovesLeft: game.MovesLeft,
	}
	h.broadcastToGame(game, &moveMsg)

	log.Printf("Bot player %d moved to (%d,%d), %d moves left", player, row, col, game.MovesLeft)

	// Check if turn is over
	if game.MovesLeft == 0 {
		log.Printf("Bot turn ending for game %s", game.ID)
		h.endTurn(game)
	} else {
		// Bot makes another move (has 3 moves per turn)
		go func() {
			if !game.GameOver && game.CurrentPlayer == player {
				h.makeBotMove(game, player)
			}
		}()
	}

	// Check win condition
	h.checkMultiplayerStatus(game)
}
