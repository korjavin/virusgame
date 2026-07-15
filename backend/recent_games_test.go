package main

import (
	"bytes"
	"compress/gzip"
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	_ "modernc.org/sqlite"
)

func TestRecentGamesLimitsOrderAndMetadata(t *testing.T) {
	database := newRecentGamesTestDB(t)
	base := time.Date(2026, 7, 15, 12, 0, 0, 0, time.UTC)
	for i := 0; i < 25; i++ {
		insertRecentGame(t, database, recentGameFixture{
			id:          fmt.Sprintf("game-%02d", i),
			startedAt:   base.Add(time.Duration(i) * time.Minute),
			endedAt:     base.Add(time.Duration(i) * time.Minute).Add(30 * time.Second),
			rows:        5 + i,
			cols:        50 - i,
			player1:     "Игрок 🦠",
			player2:     "Bot",
			result:      1,
			termination: "normal",
			history:     fmt.Sprintf(`[{"turn":%d,"player":1,"moves":[{"type":"place","row":1,"col":2,"duration_cs":3}]}]`, i),
		})
	}

	tests := []struct {
		name      string
		query     string
		wantCount int
	}{
		{name: "default", wantCount: 10},
		{name: "five", query: "?limit=5", wantCount: 5},
		{name: "ten", query: "?limit=10", wantCount: 10},
		{name: "twenty", query: "?limit=20", wantCount: 20},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			response := performRecentGamesRequest(database, http.MethodGet, "/last_games"+test.query, "")
			if response.Code != http.StatusOK {
				t.Fatalf("status = %d, body = %s", response.Code, response.Body.String())
			}
			var payload recentGamesResponse
			if err := json.Unmarshal(response.Body.Bytes(), &payload); err != nil {
				t.Fatal(err)
			}
			if len(payload.Games) != test.wantCount {
				t.Fatalf("games = %d, want %d", len(payload.Games), test.wantCount)
			}
			if payload.Games[0].ID != "game-24" || payload.Games[len(payload.Games)-1].ID != fmt.Sprintf("game-%02d", 25-test.wantCount) {
				t.Fatalf("unexpected order: first=%q last=%q", payload.Games[0].ID, payload.Games[len(payload.Games)-1].ID)
			}
			first := payload.Games[0]
			if first.Player1Name != "Игрок 🦠" || first.Player2Name != "Bot" || first.Player3Name != "" || first.Player4Name != "" ||
				first.Rows != 29 || first.Cols != 26 || first.Result != 1 || first.Termination != "normal" ||
				!first.StartedAt.Equal(base.Add(24*time.Minute)) || !first.EndedAt.Equal(base.Add(24*time.Minute+30*time.Second)) {
				t.Fatalf("metadata mismatch: %+v", first)
			}
			var turns []PGNTurn
			if err := json.Unmarshal(first.PGNContent, &turns); err != nil {
				t.Fatalf("turns are not decoded JSON: %v", err)
			}
			if len(turns) != 1 || turns[0].Turn != 24 {
				t.Fatalf("turns = %+v", turns)
			}
			if response.Header().Get("Cache-Control") != "no-store" || response.Header().Get("Content-Type") != "application/json; charset=utf-8" {
				t.Fatalf("safety headers missing: %v", response.Header())
			}
		})
	}
}

func TestRecentGamesEmptyDatabase(t *testing.T) {
	response := performRecentGamesRequest(newRecentGamesTestDB(t), http.MethodGet, "/last_games", "")
	if response.Code != http.StatusOK {
		t.Fatalf("status = %d", response.Code)
	}
	if got := strings.TrimSpace(response.Body.String()); got != `{"games":[]}` {
		t.Fatalf("body = %s", got)
	}
}

func TestRecentGamesGzipRoundTrip(t *testing.T) {
	database := newRecentGamesTestDB(t)
	insertRecentGame(t, database, recentGameFixture{id: "one", history: `[]`})
	plain := performRecentGamesRequest(database, http.MethodGet, "/last_games", "")
	compressed := performRecentGamesRequest(database, http.MethodGet, "/last_games", "br, gzip")

	if compressed.Header().Get("Content-Encoding") != "gzip" {
		t.Fatalf("Content-Encoding = %q", compressed.Header().Get("Content-Encoding"))
	}
	if compressed.Header().Get("Vary") != "Accept-Encoding" {
		t.Fatalf("Vary = %q", compressed.Header().Get("Vary"))
	}
	zr, err := gzip.NewReader(bytes.NewReader(compressed.Body.Bytes()))
	if err != nil {
		t.Fatal(err)
	}
	uncompressed, err := io.ReadAll(zr)
	if err != nil {
		t.Fatal(err)
	}
	if err := zr.Close(); err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(uncompressed, plain.Body.Bytes()) {
		t.Fatalf("gzip payload differs from plain payload")
	}

	qZero := performRecentGamesRequest(database, http.MethodGet, "/last_games", "gzip; q=0.0")
	if qZero.Header().Get("Content-Encoding") != "" {
		t.Fatalf("gzip used despite q=0")
	}
}

func TestRecentGamesMethodAndLimitValidation(t *testing.T) {
	database := newRecentGamesTestDB(t)
	method := performRecentGamesRequest(database, http.MethodPost, "/last_games", "")
	if method.Code != http.StatusMethodNotAllowed || method.Header().Get("Allow") != http.MethodGet {
		t.Fatalf("method response = %d, Allow=%q", method.Code, method.Header().Get("Allow"))
	}
	for _, value := range []string{"", "0", "6", "100", "nope", "5&limit=20"} {
		response := performRecentGamesRequest(database, http.MethodGet, "/last_games?limit="+value, "")
		if response.Code != http.StatusBadRequest {
			t.Errorf("limit %q status = %d", value, response.Code)
		}
	}
	if method.Header().Get("Cache-Control") != "no-store" || method.Header().Get("Content-Type") != "application/json; charset=utf-8" {
		t.Fatalf("safety headers missing: %v", method.Header())
	}
}

func TestRecentGamesRejectsCorruptHistoryWithoutLeakingIt(t *testing.T) {
	database := newRecentGamesTestDB(t)
	insertRecentGame(t, database, recentGameFixture{id: "secret-id", history: `private invalid JSON`})
	response := performRecentGamesRequest(database, http.MethodGet, "/last_games", "")
	if response.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d", response.Code)
	}
	if strings.Contains(response.Body.String(), "private") || strings.Contains(response.Body.String(), "secret-id") {
		t.Fatalf("response leaks stored data: %s", response.Body.String())
	}
}

func TestRecentGamesDatabaseFailureIsGeneric(t *testing.T) {
	database := newRecentGamesTestDB(t)
	if err := database.Close(); err != nil {
		t.Fatal(err)
	}
	response := performRecentGamesRequest(database, http.MethodGet, "/last_games", "")
	if response.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d", response.Code)
	}
	if strings.Contains(strings.ToLower(response.Body.String()), "database") || strings.Contains(strings.ToLower(response.Body.String()), "sql") {
		t.Fatalf("response leaks database details: %s", response.Body.String())
	}
}

type recentGameFixture struct {
	id, player1, player2, history, termination string
	startedAt, endedAt                         time.Time
	rows, cols, result                         int
}

func newRecentGamesTestDB(t *testing.T) *sql.DB {
	t.Helper()
	database, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	database.SetMaxOpenConns(1)
	t.Cleanup(func() { _ = database.Close() })
	_, err = database.Exec(`CREATE TABLE games (
		id TEXT PRIMARY KEY, started_at DATETIME, ended_at DATETIME,
		rows INTEGER, cols INTEGER, player1_name TEXT, player2_name TEXT,
		player3_name TEXT, player4_name TEXT, result INTEGER,
		termination TEXT, pgn_content TEXT, rejected_attempt TEXT
	)`)
	if err != nil {
		t.Fatal(err)
	}
	return database
}

func insertRecentGame(t *testing.T, database *sql.DB, game recentGameFixture) {
	t.Helper()
	if game.startedAt.IsZero() {
		game.startedAt = time.Date(2026, 7, 15, 12, 0, 0, 0, time.UTC)
	}
	if game.endedAt.IsZero() {
		game.endedAt = game.startedAt.Add(time.Minute)
	}
	if game.rows == 0 {
		game.rows = 10
	}
	if game.cols == 0 {
		game.cols = 10
	}
	_, err := database.Exec(`INSERT INTO games VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		game.id, game.startedAt, game.endedAt, game.rows, game.cols,
		game.player1, game.player2, "", "", game.result, game.termination, game.history, nil)
	if err != nil {
		t.Fatal(err)
	}
}

func performRecentGamesRequest(database *sql.DB, method, target, encoding string) *httptest.ResponseRecorder {
	request := httptest.NewRequest(method, target, nil)
	request.Header.Set("Accept-Encoding", encoding)
	response := httptest.NewRecorder()
	recentGamesHandler(database).ServeHTTP(response, request)
	return response
}
