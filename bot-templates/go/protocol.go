package main

type Message struct {
	Type        string           `json:"type"`
	UserID      string           `json:"userId,omitempty"`
	Username    string           `json:"username,omitempty"`
	LobbyID     string           `json:"lobbyId,omitempty"`
	GameID      string           `json:"gameId,omitempty"`
	YourPlayer  int              `json:"yourPlayer,omitempty"`
	Player      int              `json:"player,omitempty"`
	Row         *int             `json:"row,omitempty"`
	Col         *int             `json:"col,omitempty"`
	Rows        int              `json:"rows,omitempty"`
	Cols        int              `json:"cols,omitempty"`
	MovesLeft   int              `json:"movesLeft,omitempty"`
	Winner      int              `json:"winner,omitempty"`
	GamePlayers []GamePlayerInfo `json:"gamePlayers,omitempty"`
	BotSettings *BotSettings     `json:"botSettings,omitempty"`
	Lobby       *LobbyInfo       `json:"lobby,omitempty"`

	// Diagnostics
	Score            *float64          `json:"score,omitempty"`
	Depth            *int              `json:"depth,omitempty"`
	NodesEvaluated   *int              `json:"nodesEvaluated,omitempty"`
	TimeMs           *int64            `json:"timeMs,omitempty"`
	AlternativeMoves []AlternativeMove `json:"alternativeMoves,omitempty"`
}

type AlternativeMove struct {
	Row   int     `json:"row"`
	Col   int     `json:"col"`
	Score float64 `json:"score"`
}

type GamePlayerInfo struct {
	PlayerIndex int    `json:"playerIndex"`
	Username    string `json:"username"`
	Symbol      string `json:"symbol"`
	IsBot       bool   `json:"isBot"`
	IsActive    bool   `json:"isActive"`
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
	LobbyID    string `json:"lobbyId"`
	HostName   string `json:"hostName"`
	MaxPlayers int    `json:"maxPlayers"`
}
