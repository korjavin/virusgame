package main

import (
	"fmt"
	"math"
	"syscall/js"
)

// Global variables
var (
	rows           int
	cols           int
	player1BaseRow int
	player1BaseCol int
	player2BaseRow int
	player2BaseCol int
	progressCurrent int
	progressTotal   int

	// Transposition table for memoization
	transpositionTable map[string]MinimaxResult
	ttHits             int
	ttMisses           int
	alphaBetaCutoffs   int

	// AI coefficients (tunable)
	aiCoeffs struct {
		cellValue          float64
		fortifiedValue     float64
		mobilityValue      float64
		aggressionValue    float64
		connectionValue    float64
		attackValue        float64
		redundancyValue    float64
		defensibilityValue float64
	}
)

// BoardState represents the game board
type BoardState [][]interface{}

// Move represents a game move
type Move struct {
	Row   int
	Col   int
	Score float64
}

// MinimaxResult holds the result of minimax
type MinimaxResult struct {
	Score float64
	Move  *Move
}

// Main function - required for WASM
func main() {
	c := make(chan struct{})

	// Initialize defaults
	aiCoeffs.cellValue = 10
	aiCoeffs.fortifiedValue = 15
	aiCoeffs.mobilityValue = 5
	aiCoeffs.aggressionValue = 1
	aiCoeffs.connectionValue = 3
	aiCoeffs.attackValue = 8
	aiCoeffs.redundancyValue = 5
	aiCoeffs.defensibilityValue = 3

	transpositionTable = make(map[string]MinimaxResult)

	// Export functions to JavaScript
	js.Global().Set("wasmGetAIMove", js.FuncOf(wasmGetAIMove))
	js.Global().Set("wasmSetCoeffs", js.FuncOf(wasmSetCoeffs))
	js.Global().Set("wasmReady", js.ValueOf(true))

	fmt.Println("Go WASM AI initialized")
	<-c
}

// wasmSetCoeffs allows JS to set coefficients
func wasmSetCoeffs(this js.Value, args []js.Value) interface{} {
	coeffs := args[0]
	aiCoeffs.cellValue = coeffs.Get("cellValue").Float()
	aiCoeffs.fortifiedValue = coeffs.Get("fortifiedValue").Float()
	aiCoeffs.mobilityValue = coeffs.Get("mobilityValue").Float()
	aiCoeffs.aggressionValue = coeffs.Get("aggressionValue").Float()
	aiCoeffs.connectionValue = coeffs.Get("connectionValue").Float()
	aiCoeffs.attackValue = coeffs.Get("attackValue").Float()
	aiCoeffs.redundancyValue = coeffs.Get("redundancyValue").Float()
	aiCoeffs.defensibilityValue = coeffs.Get("defensibilityValue").Float()
	return nil
}

// wasmGetAIMove is the exported function called from JavaScript
func wasmGetAIMove(this js.Value, args []js.Value) interface{} {
	// Parse arguments: board, rows, cols, depth, bases
	boardJS := args[0]
	rows = args[1].Int()
	cols = args[2].Int()
	depth := args[3].Int()
	player1BaseRow = args[4].Int()
	player1BaseCol = args[5].Int()
	player2BaseRow = args[6].Int()
	player2BaseCol = args[7].Int()

	// Convert JS board to Go board
	board := jsArrayToBoard(boardJS)

	// Get all valid moves
	possibleMoves := getAllValidMoves(board, 2)

	// DEBUG: Log board state
	fmt.Printf("WASM DEBUG: Board size: %dx%d\n", rows, cols)
	fmt.Printf("WASM DEBUG: Found %d valid moves for player 2\n", len(possibleMoves))
	fmt.Printf("Valid moves: ")
	for i, m := range possibleMoves {
		if i < 10 {
			fmt.Printf("[%d,%d] ", m.Row, m.Col)
		}
	}
	fmt.Printf("\n")

	// Check what cells player 2 has
	player2Cells := 0
	for r := 0; r < rows; r++ {
		for c := 0; c < cols; c++ {
			cell := board[r][c]
			cellStr := cellToString(cell)
			if startsWithPlayer(cellStr, 2) {
				player2Cells++
				fmt.Printf("Player 2 cell at [%d,%d]: %v\n", r, c, cell)
			}
		}
	}
	fmt.Printf("WASM DEBUG: Player 2 has %d cells total\n", player2Cells)

	if len(possibleMoves) == 0 {
		return js.Null()
	}

	// Update progress
	progressCurrent = 0
	progressTotal = len(possibleMoves)
	updateProgress()

	// Clear transposition table
	transpositionTable = make(map[string]MinimaxResult)
	ttHits = 0
	ttMisses = 0
	alphaBetaCutoffs = 0

	// Run minimax
	result := minimax(board, depth, math.Inf(-1), math.Inf(1), true, true)

	fmt.Printf("TT hits: %d, misses: %d, AB cutoffs: %d\n", ttHits, ttMisses, alphaBetaCutoffs)

	if result.Move == nil {
		return js.Null()
	}

	// Return move as JS object
	moveObj := js.Global().Get("Object").New()
	moveObj.Set("row", result.Move.Row)
	moveObj.Set("col", result.Move.Col)
	moveObj.Set("score", result.Move.Score)

	return moveObj
}

// hashBoard creates a string hash of the board state
func hashBoard(board BoardState) string {
	hash := ""
	for r := 0; r < rows; r++ {
		for c := 0; c < cols; c++ {
			cell := board[r][c]
			if cell == nil {
				hash += "0,"
			} else if num, ok := cell.(int); ok {
				hash += fmt.Sprintf("%d,", num)
			} else if str, ok := cell.(string); ok {
				hash += str + ","
			}
		}
	}
	return hash
}

// scoreMove provides a heuristic score for move ordering
func scoreMove(board BoardState, move Move, player int) float64 {
	cellValue := board[move.Row][move.Col]
	cellStr := cellToString(cellValue)
	opponent := 1
	if player == 1 {
		opponent = 2
	}

	score := 0.0

	// 1. Capturing opponent cells (fortifying)
	if startsWithPlayer(cellStr, opponent) {
		score += 1000
		if containsString(cellStr, "fortified") {
			score += 500
		}
	}

	// 2. Count friendly and opponent neighbors
	friendlyNeighbors := 0
	opponentNeighbors := 0
	emptyNeighbors := 0

	for i := -1; i <= 1; i++ {
		for j := -1; j <= 1; j++ {
			if i == 0 && j == 0 {
				continue
			}
			nr := move.Row + i
			nc := move.Col + j
			if nr >= 0 && nr < rows && nc >= 0 && nc < cols {
				neighbor := board[nr][nc]
				neighborStr := cellToString(neighbor)
				if startsWithPlayer(neighborStr, player) {
					friendlyNeighbors++
				} else if startsWithPlayer(neighborStr, opponent) {
					opponentNeighbors++
				} else if neighbor == nil {
					emptyNeighbors++
				}
			}
		}
	}

	score += float64(friendlyNeighbors * 50)
	score += float64(opponentNeighbors * 30)
	score += float64(emptyNeighbors * 10)

	// 3. Distance to opponent base
	opponentBaseRow := player1BaseRow
	opponentBaseCol := player1BaseCol
	if player == 2 {
		opponentBaseRow = player2BaseRow
		opponentBaseCol = player2BaseCol
	}
	distToOpponentBase := abs(move.Row-opponentBaseRow) + abs(move.Col-opponentBaseCol)
	score -= float64(distToOpponentBase * 3)

	// 4. Distance to own base (penalize overextension)
	ownBaseRow := player2BaseRow
	ownBaseCol := player2BaseCol
	if player == 1 {
		ownBaseRow = player1BaseRow
		ownBaseCol = player1BaseCol
	}
	distToOwnBase := abs(move.Row-ownBaseRow) + abs(move.Col-ownBaseCol)
	if distToOwnBase > 8 {
		score -= float64((distToOwnBase - 8) * 5)
	}

	return score
}

// sortMovesByScore sorts moves by their heuristic score
func sortMovesByScore(board BoardState, moves []Move, player int, descending bool) {
	// Simple bubble sort (good enough for small move lists)
	for i := 0; i < len(moves)-1; i++ {
		for j := 0; j < len(moves)-i-1; j++ {
			scoreA := scoreMove(board, moves[j], player)
			scoreB := scoreMove(board, moves[j+1], player)

			shouldSwap := false
			if descending {
				shouldSwap = scoreB > scoreA
			} else {
				shouldSwap = scoreA > scoreB
			}

			if shouldSwap {
				moves[j], moves[j+1] = moves[j+1], moves[j]
			}
		}
	}
}

// minimax implements the minimax algorithm with alpha-beta pruning
func minimax(board BoardState, depth int, alpha, beta float64, isMaximizing, isTopLevel bool) MinimaxResult {
	// Check transposition table
	boardHash := hashBoard(board)
	ttKey := fmt.Sprintf("%s|%d|%t", boardHash, depth, isMaximizing)

	if cached, ok := transpositionTable[ttKey]; ok {
		ttHits++
		return cached
	}
	ttMisses++

	// Base case: reached max depth
	if depth == 0 {
		result := MinimaxResult{
			Score: evaluateBoard(board),
			Move:  nil,
		}
		transpositionTable[ttKey] = result
		return result
	}

	player := 2
	if !isMaximizing {
		player = 1
	}

	possibleMoves := getAllValidMoves(board, player)

	// Move ordering: sort to try best moves first
	sortMovesByScore(board, possibleMoves, player, isMaximizing)

	// Terminal state: no moves available
	if len(possibleMoves) == 0 {
		score := evaluateBoard(board)
		if isMaximizing {
			score -= 10000
		} else {
			score += 10000
		}
		return MinimaxResult{Score: score, Move: nil}
	}

	if isMaximizing {
		maxScore := math.Inf(-1)
		var bestMove *Move

		for i, move := range possibleMoves {
			// Update progress at top level
			if isTopLevel {
				progressCurrent = i + 1
				updateProgress()
			}

			// Try this move
			newBoard := applyMove(board, move.Row, move.Col, player)

			// Recursively evaluate
			result := minimax(newBoard, depth-1, alpha, beta, false, false)

			if result.Score > maxScore {
				maxScore = result.Score
				bestMove = &move
			}

			// Alpha-beta pruning
			alpha = math.Max(alpha, result.Score)
			if beta <= alpha {
				alphaBetaCutoffs++
				break
			}
		}

		result := MinimaxResult{Score: maxScore, Move: bestMove}
		transpositionTable[ttKey] = result
		return result
	} else {
		minScore := math.Inf(1)
		var bestMove *Move

		for _, move := range possibleMoves {
			newBoard := applyMove(board, move.Row, move.Col, player)
			result := minimax(newBoard, depth-1, alpha, beta, true, false)

			if result.Score < minScore {
				minScore = result.Score
				bestMove = &move
			}

			beta = math.Min(beta, result.Score)
			if beta <= alpha {
				alphaBetaCutoffs++
				break
			}
		}

		result := MinimaxResult{Score: minScore, Move: bestMove}
		transpositionTable[ttKey] = result
		return result
	}
}

// evaluateBoard evaluates the board position
func evaluateBoard(board BoardState) float64 {
	score := 0.0

	// 1. Material advantage
	aiCells := 0
	opponentCells := 0
	aiFortified := 0
	opponentFortified := 0

	for r := 0; r < rows; r++ {
		for c := 0; c < cols; c++ {
			cell := board[r][c]
			cellStr := cellToString(cell)

			if startsWithPlayer(cellStr, 2) {
				aiCells++
				if containsString(cellStr, "fortified") {
					aiFortified++
				}
			} else if startsWithPlayer(cellStr, 1) {
				opponentCells++
				if containsString(cellStr, "fortified") {
					opponentFortified++
				}
			}
		}
	}

	score += float64(aiCells*10 + aiFortified*15 - opponentCells*10 - opponentFortified*15)

	// 2. Mobility advantage
	aiMoves := len(getAllValidMoves(board, 2))
	opponentMoves := len(getAllValidMoves(board, 1))
	score += float64((aiMoves - opponentMoves) * 5)

	// 3. Positional advantage
	for r := 0; r < rows; r++ {
		for c := 0; c < cols; c++ {
			cell := board[r][c]
			cellStr := cellToString(cell)

			if startsWithPlayer(cellStr, 2) {
				// Reward aggressive positioning
				distToOpponent := abs(r-player1BaseRow) + abs(c-player1BaseCol)
				score += float64(rows + cols - distToOpponent)

				// Reward connections
				connections := countAdjacentCells(board, r, c, 2)
				score += float64(connections * 3)
			} else if startsWithPlayer(cellStr, 1) {
				distToAI := abs(r-player2BaseRow) + abs(c-player2BaseCol)
				score -= float64(rows + cols - distToAI)

				connections := countAdjacentCells(board, r, c, 1)
				score -= float64(connections * 3)
			}
		}
	}

	// 4. Attack opportunities
	aiAttacks := 0
	opponentAttacks := 0

	for r := 0; r < rows; r++ {
		for c := 0; c < cols; c++ {
			cell := board[r][c]
			cellStr := cellToString(cell)

			if startsWithPlayer(cellStr, 1) {
				if countAdjacentCells(board, r, c, 2) > 0 {
					aiAttacks++
				}
			}
			if startsWithPlayer(cellStr, 2) {
				if countAdjacentCells(board, r, c, 1) > 0 {
					opponentAttacks++
				}
			}
		}
	}

	score += float64((aiAttacks - opponentAttacks) * 8)

	return score
}

// getAllValidMoves returns all valid moves for a player
func getAllValidMoves(board BoardState, player int) []Move {
	var moves []Move

	for r := 0; r < rows; r++ {
		for c := 0; c < cols; c++ {
			if isValidMove(board, r, c, player) {
				moves = append(moves, Move{Row: r, Col: c})
			}
		}
	}

	// DEBUG: For first turn, check why [8,9] isn't valid
	if len(moves) == 3 && player == 2 {
		fmt.Printf("DEBUG: Why is [8,9] not valid? Checking...\n")
		testRow, testCol := 8, 9
		cell := board[testRow][testCol]
		fmt.Printf("  Cell at [8,9]: %v\n", cell)
		fmt.Printf("  Is adjacent to player? %v\n", isAdjacentToPlayer(board, testRow, testCol, player))
		// Check if any adjacent cell is connected
		for i := -1; i <= 1; i++ {
			for j := -1; j <= 1; j++ {
				if i == 0 && j == 0 {
					continue
				}
				adjRow := testRow + i
				adjCol := testCol + j
				if adjRow >= 0 && adjRow < rows && adjCol >= 0 && adjCol < cols {
					adjCell := board[adjRow][adjCol]
					adjStr := cellToString(adjCell)
					if startsWithPlayer(adjStr, player) {
						connected := isConnectedToBase(board, adjRow, adjCol, player)
						fmt.Printf("  Adjacent [%d,%d]=%v connected=%v\n", adjRow, adjCol, adjCell, connected)
					}
				}
			}
		}
	}

	return moves
}

// isValidMove checks if a move is valid
func isValidMove(board BoardState, row, col, player int) bool {
	// DEBUG for [8,9]
	debug := row == 8 && col == 9 && player == 2
	if debug {
		fmt.Printf(">>> isValidMove [8,9] player 2\n")
	}

	cell := board[row][col]
	cellStr := cellToString(cell)
	opponent := 1
	if player == 1 {
		opponent = 2
	}

	// Cannot move on fortified or base cells
	if containsString(cellStr, "fortified") || containsString(cellStr, "base") {
		if debug {
			fmt.Printf(">>> FAILED: cell contains fortified or base\n")
		}
		return false
	}

	// Can only attack opponent or expand to empty
	if cell != nil && !startsWithPlayer(cellStr, opponent) {
		if debug {
			fmt.Printf(">>> FAILED: cell not nil and doesn't start with opponent\n")
		}
		return false
	}

	// Must be adjacent to own territory
	if !isAdjacentToPlayer(board, row, col, player) {
		if debug {
			fmt.Printf(">>> FAILED: not adjacent to player\n")
		}
		return false
	}

	if debug {
		fmt.Printf(">>> Checking adjacent cells for base connectivity...\n")
	}

	// Check if adjacent cell is connected to base
	for i := -1; i <= 1; i++ {
		for j := -1; j <= 1; j++ {
			if i == 0 && j == 0 {
				continue
			}
			adjRow := row + i
			adjCol := col + j

			if adjRow >= 0 && adjRow < rows && adjCol >= 0 && adjCol < cols {
				adjCell := board[adjRow][adjCol]
				adjStr := cellToString(adjCell)
				if startsWithPlayer(adjStr, player) {
					connected := isConnectedToBase(board, adjRow, adjCol, player)
					if debug {
						fmt.Printf(">>> Adjacent [%d,%d]=%v starts with player, connected=%v\n", adjRow, adjCol, adjCell, connected)
					}
					if connected {
						if debug {
							fmt.Printf(">>> SUCCESS: Found connected adjacent cell!\n")
						}
						return true
					}
				}
			}
		}
	}

	if debug {
		fmt.Printf(">>> FAILED: No connected adjacent cells found\n")
	}
	return false
}

// isAdjacentToPlayer checks if a cell is adjacent to player's territory
func isAdjacentToPlayer(board BoardState, row, col, player int) bool {
	for i := -1; i <= 1; i++ {
		for j := -1; j <= 1; j++ {
			if i == 0 && j == 0 {
				continue
			}
			newRow := row + i
			newCol := col + j

			if newRow >= 0 && newRow < rows && newCol >= 0 && newCol < cols {
				cell := board[newRow][newCol]
				cellStr := cellToString(cell)
				if startsWithPlayer(cellStr, player) {
					return true
				}
			}
		}
	}
	return false
}

// isConnectedToBase checks if a cell is connected to player's base
func isConnectedToBase(board BoardState, startRow, startCol, player int) bool {
	baseRow := player1BaseRow
	baseCol := player1BaseCol
	if player == 2 {
		baseRow = player2BaseRow
		baseCol = player2BaseCol
	}

	visited := make(map[string]bool)
	stack := []struct{ row, col int }{{startRow, startCol}}
	visited[fmt.Sprintf("%d,%d", startRow, startCol)] = true

	for len(stack) > 0 {
		pos := stack[len(stack)-1]
		stack = stack[:len(stack)-1]

		if pos.row == baseRow && pos.col == baseCol {
			return true
		}

		for i := -1; i <= 1; i++ {
			for j := -1; j <= 1; j++ {
				if i == 0 && j == 0 {
					continue
				}
				newRow := pos.row + i
				newCol := pos.col + j
				key := fmt.Sprintf("%d,%d", newRow, newCol)

				if newRow >= 0 && newRow < rows && newCol >= 0 && newCol < cols && !visited[key] {
					cell := board[newRow][newCol]
					cellStr := cellToString(cell)
					if startsWithPlayer(cellStr, player) {
						visited[key] = true
						stack = append(stack, struct{ row, col int }{newRow, newCol})
					}
				}
			}
		}
	}

	return false
}

// countAdjacentCells counts adjacent cells belonging to a player
func countAdjacentCells(board BoardState, row, col, player int) int {
	count := 0
	for i := -1; i <= 1; i++ {
		for j := -1; j <= 1; j++ {
			if i == 0 && j == 0 {
				continue
			}
			newRow := row + i
			newCol := col + j

			if newRow >= 0 && newRow < rows && newCol >= 0 && newCol < cols {
				cell := board[newRow][newCol]
				cellStr := cellToString(cell)
				if startsWithPlayer(cellStr, player) {
					count++
				}
			}
		}
	}
	return count
}

// applyMove applies a move to the board and returns a new board
func applyMove(board BoardState, row, col, player int) BoardState {
	newBoard := make(BoardState, rows)
	for i := range board {
		newBoard[i] = make([]interface{}, cols)
		copy(newBoard[i], board[i])
	}

	cell := newBoard[row][col]
	opponent := 1
	if player == 1 {
		opponent = 2
	}

	if cell == nil {
		newBoard[row][col] = player
	} else if startsWithPlayer(cellToString(cell), opponent) {
		newBoard[row][col] = fmt.Sprintf("%d-fortified", player)
	}

	return newBoard
}

// Helper functions

func jsArrayToBoard(jsArray js.Value) BoardState {
	board := make(BoardState, rows)
	for r := 0; r < rows; r++ {
		board[r] = make([]interface{}, cols)
		rowJS := jsArray.Index(r)
		for c := 0; c < cols; c++ {
			cell := rowJS.Index(c)
			if cell.IsNull() || cell.IsUndefined() {
				board[r][c] = nil
			} else if cell.Type() == js.TypeNumber {
				board[r][c] = cell.Int()
			} else if cell.Type() == js.TypeString {
				board[r][c] = cell.String()
			}
		}
	}
	return board
}

func cellToString(cell interface{}) string {
	if cell == nil {
		return ""
	}
	if str, ok := cell.(string); ok {
		return str
	}
	if num, ok := cell.(int); ok {
		return fmt.Sprintf("%d", num)
	}
	return ""
}

func startsWithPlayer(cellStr string, player int) bool {
	if len(cellStr) == 0 {
		return false
	}
	playerStr := fmt.Sprintf("%d", player)
	return len(cellStr) > 0 && string(cellStr[0]) == playerStr
}

func containsString(str, substr string) bool {
	return len(str) > 0 && len(substr) > 0 &&
		len(str) >= len(substr) &&
		containsSubstring(str, substr)
}

func containsSubstring(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

func abs(x int) int {
	if x < 0 {
		return -x
	}
	return x
}

func updateProgress() {
	js.Global().Call("updateAIProgressFromWasm", progressCurrent, progressTotal)
}
