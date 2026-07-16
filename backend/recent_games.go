package main

import (
	"bytes"
	"compress/gzip"
	"context"
	"database/sql"
	"encoding/json"
	"log"
	"net/http"
	"strconv"
	"strings"
	"time"
)

const defaultRecentGamesLimit = 10

type recentGamesResponse struct {
	Games []recentGame `json:"games"`
}

type recentGame struct {
	ID              string           `json:"id"`
	StartedAt       time.Time        `json:"started_at"`
	EndedAt         time.Time        `json:"ended_at"`
	Rows            int              `json:"rows"`
	Cols            int              `json:"cols"`
	Player1Name     string           `json:"player1_name"`
	Player2Name     string           `json:"player2_name"`
	Player3Name     string           `json:"player3_name"`
	Player4Name     string           `json:"player4_name"`
	Result          int              `json:"result"`
	Termination     string           `json:"termination"`
	PGNContent      json.RawMessage  `json:"pgn_content"`
	RejectedAttempt *RejectedAttempt `json:"rejected_attempt,omitempty"`
}

func recentGamesHandler(database *sql.DB) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store")
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		w.Header().Set("Vary", "Accept-Encoding")
		// Bind instance/build/persistence provenance as headers so a stale answer
		// from a mis-routed instance is detectable without changing the JSON body.
		setProvenanceHeaders(w, persistHealth.snapshot())

		if r.Method != http.MethodGet {
			w.Header().Set("Allow", http.MethodGet)
			writeJSONError(w, http.StatusMethodNotAllowed, "method not allowed")
			return
		}

		limitValues, hasLimit := r.URL.Query()["limit"]
		if hasLimit && len(limitValues) != 1 {
			writeJSONError(w, http.StatusBadRequest, "invalid limit")
			return
		}
		rawLimit := ""
		if hasLimit {
			rawLimit = limitValues[0]
		}
		limit, ok := recentGamesLimit(rawLimit, hasLimit)
		if !ok {
			writeJSONError(w, http.StatusBadRequest, "invalid limit")
			return
		}

		games, err := loadRecentGames(r.Context(), database, limit)
		if err != nil {
			log.Printf("Error loading recent games: %v", err)
			writeJSONError(w, http.StatusInternalServerError, "unable to load games")
			return
		}

		payload, err := json.Marshal(recentGamesResponse{Games: games})
		if err != nil {
			writeJSONError(w, http.StatusInternalServerError, "unable to encode games")
			return
		}

		if acceptsGzip(r.Header.Get("Accept-Encoding")) {
			var compressed bytes.Buffer
			zw := gzip.NewWriter(&compressed)
			if _, err := zw.Write(payload); err != nil {
				writeJSONError(w, http.StatusInternalServerError, "unable to encode games")
				return
			}
			if err := zw.Close(); err != nil {
				writeJSONError(w, http.StatusInternalServerError, "unable to encode games")
				return
			}
			w.Header().Set("Content-Encoding", "gzip")
			_, _ = w.Write(compressed.Bytes())
			return
		}

		_, _ = w.Write(payload)
	})
}

func recentGamesLimit(raw string, present bool) (int, bool) {
	if !present {
		return defaultRecentGamesLimit, true
	}
	limit, err := strconv.Atoi(raw)
	if err != nil {
		return 0, false
	}
	switch limit {
	case 5, 10, 20:
		return limit, true
	default:
		return 0, false
	}
}

func loadRecentGames(ctx context.Context, database *sql.DB, limit int) ([]recentGame, error) {
	if database == nil {
		return nil, sql.ErrConnDone
	}
	// Keep the query fixed except for the bounded integer parameter. The
	// secondary ID ordering makes equal timestamps deterministic.
	rows, err := database.QueryContext(ctx, `
		SELECT id, started_at, ended_at, rows, cols,
		       player1_name, player2_name, player3_name, player4_name,
		       result, termination, pgn_content, rejected_attempt
		FROM games
		ORDER BY ended_at DESC, id DESC
		LIMIT ?`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	games := make([]recentGame, 0, limit)
	for rows.Next() {
		var game recentGame
		var history []byte
		var rejected sql.NullString
		var player1, player2, player3, player4 sql.NullString
		if err := rows.Scan(
			&game.ID, &game.StartedAt, &game.EndedAt, &game.Rows, &game.Cols,
			&player1, &player2, &player3, &player4,
			&game.Result, &game.Termination, &history, &rejected,
		); err != nil {
			return nil, err
		}
		game.Player1Name = player1.String
		game.Player2Name = player2.String
		game.Player3Name = player3.String
		game.Player4Name = player4.String
		if !json.Valid(history) {
			return nil, &corruptGameHistoryError{}
		}
		game.PGNContent = append(json.RawMessage(nil), history...)
		if rejected.Valid {
			var attempt RejectedAttempt
			if !json.Valid([]byte(rejected.String)) || json.Unmarshal([]byte(rejected.String), &attempt) != nil {
				return nil, &corruptGameHistoryError{}
			}
			game.RejectedAttempt = &attempt
		}
		games = append(games, game)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return games, nil
}

type corruptGameHistoryError struct{}

func (*corruptGameHistoryError) Error() string { return "corrupt game history" }

func acceptsGzip(header string) bool {
	for _, value := range strings.Split(header, ",") {
		parts := strings.Split(strings.TrimSpace(value), ";")
		if !strings.EqualFold(strings.TrimSpace(parts[0]), "gzip") {
			continue
		}
		for _, parameter := range parts[1:] {
			name, rawQuality, found := strings.Cut(strings.TrimSpace(parameter), "=")
			if !found || !strings.EqualFold(strings.TrimSpace(name), "q") {
				continue
			}
			quality, err := strconv.ParseFloat(strings.TrimSpace(rawQuality), 64)
			if err != nil || quality <= 0 || quality > 1 {
				return false
			}
		}
		return true
	}
	return false
}

func writeJSONError(w http.ResponseWriter, status int, message string) {
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]string{"error": message})
}
