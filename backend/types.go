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
	Users        []UserInfo  `json:"users,omitempty"`
	Cells        []CellPos   `json:"cells,omitempty"`
}

type UserInfo struct {
	UserID   string `json:"userId"`
	Username string `json:"username"`
	InGame   bool   `json:"inGame"`
}

type CellPos struct {
	Row int `json:"row"`
	Col int `json:"col"`
}

// User represents a connected client
type User struct {
	ID       string
	Username string
	Client   *Client
	InGame   bool
}

// Challenge represents a game challenge between two users
type Challenge struct {
	ID        string
	FromUser  *User
	ToUser    *User
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
}
