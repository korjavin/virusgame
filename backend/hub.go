package main

import (
	"encoding/json"
	"fmt"
	"log"
	"time"

	"github.com/google/uuid"
)

// MessageWrapper wraps a message with its client
type MessageWrapper struct {
	client  *Client
	message *Message
}

// BotRequest tracks a single bot request to prevent multiple bots from joining
type BotRequest struct {
	LobbyID     string
	RequestID   string
	BotSettings *BotSettings
	Fulfilled   bool
	CreatedAt   time.Time
}

// Hub maintains the set of active clients and broadcasts messages
type Hub struct {
	clients       map[*Client]bool
	users         map[string]*User
	challenges    map[string]*Challenge
	games         map[string]*Game
	lobbies       map[string]*Lobby
	botRequests   map[string]*BotRequest // requestID -> BotRequest
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
		botRequests:   make(map[string]*BotRequest),
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
	// Internal messages (from timers - no client)
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

	board := make(Board, rows)
	for i := range board {
		board[i] = make([]CellValue, cols)
	}

	// Set base positions
	board[0][0] = NewCell(1, CellFlagBase)
	board[rows-1][cols-1] = NewCell(2, CellFlagBase)

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
		StartTime:     time.Now(),
		LastActionTime: time.Now(),
		TurnCount:     1,
		MoveHistory:   []MoveAction{},
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
	if cellValue == 0 {
		isValidTarget = true
	} else {
		// Can attack opponent's non-fortified, non-base, non-killed cells
        if cellValue.Player() != playerNum && cellValue.CanBeAttacked() {
			isValidTarget = true
		}
	}

	if !isValidTarget {
		return
	}

	// Apply move
	moveType := "place"
	if cellValue == 0 {
		game.Board[row][col] = NewCell(playerNum, CellFlagNormal)
	} else {
		// Attacking opponent cell - fortify it
		game.Board[row][col] = NewCell(playerNum, CellFlagFortified)
		moveType = "attack"
	}

	// Record move
	now := time.Now()
	duration := int(now.Sub(game.LastActionTime).Milliseconds() / 10) // centiseconds
	game.LastActionTime = now

	moveAction := MoveAction{
		Player:     playerNum,
		Type:       moveType,
		Row:        row,
		Col:        col,
		DurationCS: duration,
		TurnNumber: game.TurnCount,
	}
	game.MoveHistory = append(game.MoveHistory, moveAction)

	game.MovesLeft--

	// Cancel and restart move timer (player made a move, reset their time)
	if game.MoveTimer != nil {
		game.MoveTimer.Stop()
		game.MoveTimer = nil
	}
	h.startMoveTimer(game)

	log.Printf("Move made in game %s: player %d moved to (%d,%d), %d moves left (about to check for eliminations)", game.ID, playerNum, row, col, game.MovesLeft)

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

	// IMPORTANT: Check for eliminations after EVERY move
	// This ensures that if a player's move disconnected an opponent from their base,
	// the opponent is eliminated immediately
	h.eliminateDisconnectedPlayers(game)

	// Check if game is over after potential eliminations
	if game.GameOver {
		return
	}

	// Check if turn is over OR if current player has no more valid moves
	log.Printf("Checking if player %d can continue turn (movesLeft: %d)", playerNum, game.MovesLeft)
	hasValidMoves := h.canMakeAnyMove(game, playerNum)
	log.Printf("Player %d hasValidMoves: %v", playerNum, hasValidMoves)
	if game.MovesLeft == 0 || !hasValidMoves {
		if !hasValidMoves && game.MovesLeft > 0 {
			log.Printf("Player %d has no more valid moves (had %d moves left), eliminating player", playerNum, game.MovesLeft)

			// Eliminate this player in multiplayer games
			if game.IsMultiplayer {
				// Remove all pieces for this player
				for i := 0; i < game.Rows; i++ {
					for j := 0; j < game.Cols; j++ {
						cell := game.Board[i][j]
						if cell != 0 && cell.Player() == playerNum {
							game.Board[i][j] = 0
						}
					}
				}

				// Send player_eliminated message
				elimMsg := Message{
					Type:             "player_eliminated",
					GameID:           game.ID,
					EliminatedPlayer: playerNum,
				}
				h.broadcastToGame(game, &elimMsg)

				// Check if game should end
				h.checkMultiplayerStatus(game)
				if game.GameOver {
					return
				}
			}
		}

		log.Printf("Turn ending for game %s, calling endTurn()", game.ID)
		h.endTurn(game)
	}
}

func (h *Hub) handleNeutrals(user *User, msg *Message) {
	game, exists := h.games[msg.GameID]
	if !exists {
		return
	}

	var playerNum int

	// Determine player number based on game type
	if game.IsMultiplayer {
		// Multiplayer lobby game (3-4 players)
		found := false
		for i, player := range game.Players {
			if player != nil && player.User != nil && player.User.ID == user.ID {
				playerNum = i + 1
				found = true
				break
			}
		}
		if !found {
			return
		}
	} else {
		// 1v1 game
		if game.Player1 != nil && game.Player1.ID == user.ID {
			playerNum = 1
		} else if game.Player2 != nil && game.Player2.ID == user.ID {
			playerNum = 2
		} else {
			return
		}
	}

	if game.CurrentPlayer != playerNum || game.GameOver {
		return
	}

	// Mark cells as killed
	for _, cell := range msg.Cells {
		if game.Board[cell.Row][cell.Col].Player() == playerNum {
            // Note: Killed cell has no player (Player 0) but has FlagKilled (0x30)
            // But wait, if it's killed, it should probably belong to no one.
            // "0x30 = neutral/killed (0x30 flag + 0x00 no player)"
            // So we use NewCell(0, CellFlagKilled)
			game.Board[cell.Row][cell.Col] = NewCell(0, CellFlagKilled)
		}
	}

	// Mark neutrals as used
	if game.IsMultiplayer {
		// Multiplayer: use array (index 0-3 for players 1-4)
		if playerNum >= 1 && playerNum <= 4 {
			game.NeutralsUsed[playerNum-1] = true
		}
	} else {
		// 1v1: use individual fields
		if playerNum == 1 {
			game.Player1NeutralsUsed = true
		} else {
			game.Player2NeutralsUsed = true
		}
	}

	// Record move
	now := time.Now()
	duration := int(now.Sub(game.LastActionTime).Milliseconds() / 10) // centiseconds
	game.LastActionTime = now

	moveAction := MoveAction{
		Player:     playerNum,
		Type:       "neutral",
		Cells:      msg.Cells,
		DurationCS: duration,
		TurnNumber: game.TurnCount,
	}
	game.MoveHistory = append(game.MoveHistory, moveAction)

	// Broadcast to other players
	neutralsMsg := Message{
		Type:   "neutrals_placed",
		GameID: msg.GameID,
		Player: playerNum,
		Cells:  msg.Cells,
	}

	if game.IsMultiplayer {
		// Broadcast to all other players in the game
		for i, player := range game.Players {
			if player != nil && player.User != nil && (i+1) != playerNum {
				h.sendToUser(player.User, &neutralsMsg)
			}
		}
	} else {
		// Send to opponent in 1v1
		var opponentUser *User
		if playerNum == 1 {
			opponentUser = game.Player2
		} else {
			opponentUser = game.Player1
		}
		if opponentUser != nil {
			h.sendToUser(opponentUser, &neutralsMsg)
		}
	}

	// End turn
	if game.IsMultiplayer {
		// Multiplayer: advance to next active player
		nextPlayer := game.CurrentPlayer
		for attempts := 0; attempts < game.ActivePlayers; attempts++ {
			nextPlayer++
			if nextPlayer > 4 {
				nextPlayer = 1
			}

			// Check if this player is active
			if game.Players[nextPlayer-1] != nil {
				pieceCount := h.countPlayerPieces(game, nextPlayer)
				// Check if player has any pieces left
				if pieceCount > 0 {
					game.CurrentPlayer = nextPlayer
					game.MovesLeft = 3
					break
				}
			}
		}
	} else {
		// 1v1: toggle between 1 and 2
		game.CurrentPlayer = 3 - playerNum
		game.MovesLeft = 3
	}

	// Increment TurnCount when turn actually changes
	game.TurnCount++

	turnMsg := Message{
		Type:      "turn_change",
		GameID:    msg.GameID,
		Player:    game.CurrentPlayer,
		MovesLeft: game.MovesLeft,
	}

	// Send turn change to all players based on game type
	if game.IsMultiplayer {
		// Multiplayer lobby game: send to all players
		for _, player := range game.Players {
			if player != nil && player.User != nil {
				h.sendToUser(player.User, &turnMsg)
			}
		}
	} else {
		// 1v1 game: send to both players
		if game.Player1 != nil {
			h.sendToUser(game.Player1, &turnMsg)
		}
		if game.Player2 != nil {
			h.sendToUser(game.Player2, &turnMsg)
		}
	}
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
				if cell != 0 && cell.Player() == resignedPlayer {
					// Mark as killed (neutral)
                    game.Board[i][j] = NewCell(0, CellFlagKilled)
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

// cleanupBotRequestsForLobby removes all bot requests for a specific lobby
func (h *Hub) cleanupBotRequestsForLobby(lobbyID string) {
	cleaned := 0
	for requestID, botRequest := range h.botRequests {
		if botRequest.LobbyID == lobbyID {
			delete(h.botRequests, requestID)
			cleaned++
			log.Printf("Cleaned up bot request %s (fulfilled: %v) for lobby %s", requestID, botRequest.Fulfilled, lobbyID)
		}
	}
	log.Printf("Cleaned up %d bot request(s) for lobby %s (total remaining: %d)", cleaned, lobbyID, len(h.botRequests))
}

// cleanupStaleGames removes games that are finished or have no human players
// This runs periodically to prevent memory leaks
func (h *Hub) cleanupStaleGames() {
	now := time.Now()
	cleanedCount := 0

	// Clean up old bot requests (older than 5 minutes)
	botRequestsCleaned := 0
	for requestID, botRequest := range h.botRequests {
		age := now.Sub(botRequest.CreatedAt)
		if age > 5*time.Minute {
			delete(h.botRequests, requestID)
			botRequestsCleaned++
		}
	}
	if botRequestsCleaned > 0 {
		log.Printf("Periodic cleanup: removed %d old bot requests", botRequestsCleaned)
	}

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
			// Save aborted/abandoned games if not already saved
			if !game.GameOver && len(game.MoveHistory) > 0 {
				SaveGame(game, "abandoned")
			}

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
			if cell == 0 {
				continue
			}
            if cell.Player() == 1 {
				player1Count++
			} else if cell.Player() == 2 {
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

		SaveGame(game, "normal")

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

	// Can't attack fortified, base, or neutral (killed) cells
	if cellValue != 0 {
        if !cellValue.CanBeAttacked() {
			return false
		}
	}

	// Must be empty or opponent cell (not own cell)
	if cellValue != 0 {
        // If cell belongs to this player, it's not a valid target
		if cellValue.Player() == player {
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
				if adjCell != 0 {
                    if adjCell.Player() == player {
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
						if cell != 0 {
                            if cell.Player() == player {
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

// broadcast sends a message to all connected clients
func (h *Hub) broadcast(msg *Message) {
	data, err := json.Marshal(msg)
	if err != nil {
		log.Printf("Error marshaling broadcast message: %v", err)
		return
	}

	for client := range h.clients {
		// Try to send without blocking
		select {
		case client.send <- data:
			// Message sent successfully
		default:
			// Channel is full or closed, clean up
			log.Printf("Failed to broadcast to client, removing from clients map")
			delete(h.clients, client)
		}
	}
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

	h.broadcast(&msg)
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

	// Check if this is a response to a bot_wanted request
	isBot := false
	if msg.RequestID != "" {
		botRequest, exists := h.botRequests[msg.RequestID]
		if !exists {
			// Request not found - ignore this join attempt
			log.Printf("Bot %s tried to join with invalid requestID %s", user.Username, msg.RequestID)
			return
		}

		if botRequest.Fulfilled {
			// Request already fulfilled by another bot - ignore
			log.Printf("Bot %s tried to join but request %s already fulfilled", user.Username, msg.RequestID)
			return
		}

		if botRequest.LobbyID != msg.LobbyID {
			// Request is for a different lobby - ignore
			log.Printf("Bot %s tried to join lobby %s but request %s is for lobby %s",
				user.Username, msg.LobbyID, msg.RequestID, botRequest.LobbyID)
			return
		}

		// Mark request as fulfilled
		botRequest.Fulfilled = true
		isBot = true
		log.Printf("Bot %s fulfilled request %s for lobby %s", user.Username, msg.RequestID, msg.LobbyID)
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
		IsBot:  isBot,
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

	// NEW: Create a bot request and broadcast bot_wanted signal to all clients
	requestID := uuid.New().String()

	// Store the bot request to track it
	h.botRequests[requestID] = &BotRequest{
		LobbyID:     lobby.ID,
		RequestID:   requestID,
		BotSettings: botSettings,
		Fulfilled:   false,
		CreatedAt:   time.Now(),
	}

	log.Printf("Created bot request %s for lobby %s (total requests: %d)", requestID, lobby.ID, len(h.botRequests))

	botWantedMsg := Message{
		Type:        "bot_wanted",
		LobbyID:     lobby.ID,
		RequestID:   requestID,
		BotSettings: botSettings,
		Rows:        lobby.Rows,
		Cols:        lobby.Cols,
	}
	h.broadcast(&botWantedMsg)

	log.Printf("Broadcasted bot_wanted for lobby %s (requestId: %s) to %d clients", lobby.ID, requestID, len(h.clients))

	// Note: We don't create a LobbyPlayer here anymore!
	// The bot will join via regular join_lobby message
	// The lobby update will happen when bot actually joins
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

	// Notify bot that it was removed from lobby
	if player.User != nil {
		player.User.InLobby = false
		player.User.LobbyID = ""

		kickMsg := Message{
			Type:     "lobby_closed",
			LobbyID:  lobby.ID,
			Username: "You were removed from the lobby",
		}
		h.sendToUser(player.User, &kickMsg)
		log.Printf("Sent lobby_closed to bot %s (removed from slot %d)", player.User.Username, slotIndex)
	}

	// Remove bot from lobby
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
		// Clean up bot requests for this lobby
		h.cleanupBotRequestsForLobby(lobby.ID)
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

	board := make(Board, rows)
	for i := range board {
		board[i] = make([]CellValue, cols)
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
            // Set base cell for player i+1
			board[basePositions[i].Row][basePositions[i].Col] = NewCell(i+1, CellFlagBase)
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
		StartTime:     time.Now(),
		LastActionTime: time.Now(),
		TurnCount:     1,
		MoveHistory:   []MoveAction{},
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

	// Clean up bot requests for this lobby
	h.cleanupBotRequestsForLobby(lobby.ID)

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
		// Find next active player who can actually make moves
		nextPlayer := game.CurrentPlayer
		foundValidPlayer := false
		for attempts := 0; attempts < 4; attempts++ {
			nextPlayer++
			if nextPlayer > 4 {
				nextPlayer = 1
			}

			log.Printf("endTurn: Attempt %d, checking player %d", attempts, nextPlayer)

			// Check if this player is active and has pieces
			if game.Players[nextPlayer-1] != nil {
				pieceCount := h.countPlayerPieces(game, nextPlayer)
				log.Printf("endTurn: Player %d exists, has %d pieces", nextPlayer, pieceCount)

				if pieceCount > 0 {
					// IMPORTANT: Also check if this player can actually make valid moves
					// This prevents selecting a player who has pieces but is stuck
					game.CurrentPlayer = nextPlayer
					game.MovesLeft = 3
					canMove := h.canMakeAnyMove(game, nextPlayer)
					log.Printf("endTurn: Player %d can make moves: %v", nextPlayer, canMove)

					if canMove {
						foundValidPlayer = true
						log.Printf("endTurn: Selected player %d as next player (has pieces and can move)", nextPlayer)
						break
					} else {
						// This player has pieces but can't move - eliminate them now
						log.Printf("endTurn: Player %d has pieces but no valid moves, eliminating immediately", nextPlayer)
						for i := 0; i < game.Rows; i++ {
							for j := 0; j < game.Cols; j++ {
								cell := game.Board[i][j]
								if cell != 0 {
                                    if cell.Player() == nextPlayer {
										game.Board[i][j] = 0
									}
								}
							}
						}

						// Send player_eliminated message
						elimMsg := Message{
							Type:             "player_eliminated",
							GameID:           game.ID,
							EliminatedPlayer: nextPlayer,
						}
						h.broadcastToGame(game, &elimMsg)

						// Notify about elimination
						h.checkMultiplayerStatus(game)
						if game.GameOver {
							return
						}
					}
				}
			} else {
				log.Printf("endTurn: Player %d slot is nil", nextPlayer)
			}
		}

		if !foundValidPlayer {
			// No valid player found after checking all players - game should be over
			log.Printf("endTurn: No valid player found, checking game status")
			h.checkMultiplayerStatus(game)
			return
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

	// Increment TurnCount
	game.TurnCount++

	// For 1v1 games, check if the new current player can make any moves
	if !game.IsMultiplayer {
		canMove := h.canMakeAnyMove(game, game.CurrentPlayer)
		log.Printf("endTurn: Checking if 1v1 player %d can make moves: %v", game.CurrentPlayer, canMove)
		if !canMove {
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

			SaveGame(game, "no_moves")

			log.Printf("Game ended: %s (winner: player %d, opponent had no moves)", game.ID, game.Winner)
			return
		}
	}
	// Note: For multiplayer, the check already happened during rotation above

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

	// Note: Bots are now handled by the bot-hoster service
	// Bot players receive "your_turn" message just like human players
	// and make moves via normal WebSocket connection
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

		SaveGame(game, "normal")

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
			if cell != 0 {
                if cell.Player() == player {
					count++
				}
			}
		}
	}
	return count
}

// eliminateDisconnectedPlayers checks all players and eliminates those who have pieces
// but cannot make any valid moves (disconnected from base or completely surrounded)
// This should be called after every move to ensure immediate elimination
func (h *Hub) eliminateDisconnectedPlayers(game *Game) {
	if game.GameOver {
		return
	}

	if game.IsMultiplayer {
		// Check all players in multiplayer game
		for i := 1; i <= 4; i++ {
			if game.Players[i-1] != nil {
				pieceCount := h.countPlayerPieces(game, i)
				if pieceCount > 0 {
					// Player has pieces, check if they can make any valid moves
					canMove := h.canMakeAnyMove(game, i)
					if !canMove {
						log.Printf("Player %d has %d pieces but no valid moves - eliminating", i, pieceCount)

						// Remove all pieces for this player
						for row := 0; row < game.Rows; row++ {
							for col := 0; col < game.Cols; col++ {
								cell := game.Board[row][col]
								if cell != 0 {
                                    if cell.Player() == i {
										game.Board[row][col] = 0
									}
								}
							}
						}

						// Send player_eliminated message
						elimMsg := Message{
							Type:             "player_eliminated",
							GameID:           game.ID,
							EliminatedPlayer: i,
						}
						h.broadcastToGame(game, &elimMsg)

						// Check if game should end after elimination
						h.checkMultiplayerStatus(game)
						if game.GameOver {
							return
						}
					}
				}
			}
		}
	} else {
		// For 1v1 games, check both players
		for _, playerNum := range []int{1, 2} {
			pieceCount := h.countPlayerPieces(game, playerNum)
			if pieceCount > 0 {
				canMove := h.canMakeAnyMove(game, playerNum)
				if !canMove {
					log.Printf("1v1 Player %d has %d pieces but no valid moves - other player wins", playerNum, pieceCount)

					// In 1v1, if a player can't move, the other player wins
					game.GameOver = true
					game.Winner = 3 - playerNum // The other player

					endMsg := Message{
						Type:   "game_end",
						GameID: game.ID,
						Winner: game.Winner,
					}
					h.broadcastToGame(game, &endMsg)

					// Mark users as not in game
					if game.Player1 != nil {
						game.Player1.InGame = false
					}
					if game.Player2 != nil {
						game.Player2.InGame = false
					}

					h.broadcastUserList()

					SaveGame(game, "no_moves")

					log.Printf("Game ended: %s (winner: player %d, opponent had no valid moves)", game.ID, game.Winner)
					return
				}
			}
		}
	}
}
