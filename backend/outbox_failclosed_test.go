package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// TestMain establishes a healthy, initialized outbox (and DB) before any test
// runs. Game creation now requires a reservable durable custody slot, so a
// healthy default is needed for the many tests that create games without calling
// InitDB themselves. Tests that exercise failure/uninitialized states install
// their own filesystem and restore a healthy outbox via t.Cleanup.
func TestMain(m *testing.M) {
	dir, err := os.MkdirTemp("", "virusgame-testmain-*")
	if err != nil {
		panic(err)
	}
	InitDB(filepath.Join(dir, "games.db"))
	code := m.Run()
	if db != nil {
		_ = db.Close()
	}
	_ = os.RemoveAll(dir)
	os.Exit(code)
}

// initFailFS injects a failure at a specific outbox-init durability step so the
// fail-closed behavior can be proven.
type initFailFS struct {
	osFS
	failMkdir      bool
	failParentSync bool
	failOutboxSync bool
}

func (f initFailFS) MkdirAll(p string, m os.FileMode) error {
	if f.failMkdir {
		return fmt.Errorf("injected mkdir failure")
	}
	return os.MkdirAll(p, m)
}

func (f initFailFS) SyncDir(dir string) error {
	base := filepath.Base(dir)
	if f.failOutboxSync && base == "outbox" {
		return fmt.Errorf("injected outbox dir sync failure")
	}
	if f.failParentSync && base != "outbox" && base != "quarantine" {
		return fmt.Errorf("injected parent dir sync failure")
	}
	return osFS{}.SyncDir(dir)
}

func setSpoolFS(fs fileSystem) {
	spool.mu.Lock()
	spool.fs = fs
	spool.mu.Unlock()
}

// TestOutboxInitFailClosedRefusesAdmission proves that a failure at any outbox
// init durability step (mkdir, parent fsync, outbox fsync) leaves the outbox
// unavailable, so Reserve fails closed and BOTH game-creation paths refuse with
// the existing admission message and allocate zero Game — with recovery only
// after a successful re-init.
func TestOutboxInitFailClosedRefusesAdmission(t *testing.T) {
	cases := map[string]initFailFS{
		"mkdir":       {failMkdir: true},
		"parent_sync": {failParentSync: true},
		"outbox_sync": {failOutboxSync: true},
	}
	for name, fs := range cases {
		t.Run(name, func(t *testing.T) {
			dir := t.TempDir()
			// Restore a healthy outbox for later tests regardless of outcome.
			t.Cleanup(func() {
				setSpoolFS(osFS{})
				InitDB(filepath.Join(dir, "restore.db"))
			})

			setSpoolFS(fs)
			InitDB(filepath.Join(dir, "games.db")) // init runs the failing step

			if spool.initialized {
				t.Fatal("outbox marked initialized despite a failed durability step")
			}
			if got := persistHealth.snapshot(); got.PersistStatus == "ok" {
				t.Fatalf("status is ok despite init failure: %+v", got)
			}
			if spool.Reserve() {
				t.Fatal("Reserve admitted a game with an unavailable outbox")
			}

			h := newHub()

			// 1v1 challenge path.
			from := admissionTestUser(h, "ff", "From")
			to := admissionTestUser(h, "tt", "To")
			h.challenges["c1"] = &Challenge{ID: "c1", FromUser: from, ToUser: to, Rows: 2, Cols: 2, Timestamp: time.Now()}
			h.handleAcceptChallenge(to, &Message{ChallengeID: "c1"})
			if len(h.games) != 0 {
				t.Fatal("challenge admitted a game with an unavailable outbox")
			}
			if !sawErrorMessage(to.Client, gameAdmissionRefusedMessage) {
				t.Fatal("challenged user received no admission error")
			}

			// Lobby/multiplayer path.
			host := admissionTestUser(h, "hh", "Host")
			p2 := admissionTestUser(h, "pp", "P2")
			lobby := &Lobby{
				ID: "lb", Host: host, MaxPlayers: 2, Rows: 2, Cols: 2,
				Players: [4]*LobbyPlayer{{User: host, Index: 0}, {User: p2, Index: 1}},
			}
			drainClient(host.Client)
			drainClient(p2.Client)
			h.createMultiplayerGame(lobby)
			if len(h.games) != 0 {
				t.Fatal("multiplayer admitted a game with an unavailable outbox")
			}
			if !sawErrorMessage(host.Client, gameAdmissionRefusedMessage) {
				t.Fatal("lobby host received no admission error")
			}

			// Recovery only after a successful re-init.
			setSpoolFS(osFS{})
			InitDB(filepath.Join(dir, "recovered.db"))
			if !spool.Reserve() {
				t.Fatal("admission did not recover after a successful re-init")
			}
			spool.release()
		})
	}
}

// TestQuarantineInvalidTempSurvivesRestart proves an invalid crash-orphaned temp
// file that gets quarantined (keeping its .tmp name) is still counted after a
// restart, so the health status stays non-ok rather than falsely reading healthy.
func TestQuarantineInvalidTempSurvivesRestart(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "games.db")
	InitDB(dbPath)

	// A truncated/unparseable crash-orphaned temp file.
	if err := os.WriteFile(filepath.Join(spool.dir, ".spool-bad.tmp"), []byte("{trunc"), 0o644); err != nil {
		t.Fatal(err)
	}
	newHub().replayOutbox() // recovery quarantines the invalid temp (keeps .tmp name)
	if spool.quarantineDepth() != 1 {
		t.Fatalf("invalid temp not quarantined: depth=%d", spool.quarantineDepth())
	}

	// Restart: the .tmp-named quarantine entry must still be counted (a suffix
	// filter would have dropped it and falsely reported healthy).
	_ = db.Close()
	db = nil
	InitDB(dbPath)
	t.Cleanup(closePersistenceTestDB)

	if spool.quarantineDepth() != 1 {
		t.Fatalf("quarantined .tmp lost across restart: depth=%d", spool.quarantineDepth())
	}
	got := persistHealth.snapshot()
	if got.QuarantineDepth != 1 || got.PersistStatus == "ok" {
		t.Fatalf("quarantine not reflected as non-healthy across restart: %+v", got)
	}
}

// TestPromotionSyncFailureMarksUnhealthy proves a directory-fsync failure during
// temp promotion is not swallowed: it marks the outbox unhealthy even though the
// record is promoted (not quarantined), so an unconfirmed durability op never
// reads as healthy closure.
func TestPromotionSyncFailureMarksUnhealthy(t *testing.T) {
	dir := t.TempDir()
	InitDB(filepath.Join(dir, "games.db"))
	// Restore a healthy (non-degraded) outbox for later tests: swap the real FS
	// back BEFORE re-initializing so init's own syncs succeed and clear the flag.
	t.Cleanup(func() {
		setSpoolFS(osFS{})
		InitDB(filepath.Join(dir, "restore.db"))
		closePersistenceTestDB()
	})

	// A VALID crash-orphaned temp: recovery promotes it (rename succeeds), but the
	// post-promote directory fsync fails. It must NOT be quarantined, yet the
	// unconfirmed durability must surface as unhealthy.
	data, err := json.Marshal(sampleRecord("promote-syncfail"))
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(spool.dir, ".spool-x.tmp"), data, 0o644); err != nil {
		t.Fatal(err)
	}
	setSpoolFS(&recordingFS{failErr: errors.New("injected dir sync failure"), failAt: "dir.sync"})

	newHub().replayOutbox()

	got := persistHealth.snapshot()
	if got.QuarantineDepth != 0 {
		t.Fatalf("valid temp was quarantined instead of promoted: depth=%d", got.QuarantineDepth)
	}
	if got.PersistStatus != "unhealthy" {
		t.Fatalf("promotion dir-sync failure not surfaced as unhealthy: status=%q", got.PersistStatus)
	}
}
