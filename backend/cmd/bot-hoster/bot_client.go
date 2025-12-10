package main

import (
	"encoding/json"
	"fmt"
	"log"
	"sync"
	"time"

	"github.com/gorilla/websocket"
)

// BotState represents the current state of a bot
type BotState int

const (
	BotIdle BotState = iota
	BotInLobby
	BotInGame
	BotDisconnected
)

func (s BotState) String() string {
	switch s {
	case BotIdle:
		return "IDLE"
	case BotInLobby:
		return "IN_LOBBY"
	case BotInGame:
		return "IN_GAME"
	case BotDisconnected:
		return "DISCONNECTED"
	default:
		return "UNKNOWN"
	}
}

// Bot represents a single bot client
type Bot struct {
	ID         string
	Username   string
	UserID     string
	WS         *websocket.Conn
	State      BotState
	Manager    *BotManager
	BackendURL string

	// Current activity
	CurrentLobby string
	CurrentGame  string
	YourPlayer   int
	BotSettings  *BotSettings

	// Game state (maintained locally like a human client)
	Board       [][]interface{}
	GamePlayers []GamePlayerInfo
	PlayerBases [4]CellPos
	Rows        int
	Cols        int

	// AI
	AIEngine *AIEngine // NEW

	// Communication channels
	send chan []byte
	done chan bool

	// Synchronization
	mu sync.RWMutex
}

// Import Message and other types from parent package
type Message struct {
	Type             string           `json:"type"`
	UserID           string           `json:"userId,omitempty"`
	Username         string           `json:"username,omitempty"`
	LobbyID          string           `json:"lobbyId,omitempty"`
	RequestID        string           `json:"requestId,omitempty"`
	BotSettings      *BotSettings     `json:"botSettings,omitempty"`
	Rows             int              `json:"rows,omitempty"`
	Cols             int              `json:"cols,omitempty"`
	GameID           string           `json:"gameId,omitempty"`
	YourPlayer       int              `json:"yourPlayer,omitempty"`
	Player           int              `json:"player,omitempty"`
	Row              *int             `json:"row,omitempty"`
	Col              *int             `json:"col,omitempty"`
	MovesLeft        int              `json:"movesLeft,omitempty"`
	Winner           int              `json:"winner,omitempty"`
	Lobby            *LobbyInfo       `json:"lobby,omitempty"`
	GamePlayers      []GamePlayerInfo `json:"gamePlayers,omitempty"`
	EliminatedPlayer int              `json:"eliminatedPlayer,omitempty"`
}

type BotSettings struct {
	MaterialWeight   float64 `json:"materialWeight"`
	MobilityWeight   float64 `json:"mobilityWeight"`
	PositionWeight   float64 `json:"positionWeight"`
	RedundancyWeight float64 `json:"redundancyWeight"`
	CohesionWeight   float64 `json:"cohesionWeight"`
	SearchDepth      int     `json:"searchDepth"`
}

type LobbyInfo struct {
	LobbyID    string            `json:"lobbyId"`
	HostName   string            `json:"hostName"`
	Players    []LobbyPlayerInfo `json:"players"`
	MaxPlayers int               `json:"maxPlayers"`
}

type LobbyPlayerInfo struct {
	Username string `json:"username,omitempty"`
	IsBot    bool   `json:"isBot"`
	Symbol   string `json:"symbol"`
}

type GamePlayerInfo struct {
	PlayerIndex int    `json:"playerIndex"`
	Username    string `json:"username"`
	Symbol      string `json:"symbol"`
	IsBot       bool   `json:"isBot"`
	IsActive    bool   `json:"isActive"`
}

type CellPos struct {
	Row int `json:"row"`
	Col int `json:"col"`
}

// NewBot creates a new bot instance
func NewBot(backendURL string, manager *BotManager) *Bot {
	return &Bot{
		ID:         fmt.Sprintf("bot-%d", time.Now().UnixNano()),
		Manager:    manager,
		BackendURL: backendURL,
		State:      BotDisconnected,
		send:       make(chan []byte, 256),
		done:       make(chan bool),
	}
}

// Connect establishes WebSocket connection to backend
func (b *Bot) Connect() error {
	ws, _, err := websocket.DefaultDialer.Dial(b.BackendURL, nil)
	if err != nil {
		return fmt.Errorf("failed to connect to %s: %w", b.BackendURL, err)
	}

	b.mu.Lock()
	b.WS = ws
	b.State = BotIdle
	b.mu.Unlock()

	log.Printf("[Bot %s] Connected to %s", b.ID, b.BackendURL)
	return nil
}

// Run starts the bot's message loop
func (b *Bot) Run() {
	defer b.Disconnect()

	// Start writer goroutine
	go b.writePump()

	// Read messages from server
	for {
		select {
		case <-b.done:
			log.Printf("[Bot %s] Shutting down", b.Username)
			return
		default:
			var msg Message
			err := b.WS.ReadJSON(&msg)
			if err != nil {
				log.Printf("[Bot %s] Read error: %v", b.Username, err)

				// Attempt to reconnect
				if b.reconnect() {
					continue
				}
				return
			}

			b.handleMessage(&msg)
		}
	}
}

// writePump sends messages from the send channel to WebSocket
func (b *Bot) writePump() {
	ticker := time.NewTicker(54 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case message, ok := <-b.send:
			if !ok {
				b.WS.WriteMessage(websocket.CloseMessage, []byte{})
				return
			}

			if err := b.WS.WriteMessage(websocket.TextMessage, message); err != nil {
				log.Printf("[Bot %s] Write error: %v", b.Username, err)
				return
			}

		case <-ticker.C:
			// Send ping to keep connection alive
			if err := b.WS.WriteMessage(websocket.PingMessage, nil); err != nil {
				return
			}

		case <-b.done:
			return
		}
	}
}

// reconnect attempts to reconnect the bot
func (b *Bot) reconnect() bool {
	log.Printf("[Bot %s] Attempting to reconnect...", b.ID)

	b.mu.Lock()
	b.State = BotDisconnected
	if b.WS != nil {
		b.WS.Close()
	}
	b.mu.Unlock()

	// Wait before reconnecting
	time.Sleep(5 * time.Second)

	if err := b.Connect(); err != nil {
		log.Printf("[Bot %s] Reconnection failed: %v", b.ID, err)
		return false
	}

	log.Printf("[Bot %s] Reconnected successfully", b.ID)
	return true
}

// Disconnect closes the bot's connection
func (b *Bot) Disconnect() {
	b.mu.Lock()
	defer b.mu.Unlock()

	if b.State == BotDisconnected {
		return
	}

	close(b.done)

	if b.WS != nil {
		b.WS.Close()
	}

	b.State = BotDisconnected
	log.Printf("[Bot %s] Disconnected", b.Username)
}

// handleMessage processes messages from the server
func (b *Bot) handleMessage(msg *Message) {
	switch msg.Type {
	case "welcome":
		b.handleWelcome(msg)

	case "bot_wanted":
		b.handleBotWanted(msg)

	case "lobby_joined":
		b.handleLobbyJoined(msg)

	case "multiplayer_game_start":
		b.handleGameStart(msg)

	case "move_made":
		b.handleMoveMade(msg)

	case "turn_change":
		b.handleTurnChange(msg)

	case "game_end":
		b.handleGameEnd(msg)

	case "player_eliminated":
		b.handlePlayerEliminated(msg)

	case "lobby_closed":
		b.handleLobbyClosed(msg)

	default:
		// Ignore other message types (users_update, etc.)
	}
}

func (b *Bot) handleWelcome(msg *Message) {
	b.mu.Lock()
	b.UserID = msg.UserID
	b.Username = msg.Username
	b.State = BotIdle
	b.mu.Unlock()

	log.Printf("[Bot %s] Registered as %s (ID: %s)", b.ID, b.Username, b.UserID)
}

func (b *Bot) handleBotWanted(msg *Message) {
	b.mu.RLock()
	isIdle := b.State == BotIdle
	b.mu.RUnlock()

	if !isIdle {
		// Bot is busy, ignore signal
		return
	}

	log.Printf("[Bot %s] Received bot_wanted signal for lobby %s", b.Username, msg.LobbyID)

	// Join the lobby
	b.JoinLobby(msg.LobbyID, msg.BotSettings)
}

func (b *Bot) handleLobbyJoined(msg *Message) {
	b.mu.Lock()
	b.State = BotInLobby
	b.CurrentLobby = msg.Lobby.LobbyID
	b.mu.Unlock()

	log.Printf("[Bot %s] Joined lobby %s", b.Username, b.CurrentLobby)
}

func (b *Bot) handleGameStart(msg *Message) {
	b.mu.Lock()
	b.State = BotInGame
	b.CurrentGame = msg.GameID
	b.YourPlayer = msg.YourPlayer
	b.Rows = msg.Rows
	b.Cols = msg.Cols
	b.GamePlayers = msg.GamePlayers

	// Initialize board
	b.Board = make([][]interface{}, b.Rows)
	for i := range b.Board {
		b.Board[i] = make([]interface{}, b.Cols)
	}

	// TODO: Extract PlayerBases from message (might need backend change)
	// For now, assume standard positions
	b.PlayerBases[0] = CellPos{Row: 0, Col: 0}
	b.PlayerBases[1] = CellPos{Row: b.Rows - 1, Col: b.Cols - 1}
	b.PlayerBases[2] = CellPos{Row: 0, Col: b.Cols - 1}
	b.PlayerBases[3] = CellPos{Row: b.Rows - 1, Col: 0}

	// Place bases on board
	if len(b.Board) > b.PlayerBases[0].Row && len(b.Board[0]) > b.PlayerBases[0].Col {
		b.Board[b.PlayerBases[0].Row][b.PlayerBases[0].Col] = "1-base"
	}
	if len(b.Board) > b.PlayerBases[1].Row && len(b.Board[0]) > b.PlayerBases[1].Col {
		b.Board[b.PlayerBases[1].Row][b.PlayerBases[1].Col] = "2-base"
	}
	if len(b.GamePlayers) > 2 {
		if len(b.Board) > b.PlayerBases[2].Row && len(b.Board[0]) > b.PlayerBases[2].Col {
			b.Board[b.PlayerBases[2].Row][b.PlayerBases[2].Col] = "3-base"
		}
	}
	if len(b.GamePlayers) > 3 {
		if len(b.Board) > b.PlayerBases[3].Row && len(b.Board[0]) > b.PlayerBases[3].Col {
			b.Board[b.PlayerBases[3].Row][b.PlayerBases[3].Col] = "4-base"
		}
	}

	// NEW: Initialize AI engine with bot settings
	if b.BotSettings != nil {
		b.AIEngine = NewAIEngine(b.BotSettings)
	} else {
		// Use defaults
		b.AIEngine = NewAIEngine(&BotSettings{
			MaterialWeight:   30.0,
			MobilityWeight:   150.0,
			PositionWeight:   130.0,
			RedundancyWeight: 40.0,
			CohesionWeight:   40.0,
			SearchDepth:      3,
		})
	}

	b.mu.Unlock()

	log.Printf("[Bot %s] Game started as player %d in game %s (AI ready)",
		b.Username, b.YourPlayer, b.CurrentGame)
}

func (b *Bot) handleMoveMade(msg *Message) {
	if msg.Row == nil || msg.Col == nil {
		return
	}

	b.mu.Lock()
	b.applyMove(*msg.Row, *msg.Col, msg.Player)
	isMyTurn := msg.Player == b.YourPlayer
	movesLeft := msg.MovesLeft
	gameID := b.CurrentGame
	b.mu.Unlock()

	log.Printf("[Bot %s] Move made by player %d at (%d, %d). Moves left: %d",
		b.Username, msg.Player, *msg.Row, *msg.Col, movesLeft)

	// If it's my turn and I have moves left, calculate next move
	if isMyTurn && movesLeft > 0 {
		log.Printf("[Bot %s] Still my turn (%d moves left). Calculating next move...", b.Username, movesLeft)
		go b.calculateAndSendMove(gameID)
	}
}

func (b *Bot) handleTurnChange(msg *Message) {
	b.mu.RLock()
	isMyTurn := msg.Player == b.YourPlayer
	gameID := b.CurrentGame
	b.mu.RUnlock()

	if isMyTurn {
		log.Printf("[Bot %s] My turn! Calculating move...", b.Username)
		go b.calculateAndSendMove(gameID)
	}
}

// calculateAndSendMove runs AI to find best move and sends it
func (b *Bot) calculateAndSendMove(gameID string) {
	b.mu.RLock()

	// Create game state snapshot
	state := &GameState{
		Board:       b.copyBoardLocal(b.Board),
		Rows:        b.Rows,
		Cols:        b.Cols,
		PlayerBases: b.PlayerBases,
		Players:     b.GamePlayers,
	}
	player := b.YourPlayer
	aiEngine := b.AIEngine

	b.mu.RUnlock()

	if aiEngine == nil {
		log.Printf("[Bot %s] ERROR: AI engine not initialized!", b.Username)
		return
	}

	// Calculate move (may take 500ms - 2s)
	row, col, ok := aiEngine.CalculateMove(state, player)

	if !ok {
		log.Printf("[Bot %s] No valid moves available!", b.Username)
		// TODO: Could send resign message here
		return
	}

	// Send move
	rowPtr := row
	colPtr := col
	msg := Message{
		Type:   "move",
		GameID: gameID,
		Row:    &rowPtr,
		Col:    &colPtr,
	}

	b.sendMessage(&msg)

	log.Printf("[Bot %s] Sent move: (%d, %d)", b.Username, row, col)
}

func (b *Bot) copyBoardLocal(board [][]interface{}) [][]interface{} {
	newBoard := make([][]interface{}, len(board))
	for i := range board {
		newBoard[i] = make([]interface{}, len(board[i]))
		copy(newBoard[i], board[i])
	}
	return newBoard
}

func (b *Bot) handleGameEnd(msg *Message) {
	b.mu.Lock()
	b.State = BotIdle
	b.CurrentGame = ""
	b.CurrentLobby = ""
	b.Board = nil
	b.mu.Unlock()

	log.Printf("[Bot %s] Game ended. Winner: player %d. Returning to pool.",
		b.Username, msg.Winner)
}

func (b *Bot) handlePlayerEliminated(msg *Message) {
	b.mu.Lock()
	for i := range b.GamePlayers {
		if b.GamePlayers[i].PlayerIndex == msg.EliminatedPlayer {
			b.GamePlayers[i].IsActive = false
		}
	}
	b.mu.Unlock()

	log.Printf("[Bot %s] Player %d eliminated", b.Username, msg.EliminatedPlayer)
}

func (b *Bot) handleLobbyClosed(msg *Message) {
	b.mu.Lock()
	b.State = BotIdle
	b.CurrentLobby = ""
	b.mu.Unlock()

	log.Printf("[Bot %s] Lobby closed. Returning to pool.", b.Username)
}

// applyMove updates the local board state
func (b *Bot) applyMove(row, col, player int) {
	cell := b.Board[row][col]
	if cell == nil {
		b.Board[row][col] = player
	} else {
		b.Board[row][col] = fmt.Sprintf("%d-fortified", player)
	}
}

// JoinLobby sends a join_lobby message
func (b *Bot) JoinLobby(lobbyID string, botSettings *BotSettings) {
	b.mu.Lock()
	b.BotSettings = botSettings
	b.mu.Unlock()

	msg := Message{
		Type:    "join_lobby",
		LobbyID: lobbyID,
	}

	b.sendMessage(&msg)
	log.Printf("[Bot %s] Sent join_lobby for %s", b.Username, lobbyID)
}

// sendMessage marshals and sends a message
func (b *Bot) sendMessage(msg *Message) {
	data, err := json.Marshal(msg)
	if err != nil {
		log.Printf("[Bot %s] Failed to marshal message: %v", b.Username, err)
		return
	}

	select {
	case b.send <- data:
	case <-time.After(time.Second):
		log.Printf("[Bot %s] Send timeout", b.Username)
	}
}
