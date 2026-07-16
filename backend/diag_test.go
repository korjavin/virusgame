package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
)

// TestOutboxRecoversTerminalWriteAfterDBOutage exercises the P0 core: a terminal
// producer whose durable write fails must not lose the game. The record is
// spooled to the durable outbox and a later replay commits it, with diagnostics
// reflecting the degraded-then-recovered state.
func TestOutboxRecoversTerminalWriteAfterDBOutage(t *testing.T) {
	InitDB(filepath.Join(t.TempDir(), "retry-queue.db"))
	t.Cleanup(closePersistenceTestDB)

	h := newHub()
	player1 := persistenceTestUser("human", "Human")
	player2 := persistenceTestUser("bot", "OnlineBot")
	game := persistenceTestGame("retry-queue-game", player1, player2)
	h.games[game.ID] = game

	// Inject a write failure at the moment of termination.
	good := db
	db = nil
	if !h.persistTerminal(game, "resignation") {
		t.Fatal("terminal record was neither committed nor spooled — data loss")
	}
	if game.persisted {
		t.Fatal("game marked committed despite DB outage")
	}
	if spool.depth() != 1 {
		t.Fatalf("expected 1 spooled record, got %d", spool.depth())
	}
	if got := persistHealth.snapshot(); got.OutboxDepth != 1 || got.PersistStatus != "degraded" {
		t.Fatalf("diagnostics not degraded: depth=%d status=%q", got.OutboxDepth, got.PersistStatus)
	}

	// Restore the database and replay the outbox.
	db = good
	h.replayOutbox()

	if spool.depth() != 0 {
		t.Fatalf("outbox not drained after recovery: depth=%d", spool.depth())
	}
	got := persistHealth.snapshot()
	if got.PersistStatus != "ok" {
		t.Fatalf("status = %q after recovery, want ok", got.PersistStatus)
	}

	var count int
	termination := ""
	if err := db.QueryRow("SELECT COUNT(*), termination FROM games WHERE id = ?", game.ID).Scan(&count, &termination); err != nil {
		t.Fatal(err)
	}
	if count != 1 || termination != "resignation" {
		t.Fatalf("expected 1 durable row termination=resignation, got count=%d termination=%q", count, termination)
	}
}

func TestDiagAndLastGamesProvenance(t *testing.T) {
	InitDB(filepath.Join(t.TempDir(), "games.db"))
	t.Cleanup(closePersistenceTestDB)

	snap := persistHealth.snapshot()
	if snap.InstanceID == "" || snap.DBID == "" {
		t.Fatalf("instance/db id not set: %+v", snap)
	}
	if snap.DBID != dbIdentity {
		t.Fatalf("db id %q != minted identity %q", snap.DBID, dbIdentity)
	}

	// /diag JSON exposes the same provenance and NO path/raw-error fields.
	diagResp := httptest.NewRecorder()
	diagHandler().ServeHTTP(diagResp, httptest.NewRequest(http.MethodGet, "/diag", nil))
	if diagResp.Code != http.StatusOK {
		t.Fatalf("/diag status = %d", diagResp.Code)
	}
	rawBody := diagResp.Body.String()
	if strings.Contains(rawBody, "db_path") || strings.Contains(rawBody, string(filepath.Separator)+"games.db") {
		t.Fatalf("/diag leaked a filesystem path: %s", rawBody)
	}
	if strings.Contains(rawBody, "last_persist_error\"") {
		t.Fatalf("/diag leaked a raw error field: %s", rawBody)
	}
	var body provenance
	if err := json.Unmarshal(diagResp.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body.DBID != snap.DBID || body.BuildSHA != buildSHA {
		t.Fatalf("/diag body mismatch: %+v", body)
	}

	// /last_games binds the same provenance as headers without altering the body.
	resp := performRecentGamesTestRequest(db, http.MethodGet, "/last_games?limit=5", "")
	if resp.Code != http.StatusOK {
		t.Fatalf("/last_games status = %d", resp.Code)
	}
	if got := resp.Header().Get("X-DB-Id"); got != snap.DBID {
		t.Fatalf("X-DB-Id = %q, want %q", got, snap.DBID)
	}
	if got := resp.Header().Get("X-Instance-ID"); got != snap.InstanceID {
		t.Fatalf("X-Instance-ID = %q, want %q", got, snap.InstanceID)
	}
	if resp.Header().Get("X-DB-Identity") != "" {
		t.Fatal("stale absolute-path header X-DB-Identity must not be present")
	}
	// Existing clients still receive valid JSON.
	var payload recentGamesResponse
	if err := json.Unmarshal(resp.Body.Bytes(), &payload); err != nil {
		t.Fatalf("last_games body no longer decodes: %v", err)
	}
}

// TestPublicSurfaceHidesRawErrorAndPath verifies blocker 1: an injected failure
// with a distinctive raw message never reaches the unauthenticated surface;
// only a safe category does.
func TestPublicSurfaceHidesRawErrorAndPath(t *testing.T) {
	InitDB(filepath.Join(t.TempDir(), "secrecy.db"))
	t.Cleanup(closePersistenceTestDB)

	game := persistenceTestGame("secrecy-game", persistenceTestUser("p1", "Human"), persistenceTestUser("p2", "OnlineBot"))
	good := db
	db = nil
	if PersistGameOnce(game, "normal") { // fails: "database not initialized"
		t.Fatal("expected failure with nil db")
	}
	db = good

	diagResp := httptest.NewRecorder()
	diagHandler().ServeHTTP(diagResp, httptest.NewRequest(http.MethodGet, "/diag", nil))
	body := diagResp.Body.String()
	if strings.Contains(body, "database not initialized") {
		t.Fatalf("/diag leaked raw error text: %s", body)
	}
	var p provenance
	if err := json.Unmarshal(diagResp.Body.Bytes(), &p); err != nil {
		t.Fatal(err)
	}
	if p.LastErrorCategory != "db_unavailable" {
		t.Fatalf("error category = %q, want db_unavailable", p.LastErrorCategory)
	}
}

func TestWelcomeMessageCarriesProvenance(t *testing.T) {
	InitDB(filepath.Join(t.TempDir(), "welcome.db"))
	t.Cleanup(closePersistenceTestDB)

	h := newHub()
	client := &Client{send: make(chan []byte, 8)}
	h.clients[client] = true // run() registers before handleConnect

	h.handleConnect(client)

	var welcome *Message
	for {
		select {
		case raw := <-client.send:
			var msg Message
			if err := json.Unmarshal(raw, &msg); err != nil {
				t.Fatal(err)
			}
			if msg.Type == "welcome" {
				welcome = &msg
			}
		default:
			if welcome == nil {
				t.Fatal("no welcome message sent")
			}
			if welcome.InstanceID != instanceID || welcome.BuildSHA != buildSHA || welcome.DBID != dbIdentity {
				t.Fatalf("welcome provenance mismatch: instance=%q build=%q db=%q", welcome.InstanceID, welcome.BuildSHA, welcome.DBID)
			}
			return
		}
	}
}
