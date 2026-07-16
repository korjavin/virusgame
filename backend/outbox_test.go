package main

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"
)

// recordingFS wraps the real filesystem, recording the durability-critical call
// sequence and injecting a failure at a chosen step. It proves the exact
// write->file-sync->close->rename->dir-sync ordering and failure propagation.
type recordingFS struct {
	osFS
	ops     []string
	failAt  string
	failErr error
}

type recordingFile struct {
	*os.File
	fs *recordingFS
}

func (f *recordingFile) Write(p []byte) (int, error) {
	f.fs.ops = append(f.fs.ops, "write")
	if f.fs.failAt == "write" {
		return 0, f.fs.failErr
	}
	return f.File.Write(p)
}

func (f *recordingFile) Sync() error {
	f.fs.ops = append(f.fs.ops, "file.sync")
	if f.fs.failAt == "file.sync" {
		return f.fs.failErr
	}
	return f.File.Sync()
}

func (f *recordingFile) Close() error {
	f.fs.ops = append(f.fs.ops, "close")
	return f.File.Close()
}

func (f *recordingFile) Name() string { return f.File.Name() }

func (r *recordingFS) CreateTemp(dir, pattern string) (spoolFile, error) {
	r.ops = append(r.ops, "createtemp")
	f, err := os.CreateTemp(dir, pattern)
	if err != nil {
		return nil, err
	}
	return &recordingFile{File: f, fs: r}, nil
}

func (r *recordingFS) Rename(a, b string) error {
	r.ops = append(r.ops, "rename")
	if r.failAt == "rename" {
		return r.failErr
	}
	return os.Rename(a, b)
}

func (r *recordingFS) Remove(n string) error {
	r.ops = append(r.ops, "remove")
	return os.Remove(n)
}

func (r *recordingFS) SyncDir(dir string) error {
	r.ops = append(r.ops, "dir.sync")
	if r.failAt == "dir.sync" {
		return r.failErr
	}
	return osFS{}.SyncDir(dir)
}

// installRecordingFS swaps in a recording filesystem after InitDB and restores
// the real one on cleanup. Ops recorded before the returned reset point are
// cleared so a test sees only its own operations.
func installRecordingFS(t *testing.T) *recordingFS {
	t.Helper()
	rec := &recordingFS{failErr: errors.New("injected fs failure")}
	spool.mu.Lock()
	spool.fs = rec
	spool.mu.Unlock()
	t.Cleanup(func() {
		spool.mu.Lock()
		spool.fs = osFS{}
		spool.mu.Unlock()
	})
	return rec
}

// TestOutboxSharesDatabaseVolume proves the durable spool lives beside the
// database, so both survive on the same mounted volume across a restart.
func TestOutboxSharesDatabaseVolume(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "games.db")
	InitDB(dbPath)
	t.Cleanup(closePersistenceTestDB)
	want := filepath.Join(filepath.Dir(dbPath), "outbox")
	if spool.dir != want {
		t.Fatalf("outbox dir = %q, want %q (must share the DB volume)", spool.dir, want)
	}
}

func sampleRecord(id string) terminalRecord {
	now := time.Date(2026, time.July, 16, 0, 24, 0, 0, time.UTC)
	return terminalRecord{
		ID:          id,
		StartedAt:   now.Add(-time.Minute),
		EndedAt:     now,
		Rows:        12,
		Cols:        12,
		Player1Name: "Human",
		Player2Name: "OnlineBot",
		Result:      1,
		Termination: "normal",
		PGNContent:  `[{"turn":1,"player":1,"moves":[]}]`,
	}
}

// TestOutboxRestartRecovery proves durability across a process restart: a record
// spooled before "restart" is committed by replay on the fresh DB/hub.
func TestOutboxRestartRecovery(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "games.db")
	InitDB(dbPath)

	rec := sampleRecord("restart-game")
	if err := spool.Spool(rec); err != nil {
		t.Fatalf("spool: %v", err)
	}
	if spool.depth() != 1 {
		t.Fatalf("expected 1 spooled, got %d", spool.depth())
	}

	// Simulate a restart: close and reopen the same database, new hub.
	_ = db.Close()
	db = nil
	InitDB(dbPath)
	t.Cleanup(closePersistenceTestDB)
	h := newHub()

	h.replayOutbox() // startup replay

	if spool.depth() != 0 {
		t.Fatalf("outbox not drained on restart: depth=%d", spool.depth())
	}
	var count int
	if err := db.QueryRow("SELECT COUNT(*) FROM games WHERE id = ?", rec.ID).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Fatalf("restart replay did not persist exactly once, count=%d", count)
	}
}

// TestOutboxReservationBounds proves reservations enforce the global bound
// (files + reservations <= cap) and that releasing frees admission again.
func TestOutboxReservationBounds(t *testing.T) {
	InitDB(filepath.Join(t.TempDir(), "games.db"))
	t.Cleanup(closePersistenceTestDB)

	prev := outboxMaxFiles
	outboxMaxFiles = 2
	t.Cleanup(func() { outboxMaxFiles = prev })

	if !spool.Reserve() || !spool.Reserve() {
		t.Fatal("first two reservations should succeed up to capacity")
	}
	if spool.Reserve() {
		t.Fatal("reservation past capacity must be refused")
	}
	// A spooled record occupies a real slot: it must not exceed the cap alongside
	// the outstanding reservation it replaces.
	spool.release() // one game's terminal committed to DB -> slot freed
	if !spool.Reserve() {
		t.Fatal("admission did not recover after a release")
	}
	spool.release()
	spool.release()
}

// TestOutboxCorruptQuarantine proves a corrupt spool file is quarantined and
// observable, not retried forever, while valid records still commit.
func TestOutboxCorruptQuarantine(t *testing.T) {
	InitDB(filepath.Join(t.TempDir(), "games.db"))
	t.Cleanup(closePersistenceTestDB)
	h := newHub()

	// A valid record and a corrupt one share the spool.
	if err := spool.Spool(sampleRecord("valid-game")); err != nil {
		t.Fatal(err)
	}
	corrupt := filepath.Join(spool.dir, "corrupt-game.json")
	if err := os.WriteFile(corrupt, []byte("{not json"), 0o644); err != nil {
		t.Fatal(err)
	}

	before := spool.quarantineDepth()
	h.replayOutbox()

	if spool.quarantineDepth() != before+1 {
		t.Fatalf("corrupt file not quarantined: quarantine_depth=%d", spool.quarantineDepth())
	}
	if _, err := os.Stat(corrupt); !os.IsNotExist(err) {
		t.Fatal("corrupt file was left in the active spool")
	}
	if _, err := os.Stat(filepath.Join(spool.quarantineDir, "corrupt-game.json")); err != nil {
		t.Fatalf("corrupt file not moved to quarantine: %v", err)
	}
	// Quarantine is an unhealthy state, never equated with successful closure.
	if got := persistHealth.snapshot(); got.QuarantineDepth < 1 || got.PersistStatus != "unhealthy" {
		t.Fatalf("diagnostics not unhealthy with quarantine: depth=%d status=%q", got.QuarantineDepth, got.PersistStatus)
	}
	var count int
	if err := db.QueryRow("SELECT COUNT(*) FROM games WHERE id = ?", "valid-game").Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Fatalf("valid record not committed alongside quarantine, count=%d", count)
	}
}

// TestOutboxReplayNeverDuplicates proves replay of a record whose row already
// exists removes the spool file without inserting a duplicate.
func TestOutboxReplayNeverDuplicates(t *testing.T) {
	InitDB(filepath.Join(t.TempDir(), "games.db"))
	t.Cleanup(closePersistenceTestDB)
	h := newHub()

	rec := sampleRecord("dup-game")
	if err := saveRecord(rec); err != nil { // row already durable
		t.Fatal(err)
	}
	if err := spool.Spool(rec); err != nil { // stale spooled copy of the same id
		t.Fatal(err)
	}

	h.replayOutbox()

	if spool.depth() != 0 {
		t.Fatalf("stale spool file not removed: depth=%d", spool.depth())
	}
	var count int
	if err := db.QueryRow("SELECT COUNT(*) FROM games WHERE id = ?", rec.ID).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Fatalf("replay duplicated a durable row, count=%d", count)
	}
}

// TestTerminalTransitionLoggedOnce proves the terminal transition is idempotent
// and separate from persistence: a second signal keeps the first winner/reason
// and does not re-fire the transition.
func TestTerminalTransitionLoggedOnce(t *testing.T) {
	h := newHub()
	game := persistenceTestGame("once-game", persistenceTestUser("p1", "Human"), persistenceTestUser("p2", "OnlineBot"))
	game.Winner = 1

	if !h.markTerminal(game, "normal") {
		t.Fatal("first terminal transition should report true")
	}
	if h.markTerminal(game, "disconnect") {
		t.Fatal("second terminal signal must not re-fire the transition")
	}
	if game.persistenceTermination != "normal" {
		t.Fatalf("first termination reason not preserved: %q", game.persistenceTermination)
	}
}

// TestOutboxDurableWriteOrdering proves the exact crash-safe sequence:
// temp create -> write -> file fsync -> close -> atomic rename -> directory fsync.
func TestOutboxDurableWriteOrdering(t *testing.T) {
	InitDB(filepath.Join(t.TempDir(), "games.db"))
	t.Cleanup(closePersistenceTestDB)
	rec := installRecordingFS(t)

	if err := spool.Spool(sampleRecord("ordering")); err != nil {
		t.Fatalf("spool: %v", err)
	}
	want := []string{"createtemp", "write", "file.sync", "close", "rename", "dir.sync"}
	if !reflect.DeepEqual(rec.ops, want) {
		t.Fatalf("durability ordering = %v, want %v", rec.ops, want)
	}
}

// TestOutboxWriteFailurePropagation proves a failure at any durability step is
// propagated (so custody is retained), never swallowed.
func TestOutboxWriteFailurePropagation(t *testing.T) {
	for _, step := range []string{"write", "file.sync", "rename", "dir.sync"} {
		t.Run(step, func(t *testing.T) {
			InitDB(filepath.Join(t.TempDir(), "games.db"))
			t.Cleanup(closePersistenceTestDB)
			rec := installRecordingFS(t)
			rec.failAt = step

			err := spool.Spool(sampleRecord("fail-" + step))
			if err == nil {
				t.Fatalf("failure at %q was not propagated", step)
			}
			// A durability failure must leave persistTerminal reporting not-durable
			// so the in-memory game is retained.
			h := newHub()
			game := persistenceTestGame("retain-"+step, persistenceTestUser("p1", "H"), persistenceTestUser("p2", "B"))
			game.reserved = true
			good := db
			db = nil
			if h.persistTerminal(game, "normal") {
				t.Fatalf("persistTerminal claimed durability despite %q failure", step)
			}
			if !game.reserved {
				t.Fatal("reservation released despite non-durable custody")
			}
			db = good
		})
	}
}

// TestOutboxTempRecoveryPromotesAndQuarantines proves crash-orphaned temp files
// (fsynced before rename) are recovered: valid ones promoted, invalid quarantined,
// with a directory sync after the quarantine move.
func TestOutboxTempRecoveryPromotesAndQuarantines(t *testing.T) {
	InitDB(filepath.Join(t.TempDir(), "games.db"))
	t.Cleanup(closePersistenceTestDB)
	h := newHub()

	// A valid orphaned temp (as if crash happened after fsync, before rename).
	valid, err := os.CreateTemp(spool.dir, spoolTempPattern)
	if err != nil {
		t.Fatal(err)
	}
	data, _ := json.Marshal(sampleRecord("promoted-game"))
	if _, err := valid.Write(data); err != nil {
		t.Fatal(err)
	}
	valid.Close()

	// An invalid orphaned temp.
	invalid, err := os.CreateTemp(spool.dir, spoolTempPattern)
	if err != nil {
		t.Fatal(err)
	}
	invalid.WriteString("{garbage")
	invalid.Close()

	rec := installRecordingFS(t)
	h.replayOutbox()

	// Valid temp promoted to a .json and then committed by replay.
	var count int
	if err := db.QueryRow("SELECT COUNT(*) FROM games WHERE id = ?", "promoted-game").Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Fatalf("valid orphaned temp not recovered+committed, count=%d", count)
	}
	// Invalid temp quarantined, and a dir sync followed the quarantine move.
	if spool.quarantineDepth() < 1 {
		t.Fatal("invalid orphaned temp not quarantined")
	}
	if !containsOp(rec.ops, "dir.sync") {
		t.Fatalf("no directory sync recorded during recovery: %v", rec.ops)
	}
}

// TestQuarantinePersistsAcrossRestart proves quarantine depth is a persistent
// degraded state: it is recounted on restart and keeps status unhealthy.
func TestQuarantinePersistsAcrossRestart(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "games.db")
	InitDB(dbPath)

	// Quarantine a corrupt record.
	if err := os.WriteFile(filepath.Join(spool.dir, "bad.json"), []byte("{nope"), 0o644); err != nil {
		t.Fatal(err)
	}
	newHub().replayOutbox()
	if spool.quarantineDepth() != 1 {
		t.Fatalf("expected 1 quarantined, got %d", spool.quarantineDepth())
	}

	// Restart: the quarantined record (a lost game) must still be counted and the
	// status must remain unhealthy — never silently forgotten.
	_ = db.Close()
	db = nil
	InitDB(dbPath)
	t.Cleanup(closePersistenceTestDB)
	if got := persistHealth.snapshot(); got.QuarantineDepth != 1 || got.PersistStatus != "unhealthy" {
		t.Fatalf("quarantine not persistent across restart: depth=%d status=%q", got.QuarantineDepth, got.PersistStatus)
	}
}

// TestGameAdmissionChallengePathBackpressureAndRecovery covers the 1v1 creation
// path: at capacity a new challenge is refused with a user-visible message and no
// game state is allocated; admission recovers once the outbox drains.
func TestGameAdmissionChallengePathBackpressureAndRecovery(t *testing.T) {
	InitDB(filepath.Join(t.TempDir(), "games.db"))
	t.Cleanup(closePersistenceTestDB)
	prev := outboxMaxFiles
	outboxMaxFiles = 1
	t.Cleanup(func() { outboxMaxFiles = prev })

	h := newHub()
	from := admissionTestUser(h, "from", "From")
	to := admissionTestUser(h, "to", "To")
	h.challenges["c1"] = &Challenge{ID: "c1", FromUser: from, ToUser: to, Rows: 2, Cols: 2, Timestamp: time.Now()}
	h.handleAcceptChallenge(to, &Message{ChallengeID: "c1"})
	if len(h.games) != 1 {
		t.Fatalf("first game not admitted: games=%d", len(h.games))
	}
	var g1 *Game
	for _, g := range h.games {
		g1 = g
	}
	if !g1.reserved {
		t.Fatal("admitted game did not hold a reservation")
	}

	// Second challenge at capacity -> refused before allocating any game state.
	a := admissionTestUser(h, "a", "A")
	b := admissionTestUser(h, "b", "B")
	drainClient(a.Client)
	drainClient(b.Client)
	h.challenges["c2"] = &Challenge{ID: "c2", FromUser: a, ToUser: b, Rows: 2, Cols: 2, Timestamp: time.Now()}
	h.handleAcceptChallenge(b, &Message{ChallengeID: "c2"})
	if len(h.games) != 1 {
		t.Fatalf("refused game was allocated anyway: games=%d", len(h.games))
	}
	if !sawErrorMessage(b.Client, gameAdmissionRefusedMessage) {
		t.Fatal("refused user received no visible message")
	}

	// game1 ends while the DB is down -> spooled; the file still occupies the slot.
	good := db
	db = nil
	if !h.persistTerminal(g1, "normal") {
		t.Fatal("terminal record not made durable via outbox")
	}
	db = good
	if g1.reserved {
		t.Fatal("reservation not released after durable custody")
	}
	if spool.Reserve() {
		spool.release()
		t.Fatal("admission recovered before the spooled record drained (bound violated)")
	}

	// Recovery: draining the outbox restores admission capacity.
	h.replayOutbox()
	if !spool.Reserve() {
		t.Fatal("admission did not recover after the outbox drained")
	}
	spool.release()
}

// TestGameAdmissionMultiplayerPathRefused covers the lobby/multiplayer creation
// path: with no capacity the game is not created and every human is told why.
func TestGameAdmissionMultiplayerPathRefused(t *testing.T) {
	InitDB(filepath.Join(t.TempDir(), "games.db"))
	t.Cleanup(closePersistenceTestDB)
	prev := outboxMaxFiles
	outboxMaxFiles = 0 // no capacity at all
	t.Cleanup(func() { outboxMaxFiles = prev })

	h := newHub()
	host := admissionTestUser(h, "host", "Host")
	p2 := admissionTestUser(h, "p2", "P2")
	lobby := &Lobby{
		ID: "lob", Host: host, MaxPlayers: 2, Rows: 2, Cols: 2,
		Players: [4]*LobbyPlayer{
			{User: host, Symbol: "X", Index: 0},
			{User: p2, Symbol: "O", Index: 1},
		},
	}
	drainClient(host.Client)
	drainClient(p2.Client)

	h.createMultiplayerGame(lobby)

	if len(h.games) != 0 {
		t.Fatalf("multiplayer game admitted with no capacity: games=%d", len(h.games))
	}
	if !sawErrorMessage(host.Client, gameAdmissionRefusedMessage) || !sawErrorMessage(p2.Client, gameAdmissionRefusedMessage) {
		t.Fatal("lobby players were not told the game was refused")
	}
}

func admissionTestUser(h *Hub, id, name string) *User {
	u := persistenceTestUser(id, name)
	u.InGame = false
	u.GameID = ""
	h.users[u.ID] = u
	h.clients[u.Client] = true
	return u
}

func drainClient(c *Client) {
	for {
		select {
		case <-c.send:
		default:
			return
		}
	}
}

func sawErrorMessage(c *Client, want string) bool {
	for {
		select {
		case raw := <-c.send:
			var msg Message
			if err := json.Unmarshal(raw, &msg); err != nil {
				continue
			}
			if msg.Type == "error" && msg.Username == want {
				return true
			}
		default:
			return false
		}
	}
}

func containsOp(ops []string, want string) bool {
	for _, op := range ops {
		if op == want {
			return true
		}
	}
	return false
}
