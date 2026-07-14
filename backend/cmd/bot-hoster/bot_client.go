package main

import (
	"encoding/json"
	"fmt"
	"log"
	"math/rand"
	"strings"
	"sync"
	"time"

	"github.com/gorilla/websocket"
	"virusgame/game"
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

	// Game state received authoritatively from the server.
	Position    game.State
	GamePlayers []GamePlayerInfo

	// AI
	AIEngine AIEngineInterface // AI engine (can be Minimax or MCTS)
	AIType   string            // "minimax" or "mcts" for logging

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
	Cells            []CellPos        `json:"cells,omitempty"`
	MovesLeft        int              `json:"movesLeft,omitempty"`
	Winner           int              `json:"winner,omitempty"`
	Lobby            *LobbyInfo       `json:"lobby,omitempty"`
	GamePlayers      []GamePlayerInfo `json:"gamePlayers,omitempty"`
	EliminatedPlayer int              `json:"eliminatedPlayer,omitempty"`
	// 1v1 Challenge fields
	ChallengeID      string         `json:"challengeId,omitempty"`
	FromUserID       string         `json:"fromUserId,omitempty"`
	FromUsername     string         `json:"fromUsername,omitempty"`
	OpponentID       string         `json:"opponentId,omitempty"`
	OpponentUsername string         `json:"opponentUsername,omitempty"`
	PlayerSymbol     string         `json:"playerSymbol,omitempty"`
	IsMultiplayer    bool           `json:"isMultiplayer,omitempty"`
	Snapshot         *game.Snapshot `json:"snapshot,omitempty"`
}

type BotSettings struct {
	MaterialWeight   float64 `json:"materialWeight"`
	MobilityWeight   float64 `json:"mobilityWeight"`
	PositionWeight   float64 `json:"positionWeight"`
	RedundancyWeight float64 `json:"redundancyWeight"`
	CohesionWeight   float64 `json:"cohesionWeight"`
	SearchDepth      int     `json:"searchDepth"`
}

// randomizeWeight adds ±50% randomization to a weight value
func randomizeWeight(baseWeight float64) float64 {
	// Generate random factor between 0.5 and 1.5 (±50%)
	randomFactor := 0.5 + rand.Float64()
	return baseWeight * randomFactor
}

// createRandomizedBotSettings creates bot settings with randomized weights for variety
func createRandomizedBotSettings() *BotSettings {
	settings := &BotSettings{
		MaterialWeight:   randomizeWeight(30.0),
		MobilityWeight:   randomizeWeight(150.0),
		PositionWeight:   randomizeWeight(130.0),
		RedundancyWeight: randomizeWeight(40.0),
		CohesionWeight:   randomizeWeight(40.0),
		SearchDepth:      3,
	}
	log.Printf("[AI] Randomized bot settings: Material=%.1f, Mobility=%.1f, Position=%.1f, Redundancy=%.1f, Cohesion=%.1f",
		settings.MaterialWeight, settings.MobilityWeight, settings.PositionWeight,
		settings.RedundancyWeight, settings.CohesionWeight)
	return settings
}

// createRandomAIEngine creates either a Minimax or MCTS engine randomly
func createRandomAIEngine(settings *BotSettings) (AIEngineInterface, string) {
	// 50/50 chance between minimax and MCTS
	if rand.Float64() < 0.5 {
		log.Printf("[AI] Creating Minimax engine")
		return NewAIEngine(settings), "minimax"
	}
	log.Printf("[AI] Creating MCTS engine")
	return NewMCTSEngine(settings), "mcts"
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
	url := b.BackendURL
	if strings.Contains(url, "?") {
		url += "&bot=true"
	} else {
		url += "?bot=true"
	}

	ws, _, err := websocket.DefaultDialer.Dial(url, nil)
	if err != nil {
		return fmt.Errorf("failed to connect to %s: %w", url, err)
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

	case "challenge_received":
		b.handleChallengeReceived(msg)

	case "bot_wanted":
		b.handleBotWanted(msg)

	case "lobby_joined":
		b.handleLobbyJoined(msg)

	case "game_start":
		b.handleGameStart1v1(msg)

	case "multiplayer_game_start":
		b.handleGameStart(msg)

	case "game_state":
		b.handleGameState(msg)

	case "move_made":
		b.handleMoveMade(msg)

	case "neutrals_placed":
		b.handleNeutralsPlaced(msg)

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

func (b *Bot) handleChallengeReceived(msg *Message) {
	b.mu.RLock()
	isIdle := b.State == BotIdle
	b.mu.RUnlock()

	if !isIdle {
		// Bot is busy, decline the challenge
		log.Printf("[Bot %s] Received challenge from %s but bot is busy, declining",
			b.Username, msg.FromUsername)
		b.declineChallenge(msg.ChallengeID)
		return
	}

	log.Printf("[Bot %s] Received 1v1 challenge from %s (%dx%d), accepting...",
		b.Username, msg.FromUsername, msg.Rows, msg.Cols)

	// Accept the challenge
	b.acceptChallenge(msg.ChallengeID)
}

func (b *Bot) acceptChallenge(challengeID string) {
	msg := Message{
		Type:        "accept_challenge",
		ChallengeID: challengeID,
	}
	b.sendMessage(&msg)
	log.Printf("[Bot %s] Sent accept_challenge for %s", b.Username, challengeID)
}

func (b *Bot) declineChallenge(challengeID string) {
	msg := Message{
		Type:        "decline_challenge",
		ChallengeID: challengeID,
	}
	b.sendMessage(&msg)
	log.Printf("[Bot %s] Sent decline_challenge for %s", b.Username, challengeID)
}

func (b *Bot) handleGameStart1v1(msg *Message) {
	position, err := decodeSnapshot(msg)
	if err != nil {
		log.Printf("[Bot %s] Rejected game start snapshot: %v", b.Username, err)
		return
	}

	b.mu.Lock()
	b.State = BotInGame
	b.CurrentGame = msg.GameID
	b.YourPlayer = msg.YourPlayer
	b.Position = position
	b.GamePlayers = []GamePlayerInfo{
		{PlayerIndex: 1, Username: "Player 1", IsBot: false, IsActive: true},
		{PlayerIndex: 2, Username: "Player 2", IsBot: false, IsActive: true},
	}
	settings := createRandomizedBotSettings()
	b.AIEngine, b.AIType = createRandomAIEngine(settings)
	isMyTurn := int(position.CurrentPlayer()) == b.YourPlayer
	gameID := b.CurrentGame
	b.mu.Unlock()

	log.Printf("[Bot %s] 1v1 game started as player %d vs %s in game %s (using %s)",
		b.Username, b.YourPlayer, msg.OpponentUsername, b.CurrentGame, b.AIType)
	if isMyTurn {
		go b.calculateAndSendMove(gameID)
	}
}

func (b *Bot) handleBotWanted(msg *Message) {
	b.mu.RLock()
	isIdle := b.State == BotIdle
	b.mu.RUnlock()

	if !isIdle {
		// Bot is busy, ignore signal
		return
	}

	log.Printf("[Bot %s] Received bot_wanted signal for lobby %s (requestID: %s)",
		b.Username, msg.LobbyID, msg.RequestID)

	// Join the lobby with the requestID
	b.JoinLobby(msg.LobbyID, msg.RequestID, msg.BotSettings)
}

func (b *Bot) handleLobbyJoined(msg *Message) {
	b.mu.Lock()
	b.State = BotInLobby
	b.CurrentLobby = msg.Lobby.LobbyID
	b.mu.Unlock()

	log.Printf("[Bot %s] Joined lobby %s", b.Username, b.CurrentLobby)
}

func (b *Bot) handleGameStart(msg *Message) {
	position, err := decodeSnapshot(msg)
	if err != nil {
		log.Printf("[Bot %s] Rejected game start snapshot: %v", b.Username, err)
		return
	}

	b.mu.Lock()
	b.State = BotInGame
	b.CurrentGame = msg.GameID
	b.YourPlayer = msg.YourPlayer
	b.Position = position
	b.GamePlayers = msg.GamePlayers

	var settings *BotSettings
	if b.BotSettings != nil {
		settings = b.BotSettings
	} else {
		settings = createRandomizedBotSettings()
	}
	b.AIEngine, b.AIType = createRandomAIEngine(settings)
	isMyTurn := int(position.CurrentPlayer()) == b.YourPlayer
	gameID := b.CurrentGame
	b.mu.Unlock()

	log.Printf("[Bot %s] Game started as player %d in game %s (using %s)",
		b.Username, b.YourPlayer, b.CurrentGame, b.AIType)
	if isMyTurn {
		go b.calculateAndSendMove(gameID)
	}
}

func (b *Bot) handleMoveMade(msg *Message) {
	if msg.Row == nil || msg.Col == nil {
		return
	}
	if err := b.updatePosition(msg); err != nil {
		log.Printf("[Bot %s] Rejected move snapshot: %v", b.Username, err)
		return
	}
	log.Printf("[Bot %s] Move made by player %d at (%d, %d). Moves left: %d",
		b.Username, msg.Player, *msg.Row, *msg.Col, msg.MovesLeft)
}

func (b *Bot) handleNeutralsPlaced(msg *Message) {
	if err := b.updatePosition(msg); err != nil {
		log.Printf("[Bot %s] Rejected neutral snapshot: %v", b.Username, err)
		return
	}
	log.Printf("[Bot %s] Neutrals placed by player %d (%d cells)", b.Username, msg.Player, len(msg.Cells))
}

func (b *Bot) handleTurnChange(msg *Message) {
	b.handleGameState(msg)
}

func (b *Bot) handleGameState(msg *Message) {
	if err := b.updatePosition(msg); err != nil {
		log.Printf("[Bot %s] Rejected game snapshot: %v", b.Username, err)
		return
	}
	b.mu.RLock()
	isMyTurn := int(b.Position.CurrentPlayer()) == b.YourPlayer && b.Position.MovesLeft() > 0
	gameID := b.CurrentGame
	b.mu.RUnlock()
	if isMyTurn {
		log.Printf("[Bot %s] My turn! Calculating move...", b.Username)
		go b.calculateAndSendMove(gameID)
	}
}

func decodeSnapshot(msg *Message) (game.State, error) {
	if msg.Snapshot == nil {
		return game.State{}, fmt.Errorf("missing snapshot")
	}
	position, err := game.FromSnapshot(*msg.Snapshot)
	if err != nil {
		return game.State{}, err
	}
	if msg.Type == "game_start" || msg.Type == "multiplayer_game_start" {
		players := len(msg.Snapshot.Bases)
		if msg.YourPlayer < 1 || msg.YourPlayer > players {
			return game.State{}, fmt.Errorf("yourPlayer must be 1-based and within 1..%d", players)
		}
		if msg.Type == "multiplayer_game_start" && len(msg.GamePlayers) != players {
			return game.State{}, fmt.Errorf("got %d player records for %d-player snapshot", len(msg.GamePlayers), players)
		}
		seen := make([]bool, players)
		for _, player := range msg.GamePlayers {
			if player.PlayerIndex < 1 || player.PlayerIndex > players || seen[player.PlayerIndex-1] {
				return game.State{}, fmt.Errorf("playerIndex values must be unique and 1-based")
			}
			seen[player.PlayerIndex-1] = true
		}
	}
	return position, nil
}

func (b *Bot) updatePosition(msg *Message) error {
	position, err := decodeSnapshot(msg)
	if err != nil {
		return err
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.CurrentGame == "" || msg.GameID != b.CurrentGame {
		return fmt.Errorf("snapshot game %q does not match current game %q", msg.GameID, b.CurrentGame)
	}
	b.Position = position
	for index := range b.GamePlayers {
		player := game.Player(b.GamePlayers[index].PlayerIndex)
		b.GamePlayers[index].IsActive = position.Active(player)
	}
	return nil
}

// calculateAndSendMove runs AI to find best move and sends it
func (b *Bot) calculateAndSendMove(gameID string) {
	b.mu.RLock()
	state := legacyGameState(b.Position, b.GamePlayers, b.YourPlayer)
	player := b.YourPlayer
	aiEngine := b.AIEngine
	b.mu.RUnlock()

	if aiEngine == nil {
		log.Printf("[Bot %s] ERROR: AI engine not initialized!", b.Username)
		return
	}

	// Calculate move (may take 500ms - 2s)
	move, ok := aiEngine.CalculateMove(state, player)

	if !ok || move == nil {
		log.Printf("[Bot %s] No valid moves available! Waiting for server to handle elimination.", b.Username)
		// Don't send anything - server will detect no valid moves and eliminate this player
		return
	}

	// Send move based on type
	if move.Type == MoveTypeNeutral {
		msg := Message{
			Type:   "neutrals",
			GameID: gameID,
			Cells:  move.Cells,
		}
		b.sendMessage(&msg)

		log.Printf("[Bot %s] Sent NEUTRAL move with %d cells", b.Username, len(move.Cells))
	} else {
		// Standard Move
		rowPtr := move.Row
		colPtr := move.Col
		msg := Message{
			Type:   "move",
			GameID: gameID,
			Row:    &rowPtr,
			Col:    &colPtr,
		}
		b.sendMessage(&msg)
		log.Printf("[Bot %s] Sent move: (%d, %d)", b.Username, move.Row, move.Col)
	}
}

func (b *Bot) handleGameEnd(msg *Message) {
	b.mu.Lock()
	b.State = BotIdle
	b.CurrentGame = ""
	b.CurrentLobby = ""
	b.mu.Unlock()

	log.Printf("[Bot %s] Game ended. Winner: player %d. Returning to pool.",
		b.Username, msg.Winner)
}

func (b *Bot) handlePlayerEliminated(msg *Message) {
	if err := b.updatePosition(msg); err != nil {
		log.Printf("[Bot %s] Rejected elimination snapshot: %v", b.Username, err)
		return
	}
	log.Printf("[Bot %s] Player %d eliminated", b.Username, msg.EliminatedPlayer)
}

func (b *Bot) handleLobbyClosed(msg *Message) {
	b.mu.Lock()
	b.State = BotIdle
	b.CurrentLobby = ""
	b.mu.Unlock()

	log.Printf("[Bot %s] Lobby closed. Returning to pool.", b.Username)
}

// JoinLobby sends a join_lobby message
func (b *Bot) JoinLobby(lobbyID string, requestID string, botSettings *BotSettings) {
	b.mu.Lock()
	b.BotSettings = botSettings
	b.mu.Unlock()

	msg := Message{
		Type:      "join_lobby",
		LobbyID:   lobbyID,
		RequestID: requestID,
	}

	b.sendMessage(&msg)
	log.Printf("[Bot %s] Sent join_lobby for %s (requestID: %s)", b.Username, lobbyID, requestID)
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
