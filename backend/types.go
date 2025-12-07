package main

import (
	"time"
)

// Message types sent between client and server
type Message struct {
	Type         string      `json:"type"`
	UserID       string      `json:"userId,omitempty"`
	Username     string      `json:"username,omitempty"`
	TargetUserID string      `json:"targetUserId,omitempty"`
	ChallengeID  string      `json:"challengeId,omitempty"`
	GameID       string      `json:"gameId,omitempty"`
	FromUserID   string      `json:"fromUserId,omitempty"`
	FromUsername string      `json:"fromUsername,omitempty"`
	OpponentID   string      `json:"opponentId,omitempty"`
	OpponentUsername string  `json:"opponentUsername,omitempty"`
	YourPlayer   int         `json:"yourPlayer,omitempty"`
	Rows         int         `json:"rows,omitempty"`
	Cols         int         `json:"cols,omitempty"`
	Row          *int        `json:"row,omitempty"`
	Col          *int        `json:"col,omitempty"`
	Player       int         `json:"player,omitempty"`
	Winner       int         `json:"winner,omitempty"`
	MovesLeft    int         `json:"movesLeft,omitempty"`
	Users        []UserInfo  `json:"users,omitempty"`
	Cells        []CellPos   `json:"cells,omitempty"`
	// Lobby fields
	LobbyID      string      `json:"lobbyId,omitempty"`
	MaxPlayers   int         `json:"maxPlayers,omitempty"`
	SlotIndex    int         `json:"slotIndex,omitempty"`
	Lobby        *LobbyInfo  `json:"lobby,omitempty"`
	Lobbies      []LobbyInfo `json:"lobbies,omitempty"`
	// Multiplayer game fields
	IsMultiplayer bool             `json:"isMultiplayer,omitempty"`
	PlayerSymbol  string           `json:"playerSymbol,omitempty"`
	GamePlayers   []GamePlayerInfo `json:"gamePlayers,omitempty"`
	EliminatedPlayer int           `json:"eliminatedPlayer,omitempty"`
	// Bot settings
	BotSettings   *BotSettings     `json:"botSettings,omitempty"`
}

type UserInfo struct {
	UserID   string `json:"userId"`
	Username string `json:"username"`
	InGame   bool   `json:"inGame"`
	InLobby  bool   `json:"inLobby"`
}

type CellPos struct {
	Row int `json:"row"`
	Col int `json:"col"`
}

type LobbyInfo struct {
	LobbyID    string             `json:"lobbyId"`
	HostName   string             `json:"hostName"`
	Players    []LobbyPlayerInfo  `json:"players"`
	MaxPlayers int                `json:"maxPlayers"`
	Status     string             `json:"status"`
}

type LobbyPlayerInfo struct {
	Username string `json:"username,omitempty"`
	IsBot    bool   `json:"isBot"`
	Symbol   string `json:"symbol"`
	Ready    bool   `json:"ready"`
	IsEmpty  bool   `json:"isEmpty"`
}

type GamePlayerInfo struct {
	PlayerIndex int    `json:"playerIndex"`
	Username    string `json:"username"`
	Symbol      string `json:"symbol"`
	IsBot       bool   `json:"isBot"`
	IsActive    bool   `json:"isActive"` // false if eliminated
}

// User represents a connected client
type User struct {
	ID       string
	Username string
	Client   *Client
	InGame   bool
	GameID   string // ID of game user is in
	InLobby  bool
	LobbyID  string // ID of lobby user is in
}

// Challenge represents a game challenge between two users
type Challenge struct {
	ID        string
	FromUser  *User
	ToUser    *User
	Rows      int
	Cols      int
	Timestamp time.Time
}

// Game represents an active game session
type Game struct {
	ID            string
	Player1       *User
	Player2       *User
	Board         [][]interface{}
	CurrentPlayer int
	MovesLeft     int
	Player1Base   CellPos
	Player2Base   CellPos
	GameOver      bool
	Winner        int
	Player1NeutralsUsed bool
	Player2NeutralsUsed bool
	Rows          int
	Cols          int
	// Multiplayer mode fields
	IsMultiplayer bool
	Players       [4]*LobbyPlayer  // For 3-4 player games
	PlayerBases   [4]CellPos       // Bases for each player
	NeutralsUsed  [4]bool          // Track neutrals usage
	ActivePlayers int              // Number of active players
	MoveTimer     *time.Timer      // Timer for auto-resign after 120 seconds
}

// Lobby represents a multiplayer game lobby
type Lobby struct {
	ID         string
	Host       *User
	Players    [4]*LobbyPlayer
	MaxPlayers int    // 3 or 4
	Status     string // "waiting", "ready", "starting"
	Rows       int
	Cols       int
	CreatedAt  time.Time
}

// BotSettings contains AI configuration for bots
type BotSettings struct {
	MaterialWeight   float64 `json:"materialWeight"`
	MobilityWeight   float64 `json:"mobilityWeight"`
	PositionWeight   float64 `json:"positionWeight"`
	RedundancyWeight float64 `json:"redundancyWeight"`
	CohesionWeight   float64 `json:"cohesionWeight"`
	SearchDepth      int     `json:"searchDepth"`
}

// LobbyPlayer represents a player slot in a lobby
type LobbyPlayer struct {
	User        *User        // nil if slot is empty
	IsBot       bool         // true if AI bot
	Symbol      string       // "X", "O", "△", "□"
	Ready       bool         // ready status
	Index       int          // 0-3, player index
	BotSettings *BotSettings // AI settings for bots (nil for human players)
}
