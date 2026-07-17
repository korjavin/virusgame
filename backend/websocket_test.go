package main

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gorilla/websocket"
)

func TestWebSocketIntegration(t *testing.T) {
	// Setup Hub and Server
	h := newHub()
	go h.run()

	// Setup an isolated router that replicates main.go routing without
	// polluting http.DefaultServeMux across repeated test runs.
	mux := http.NewServeMux()
	mux.HandleFunc("/ws", func(w http.ResponseWriter, r *http.Request) {
		serveWs(h, w, r)
	})

	server := httptest.NewServer(mux)
	defer server.Close()

	// Convert http URL to ws URL
	wsURL := "ws" + strings.TrimPrefix(server.URL, "http") + "/ws"

	// Connect Client 1
	ws1, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err != nil {
		t.Fatalf("Failed to connect C1: %v", err)
	}
	defer ws1.Close()

	// Read Welcome
	var msg1 Message
	if err := ws1.ReadJSON(&msg1); err != nil {
		t.Fatalf("C1 failed to read welcome: %v", err)
	}
	if msg1.Type != "welcome" {
		t.Errorf("C1 expected welcome, got %s", msg1.Type)
	}

	// Connect Client 2
	ws2, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err != nil {
		t.Fatalf("Failed to connect C2: %v", err)
	}
	defer ws2.Close()

	// Read Welcome
	var msg2 Message
	if err := ws2.ReadJSON(&msg2); err != nil {
		t.Fatalf("C2 failed to read welcome: %v", err)
	}

	// C1 creates lobby
	createMsg := Message{Type: "create_lobby", Rows: 10, Cols: 10}
	if err := ws1.WriteJSON(createMsg); err != nil {
		t.Fatalf("C1 failed to write: %v", err)
	}

	// C1 reads updates until lobby_created
	for {
		var msg Message
		if err := ws1.ReadJSON(&msg); err != nil {
			t.Fatal(err)
		}
		if msg.Type == "lobby_created" {
			break
		}
	}
}

func TestWebSocketBotConnection(t *testing.T) {
	h := newHub()
	go h.run()

	// Route for bot on a test-local mux.
	mux := http.NewServeMux()
	mux.HandleFunc("/ws/bot", func(w http.ResponseWriter, r *http.Request) {
		serveWs(h, w, r)
	})

	server := httptest.NewServer(mux)
	defer server.Close()

	// Pass ?bot=true
	wsURL := "ws" + strings.TrimPrefix(server.URL, "http") + "/ws/bot?bot=true"

	ws, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err != nil {
		t.Fatalf("Failed to connect Bot: %v", err)
	}
	defer ws.Close()

	var msg Message
	if err := ws.ReadJSON(&msg); err != nil {
		t.Fatal(err)
	}
	if msg.Type != "welcome" {
		t.Errorf("Expected welcome, got %s", msg.Type)
	}

	if !strings.HasPrefix(msg.Username, "Bot ") {
		t.Errorf("Expected Bot name, got %s", msg.Username)
	}
}

// End-to-end proof of the canary-bot naming path: a bot connecting with
// ?namePrefix=Canary receives a welcome username of "Canary Bot NNNN".
func TestWebSocketCanaryBotName(t *testing.T) {
	h := newHub()
	go h.run()

	mux := http.NewServeMux()
	mux.HandleFunc("/ws/bot", func(w http.ResponseWriter, r *http.Request) {
		serveWs(h, w, r)
	})

	server := httptest.NewServer(mux)
	defer server.Close()

	wsURL := "ws" + strings.TrimPrefix(server.URL, "http") + "/ws/bot?bot=true&namePrefix=Canary"

	ws, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err != nil {
		t.Fatalf("Failed to connect canary bot: %v", err)
	}
	defer ws.Close()

	var msg Message
	if err := ws.ReadJSON(&msg); err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(msg.Username, "Canary Bot ") {
		t.Errorf("Expected 'Canary Bot ...', got %s", msg.Username)
	}
}
