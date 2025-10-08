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

// Hub maintains the set of active clients and broadcasts messages
type Hub struct {
	clients       map[*Client]bool
	users         map[string]*User
	challenges    map[string]*Challenge
	games         map[string]*Game
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
		register:      make(chan *Client),
		unregister:    make(chan *Client),
		handleMessage: make(chan *MessageWrapper),
	}
}

func (h *Hub) run() {
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

	// Remove user from active games
	for gameID, game := range h.games {
		if game.Player1.ID == user.ID || game.Player2.ID == user.ID {
			// Notify opponent
			var opponent *User
			if game.Player1.ID == user.ID {
				opponent = game.Player2
			} else {
				opponent = game.Player1
			}

			msg := Message{
				Type:   "opponent_disconnected",
				GameID: gameID,
			}
			h.sendToUser(opponent, &msg)

			delete(h.games, gameID)
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

	challengeID := uuid.New().String()
	challenge := &Challenge{
		ID:        challengeID,
		FromUser:  from,
		ToUser:    to,
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

	log.Printf("Challenge created: %s -> %s", from.Username, to.Username)
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

	// Create game
	gameID := uuid.New().String()
	rows := 10
	cols := 10

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

	// Verify it's the user's turn
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

	// Validate and apply move (simplified - full validation would match frontend logic)
	opponent := 3 - playerNum // 1->2, 2->1
	cellValue := game.Board[row][col]

	if cellValue == nil {
		game.Board[row][col] = playerNum
	} else if cellValue == opponent {
		game.Board[row][col] = fmt.Sprintf("%d-fortified", playerNum)
	} else {
		return // Invalid move
	}

	game.MovesLeft--

	// Broadcast move to opponent
	var opponentUser *User
	if playerNum == 1 {
		opponentUser = game.Player2
	} else {
		opponentUser = game.Player1
	}

	moveMsg := Message{
		Type:   "move_made",
		GameID: msg.GameID,
		Row:    msg.Row,
		Col:    msg.Col,
		Player: playerNum,
	}
	h.sendToUser(opponentUser, &moveMsg)

	// Check if turn is over
	if game.MovesLeft == 0 {
		game.CurrentPlayer = opponent
		game.MovesLeft = 3

		turnMsg := Message{
			Type:   "turn_change",
			GameID: msg.GameID,
			Player: game.CurrentPlayer,
		}
		h.sendToUser(game.Player1, &turnMsg)
		h.sendToUser(game.Player2, &turnMsg)
	}

	// Check win condition (simplified)
	h.checkWinCondition(game)
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

func (h *Hub) broadcastUserList() {
	users := make([]UserInfo, 0, len(h.users))
	for _, user := range h.users {
		users = append(users, UserInfo{
			UserID:   user.ID,
			Username: user.Username,
			InGame:   user.InGame,
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
	data, err := json.Marshal(msg)
	if err != nil {
		log.Printf("Error marshaling message: %v", err)
		return
	}

	select {
	case client.send <- data:
	default:
		close(client.send)
		delete(h.clients, client)
	}
}

func (h *Hub) sendToUser(user *User, msg *Message) {
	if user.Client != nil {
		h.sendToClient(user.Client, msg)
	}
}
