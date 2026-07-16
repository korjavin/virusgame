package main

import (
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func actionTestGame() (*Hub, *Game, *User, *User) {
	hub := newHub()
	player1 := persistenceTestUser("p1", "Player One")
	player2 := persistenceTestUser("p2", "Player Two")
	board := make(Board, 5)
	for row := range board {
		board[row] = make([]CellValue, 5)
	}
	board[0][0] = NewCell(1, CellFlagBase)
	board[4][4] = NewCell(2, CellFlagBase)
	game := &Game{
		ID: "requests", Player1: player1, Player2: player2, Board: board,
		Rows: 5, Cols: 5, CurrentPlayer: 1, MovesLeft: 3,
		Player1Base: CellPos{0, 0}, Player2Base: CellPos{4, 4},
		StartTime: time.Now(), LastActionTime: time.Now(), TurnCount: 1,
	}
	hub.clients[player1.Client] = true
	hub.clients[player2.Client] = true
	hub.games[game.ID] = game
	return hub, game, player1, player2
}

func TestActionRequestDuplicateIsIdempotentButDistinctIllegalStillLoses(t *testing.T) {
	hub, game, player1, _ := actionTestGame()
	row, col := 0, 1
	request := &Message{Type: "move", GameID: game.ID, Row: &row, Col: &col, RequestID: "17"}
	hub.handleMove(player1, request)
	if len(game.MoveHistory) != 1 || game.GameOver {
		t.Fatalf("first request was not accepted once: history=%d over=%v", len(game.MoveHistory), game.GameOver)
	}
	accepted := waitForMessage(t, player1.Client, "move_made")
	if accepted == nil || accepted.RequestID != "17" || accepted.Snapshot == nil {
		t.Fatalf("accepted action did not echo request ID: %#v", accepted)
	}
	hub.handleMove(player1, request)
	if len(game.MoveHistory) != 1 || game.GameOver {
		t.Fatalf("same request replay changed game: history=%d over=%v", len(game.MoveHistory), game.GameOver)
	}
	ack := waitForMessage(t, player1.Client, "action_ack")
	if ack == nil || ack.RequestID != "17" || ack.Snapshot == nil {
		t.Fatalf("duplicate acknowledgement = %#v", ack)
	}

	distinct := &Message{Type: "move", GameID: game.ID, Row: &row, Col: &col, RequestID: "18"}
	hub.handleMove(player1, distinct)
	if !game.GameOver || game.Winner != 2 || game.RejectedAttempt == nil || game.RejectedAttempt.RequestID != "18" {
		t.Fatalf("distinct illegal request did not retain defeat semantics: over=%v winner=%d attempt=%#v", game.GameOver, game.Winner, game.RejectedAttempt)
	}
	if game.MoveTimer != nil {
		game.MoveTimer.Stop()
	}
}

func TestActionRequestHistoryIsPerPlayerAndBounded(t *testing.T) {
	_, game, _, _ := actionTestGame()
	game.rememberActionRequest(1, &Message{Type: "move", RequestID: "same"})
	if !game.hasActionRequest(1, "same") || game.hasActionRequest(2, "same") {
		t.Fatal("request IDs are not scoped per player")
	}
	for index := 0; index <= actionRequestHistoryLimit; index++ {
		game.rememberActionRequest(1, &Message{Type: "move", RequestID: string(rune('a' + index))})
	}
	if len(game.requestHistory(1).order) != actionRequestHistoryLimit || game.hasActionRequest(1, "same") {
		t.Fatalf("history was not bounded: %#v", game.requestHistory(1).order)
	}
}

func TestInvalidRequestIDIsRejectedWithoutMutation(t *testing.T) {
	hub, game, player1, _ := actionTestGame()
	row, col := 0, 1
	for _, requestID := range []string{strings.Repeat("x", 129), "contains space"} {
		hub.handleMove(player1, &Message{Type: "move", GameID: game.ID, Row: &row, Col: &col, RequestID: requestID})
		errorMessage := waitForMessage(t, player1.Client, "error")
		if errorMessage == nil || errorMessage.RequestID != requestID || errorMessage.Snapshot == nil {
			t.Fatalf("invalid ID response = %#v", errorMessage)
		}
	}
	if len(game.MoveHistory) != 0 || game.Board[row][col] != 0 || game.GameOver || len(game.requestHistory(1).order) != 0 {
		t.Fatalf("invalid request ID mutated state: history=%d cell=%v over=%v IDs=%v", len(game.MoveHistory), game.Board[row][col], game.GameOver, game.requestHistory(1).order)
	}
}

func TestOutsiderCannotProbeGameSnapshotWithMalformedActions(t *testing.T) {
	hub, game, _, _ := actionTestGame()
	outsider := persistenceTestUser("outsider", "Outsider")
	hub.clients[outsider.Client] = true

	hub.handleMove(outsider, &Message{Type: "move", GameID: game.ID, RequestID: "probe-move"})
	hub.handleNeutrals(outsider, &Message{Type: "neutrals", GameID: game.ID, RequestID: "probe-neutral", Cells: []CellPos{{-1, -1}}})
	select {
	case response := <-outsider.Client.send:
		t.Fatalf("outsider received game-specific response: %s", response)
	default:
	}
	if len(game.MoveHistory) != 0 || game.GameOver || game.RejectedAttempt != nil {
		t.Fatalf("outsider mutated game: history=%d over=%v rejection=%#v", len(game.MoveHistory), game.GameOver, game.RejectedAttempt)
	}
}

func TestMoveRequestIDConflictIsNonPunitive(t *testing.T) {
	hub, game, player1, _ := actionTestGame()
	row, firstCol := 0, 1
	hub.handleMove(player1, &Message{Type: "move", GameID: game.ID, Row: &row, Col: &firstCol, RequestID: "conflict"})
	secondCol := 2
	hub.handleMove(player1, &Message{Type: "move", GameID: game.ID, Row: &row, Col: &secondCol, RequestID: "conflict"})
	errorMessage := waitForMessage(t, player1.Client, "error")
	if errorMessage == nil || errorMessage.RequestID != "conflict" || !strings.Contains(errorMessage.Username, "different content") {
		t.Fatalf("move conflict response = %#v", errorMessage)
	}
	if len(game.MoveHistory) != 1 || game.Board[row][secondCol] != 0 || game.GameOver {
		t.Fatalf("move conflict mutated/punished game: history=%d target=%v over=%v", len(game.MoveHistory), game.Board[row][secondCol], game.GameOver)
	}
	hub.handleMove(player1, &Message{Type: "move", GameID: game.ID, Row: &row, Col: &firstCol, RequestID: "conflict", Cells: []CellPos{{1, 1}}})
	presenceConflict := waitForMessage(t, player1.Client, "error")
	if presenceConflict == nil || !strings.Contains(presenceConflict.Username, "different content") {
		t.Fatalf("move ignored-field conflict response = %#v", presenceConflict)
	}
	newID := "extraneous-move-cells"
	hub.handleMove(player1, &Message{Type: "move", GameID: game.ID, Row: &row, Col: &secondCol, RequestID: newID, Cells: []CellPos{{1, 1}}})
	schemaError := waitForMessage(t, player1.Client, "error")
	if schemaError == nil || schemaError.RequestID != newID || !strings.Contains(schemaError.Username, "must not contain") {
		t.Fatalf("move strict-schema response = %#v", schemaError)
	}
	if game.MoveTimer != nil {
		game.MoveTimer.Stop()
	}
}

func TestMalformedCoordinatesCannotPanicAndPersistExactEvidence(t *testing.T) {
	InitDB(filepath.Join(t.TempDir(), "games.db"))
	t.Cleanup(func() { _ = db.Close(); db = nil })
	hub, game, player1, _ := actionTestGame()
	game.ID = "bounds-evidence"
	hub.games[game.ID] = game
	row, col := 0, -1
	hub.handleMove(player1, &Message{Type: "move", GameID: game.ID, Row: &row, Col: &col, RequestID: "bad-bounds"})
	if !game.GameOver || game.RejectedAttempt == nil {
		t.Fatal("malformed coordinate did not follow illegal-action path")
	}
	attempt := game.RejectedAttempt
	if attempt.Row == nil || *attempt.Row != 0 || attempt.Col == nil || *attempt.Col != -1 ||
		attempt.Player != 1 || attempt.Action != "move" || attempt.Turn != 1 || attempt.MovesLeft != 3 ||
		!strings.HasPrefix(attempt.StateHash, "sha256:") || attempt.Snapshot == nil || attempt.Snapshot.Board[0][0].Owner != 1 {
		t.Fatalf("incomplete rejection evidence: %#v", attempt)
	}

	response := performRecentGamesRequest(db, "GET", "/last_games?limit=5", "")
	if response.Code != 200 {
		t.Fatalf("last_games status=%d body=%s", response.Code, response.Body.String())
	}
	var payload recentGamesResponse
	if err := json.Unmarshal(response.Body.Bytes(), &payload); err != nil {
		t.Fatal(err)
	}
	if len(payload.Games) != 1 || payload.Games[0].RejectedAttempt == nil ||
		payload.Games[0].RejectedAttempt.RequestID != "bad-bounds" || *payload.Games[0].RejectedAttempt.Row != 0 {
		t.Fatalf("last_games lost rejection evidence: %#v", payload.Games)
	}
}

func TestPGNPreservesZeroCoordinates(t *testing.T) {
	game := &Game{MoveHistory: []MoveAction{{Player: 1, Type: "place", Row: 0, Col: 0, TurnNumber: 1}}}
	content, err := generatePGN(game)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(content, `"row":0`) || !strings.Contains(content, `"col":0`) {
		t.Fatalf("zero coordinate omitted: %s", content)
	}
}

func TestLegacyRequestsWithoutIDRemainSequential(t *testing.T) {
	hub, game, player1, _ := actionTestGame()
	row0, col0 := 0, 1
	hub.handleMove(player1, &Message{Type: "move", GameID: game.ID, Row: &row0, Col: &col0})
	row1, col1 := 0, 2
	hub.handleMove(player1, &Message{Type: "move", GameID: game.ID, Row: &row1, Col: &col1})
	if len(game.MoveHistory) != 2 || game.GameOver {
		t.Fatalf("legacy requests were deduplicated or rejected: history=%d over=%v", len(game.MoveHistory), game.GameOver)
	}
	if game.MoveTimer != nil {
		game.MoveTimer.Stop()
	}
}

func TestDistinctRequestIDsAllowSequentialLegalMoves(t *testing.T) {
	hub, game, player1, _ := actionTestGame()
	row0, col0 := 0, 1
	hub.handleMove(player1, &Message{Type: "move", GameID: game.ID, Row: &row0, Col: &col0, RequestID: "one"})
	row1, col1 := 0, 2
	hub.handleMove(player1, &Message{Type: "move", GameID: game.ID, Row: &row1, Col: &col1, RequestID: "two"})
	if len(game.MoveHistory) != 2 || !game.hasActionRequest(1, "one") || !game.hasActionRequest(1, "two") {
		t.Fatalf("distinct requests did not both apply: history=%d", len(game.MoveHistory))
	}
	if game.MoveTimer != nil {
		game.MoveTimer.Stop()
	}
}

func TestNeutralRequestReplayIsIdempotent(t *testing.T) {
	hub, game, player1, _ := actionTestGame()
	game.Board[0][1] = NewCell(1, CellFlagNormal)
	game.Board[1][0] = NewCell(1, CellFlagNormal)
	request := &Message{Type: "neutrals", GameID: game.ID, RequestID: "neutral-1", Cells: []CellPos{{0, 1}, {1, 0}}}
	hub.handleNeutrals(player1, request)
	hub.handleNeutrals(player1, request)
	if len(game.MoveHistory) != 1 || game.MoveHistory[0].Type != "neutral" || game.GameOver {
		t.Fatalf("neutral replay changed game twice: history=%#v over=%v", game.MoveHistory, game.GameOver)
	}
	ack := waitForMessage(t, player1.Client, "action_ack")
	if ack == nil || ack.RequestID != "neutral-1" || ack.Snapshot == nil {
		t.Fatalf("neutral duplicate acknowledgement = %#v", ack)
	}
	if game.MoveTimer != nil {
		game.MoveTimer.Stop()
	}
}

func TestNeutralRequestIDConflictIsNonPunitive(t *testing.T) {
	hub, game, player1, _ := actionTestGame()
	game.Board[0][1] = NewCell(1, CellFlagNormal)
	game.Board[1][0] = NewCell(1, CellFlagNormal)
	first := []CellPos{{0, 1}, {1, 0}}
	hub.handleNeutrals(player1, &Message{Type: "neutrals", GameID: game.ID, RequestID: "neutral-conflict", Cells: first})
	hub.handleNeutrals(player1, &Message{Type: "neutrals", GameID: game.ID, RequestID: "neutral-conflict", Cells: []CellPos{first[1], first[0]}})
	errorMessage := waitForMessage(t, player1.Client, "error")
	if errorMessage == nil || errorMessage.RequestID != "neutral-conflict" || !strings.Contains(errorMessage.Username, "different content") {
		t.Fatalf("neutral conflict response = %#v", errorMessage)
	}
	if len(game.MoveHistory) != 1 || game.GameOver {
		t.Fatalf("neutral conflict mutated/punished game: history=%#v over=%v", game.MoveHistory, game.GameOver)
	}
	row := 0
	hub.handleNeutrals(player1, &Message{Type: "neutrals", GameID: game.ID, RequestID: "neutral-conflict", Cells: first, Row: &row})
	presenceConflict := waitForMessage(t, player1.Client, "error")
	if presenceConflict == nil || !strings.Contains(presenceConflict.Username, "different content") {
		t.Fatalf("neutral target-presence conflict response = %#v", presenceConflict)
	}
	hub.handleNeutrals(player1, &Message{Type: "neutrals", GameID: game.ID, RequestID: "extraneous-neutral-target", Cells: first, Row: &row})
	schemaError := waitForMessage(t, player1.Client, "error")
	if schemaError == nil || !strings.Contains(schemaError.Username, "must not contain") {
		t.Fatalf("neutral strict-schema response = %#v", schemaError)
	}
	if game.MoveTimer != nil {
		game.MoveTimer.Stop()
	}
}
