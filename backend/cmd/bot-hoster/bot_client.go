package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"reflect"
	"strings"
	"sync"
	"time"

	"github.com/gorilla/websocket"
	"virusgame/game"
	gamesearch "virusgame/search"
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

	// Game state received authoritatively from the server.
	Position    game.State
	GamePlayers []GamePlayerInfo

	// Communication channels
	send chan outboundMessage
	done chan bool

	// Synchronization
	mu              sync.RWMutex
	positionVersion uint64
	searchVersion   uint64
	searchCancel    context.CancelFunc
	choose          func(context.Context, game.State) (gamesearch.Result, bool)
}

type outboundMessage struct {
	data       []byte
	gameID     string
	version    uint64
	gameAction bool
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

// BotSettings is retained only to decode the additive legacy wire field; production ignores every value.
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
		send:       make(chan outboundMessage, 256),
		done:       make(chan bool),
		choose:     gamesearch.Choose,
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

			b.mu.RLock()
			valid := !message.gameAction || b.State == BotInGame && b.CurrentGame == message.gameID && b.positionVersion == message.version
			if !valid {
				b.mu.RUnlock()
				continue
			}
			if err := b.WS.WriteMessage(websocket.TextMessage, message.data); err != nil {
				b.mu.RUnlock()
				log.Printf("[Bot %s] Write error: %v", b.Username, err)
				return
			}
			b.mu.RUnlock()

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
	b.cancelSearchLocked()
	b.positionVersion++
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
	b.cancelSearchLocked()
	b.positionVersion++

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
	b.startGame(msg.GameID, msg.YourPlayer, position, []GamePlayerInfo{
		{PlayerIndex: 1, Username: "Player 1", IsActive: true},
		{PlayerIndex: 2, Username: "Player 2", IsActive: true},
	})
	log.Printf("[Bot %s] 1v1 game started as player %d vs %s in game %s",
		b.Username, msg.YourPlayer, msg.OpponentUsername, msg.GameID)
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
	b.startGame(msg.GameID, msg.YourPlayer, position, msg.GamePlayers)
	log.Printf("[Bot %s] Game started as player %d in game %s", b.Username, msg.YourPlayer, msg.GameID)
}

func (b *Bot) startGame(gameID string, player int, position game.State, players []GamePlayerInfo) {
	b.mu.Lock()
	b.cancelSearchLocked()
	b.State = BotInGame
	b.CurrentGame = gameID
	b.YourPlayer = player
	b.Position = position
	b.GamePlayers = append([]GamePlayerInfo(nil), players...)
	b.positionVersion++
	b.searchVersion = 0
	b.mu.Unlock()
	b.startSearch()
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
	b.startSearch()
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
	if reflect.DeepEqual(b.Position.Snapshot(), position.Snapshot()) {
		return nil
	}
	b.cancelSearchLocked()
	b.Position = position
	b.positionVersion++
	for index := range b.GamePlayers {
		player := game.Player(b.GamePlayers[index].PlayerIndex)
		b.GamePlayers[index].IsActive = position.Active(player)
	}
	return nil
}

func (b *Bot) startSearch() {
	b.mu.Lock()
	if b.State != BotInGame || int(b.Position.CurrentPlayer()) != b.YourPlayer || b.Position.MovesLeft() == 0 || b.Position.GameOver() || b.searchVersion == b.positionVersion {
		b.mu.Unlock()
		return
	}
	b.cancelSearchLocked()
	ctx, cancel := context.WithTimeout(context.Background(), gamesearch.ProductionBudget)
	b.searchCancel = cancel
	b.searchVersion = b.positionVersion
	version := b.positionVersion
	gameID := b.CurrentGame
	position := b.Position
	choose := b.choose
	b.mu.Unlock()
	go b.calculateAndQueueAction(ctx, choose, position, gameID, version)
}

func (b *Bot) cancelSearchLocked() {
	if b.searchCancel != nil {
		b.searchCancel()
		b.searchCancel = nil
	}
}

func (b *Bot) calculateAndQueueAction(ctx context.Context, choose func(context.Context, game.State) (gamesearch.Result, bool), position game.State, gameID string, version uint64) {
	result, ok := choose(ctx, position)
	if !ok {
		return
	}
	message := actionMessage(gameID, result.Action)
	data, err := json.Marshal(message)
	if err != nil {
		log.Printf("[Bot %s] Failed to marshal action: %v", b.Username, err)
		return
	}
	b.mu.RLock()
	valid := b.State == BotInGame && b.CurrentGame == gameID && b.positionVersion == version && int(b.Position.CurrentPlayer()) == b.YourPlayer
	b.mu.RUnlock()
	if !valid {
		return
	}
	select {
	case b.send <- outboundMessage{data: data, gameID: gameID, version: version, gameAction: true}:
		log.Printf("[Bot %s] Queued action at depth %d after %d nodes", b.Username, result.Depth, result.Nodes)
	default:
		log.Printf("[Bot %s] Action queue full; waiting for the next authoritative snapshot", b.Username)
	}
}

func actionMessage(gameID string, action game.Action) *Message {
	if action.Kind == game.PlaceNeutrals {
		return &Message{Type: "neutrals", GameID: gameID, Cells: []CellPos{
			{Row: action.Neutrals[0].Row, Col: action.Neutrals[0].Col},
			{Row: action.Neutrals[1].Row, Col: action.Neutrals[1].Col},
		}}
	}
	row, col := action.Target.Row, action.Target.Col
	return &Message{Type: "move", GameID: gameID, Row: &row, Col: &col}
}

func (b *Bot) handleGameEnd(msg *Message) {
	b.mu.Lock()
	if msg.GameID != "" && msg.GameID != b.CurrentGame {
		b.mu.Unlock()
		return
	}
	b.cancelSearchLocked()
	b.positionVersion++
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
	b.cancelSearchLocked()
	b.positionVersion++
	b.State = BotIdle
	b.CurrentLobby = ""
	b.mu.Unlock()

	log.Printf("[Bot %s] Lobby closed. Returning to pool.", b.Username)
}

// JoinLobby sends a join_lobby message
func (b *Bot) JoinLobby(lobbyID string, requestID string, _ *BotSettings) {

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
	case b.send <- outboundMessage{data: data}:
	case <-time.After(time.Second):
		log.Printf("[Bot %s] Send timeout", b.Username)
	}
}
