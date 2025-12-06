package main

import (
	"fmt"
	"log"
	"math/rand"
	"time"
)

func init() {
	rand.Seed(time.Now().UnixNano())
}

// BotMove represents a potential move for the bot
type BotMove struct {
	Row   int
	Col   int
	Score float64
}

// makeBot move makes a move for a bot player
func (h *Hub) makeBotMove(game *Game, botPlayer int) {
	log.Printf("Bot player %d making move in game %s", botPlayer, game.ID)

	// Get all valid moves
	validMoves := h.getAllBotMoves(game, botPlayer)

	if len(validMoves) == 0 {
		log.Printf("Bot player %d has no valid moves", botPlayer)
		return
	}

	// Score and pick best move
	bestMove := h.pickBestMove(game, validMoves, botPlayer)

	log.Printf("Bot player %d selected move [%d,%d]", botPlayer, bestMove.Row, bestMove.Col)

	// Apply the move
	h.applyBotMove(game, bestMove.Row, bestMove.Col, botPlayer)
}

// getAllBotMoves gets all valid moves for the bot
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

// pickBestMove selects the best move using simple heuristics
func (h *Hub) pickBestMove(game *Game, moves []BotMove, player int) BotMove {
	// Score each move
	for i := range moves {
		moves[i].Score = h.scoreBotMove(game, moves[i], player)
	}

	// Find best scoring move
	best := moves[0]
	for _, move := range moves {
		if move.Score > best.Score {
			best = move
		}
	}

	return best
}

// scoreBotMove scores a move based on heuristics matching ai.js scoreMove function
func (h *Hub) scoreBotMove(game *Game, move BotMove, player int) float64 {
	cellValue := game.Board[move.Row][move.Col]
	cellStr := fmt.Sprintf("%v", cellValue)
	score := 0.0

	// 1. HIGHEST PRIORITY: Capturing opponent cells (fortifying)
	// Match ai.js lines 242-249: 1000 points for attacking, +500 if fortified
	if cellValue != nil && len(cellStr) > 0 {
		// Check if this is an opponent cell
		isOpponentCell := false
		for p := 1; p <= 4; p++ {
			if p != player && game.Players[p-1] != nil && cellStr[0] == byte('0'+p) {
				isOpponentCell = true
				break
			}
		}

		if isOpponentCell {
			score += 1000.0
			// Extra bonus if opponent cell is fortified (breaks their structure)
			if len(cellStr) > 2 && cellStr[len(cellStr)-9:] == "fortified" {
				score += 500.0
			}
		}
	}

	// 2. Count friendly and opponent neighbors for positional evaluation
	// Match ai.js lines 252-270
	friendlyNeighbors := 0
	opponentNeighbors := 0
	emptyNeighbors := 0

	// Only check 4 cardinal directions (not diagonals) to match ai.js
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
				} else {
					// Any non-friendly piece is an opponent
					opponentNeighbors++
				}
			} else {
				emptyNeighbors++
			}
		}
	}

	// 3. Reward moves with multiple friendly connections (stable expansion)
	// Match ai.js line 273: friendlyNeighbors * 50
	score += float64(friendlyNeighbors * 50)

	// 4. Reward moves that threaten opponent cells (attack opportunities)
	// Match ai.js line 276: opponentNeighbors * 30
	score += float64(opponentNeighbors * 30)

	// 5. Reward expansion opportunities (empty neighbors for future growth)
	// Match ai.js line 279: emptyNeighbors * 10
	score += float64(emptyNeighbors * 10)

	// 6. Distance to opponent base (aggression)
	// Match ai.js lines 281-284: closer to opponent base is better
	opponentBase := h.getClosestOpponentBase(game, player, move.Row, move.Col)
	if opponentBase != nil {
		distToOpponentBase := abs(move.Row-opponentBase.Row) + abs(move.Col-opponentBase.Col)
		score -= float64(distToOpponentBase * 3)
	}

	// 7. Distance to own base (don't overextend)
	// Match ai.js lines 286-291: penalize if too far from own base
	ownBase := game.PlayerBases[player-1]
	distToOwnBase := abs(move.Row-ownBase.Row) + abs(move.Col-ownBase.Col)
	if distToOwnBase > 8 {
		score -= float64((distToOwnBase - 8) * 5) // Penalize overextension
	}

	return score
}

func abs(x int) int {
	if x < 0 {
		return -x
	}
	return x
}

// getClosestOpponentBase finds the closest active opponent's base
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

// getOpponent returns an opponent player number for the given player
func (h *Hub) getOpponent(player int, game *Game) int {
	// Find first opponent with pieces
	for i := 1; i <= 4; i++ {
		if i != player && game.Players[i-1] != nil {
			if h.countPlayerPieces(game, i) > 0 {
				return i
			}
		}
	}
	// Fallback
	if player == 1 {
		return 2
	}
	return 1
}

// applyBotMove applies a bot's move to the game
func (h *Hub) applyBotMove(game *Game, row, col, player int) {
	cellValue := game.Board[row][col]

	// Apply move
	if cellValue == nil {
		game.Board[row][col] = player
	} else {
		// Attacking opponent cell - fortify it
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
		time.AfterFunc(500*time.Millisecond, func() {
			if !game.GameOver && game.CurrentPlayer == player {
				h.makeBotMove(game, player)
			}
		})
	}

	// Check win condition
	h.checkMultiplayerStatus(game)
}
