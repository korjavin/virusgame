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

// scoreBotMove scores a move based on simple heuristics
func (h *Hub) scoreBotMove(game *Game, move BotMove, player int) float64 {
	cellValue := game.Board[move.Row][move.Col]
	cellStr := fmt.Sprintf("%v", cellValue)
	
	opponent := h.getOpponent(player, game)
	score := 0.0

	// 1. Attacking opponent cells is highly valued
	if cellValue != nil && len(cellStr) > 0 && cellStr[0] == byte('0'+opponent) {
		score += 100.0
	}

	// 2. Count neighbors
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
			if nr >= 0 && nr < game.Rows && nc >= 0 && nc < game.Cols {
				neighbor := game.Board[nr][nc]
				neighborStr := fmt.Sprintf("%v", neighbor)
				if neighbor != nil && len(neighborStr) > 0 {
					if neighborStr[0] == byte('0'+player) {
						friendlyNeighbors++
					} else if neighborStr[0] == byte('0'+opponent) {
						opponentNeighbors++
					}
				} else {
					emptyNeighbors++
				}
			}
		}
	}

	// Prefer moves with many friendly neighbors (cohesion)
	score += float64(friendlyNeighbors * 5)
	// Prefer moves adjacent to opponents (attack opportunities)
	score += float64(opponentNeighbors * 3)
	// Prefer moves with expansion potential
	score += float64(emptyNeighbors * 2)

	// 3. Add some randomness to make bot less predictable
	score += rand.Float64() * 10

	return score
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
