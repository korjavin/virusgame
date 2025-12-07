package main

import (
	"encoding/json"
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/google/uuid"
)

// MessageWrapper wraps a message with its client
type MessageWrapper struct {
	client  *Client
	message *Message
}

// Hub maintains the set of active clients and broadcasts messages
type Hub struct {
	clients       map[*Client]bool
	users         map[string]*User
	challenges    map[string]*Challenge
	games         map[string]*Game
	lobbies       map[string]*Lobby
	register      chan *Client
	unregister    chan *Client
	handleMessage chan *MessageWrapper
}

func newHub() *Hub {
	return &Hub{
		clients:       make(map[*Client]bool),
		users:         make(map[string]*User),
		challenges:    make(map[string]*Challenge),
		games:         make(map[string]*Game),
		lobbies:       make(map[string]*Lobby),
		register:      make(chan *Client),
		unregister:    make(chan *Client),
		handleMessage: make(chan *MessageWrapper, 256), // Buffered to prevent deadlock when sending internal messages
	}
}

func (h *Hub) run() {
	// Periodic cleanup ticker - runs every 5 minutes
	cleanupTicker := time.NewTicker(5 * time.Minute)
	defer cleanupTicker.Stop()

	for {
		select {
		case client := <-h.register:
			h.clients[client] = true
			h.handleConnect(client)
		case client := <-h.unregister:
			if _, ok := h.clients[client]; ok {
				h.handleDisconnect(client)
				delete(h.clients, client)
				close(client.send)
			}
		case wrapper := <-h.handleMessage:
			h.handleClientMessage(wrapper.client, wrapper.message)
		case <-cleanupTicker.C:
			h.cleanupStaleGames()
		}
	}
}

func (h *Hub) handleConnect(client *Client) {
	// Generate random username
	username := GenerateRandomName()
	userID := uuid.New().String()

	user := &User{
		ID:       userID,
		Username: username,
		Client:   client,
		InGame:   false,
	}
	client.user = user
	h.users[userID] = user

	// Send welcome message
	msg := Message{
		Type:     "welcome",
		UserID:   userID,
		Username: username,
	}
	h.sendToClient(client, &msg)

	// Broadcast updated user list
	h.broadcastUserList()

	log.Printf("User connected: %s (%s)", username, userID)
}

func (h *Hub) handleDisconnect(client *Client) {
	if client.user == nil {
		return
	}

	user := client.user
	log.Printf("User disconnected: %s (%s)", user.Username, user.ID)

	// Remove user from lobbies
	if user.InLobby && user.LobbyID != "" {
		lobby, exists := h.lobbies[user.LobbyID]
		if exists {
			h.removeUserFromLobby(lobby, user)
		}
	}

	// Remove user from active games
	for gameID, game := range h.games {
		userInGame := false

		if game.IsMultiplayer {
			// Check if user is in multiplayer game
			for i := 0; i < 4; i++ {
				if game.Players[i] != nil && game.Players[i].User != nil && game.Players[i].User.ID == user.ID {
					userInGame = true
					break
				}
			}

			if userInGame && !game.GameOver {
				// Auto-resign the disconnected player (only if game is still active)
				log.Printf("Player %s disconnected from multiplayer game %s - auto-resigning", user.Username, gameID)
				resignMsg := &Message{
					GameID: gameID,
				}
				h.handleResign(user, resignMsg)
				// handleResign will notify other players and check if game should end
			}
		} else {
			// 1v1 game
			if (game.Player1 != nil && game.Player1.ID == user.ID) || (game.Player2 != nil && game.Player2.ID == user.ID) {
				// Notify opponent
				var opponent *User
				if game.Player1 != nil && game.Player1.ID == user.ID {
					opponent = game.Player2
				} else {
					opponent = game.Player1
				}

				// Mark opponent as no longer in game
				if opponent != nil {
					opponent.InGame = false
					msg := Message{
						Type:   "opponent_disconnected",
						GameID: gameID,
					}
					h.sendToUser(opponent, &msg)
				}

				delete(h.games, gameID)
			}
		}
	}

	// Remove pending challenges
	for challengeID, challenge := range h.challenges {
		if challenge.FromUser.ID == user.ID || challenge.ToUser.ID == user.ID {
			delete(h.challenges, challengeID)
		}
	}

	delete(h.users, user.ID)
	h.broadcastUserList()
}

func (h *Hub) handleClientMessage(client *Client, msg *Message) {
	switch msg.Type {
	case "challenge":
		h.handleChallenge(client.user, msg)
	case "accept_challenge":
		h.handleAcceptChallenge(client.user, msg)
	case "decline_challenge":
		h.handleDeclineChallenge(client.user, msg)
	case "move":
		h.handleMove(client.user, msg)
	case "neutrals":
		h.handleNeutrals(client.user, msg)
	case "rematch":
		h.handleRematch(client.user, msg)
	case "resign":
		h.handleResign(client.user, msg)
	case "leave_game":
		h.handleLeaveGame(client.user, msg)
	case "cleanup_game":
		h.handleCleanupGame(msg)
	// Internal messages (from timers/bots - no client)
	case "bot_move":
		h.handleBotMoveRequest(msg)
	case "bot_move_result":
		h.handleBotMoveResult(msg)
	case "move_timeout":
		h.handleMoveTimeout(msg)
	// Lobby messages
	case "create_lobby":
		h.handleCreateLobby(client.user, msg)
	case "join_lobby":
		h.handleJoinLobby(client.user, msg)
	case "leave_lobby":
		h.handleLeaveLobby(client.user, msg)
	case "add_bot":
		h.handleAddBot(client.user, msg)
	case "remove_bot":
		h.handleRemoveBot(client.user, msg)
	case "start_multiplayer_game":
		h.handleStartMultiplayerGame(client.user, msg)
	case "get_lobbies":
		h.handleGetLobbies(client.user, msg)
	default:
		log.Printf("Unknown message type: %s", msg.Type)
	}
}

func (h *Hub) handleChallenge(from *User, msg *Message) {
	to, exists := h.users[msg.TargetUserID]
	if !exists {
		log.Printf("Target user not found: %s", msg.TargetUserID)
		return
	}

	if to.InGame {
		errorMsg := Message{
			Type: "error",
			Username: "User is already in game",
		}
		h.sendToUser(from, &errorMsg)
		return
	}

	// Get board size from message, default to 10x10
	rows := msg.Rows
	cols := msg.Cols
	if rows < 5 || rows > 50 {
		rows = 10
	}
	if cols < 5 || cols > 50 {
		cols = 10
	}

	challengeID := uuid.New().String()
	challenge := &Challenge{
		ID:        challengeID,
		FromUser:  from,
		ToUser:    to,
		Rows:      rows,
		Cols:      cols,
		Timestamp: time.Now(),
	}
	h.challenges[challengeID] = challenge

	// Send challenge notification to target user
	challengeMsg := Message{
		Type:         "challenge_received",
		ChallengeID:  challengeID,
		FromUserID:   from.ID,
		FromUsername: from.Username,
	}
	h.sendToUser(to, &challengeMsg)

	log.Printf("Challenge created: %s -> %s (%dx%d)", from.Username, to.Username, rows, cols)
}

func (h *Hub) handleAcceptChallenge(user *User, msg *Message) {
	challenge, exists := h.challenges[msg.ChallengeID]
	if !exists {
		log.Printf("Challenge not found: %s", msg.ChallengeID)
		return
	}

	if challenge.ToUser.ID != user.ID {
		log.Printf("User %s tried to accept challenge not meant for them", user.Username)
		return
	}

	// Create game with board size from challenge
	gameID := uuid.New().String()
	rows := challenge.Rows
	cols := challenge.Cols

	board := make([][]interface{}, rows)
	for i := range board {
		board[i] = make([]interface{}, cols)
	}

	// Set base positions
	board[0][0] = "1-base"
	board[rows-1][cols-1] = "2-base"

	game := &Game{
		ID:            gameID,
		Player1:       challenge.FromUser,
		Player2:       challenge.ToUser,
		Board:         board,
		CurrentPlayer: 1,
		MovesLeft:     3,
		Player1Base:   CellPos{Row: 0, Col: 0},
		Player2Base:   CellPos{Row: rows - 1, Col: cols - 1},
		GameOver:      false,
		Winner:        0,
		Player1NeutralsUsed: false,
		Player2NeutralsUsed: false,
		Rows:          rows,
		Cols:          cols,
	}
	h.games[gameID] = game

	// Mark users as in game
	challenge.FromUser.InGame = true
	challenge.ToUser.InGame = true

	// Send game start to both players
	p1Msg := Message{
		Type:             "game_start",
		GameID:           gameID,
		OpponentID:       challenge.ToUser.ID,
		OpponentUsername: challenge.ToUser.Username,
		YourPlayer:       1,
		Rows:             rows,
		Cols:             cols,
	}
	h.sendToUser(challenge.FromUser, &p1Msg)

	p2Msg := Message{
		Type:             "game_start",
		GameID:           gameID,
		OpponentID:       challenge.FromUser.ID,
		OpponentUsername: challenge.FromUser.Username,
		YourPlayer:       2,
		Rows:             rows,
		Cols:             cols,
	}
	h.sendToUser(challenge.ToUser, &p2Msg)

	// Clean up challenge
	delete(h.challenges, msg.ChallengeID)

	// Broadcast updated user list
	h.broadcastUserList()

	log.Printf("Game started: %s vs %s (Game ID: %s)", challenge.FromUser.Username, challenge.ToUser.Username, gameID)
}

func (h *Hub) handleDeclineChallenge(user *User, msg *Message) {
	challenge, exists := h.challenges[msg.ChallengeID]
	if !exists {
		return
	}

	if challenge.ToUser.ID != user.ID {
		return
	}

	// Notify challenger
	declineMsg := Message{
		Type:        "challenge_declined",
		ChallengeID: msg.ChallengeID,
	}
	h.sendToUser(challenge.FromUser, &declineMsg)

	delete(h.challenges, msg.ChallengeID)
	log.Printf("Challenge declined: %s declined %s", user.Username, challenge.FromUser.Username)
}

func (h *Hub) handleMove(user *User, msg *Message) {
	game, exists := h.games[msg.GameID]
	if !exists {
		return
	}

	// Check Row and Col are provided
	if msg.Row == nil || msg.Col == nil {
		log.Printf("Move message missing row or col")
		return
	}

	row := *msg.Row
	col := *msg.Col

	// Find player number for this user
	var playerNum int
	if game.IsMultiplayer {
		// Find player in multiplayer game
		for i := 0; i < 4; i++ {
			if game.Players[i] != nil && game.Players[i].User != nil && game.Players[i].User.ID == user.ID {
				playerNum = i + 1
				break
			}
		}
		if playerNum == 0 {
			return // User not in this game
		}
	} else {
		// Legacy 1v1 mode
		if game.Player1.ID == user.ID {
			playerNum = 1
		} else if game.Player2.ID == user.ID {
			playerNum = 2
		} else {
			return
		}
	}

	if game.CurrentPlayer != playerNum || game.GameOver {
		return
	}

	// Validate and apply move
	cellValue := game.Board[row][col]

	// Check if it's a valid target (empty or opponent cell)
	isValidTarget := false
	if cellValue == nil {
		isValidTarget = true
	} else {
		cellStr := fmt.Sprintf("%v", cellValue)
		// Can attack opponent's non-fortified, non-base cells
		if len(cellStr) > 0 && cellStr[0] != byte('0'+playerNum) &&
		   !strings.Contains(cellStr, "fortified") && !strings.Contains(cellStr, "base") {
			isValidTarget = true
		}
	}

	if !isValidTarget {
		return
	}

	// Apply move
	if cellValue == nil {
		game.Board[row][col] = playerNum
	} else {
		// Attacking opponent cell - fortify it
		game.Board[row][col] = fmt.Sprintf("%d-fortified", playerNum)
	}

	game.MovesLeft--

	// Cancel and restart move timer (player made a move, reset their time)
	if game.MoveTimer != nil {
		game.MoveTimer.Stop()
		game.MoveTimer = nil
	}
	h.startMoveTimer(game)

	log.Printf("Move made in game %s: player %d moved to (%d,%d), %d moves left", game.ID, playerNum, row, col, game.MovesLeft)

	// Broadcast move to all players with updated movesLeft
	moveMsg := Message{
		Type:      "move_made",
		GameID:    msg.GameID,
		Row:       msg.Row,
		Col:       msg.Col,
		Player:    playerNum,
		MovesLeft: game.MovesLeft,
	}
	h.broadcastToGame(game, &moveMsg)

	// Check if turn is over OR if player has no more valid moves
	hasValidMoves := h.canMakeAnyMove(game, playerNum)
	if game.MovesLeft == 0 || !hasValidMoves {
		if !hasValidMoves && game.MovesLeft > 0 {
			log.Printf("Player %d has no more valid moves (had %d moves left), ending turn early", playerNum, game.MovesLeft)
		} else {
			log.Printf("Turn ending for game %s, calling endTurn()", game.ID)
		}
		h.endTurn(game)
	}

	// Check win condition and elimination
	h.checkMultiplayerStatus(game)
}

func (h *Hub) handleNeutrals(user *User, msg *Message) {
	game, exists := h.games[msg.GameID]
	if !exists {
		return
	}

	var playerNum int
	if game.Player1.ID == user.ID {
		playerNum = 1
	} else if game.Player2.ID == user.ID {
		playerNum = 2
	} else {
		return
	}

	if game.CurrentPlayer != playerNum || game.GameOver {
		return
	}

	// Mark cells as killed
	for _, cell := range msg.Cells {
		if game.Board[cell.Row][cell.Col] == playerNum {
			game.Board[cell.Row][cell.Col] = "killed"
		}
	}

	if playerNum == 1 {
		game.Player1NeutralsUsed = true
	} else {
		game.Player2NeutralsUsed = true
	}

	// Broadcast to opponent
	var opponentUser *User
	if playerNum == 1 {
		opponentUser = game.Player2
	} else {
		opponentUser = game.Player1
	}

	neutralsMsg := Message{
		Type:   "neutrals_placed",
		GameID: msg.GameID,
		Player: playerNum,
		Cells:  msg.Cells,
	}
	h.sendToUser(opponentUser, &neutralsMsg)

	// End turn
	game.CurrentPlayer = 3 - playerNum
	game.MovesLeft = 3

	turnMsg := Message{
		Type:   "turn_change",
		GameID: msg.GameID,
		Player: game.CurrentPlayer,
	}
	h.sendToUser(game.Player1, &turnMsg)
	h.sendToUser(game.Player2, &turnMsg)
}

func (h *Hub) handleRematch(user *User, msg *Message) {
	game, exists := h.games[msg.GameID]
	if !exists {
		return
	}

	var opponent *User
	if game.Player1.ID == user.ID {
		opponent = game.Player2
	} else if game.Player2.ID == user.ID {
		opponent = game.Player1
	} else {
		return
	}

	// Send rematch request to opponent
	rematchMsg := Message{
		Type:       "rematch_received",
		GameID:     msg.GameID,
		FromUserID: user.ID,
	}
	h.sendToUser(opponent, &rematchMsg)
}

func (h *Hub) handleResign(user *User, msg *Message) {
	game, exists := h.games[msg.GameID]
	if !exists {
		return
	}

	// Don't process resign if game is already over
	if game.GameOver {
		return
	}

	// Cancel move timer if it exists
	if game.MoveTimer != nil {
		game.MoveTimer.Stop()
		game.MoveTimer = nil
	}

	if game.IsMultiplayer {
		// Multiplayer mode - find player who resigned
		var resignedPlayer int
		for i := 0; i < 4; i++ {
			if game.Players[i] != nil && game.Players[i].User != nil && game.Players[i].User.ID == user.ID {
				resignedPlayer = i + 1
				break
			}
		}

		if resignedPlayer == 0 {
			return // User not in this game
		}

		// Remove all cells of the resigned player from the board
		for i := 0; i < game.Rows; i++ {
			for j := 0; j < game.Cols; j++ {
				cell := game.Board[i][j]
				if cell != nil {
					cellStr := fmt.Sprintf("%v", cell)
					if len(cellStr) > 0 && cellStr[0] == byte('0'+resignedPlayer) {
						game.Board[i][j] = "killed"
					}
				}
			}
		}

		// If the resigned player was the current player, pass turn to next player
		if game.CurrentPlayer == resignedPlayer && !game.GameOver {
			log.Printf("Resigned player %d was current player, passing turn", resignedPlayer)
			h.endTurn(game)
		}

		// Check if game is over (only 1 player left)
		h.checkMultiplayerStatus(game)

		log.Printf("Player %d resigned from multiplayer game %s", resignedPlayer, game.ID)
	} else {
		// 1v1 mode
		var winner int
		if game.Player1.ID == user.ID {
			winner = 2
		} else if game.Player2.ID == user.ID {
			winner = 1
		} else {
			return
		}

		game.GameOver = true
		game.Winner = winner

		endMsg := Message{
			Type:   "game_end",
			GameID: game.ID,
			Winner: winner,
		}
		h.sendToUser(game.Player1, &endMsg)
		h.sendToUser(game.Player2, &endMsg)

		// Mark users as not in game
		game.Player1.InGame = false
		game.Player2.InGame = false

		h.broadcastUserList()

		log.Printf("Game ended by resignation: %s (winner: player %d)", game.ID, winner)
	}
}

func (h *Hub) handleLeaveGame(user *User, msg *Message) {
	game, exists := h.games[msg.GameID]
	if !exists {
		return
	}

	if !game.IsMultiplayer {
		return // Only for multiplayer games
	}

	// Find player who wants to leave
	var leavingPlayerIndex int = -1
	for i := 0; i < 4; i++ {
		if game.Players[i] != nil && game.Players[i].User != nil && game.Players[i].User.ID == user.ID {
			leavingPlayerIndex = i
			break
		}
	}

	if leavingPlayerIndex == -1 {
		return // User not in this game
	}

	// Mark player as having left by setting User to nil but keeping the player slot
	// This prevents them from receiving further updates
	if game.Players[leavingPlayerIndex] != nil {
		game.Players[leavingPlayerIndex].User = nil
	}

	// Mark user as not in game
	user.InGame = false
	user.GameID = ""

	log.Printf("Player %s left game %s (player index %d)", user.Username, game.ID, leavingPlayerIndex+1)

	h.broadcastUserList()
}

func (h *Hub) handleCleanupGame(msg *Message) {
	if _, exists := h.games[msg.GameID]; exists {
		delete(h.games, msg.GameID)
		log.Printf("Cleaned up ended game: %s", msg.GameID)
	}
}

// cleanupUserFromPreviousGame removes a user from any previous game they were in
// This prevents state glitches when starting a new game
func (h *Hub) cleanupUserFromPreviousGame(user *User) {
	if !user.InGame || user.GameID == "" {
		return
	}

	oldGame, exists := h.games[user.GameID]
	if !exists {
		// Game already cleaned up, just reset user state
		user.InGame = false
		user.GameID = ""
		return
	}

	// Remove user from the old game
	for i := 0; i < 4; i++ {
		if oldGame.Players[i] != nil && oldGame.Players[i].User != nil && oldGame.Players[i].User.ID == user.ID {
			oldGame.Players[i].User = nil
			log.Printf("Removed user %s from previous game %s (slot %d)", user.Username, oldGame.ID, i+1)
			break
		}
	}

	// If old game is over and has no more human players, clean it up immediately
	if oldGame.GameOver {
		hasHumanPlayers := false
		for i := 0; i < 4; i++ {
			if oldGame.Players[i] != nil && oldGame.Players[i].User != nil {
				hasHumanPlayers = true
				break
			}
		}
		if !hasHumanPlayers {
			delete(h.games, oldGame.ID)
			log.Printf("Cleaned up orphaned game %s (no human players left)", oldGame.ID)
		}
	}

	user.InGame = false
	user.GameID = ""
}

// cleanupStaleGames removes games that are finished or have no human players
// This runs periodically to prevent memory leaks
func (h *Hub) cleanupStaleGames() {
	now := time.Now()
	cleanedCount := 0

	for gameID, game := range h.games {
		shouldClean := false
		reason := ""

		// Check if game is over and has been for a while (no cleanup timer fired)
		if game.GameOver {
			shouldClean = true
			reason = "game over"
		}

		// Check if game has no human players connected
		if !shouldClean {
			hasHumanPlayers := false
			for i := 0; i < 4; i++ {
				if game.Players[i] != nil && game.Players[i].User != nil {
					hasHumanPlayers = true
					break
				}
			}
			if !hasHumanPlayers && game.IsMultiplayer {
				shouldClean = true
				reason = "no human players"
			}
		}

		// Check for 1v1 games that are orphaned
		if !shouldClean && !game.IsMultiplayer {
			if (game.Player1 == nil || game.Player1.Client == nil) &&
				(game.Player2 == nil || game.Player2.Client == nil) {
				shouldClean = true
				reason = "1v1 orphaned"
			}
		}

		if shouldClean {
			// Cancel any timers
			if game.MoveTimer != nil {
				game.MoveTimer.Stop()
			}
			delete(h.games, gameID)
			cleanedCount++
			log.Printf("Periodic cleanup: removed game %s (reason: %s)", gameID, reason)
		}
	}

	if cleanedCount > 0 {
		log.Printf("Periodic cleanup completed: removed %d stale games, %d games remain", cleanedCount, len(h.games))
	} else {
		log.Printf("Periodic cleanup: %d games checked, none stale (time: %v)", len(h.games), now.Format("15:04:05"))
	}
}

// handleBotMoveRequest spawns a goroutine to calculate the bot's move asynchronously
// The heavy CPU work (minimax) runs in a goroutine, then sends the result back to the Hub
func (h *Hub) handleBotMoveRequest(msg *Message) {
	log.Printf("handleBotMoveRequest: received for game %s, player %d", msg.GameID, msg.Player)

	game, exists := h.games[msg.GameID]
	if !exists {
		log.Printf("handleBotMoveRequest: game %s not found", msg.GameID)
		return
	}

	if game.GameOver {
		log.Printf("handleBotMoveRequest: game %s is over", msg.GameID)
		return
	}

	// Verify it's still this player's turn
	if game.CurrentPlayer != msg.Player {
		log.Printf("handleBotMoveRequest: wrong player turn, expected %d, got %d", game.CurrentPlayer, msg.Player)
		return
	}

	log.Printf("handleBotMoveRequest: spawning goroutine for bot player %d", msg.Player)

	// Capture values needed for the goroutine
	gameID := game.ID
	botPlayer := msg.Player
	botSettings := h.getBotSettings(game, botPlayer)

	// Copy the board for safe read-only access in goroutine
	boardCopy := h.copyBoard(game.Board)

	// Copy game metadata needed for move calculation
	gameSnapshot := &Game{
		ID:            game.ID,
		Board:         boardCopy,
		Rows:          game.Rows,
		Cols:          game.Cols,
		Players:       game.Players,      // Read-only reference is safe
		PlayerBases:   game.PlayerBases,  // Copy of array
		IsMultiplayer: game.IsMultiplayer,
	}

	// Spawn goroutine to calculate move (CPU heavy, doesn't modify shared state)
	go func() {
		row, col, ok := h.calculateBotMove(gameSnapshot, botPlayer, botSettings)
		if !ok {
			log.Printf("Bot player %d has no valid moves in game %s", botPlayer, gameID)
			return
		}

		// Send result back to Hub's main loop for application
		h.handleMessage <- &MessageWrapper{
			client: nil,
			message: &Message{
				Type:   "bot_move_result",
				GameID: gameID,
				Player: botPlayer,
				Row:    &row,
				Col:    &col,
			},
		}
	}()
}

// handleBotMoveResult applies a calculated bot move to the game state
// This runs in the Hub's main loop, ensuring thread-safe state modification
func (h *Hub) handleBotMoveResult(msg *Message) {
	game, exists := h.games[msg.GameID]
	if !exists {
		log.Printf("Bot move result for non-existent game %s", msg.GameID)
		return
	}

	if game.GameOver {
		log.Printf("Bot move result for ended game %s", msg.GameID)
		return
	}

	// Verify it's still this player's turn (game state may have changed while calculating)
	if game.CurrentPlayer != msg.Player {
		log.Printf("Bot move result for wrong player: expected %d, got %d", game.CurrentPlayer, msg.Player)
		return
	}

	// Verify the move is still valid (board may have changed)
	row := *msg.Row
	col := *msg.Col
	if !h.isValidMove(game, row, col, msg.Player) {
		log.Printf("Bot calculated move [%d,%d] is no longer valid, recalculating", row, col)
		// Trigger a new calculation
		h.handleMessage <- &MessageWrapper{
			client: nil,
			message: &Message{
				Type:   "bot_move",
				GameID: msg.GameID,
				Player: msg.Player,
			},
		}
		return
	}

	// Apply the move
	h.applyBotMove(game, row, col, msg.Player)
}

// handleMoveTimeout processes a move timeout routed through the Hub channel
// This ensures timeout handling is processed in the single-threaded event loop
func (h *Hub) handleMoveTimeout(msg *Message) {
	game, exists := h.games[msg.GameID]
	if !exists {
		return
	}

	if game.GameOver {
		return
	}

	// Verify it's still this player's turn (they might have moved just in time)
	if game.CurrentPlayer != msg.Player {
		return
	}

	log.Printf("Move timeout for player %d in game %s - auto-resigning", msg.Player, msg.GameID)

	// Auto-resign the player
	if game.IsMultiplayer {
		player := game.Players[msg.Player-1]
		if player != nil && player.User != nil {
			resignMsg := &Message{GameID: game.ID}
			h.handleResign(player.User, resignMsg)
		}
	}
}

func (h *Hub) checkWinCondition(game *Game) {
	player1Count := 0
	player2Count := 0

	for i := 0; i < game.Rows; i++ {
		for j := 0; j < game.Cols; j++ {
			cell := game.Board[i][j]
			if cell == nil {
				continue
			}
			cellStr := fmt.Sprintf("%v", cell)
			if len(cellStr) > 0 && cellStr[0] == '1' {
				player1Count++
			} else if len(cellStr) > 0 && cellStr[0] == '2' {
				player2Count++
			}
		}
	}

	var winner int
	if player1Count == 0 {
		winner = 2
	} else if player2Count == 0 {
		winner = 1
	}

	if winner > 0 {
		game.GameOver = true
		game.Winner = winner

		endMsg := Message{
			Type:   "game_end",
			GameID: game.ID,
			Winner: winner,
		}
		h.sendToUser(game.Player1, &endMsg)
		h.sendToUser(game.Player2, &endMsg)

		// Mark users as not in game
		game.Player1.InGame = false
		game.Player2.InGame = false

		// Broadcast updated user list
		h.broadcastUserList()

		log.Printf("Game ended: %s (winner: player %d)", game.ID, winner)
	}
}

func (h *Hub) canMakeAnyMove(game *Game, player int) bool {
	// Check if player can make any valid move
	validMoves := 0
	for row := 0; row < game.Rows; row++ {
		for col := 0; col < game.Cols; col++ {
			if h.isValidMove(game, row, col, player) {
				validMoves++
			}
		}
	}
	log.Printf("canMakeAnyMove: Player %d has %d valid moves on a %dx%d board", player, validMoves, game.Rows, game.Cols)
	return validMoves > 0
}

func (h *Hub) isValidMove(game *Game, row, col, player int) bool {
	// Check bounds
	if row < 0 || row >= game.Rows || col < 0 || col >= game.Cols {
		return false
	}

	cellValue := game.Board[row][col]

	// Can't attack fortified or base cells
	if cellValue != nil {
		cellStr := fmt.Sprintf("%v", cellValue)
		if len(cellStr) > 0 && (strings.Contains(cellStr, "fortified") || strings.Contains(cellStr, "base")) {
			return false
		}
	}

	// Must be empty or opponent cell (not own cell)
	if cellValue != nil {
		cellStr := fmt.Sprintf("%v", cellValue)
		// If cell belongs to this player, it's not a valid target
		if len(cellStr) > 0 && cellStr[0] == byte('0'+player) {
			return false
		}
	}

	// Check if adjacent to own connected cell
	for i := -1; i <= 1; i++ {
		for j := -1; j <= 1; j++ {
			if i == 0 && j == 0 {
				continue
			}
			adjRow := row + i
			adjCol := col + j

			if adjRow >= 0 && adjRow < game.Rows && adjCol >= 0 && adjCol < game.Cols {
				adjCell := game.Board[adjRow][adjCol]
				if adjCell != nil {
					adjStr := fmt.Sprintf("%v", adjCell)
					if len(adjStr) > 0 && adjStr[0] == byte('0'+player) {
						// Check if this cell is connected to base
						if h.isConnectedToBase(game, adjRow, adjCol, player) {
							return true
						}
					}
				}
			}
		}
	}

	return false
}

func (h *Hub) isConnectedToBase(game *Game, startRow, startCol, player int) bool {
	var baseRow, baseCol int

	// Use PlayerBases array for multiplayer games
	if game.IsMultiplayer {
		baseRow = game.PlayerBases[player-1].Row
		baseCol = game.PlayerBases[player-1].Col
	} else {
		// Use Player1Base/Player2Base for 1v1 games
		if player == 1 {
			baseRow = game.Player1Base.Row
			baseCol = game.Player1Base.Col
		} else {
			baseRow = game.Player2Base.Row
			baseCol = game.Player2Base.Col
		}
	}

	visited := make(map[string]bool)
	stack := []struct{ row, col int }{{startRow, startCol}}
	visited[fmt.Sprintf("%d,%d", startRow, startCol)] = true

	for len(stack) > 0 {
		current := stack[len(stack)-1]
		stack = stack[:len(stack)-1]

		if current.row == baseRow && current.col == baseCol {
			return true
		}

		for i := -1; i <= 1; i++ {
			for j := -1; j <= 1; j++ {
				if i == 0 && j == 0 {
					continue
				}
				newRow := current.row + i
				newCol := current.col + j

				if newRow >= 0 && newRow < game.Rows && newCol >= 0 && newCol < game.Cols {
					key := fmt.Sprintf("%d,%d", newRow, newCol)
					if !visited[key] {
						cell := game.Board[newRow][newCol]
						if cell != nil {
							cellStr := fmt.Sprintf("%v", cell)
							if len(cellStr) > 0 && cellStr[0] == byte('0'+player) {
								visited[key] = true
								stack = append(stack, struct{ row, col int }{newRow, newCol})
							}
						}
					}
				}
			}
		}
	}

	return false
}

func (h *Hub) broadcastUserList() {
	users := make([]UserInfo, 0, len(h.users))
	for _, user := range h.users {
		users = append(users, UserInfo{
			UserID:   user.ID,
			Username: user.Username,
			InGame:   user.InGame,
			InLobby:  user.InLobby,
		})
	}

	msg := Message{
		Type:  "users_update",
		Users: users,
	}

	for client := range h.clients {
		h.sendToClient(client, &msg)
	}
}

func (h *Hub) sendToClient(client *Client, msg *Message) {
	// Check if client is still registered
	if _, exists := h.clients[client]; !exists {
		return // Client already disconnected
	}

	data, err := json.Marshal(msg)
	if err != nil {
		log.Printf("Error marshaling message: %v", err)
		return
	}

	// Try to send without blocking
	select {
	case client.send <- data:
		// Message sent successfully
	default:
		// Channel is full or closed, clean up
		log.Printf("Failed to send to client, removing from clients map")
		delete(h.clients, client)
	}
}

func (h *Hub) sendToUser(user *User, msg *Message) {
	if user.Client != nil {
		h.sendToClient(user.Client, msg)
	}
}

// ========== Lobby Management Functions ==========

var playerSymbols = []string{"X", "O", "△", "□"}

func (h *Hub) handleCreateLobby(user *User, msg *Message) {
	if user.InGame || user.InLobby {
		h.sendError(user, "You are already in a game or lobby")
		return
	}

	// Always create 4-slot lobbies, host decides when to start (2-4 players)
	maxPlayers := 4

	rows := msg.Rows
	cols := msg.Cols
	if rows < 5 || rows > 50 {
		rows = 10
	}
	if cols < 5 || cols > 50 {
		cols = 10
	}

	lobbyID := uuid.New().String()
	lobby := &Lobby{
		ID:         lobbyID,
		Host:       user,
		Players:    [4]*LobbyPlayer{},
		MaxPlayers: maxPlayers,
		Status:     "waiting",
		Rows:       rows,
		Cols:       cols,
		CreatedAt:  time.Now(),
	}

	// Add host as first player
	lobby.Players[0] = &LobbyPlayer{
		User:   user,
		IsBot:  false,
		Symbol: playerSymbols[0],
		Ready:  true, // Host is always ready
		Index:  0,
	}

	h.lobbies[lobbyID] = lobby
	user.InLobby = true
	user.LobbyID = lobbyID

	// Send lobby info to creator
	lobbyInfo := h.getLobbyInfo(lobby)
	responseMsg := Message{
		Type:    "lobby_created",
		LobbyID: lobbyID,
		Lobby:   lobbyInfo,
	}
	h.sendToUser(user, &responseMsg)

	// Broadcast updated user list
	h.broadcastUserList()

	// Broadcast new lobby list to all users browsing lobbies
	h.broadcastLobbiesList()

	log.Printf("Lobby created: %s by %s (max %d players, %dx%d)", lobbyID, user.Username, maxPlayers, rows, cols)
}

func (h *Hub) handleJoinLobby(user *User, msg *Message) {
	if user.InGame || user.InLobby {
		h.sendError(user, "You are already in a game or lobby")
		return
	}

	lobby, exists := h.lobbies[msg.LobbyID]
	if !exists {
		h.sendError(user, "Lobby not found")
		return
	}

	if lobby.Status != "waiting" {
		h.sendError(user, "Lobby is not accepting players")
		return
	}

	// Find empty slot
	slotIndex := -1
	for i := 0; i < lobby.MaxPlayers; i++ {
		if lobby.Players[i] == nil {
			slotIndex = i
			break
		}
	}

	if slotIndex == -1 {
		h.sendError(user, "Lobby is full")
		return
	}

	// Add player to lobby
	lobby.Players[slotIndex] = &LobbyPlayer{
		User:   user,
		IsBot:  false,
		Symbol: playerSymbols[slotIndex],
		Ready:  false,
		Index:  slotIndex,
	}

	user.InLobby = true
	user.LobbyID = lobby.ID

	// Send lobby_joined message to the joining player
	lobbyInfo := h.getLobbyInfo(lobby)
	joinedMsg := Message{
		Type:    "lobby_joined",
		LobbyID: lobby.ID,
		Lobby:   lobbyInfo,
	}
	h.sendToUser(user, &joinedMsg)

	// Broadcast lobby update to all players in lobby (including the new player)
	h.broadcastLobbyUpdate(lobby)

	// Broadcast updated user list
	h.broadcastUserList()

	// Broadcast updated lobby list to users browsing lobbies
	h.broadcastLobbiesList()

	log.Printf("User %s joined lobby %s (slot %d)", user.Username, lobby.ID, slotIndex)
}

func (h *Hub) handleLeaveLobby(user *User, msg *Message) {
	if !user.InLobby {
		return
	}

	lobby, exists := h.lobbies[user.LobbyID]
	if !exists {
		return
	}

	h.removeUserFromLobby(lobby, user)
}

func (h *Hub) handleAddBot(user *User, msg *Message) {
	if !user.InLobby {
		h.sendError(user, "You are not in a lobby")
		return
	}

	lobby, exists := h.lobbies[user.LobbyID]
	if !exists {
		return
	}

	// Only host can add bots
	if lobby.Host.ID != user.ID {
		h.sendError(user, "Only the host can add bots")
		return
	}

	// Find empty slot
	slotIndex := -1
	for i := 0; i < lobby.MaxPlayers; i++ {
		if lobby.Players[i] == nil {
			slotIndex = i
			break
		}
	}

	if slotIndex == -1 {
		h.sendError(user, "Lobby is full")
		return
	}

	// Get bot settings from message, or use defaults
	botSettings := msg.BotSettings
	if botSettings == nil {
		// Default bot settings
		botSettings = &BotSettings{
			MaterialWeight:   100.0,
			MobilityWeight:   50.0,
			PositionWeight:   30.0,
			RedundancyWeight: 40.0,
			CohesionWeight:   25.0,
			SearchDepth:      5,
		}
	}

	// Add bot to slot
	lobby.Players[slotIndex] = &LobbyPlayer{
		User:        nil,
		IsBot:       true,
		Symbol:      playerSymbols[slotIndex],
		Ready:       true,
		Index:       slotIndex,
		BotSettings: botSettings,
	}

	h.broadcastLobbyUpdate(lobby)
	h.broadcastLobbiesList()

	log.Printf("Bot added to lobby %s (slot %d)", lobby.ID, slotIndex)
}

func (h *Hub) handleRemoveBot(user *User, msg *Message) {
	if !user.InLobby {
		return
	}

	lobby, exists := h.lobbies[user.LobbyID]
	if !exists {
		return
	}

	// Only host can remove bots
	if lobby.Host.ID != user.ID {
		return
	}

	slotIndex := msg.SlotIndex
	if slotIndex < 0 || slotIndex >= 4 {
		return
	}

	player := lobby.Players[slotIndex]
	if player == nil || !player.IsBot {
		return
	}

	// Remove bot
	lobby.Players[slotIndex] = nil

	h.broadcastLobbyUpdate(lobby)
	h.broadcastLobbiesList()

	log.Printf("Bot removed from lobby %s (slot %d)", lobby.ID, slotIndex)
}

func (h *Hub) handleStartMultiplayerGame(user *User, msg *Message) {
	if !user.InLobby {
		h.sendError(user, "You are not in a lobby")
		return
	}

	lobby, exists := h.lobbies[user.LobbyID]
	if !exists {
		return
	}

	// Only host can start game
	if lobby.Host.ID != user.ID {
		h.sendError(user, "Only the host can start the game")
		return
	}

	// Count players
	playerCount := 0
	for i := 0; i < lobby.MaxPlayers; i++ {
		if lobby.Players[i] != nil {
			playerCount++
		}
	}

	if playerCount < 2 {
		h.sendError(user, "Need at least 2 players to start")
		return
	}

	// Create multiplayer game
	h.createMultiplayerGame(lobby)
}

func (h *Hub) handleGetLobbies(user *User, msg *Message) {
	lobbies := make([]LobbyInfo, 0)
	for _, lobby := range h.lobbies {
		if lobby.Status == "waiting" {
			lobbies = append(lobbies, *h.getLobbyInfo(lobby))
		}
	}

	responseMsg := Message{
		Type:    "lobbies_list",
		Lobbies: lobbies,
	}
	h.sendToUser(user, &responseMsg)
}

// Broadcast lobby list to all users who are not in a game or lobby
func (h *Hub) broadcastLobbiesList() {
	lobbies := make([]LobbyInfo, 0)
	for _, lobby := range h.lobbies {
		if lobby.Status == "waiting" {
			lobbies = append(lobbies, *h.getLobbyInfo(lobby))
		}
	}

	msg := Message{
		Type:    "lobbies_list",
		Lobbies: lobbies,
	}

	// Send to all users who are browsing lobbies (not in game, not in lobby)
	for _, user := range h.users {
		if !user.InGame && !user.InLobby {
			h.sendToUser(user, &msg)
		}
	}
}

func (h *Hub) getLobbyInfo(lobby *Lobby) *LobbyInfo {
	players := make([]LobbyPlayerInfo, lobby.MaxPlayers)
	for i := 0; i < lobby.MaxPlayers; i++ {
		if lobby.Players[i] == nil {
			players[i] = LobbyPlayerInfo{
				Symbol:  playerSymbols[i],
				IsEmpty: true,
			}
		} else {
			username := ""
			if lobby.Players[i].User != nil {
				username = lobby.Players[i].User.Username
			} else if lobby.Players[i].IsBot {
				username = fmt.Sprintf("Bot %d", i+1)
			}
			players[i] = LobbyPlayerInfo{
				Username: username,
				IsBot:    lobby.Players[i].IsBot,
				Symbol:   lobby.Players[i].Symbol,
				Ready:    lobby.Players[i].Ready,
				IsEmpty:  false,
			}
		}
	}

	return &LobbyInfo{
		LobbyID:    lobby.ID,
		HostName:   lobby.Host.Username,
		Players:    players,
		MaxPlayers: lobby.MaxPlayers,
		Status:     lobby.Status,
	}
}

func (h *Hub) broadcastLobbyUpdate(lobby *Lobby) {
	lobbyInfo := h.getLobbyInfo(lobby)
	msg := Message{
		Type:  "lobby_update",
		Lobby: lobbyInfo,
	}

	// Send to all players in lobby
	for i := 0; i < lobby.MaxPlayers; i++ {
		if lobby.Players[i] != nil && lobby.Players[i].User != nil {
			h.sendToUser(lobby.Players[i].User, &msg)
		}
	}
}

func (h *Hub) removeUserFromLobby(lobby *Lobby, user *User) {
	// Find user's slot
	slotIndex := -1
	for i := 0; i < 4; i++ {
		if lobby.Players[i] != nil && lobby.Players[i].User != nil && lobby.Players[i].User.ID == user.ID {
			slotIndex = i
			break
		}
	}

	if slotIndex == -1 {
		return
	}

	// Remove user
	lobby.Players[slotIndex] = nil
	user.InLobby = false
	user.LobbyID = ""

	// If user was host, close lobby or transfer host
	if lobby.Host.ID == user.ID {
		// Close lobby
		for i := 0; i < 4; i++ {
			if lobby.Players[i] != nil && lobby.Players[i].User != nil {
				lobby.Players[i].User.InLobby = false
				lobby.Players[i].User.LobbyID = ""
				// Notify player
				msg := Message{
					Type:     "lobby_closed",
					LobbyID:  lobby.ID,
					Username: "Host left the lobby",
				}
				h.sendToUser(lobby.Players[i].User, &msg)
			}
		}
		delete(h.lobbies, lobby.ID)
		log.Printf("Lobby %s closed (host left)", lobby.ID)
	} else {
		// Broadcast update
		h.broadcastLobbyUpdate(lobby)
		log.Printf("User %s left lobby %s", user.Username, lobby.ID)
	}

	// Broadcast updated user list
	h.broadcastUserList()

	// Broadcast updated lobby list (lobby closed or player left)
	h.broadcastLobbiesList()
}

func (h *Hub) createMultiplayerGame(lobby *Lobby) {
	gameID := uuid.New().String()
	rows := lobby.Rows
	cols := lobby.Cols

	board := make([][]interface{}, rows)
	for i := range board {
		board[i] = make([]interface{}, cols)
	}

	// Determine base positions based on number of players
	basePositions := [4]CellPos{
		{Row: 0, Col: 0},                // Player 1: top-left
		{Row: rows - 1, Col: cols - 1},  // Player 2: bottom-right
		{Row: 0, Col: cols - 1},         // Player 3: top-right
		{Row: rows - 1, Col: 0},         // Player 4: bottom-left
	}

	// Count active players and set bases
	activePlayers := 0
	gamePlayers := [4]*LobbyPlayer{}
	for i := 0; i < lobby.MaxPlayers; i++ {
		if lobby.Players[i] != nil {
			gamePlayers[i] = lobby.Players[i]
			board[basePositions[i].Row][basePositions[i].Col] = fmt.Sprintf("%d-base", i+1)
			activePlayers++
		}
	}

	game := &Game{
		ID:            gameID,
		Board:         board,
		CurrentPlayer: 1,
		MovesLeft:     3,
		GameOver:      false,
		Winner:        0,
		Rows:          rows,
		Cols:          cols,
		IsMultiplayer: true,
		Players:       gamePlayers,
		PlayerBases:   basePositions,
		NeutralsUsed:  [4]bool{false, false, false, false},
		ActivePlayers: activePlayers,
	}

	h.games[gameID] = game

	log.Printf("Multiplayer game created: %s with %d active players, starting with player %d, %d moves", gameID, activePlayers, game.CurrentPlayer, game.MovesLeft)

	// Mark users as in game and remove from lobby
	// Also cleanup any previous game state for each user
	gamePlayerInfos := make([]GamePlayerInfo, 0)
	for i := 0; i < 4; i++ {
		if gamePlayers[i] != nil {
			if gamePlayers[i].User != nil {
				// Cleanup previous game if user was in one
				h.cleanupUserFromPreviousGame(gamePlayers[i].User)
				gamePlayers[i].User.InGame = true
				gamePlayers[i].User.GameID = gameID
				gamePlayers[i].User.InLobby = false
				gamePlayers[i].User.LobbyID = ""
			}
			gamePlayerInfos = append(gamePlayerInfos, GamePlayerInfo{
				PlayerIndex: i + 1,
				Username:    h.getPlayerName(gamePlayers[i]),
				Symbol:      gamePlayers[i].Symbol,
				IsBot:       gamePlayers[i].IsBot,
				IsActive:    true,
			})
			log.Printf("  Player %d: %s (bot: %v)", i+1, h.getPlayerName(gamePlayers[i]), gamePlayers[i].IsBot)
		}
	}

	// Send game_start to all human players
	for i := 0; i < 4; i++ {
		if gamePlayers[i] != nil && gamePlayers[i].User != nil {
			startMsg := Message{
				Type:          "multiplayer_game_start",
				GameID:        gameID,
				YourPlayer:    i + 1,
				PlayerSymbol:  gamePlayers[i].Symbol,
				Rows:          rows,
				Cols:          cols,
				IsMultiplayer: true,
				GamePlayers:   gamePlayerInfos,
			}
			h.sendToUser(gamePlayers[i].User, &startMsg)
		}
	}

	// Delete lobby
	delete(h.lobbies, lobby.ID)

	// Broadcast updated user list
	h.broadcastUserList()

	// Broadcast updated lobby list (lobby no longer available)
	h.broadcastLobbiesList()

	// Start move timer for first player
	h.startMoveTimer(game)

	log.Printf("Multiplayer game started: %s with %d players", gameID, activePlayers)
}

func (h *Hub) getPlayerName(player *LobbyPlayer) string {
	if player.User != nil {
		return player.User.Username
	}
	if player.IsBot {
		return fmt.Sprintf("Bot %d", player.Index+1)
	}
	return "Unknown"
}

func (h *Hub) sendError(user *User, message string) {
	errorMsg := Message{
		Type:     "error",
		Username: message,
	}
	h.sendToUser(user, &errorMsg)
}

// ========== Multiplayer Game Logic ==========

func (h *Hub) broadcastToGame(game *Game, msg *Message) {
	if game.IsMultiplayer {
		// Send to all human players in multiplayer game
		for i := 0; i < 4; i++ {
			if game.Players[i] != nil && game.Players[i].User != nil {
				h.sendToUser(game.Players[i].User, msg)
			}
		}
	} else {
		// Send to both players in 1v1 game
		if game.Player1 != nil {
			h.sendToUser(game.Player1, msg)
		}
		if game.Player2 != nil {
			h.sendToUser(game.Player2, msg)
		}
	}
}

func (h *Hub) startMoveTimer(game *Game) {
	// Cancel existing timer if any
	if game.MoveTimer != nil {
		game.MoveTimer.Stop()
	}

	// Only start timer for multiplayer games with human players
	if !game.IsMultiplayer || game.GameOver {
		return
	}

	// Check if current player is a bot
	if game.Players[game.CurrentPlayer-1] != nil && game.Players[game.CurrentPlayer-1].IsBot {
		return // Don't set timer for bots
	}

	// Capture values for the closure (don't access game directly in timer callback)
	gameID := game.ID
	currentPlayer := game.CurrentPlayer

	// Start 120 second timer - route through Hub channel for thread safety
	game.MoveTimer = time.AfterFunc(120*time.Second, func() {
		// Send timeout message through Hub's channel instead of calling handleResign directly
		// This ensures the timeout is processed in the Hub's single-threaded event loop
		h.handleMessage <- &MessageWrapper{
			client: nil, // Internal message, no client
			message: &Message{
				Type:   "move_timeout",
				GameID: gameID,
				Player: currentPlayer,
			},
		}
	})

	log.Printf("Started 120s move timer for player %d in game %s", game.CurrentPlayer, game.ID)
}

func (h *Hub) endTurn(game *Game) {
	// Cancel move timer when turn ends
	if game.MoveTimer != nil {
		game.MoveTimer.Stop()
		game.MoveTimer = nil
	}

	if game.IsMultiplayer {
		log.Printf("endTurn: Starting turn rotation from player %d", game.CurrentPlayer)
		// Find next active player
		nextPlayer := game.CurrentPlayer
		for attempts := 0; attempts < 4; attempts++ {
			nextPlayer++
			if nextPlayer > 4 {
				nextPlayer = 1
			}

			log.Printf("endTurn: Attempt %d, checking player %d", attempts, nextPlayer)

			// Check if this player is active
			if game.Players[nextPlayer-1] != nil {
				pieceCount := h.countPlayerPieces(game, nextPlayer)
				log.Printf("endTurn: Player %d exists, has %d pieces", nextPlayer, pieceCount)
				// Check if player has any pieces left
				if pieceCount > 0 {
					game.CurrentPlayer = nextPlayer
					game.MovesLeft = 3
					log.Printf("endTurn: Selected player %d as next player", nextPlayer)
					break
				}
			} else {
				log.Printf("endTurn: Player %d slot is nil", nextPlayer)
			}
		}
		log.Printf("endTurn: Final CurrentPlayer = %d", game.CurrentPlayer)
	} else {
		// 1v1 mode - switch between players
		if game.CurrentPlayer == 1 {
			game.CurrentPlayer = 2
		} else {
			game.CurrentPlayer = 1
		}
		game.MovesLeft = 3
	}

	// Check if the new current player can make any moves
	canMove := h.canMakeAnyMove(game, game.CurrentPlayer)
	log.Printf("endTurn: Checking if player %d can make moves: %v", game.CurrentPlayer, canMove)
	if !canMove {
		// Current player has no valid moves
		log.Printf("endTurn: Player %d has no valid moves", game.CurrentPlayer)
		if game.IsMultiplayer {
			// In multiplayer, eliminate this player and check game status
			eliminatedPlayer := game.CurrentPlayer
			log.Printf("endTurn: Eliminating player %d (no valid moves)", eliminatedPlayer)

			// Remove all pieces for this player
			for i := 0; i < game.Rows; i++ {
				for j := 0; j < game.Cols; j++ {
					cell := game.Board[i][j]
					if cell != nil {
						cellStr := fmt.Sprintf("%v", cell)
						if len(cellStr) > 0 && cellStr[0] == byte('0'+eliminatedPlayer) {
							game.Board[i][j] = nil
						}
					}
				}
			}

			// Check if game should end (this will also send player_eliminated message)
			h.checkMultiplayerStatus(game)
			if game.GameOver {
				return
			}

			// Skip to next player
			h.endTurn(game)
			return
		} else {
			// In 1v1, other player wins
			game.GameOver = true
			game.Winner = 3 - game.CurrentPlayer

			endMsg := Message{
				Type:   "game_end",
				GameID: game.ID,
				Winner: game.Winner,
			}
			h.broadcastToGame(game, &endMsg)

			// Mark users as not in game
			game.Player1.InGame = false
			game.Player2.InGame = false

			h.broadcastUserList()
			log.Printf("Game ended: %s (winner: player %d, opponent had no moves)", game.ID, game.Winner)
			return
		}
	}

	// Broadcast turn change with movesLeft
	turnMsg := Message{
		Type:      "turn_change",
		GameID:    game.ID,
		Player:    game.CurrentPlayer,
		MovesLeft: game.MovesLeft,
	}
	h.broadcastToGame(game, &turnMsg)

	log.Printf("Turn changed in game %s: now player %d's turn with %d moves", game.ID, game.CurrentPlayer, game.MovesLeft)

	// Start move timer for the new current player
	h.startMoveTimer(game)

	// If current player is a bot, trigger bot move via Hub channel
	if game.IsMultiplayer && game.Players[game.CurrentPlayer-1] != nil && game.Players[game.CurrentPlayer-1].IsBot {
		log.Printf("Bot %d's turn in game %s - triggering bot move", game.CurrentPlayer, game.ID)
		// Route through Hub channel to maintain thread safety
		gameID := game.ID
		currentPlayer := game.CurrentPlayer
		h.handleMessage <- &MessageWrapper{
			client: nil,
			message: &Message{
				Type:   "bot_move",
				GameID: gameID,
				Player: currentPlayer,
			},
		}
	}
}

func (h *Hub) checkMultiplayerStatus(game *Game) {
	if !game.IsMultiplayer {
		// Use legacy win check for 1v1
		h.checkWinCondition(game)
		return
	}

	// Don't check if game is already over
	if game.GameOver {
		return
	}

	// Count active players (those with pieces)
	activePlayers := 0
	lastActivePlayer := 0
	previousActivePlayers := game.ActivePlayers

	for i := 1; i <= 4; i++ {
		if game.Players[i-1] != nil {
			pieceCount := h.countPlayerPieces(game, i)
			if pieceCount > 0 {
				activePlayers++
				lastActivePlayer = i
			}
		}
	}

	// Only send elimination messages if active count decreased
	if previousActivePlayers > activePlayers {
		// Find which player(s) were eliminated
		for i := 1; i <= 4; i++ {
			if game.Players[i-1] != nil {
				pieceCount := h.countPlayerPieces(game, i)
				if pieceCount == 0 {
					log.Printf("Player %d eliminated in game %s", i, game.ID)

					// Send player_eliminated message
					elimMsg := Message{
						Type:             "player_eliminated",
						GameID:           game.ID,
						EliminatedPlayer: i,
					}
					h.broadcastToGame(game, &elimMsg)
				}
			}
		}
	}

	// Update active players count
	game.ActivePlayers = activePlayers

	// Check for game end
	if activePlayers <= 1 {
		game.GameOver = true
		game.Winner = lastActivePlayer

		endMsg := Message{
			Type:   "game_end",
			GameID: game.ID,
			Winner: game.Winner,
		}
		h.broadcastToGame(game, &endMsg)

		// Mark all users as not in game
		for i := 0; i < 4; i++ {
			if game.Players[i] != nil && game.Players[i].User != nil {
				game.Players[i].User.InGame = false
			}
		}

		h.broadcastUserList()
		log.Printf("Multiplayer game ended: %s (winner: player %d)", game.ID, game.Winner)

		// Schedule game cleanup after a delay to allow final messages to be delivered
		gameID := game.ID
		time.AfterFunc(10*time.Second, func() {
			// Send cleanup message to hub's main goroutine
			h.handleMessage <- &MessageWrapper{
				client: nil,
				message: &Message{
					Type:   "cleanup_game",
					GameID: gameID,
				},
			}
		})
	}
}

func (h *Hub) countPlayerPieces(game *Game, player int) int {
	count := 0
	for i := 0; i < game.Rows; i++ {
		for j := 0; j < game.Cols; j++ {
			cell := game.Board[i][j]
			if cell != nil {
				cellStr := fmt.Sprintf("%v", cell)
				if len(cellStr) > 0 && cellStr[0] == byte('0'+player) {
					count++
				}
			}
		}
	}
	return count
}
