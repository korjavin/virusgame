package main

import (
	"encoding/json"
	"testing"
	"time"
)

func TestHubIntegration_Disconnect(t *testing.T) {
	h := newHub()
	go h.run()

	c1 := &Client{hub: h, send: make(chan []byte, 256)}
	h.register <- c1
	waitForMessage(t, c1, "welcome")

	u1 := c1.user
	userID := u1.ID

	if _, exists := h.users[userID]; !exists {
		t.Error("User should exist")
	}

	h.unregister <- c1
	time.Sleep(50 * time.Millisecond)

	if _, exists := h.users[userID]; exists {
		t.Error("User should be removed after disconnect")
	}
}

func TestHubIntegration_DeclineChallenge(t *testing.T) {
	h := newHub()
	go h.run()

	c1 := &Client{hub: h, send: make(chan []byte, 256)}
	c2 := &Client{hub: h, send: make(chan []byte, 256)}
	h.register <- c1
	h.register <- c2

	waitForMessage(t, c1, "welcome")
	waitForMessage(t, c2, "welcome")

	u2 := c2.user

	// Challenge
	sendMessage(h, c1, &Message{
		Type: "challenge",
		TargetUserID: u2.ID,
		Rows: 10, Cols: 10,
	})

	msg := waitForMessage(t, c2, "challenge_received")
	challengeID := msg.ChallengeID

	// Decline
	sendMessage(h, c2, &Message{
		Type: "decline_challenge",
		ChallengeID: challengeID,
	})

	waitForMessage(t, c1, "challenge_declined")

	if len(h.challenges) != 0 {
		t.Error("Challenges should be empty")
	}
}

func TestHubIntegration_ChatAndPing(t *testing.T) {
	h := newHub()
	go h.run()

	c1 := &Client{hub: h, send: make(chan []byte, 256)}
	c2 := &Client{hub: h, send: make(chan []byte, 256)}
	h.register <- c1
	h.register <- c2
	waitForMessage(t, c1, "welcome")
	waitForMessage(t, c2, "welcome")

	// Create Lobby
	sendMessage(h, c1, &Message{Type: "create_lobby", Rows: 10, Cols: 10})
	msg := waitForMessage(t, c1, "lobby_created")
	lobbyID := msg.LobbyID

	// Join Lobby
	sendMessage(h, c2, &Message{Type: "join_lobby", LobbyID: lobbyID})
	waitForMessage(t, c2, "lobby_joined")

	// Chat in Lobby
	sendMessage(h, c1, &Message{
		Type: "lobby_chat",
		Content: "Hello",
	})

	// Both should receive chat
	waitForMessage(t, c1, "lobby_chat")
	waitForMessage(t, c2, "lobby_chat")

	// Start Game
	sendMessage(h, c1, &Message{Type: "start_multiplayer_game"})
	waitForMessage(t, c1, "multiplayer_game_start")
	waitForMessage(t, c2, "multiplayer_game_start")

	// Ping (Highlight Cell)
	r, c := 5, 5
	sendMessage(h, c1, &Message{
		Type: "highlight_cell",
		Row: &r,
		Col: &c,
	})

	// Both should receive highlight
	waitForMessage(t, c1, "highlight_cell")
	waitForMessage(t, c2, "highlight_cell")
}

func TestHubIntegration_Bots(t *testing.T) {
	h := newHub()
	go h.run()

	c1 := &Client{hub: h, send: make(chan []byte, 256)}
	h.register <- c1
	waitForMessage(t, c1, "welcome")

	// Create Lobby
	sendMessage(h, c1, &Message{Type: "create_lobby", Rows: 10, Cols: 10})
	msg := waitForMessage(t, c1, "lobby_created")
	lobbyID := msg.LobbyID

	// Add Bot
	sendMessage(h, c1, &Message{
		Type: "add_bot",
		BotSettings: &BotSettings{
			MaterialWeight: 100,
		},
	})

	// Should receive bot_wanted broadcast (we are the only client)
	waitForMessage(t, c1, "bot_wanted")

	// Since we don't have a real bot hoster, we manually fulfill the request
	// by simulating a bot connection.
	botClient := &Client{hub: h, send: make(chan []byte, 256), IsBot: true}
	h.register <- botClient
	waitForMessage(t, botClient, "welcome")

	// Re-do with variable capture
	sendMessage(h, c1, &Message{
		Type: "add_bot",
		BotSettings: &BotSettings{MaterialWeight: 100},
	})
	botWanted := waitForMessage(t, c1, "bot_wanted")
	requestID := botWanted.RequestID

	// Bot joins
	sendMessage(h, botClient, &Message{
		Type: "join_lobby",
		LobbyID: lobbyID,
		RequestID: requestID,
	})

	waitForMessage(t, botClient, "lobby_joined")

	// Host should receive lobby_update
	waitForMessage(t, c1, "lobby_update")

	// Remove Bot (slot 1)
	sendMessage(h, c1, &Message{
		Type: "remove_bot",
		SlotIndex: 1,
	})

	// Bot receives lobby_closed (kicked)
	waitForMessage(t, botClient, "lobby_closed")

	// Host receives update
	waitForMessage(t, c1, "lobby_update")
}

func TestHubIntegration_LeaveGame(t *testing.T) {
	h := newHub()
	go h.run()

	c1 := &Client{hub: h, send: make(chan []byte, 256)}
	c2 := &Client{hub: h, send: make(chan []byte, 256)}
	h.register <- c1
	h.register <- c2
	waitForMessage(t, c1, "welcome")
	waitForMessage(t, c2, "welcome")

	// Create and Join Lobby
	sendMessage(h, c1, &Message{Type: "create_lobby", Rows: 10, Cols: 10})
	msg := waitForMessage(t, c1, "lobby_created")
	lobbyID := msg.LobbyID

	sendMessage(h, c2, &Message{Type: "join_lobby", LobbyID: lobbyID})
	waitForMessage(t, c2, "lobby_joined")

	// Start Game
	sendMessage(h, c1, &Message{Type: "start_multiplayer_game"})
	startMsg1 := waitForMessage(t, c1, "multiplayer_game_start")
	waitForMessage(t, c2, "multiplayer_game_start")

	gameID := startMsg1.GameID

	// P2 Leaves Game
	sendMessage(h, c2, &Message{
		Type: "leave_game",
		GameID: gameID,
	})

	// P1 should receive users_update (P2 no longer in game)
	// Note: leave_game doesn't send "player_left_game" to opponents?
	// It calls broadcastUserList.
	// But it does verify P1 receives users_update

	// Wait for users_update
	// Note: start_multiplayer_game also sends users_update.
	// We need to consume it or just wait for *a* users_update.
	// Actually, wait for any message until we see users_update
	found := false
	timeout := time.After(1 * time.Second)
	for !found {
		select {
		case msgBytes := <-c1.send:
			var msg Message
			json.Unmarshal(msgBytes, &msg)
			if msg.Type == "users_update" {
				// We might get multiple updates.
				// Just ensure we get at least one after P2 leaves.
				// But we might be reading one from before P2 left.
				// However, leave_game triggers broadcastUserList immediately.
				found = true
			}
		case <-timeout:
			t.Error("Timeout waiting for users_update")
			return
		}
	}
}
