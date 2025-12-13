package main

import (
    "encoding/json"
    "log"

    "github.com/gorilla/websocket"
)

type Bot struct {
    WS           *websocket.Conn
    BackendURL   string
    UserID       string
    Username     string
    CurrentGame  string
    YourPlayer   int
    Board        [][]CellValue // Changed to [][]CellValue
}

func NewBot(backendURL string) *Bot {
    return &Bot{BackendURL: backendURL}
}

func (b *Bot) Connect() error {
    ws, _, err := websocket.DefaultDialer.Dial(b.BackendURL, nil)
    if err != nil {
        return err
    }
    b.WS = ws
    log.Printf("Bot connected to %s", b.BackendURL)
    return nil
}

func (b *Bot) Run() {
    defer b.WS.Close()

    for {
        var msg Message
        if err := b.WS.ReadJSON(&msg); err != nil {
            log.Printf("Error: %v", err)
            return
        }

        b.handleMessage(&msg)
    }
}

func (b *Bot) handleMessage(msg *Message) {
    switch msg.Type {
    case "welcome":
        b.UserID = msg.UserID
        b.Username = msg.Username
        log.Printf("Bot registered as %s", b.Username)

    case "bot_wanted":
        log.Printf("Bot requested for lobby %s", msg.LobbyID)
        b.joinLobby(msg.LobbyID)

    case "lobby_joined":
        log.Printf("Joined lobby")

    case "multiplayer_game_start":
        b.CurrentGame = msg.GameID
        b.YourPlayer = msg.YourPlayer
        b.Board = make([][]CellValue, msg.Rows)
        for i := range b.Board {
            b.Board[i] = make([]CellValue, msg.Cols)
        }
        log.Printf("Game started, I am player %d", b.YourPlayer)

    case "turn_change":
        if msg.Player == b.YourPlayer {
            b.makeMove()
        }

    case "move_made":
        if msg.Row != nil && msg.Col != nil {
            b.applyMove(*msg.Row, *msg.Col, msg.Player)
        }

    case "game_end":
        log.Printf("Game ended, winner: player %d", msg.Winner)
        b.CurrentGame = ""
    }
}

func (b *Bot) joinLobby(lobbyID string) {
    msg := Message{
        Type:    "join_lobby",
        LobbyID: lobbyID,
    }
    b.sendMessage(&msg)
}

func (b *Bot) makeMove() {
    // Simple AI: find first valid move
    row, col := b.findValidMove()

    msg := Message{
        Type:   "move",
        GameID: b.CurrentGame,
        Row:    &row,
        Col:    &col,
    }

    b.sendMessage(&msg)
    log.Printf("Sent move: (%d, %d)", row, col)
}

func (b *Bot) applyMove(row, col, player int) {
    if row < 0 || row >= len(b.Board) || col < 0 || col >= len(b.Board[0]) {
        return
    }
    cell := b.Board[row][col]
    if cell == 0 {
        b.Board[row][col] = NewCell(player, CellFlagNormal)
    } else {
        b.Board[row][col] = NewCell(player, CellFlagFortified)
    }
}

func (b *Bot) sendMessage(msg *Message) {
    data, _ := json.Marshal(msg)
    b.WS.WriteMessage(websocket.TextMessage, data)
}
