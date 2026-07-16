package main

import (
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// terminalRecord is the immutable, self-contained snapshot of one completed
// game. It carries everything saveRecord needs so durability never depends on
// live *Game state that may be mutated or freed after the terminal transition.
// time.Time fields round-trip losslessly through JSON (RFC3339Nano).
type terminalRecord struct {
	ID           string    `json:"id"`
	StartedAt    time.Time `json:"started_at"`
	EndedAt      time.Time `json:"ended_at"`
	Rows         int       `json:"rows"`
	Cols         int       `json:"cols"`
	Player1Name  string    `json:"player1_name"`
	Player2Name  string    `json:"player2_name"`
	Player3Name  string    `json:"player3_name"`
	Player4Name  string    `json:"player4_name"`
	Result       int       `json:"result"`
	Termination  string    `json:"termination"`
	PGNContent   string    `json:"pgn_content"`
	RejectedJSON string    `json:"rejected_attempt,omitempty"`
}

// outboxMaxFiles bounds durable disk usage AND the number of concurrently
// admitted active games (files + reservations <= cap). var, not const, so tests
// can shrink it. Terminal records are tiny; this cap is only reached under a
// catastrophic sustained database outage, at which point new games are refused.
var outboxMaxFiles = 10000

// spoolFile is the subset of *os.File the spool needs; abstracting it lets tests
// prove the write->sync->close->rename->dir-sync ordering and inject failures.
type spoolFile interface {
	Write(p []byte) (int, error)
	Sync() error
	Close() error
	Name() string
}

// fileSystem abstracts the durability-critical operations. The production osFS
// performs real fsyncs; a fake proves ordering and failure propagation in tests.
type fileSystem interface {
	MkdirAll(path string, perm os.FileMode) error
	CreateTemp(dir, pattern string) (spoolFile, error)
	Rename(oldpath, newpath string) error
	Remove(name string) error
	Stat(name string) (os.FileInfo, error)
	ReadDir(name string) ([]os.DirEntry, error)
	ReadFile(name string) ([]byte, error)
	SyncDir(dir string) error
}

type osFS struct{}

func (osFS) MkdirAll(p string, m os.FileMode) error    { return os.MkdirAll(p, m) }
func (osFS) Rename(a, b string) error                  { return os.Rename(a, b) }
func (osFS) Remove(n string) error                     { return os.Remove(n) }
func (osFS) Stat(n string) (os.FileInfo, error)        { return os.Stat(n) }
func (osFS) ReadDir(n string) ([]os.DirEntry, error)   { return os.ReadDir(n) }
func (osFS) ReadFile(n string) ([]byte, error)         { return os.ReadFile(n) }
func (osFS) CreateTemp(d, p string) (spoolFile, error) { return os.CreateTemp(d, p) }

// SyncDir fsyncs a directory so a rename/create/remove of one of its entries is
// durable across a crash. Directory fsync is the POSIX way to persist the entry.
func (osFS) SyncDir(dir string) error {
	d, err := os.Open(dir)
	if err != nil {
		return err
	}
	if err := d.Sync(); err != nil {
		_ = d.Close()
		return err
	}
	return d.Close()
}

// outbox is a crash-durable spool on the SAME mounted volume as the SQLite
// database. It deliberately does NOT live inside the database: when an INSERT
// fails because the DB is unavailable, a SQLite outbox table would be unusable
// too, so records are spooled as atomically-renamed, fsynced files instead.
type outbox struct {
	mu            sync.Mutex
	fs            fileSystem
	dir           string
	parent        string // dir that contains o.dir, fsynced after mkdir
	quarantineDir string
	reserved      int // admitted active games holding a not-yet-durable custody slot
	quarantined   int // corrupt/unrecoverable records = lost games (persistent)
	initialized   bool
}

var spool = &outbox{}

const spoolTempPattern = ".spool-*.tmp"

// init points the spool at <dbDir>/outbox, fsyncing parent directories so the
// created directory entries survive a crash. Safe to call repeatedly.
func (o *outbox) init(dbDir string) {
	o.mu.Lock()
	defer o.mu.Unlock()
	if o.fs == nil {
		o.fs = osFS{}
	}
	o.parent = dbDir
	o.dir = filepath.Join(dbDir, "outbox")
	o.quarantineDir = filepath.Join(o.dir, "quarantine")
	// Fail-closed: the outbox is unavailable until EVERY durability step confirms.
	// If any step fails we cannot guarantee terminal custody, so Reserve refuses
	// admission and diagnostics report the outbox as unavailable.
	o.initialized = false
	if err := o.fs.MkdirAll(o.quarantineDir, 0o755); err != nil {
		log.Printf("event=outbox_init_error step=mkdir dir=%q error=%q", o.dir, err.Error())
		persistHealth.setOutboxAvailable(false)
		return
	}
	// The new directory entries must be persisted before the outbox is usable;
	// an unsynced entry can vanish on power loss, losing spooled records.
	if err := o.fs.SyncDir(o.parent); err != nil {
		log.Printf("event=outbox_init_error step=parent_sync dir=%q error=%q", o.parent, err.Error())
		persistHealth.setOutboxAvailable(false)
		return
	}
	if err := o.fs.SyncDir(o.dir); err != nil {
		log.Printf("event=outbox_init_error step=dir_sync dir=%q error=%q", o.dir, err.Error())
		persistHealth.setOutboxAvailable(false)
		return
	}
	o.initialized = true
	persistHealth.setOutboxAvailable(true)
	// A fresh process holds no active games, so no reservations are outstanding.
	o.reserved = 0
	// A restart inherits any quarantined (lost) records: recount them so the
	// degraded/unhealthy state is not silently forgotten across restarts.
	o.quarantined = o.countQuarantineLocked()
	persistHealth.setQuarantineDepth(o.quarantined)
	// Promote any temp files left after a crash between fsync and rename.
	o.recoverTempFilesLocked()
}

func (o *outbox) recordPath(id string) string {
	// game IDs are UUIDs; sanitize defensively so a record can never escape dir.
	safe := strings.ReplaceAll(filepath.Base(id), string(os.PathSeparator), "_")
	return filepath.Join(o.dir, safe+".json")
}

// Reserve admits one active game by reserving a durable custody slot, keeping
// files + reservations <= cap. It is the real application backpressure: when it
// returns false the caller must refuse to create the game. Runs on the hub
// goroutine.
func (o *outbox) Reserve() bool {
	o.mu.Lock()
	defer o.mu.Unlock()
	if !o.initialized {
		// Fail-closed: production always calls InitDB, so an uninitialized outbox
		// here means a durability step failed. Without a store that can guarantee
		// terminal custody we must refuse admission rather than admit a game we
		// could not persist. (Package tests establish a healthy outbox in TestMain.)
		return false
	}
	if o.countLocked()+o.reserved >= outboxMaxFiles {
		return false
	}
	o.reserved++
	return true
}

// release frees a reservation once the game's terminal record is durable
// (committed to the DB, or spooled to a file that now occupies the slot).
func (o *outbox) release() {
	o.mu.Lock()
	if o.reserved > 0 {
		o.reserved--
	}
	o.mu.Unlock()
}

// Spool writes the record durably: temp file -> write -> fsync file -> close ->
// atomic rename -> fsync directory. Any failure is propagated so the caller
// retains in-memory custody. An already-admitted game always fits (its slot was
// reserved), so Spool does not re-check the cap.
func (o *outbox) Spool(rec terminalRecord) error {
	o.mu.Lock()
	defer o.mu.Unlock()
	return o.spoolLocked(rec)
}

func (o *outbox) spoolLocked(rec terminalRecord) error {
	if !o.initialized {
		return fmt.Errorf("outbox not initialized")
	}
	data, err := json.Marshal(rec)
	if err != nil {
		return fmt.Errorf("marshal terminal record: %w", err)
	}
	return o.writeFileLocked(o.recordPath(rec.ID), data)
}

// writeFileLocked performs the durable write sequence and propagates every
// failure. The temp file is removed on any pre-rename failure.
func (o *outbox) writeFileLocked(final string, data []byte) error {
	tmp, err := o.fs.CreateTemp(o.dir, spoolTempPattern)
	if err != nil {
		return fmt.Errorf("outbox create temp: %w", err)
	}
	tmpName := tmp.Name()
	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		_ = o.fs.Remove(tmpName)
		return fmt.Errorf("outbox write: %w", err)
	}
	if err := tmp.Sync(); err != nil { // file bytes durable before rename
		_ = tmp.Close()
		_ = o.fs.Remove(tmpName)
		return fmt.Errorf("outbox sync: %w", err)
	}
	if err := tmp.Close(); err != nil {
		_ = o.fs.Remove(tmpName)
		return fmt.Errorf("outbox close: %w", err)
	}
	if err := o.fs.Rename(tmpName, final); err != nil {
		_ = o.fs.Remove(tmpName)
		return fmt.Errorf("outbox rename: %w", err)
	}
	if err := o.fs.SyncDir(o.dir); err != nil { // rename durable
		return fmt.Errorf("outbox dir sync: %w", err)
	}
	return nil
}

// discard removes a spooled record for id if present (used once the games row is
// durably committed via a live path, so replay never inserts a duplicate).
func (o *outbox) discard(id string) {
	o.mu.Lock()
	defer o.mu.Unlock()
	if !o.initialized {
		return
	}
	if err := o.fs.Remove(o.recordPath(id)); err == nil {
		_ = o.fs.SyncDir(o.dir)
	}
}

func (o *outbox) depth() int {
	o.mu.Lock()
	defer o.mu.Unlock()
	return o.countLocked()
}

func (o *outbox) quarantineDepth() int {
	o.mu.Lock()
	defer o.mu.Unlock()
	return o.quarantined
}

func (o *outbox) countLocked() int {
	return o.countJSONLocked(o.dir)
}

// countQuarantineLocked counts EVERY regular file in the quarantine directory,
// regardless of extension. Quarantined records keep their original name (a
// corrupt ".json" or an invalid ".spool-*.tmp"), so counting only ".json" would
// silently forget invalid temp files across a restart and read as healthy.
func (o *outbox) countQuarantineLocked() int {
	entries, err := o.fs.ReadDir(o.quarantineDir)
	if err != nil {
		return 0
	}
	n := 0
	for _, e := range entries {
		if !e.IsDir() {
			n++
		}
	}
	return n
}

func (o *outbox) countJSONLocked(dir string) int {
	if !o.initialized && dir == o.dir {
		return 0
	}
	entries, err := o.fs.ReadDir(dir)
	if err != nil {
		return 0
	}
	n := 0
	for _, e := range entries {
		if !e.IsDir() && strings.HasSuffix(e.Name(), ".json") {
			n++
		}
	}
	return n
}

// recoverTempFilesLocked promotes crash-orphaned temp files: a valid record that
// was fsynced but not yet renamed is promoted to its final name; anything
// unparseable is quarantined. Idempotent and safe to call on init/replay.
func (o *outbox) recoverTempFilesLocked() {
	entries, err := o.fs.ReadDir(o.dir)
	if err != nil {
		return
	}
	for _, e := range entries {
		if e.IsDir() || !strings.HasPrefix(e.Name(), ".spool-") || !strings.HasSuffix(e.Name(), ".tmp") {
			continue
		}
		path := filepath.Join(o.dir, e.Name())
		data, err := o.fs.ReadFile(path)
		if err != nil {
			continue
		}
		var rec terminalRecord
		if err := json.Unmarshal(data, &rec); err != nil || rec.ID == "" {
			o.quarantineLocked(path, e.Name())
			continue
		}
		if err := o.fs.Rename(path, o.recordPath(rec.ID)); err != nil {
			// Promotion failed: preserve the source temp (retryable custody) and
			// surface the degraded state — never hide or lose the record.
			log.Printf("event=outbox_temp_promote_error file=%q error=%q", e.Name(), err.Error())
			persistHealth.markOutboxDegraded()
			continue
		}
		if err := o.fs.SyncDir(o.dir); err != nil {
			// Rename happened but is not confirmed durable: do not swallow it. The
			// promoted .json is a valid record replay will still commit.
			log.Printf("event=outbox_temp_promote_sync_error file=%q error=%q", e.Name(), err.Error())
			persistHealth.markOutboxDegraded()
		}
		log.Printf("event=outbox_temp_promoted game=%s", rec.ID)
	}
}

// Replay commits every spooled record. A record is removed only after its games
// row is durable (fresh insert OR already committed), so a game is never
// duplicated and never lost. Corrupt records are quarantined, not retried
// forever. It also promotes crash-orphaned temp files first.
func (o *outbox) Replay(commit func(terminalRecord) error, durable func(id string) (bool, error)) {
	o.mu.Lock()
	defer o.mu.Unlock()
	if !o.initialized {
		return
	}
	o.recoverTempFilesLocked()
	entries, err := o.fs.ReadDir(o.dir)
	if err != nil {
		return
	}
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".json") {
			continue
		}
		path := filepath.Join(o.dir, e.Name())
		data, err := o.fs.ReadFile(path)
		if err != nil {
			continue // transient read error; retry next cycle
		}
		var rec terminalRecord
		if err := json.Unmarshal(data, &rec); err != nil || rec.ID == "" {
			o.quarantineLocked(path, e.Name())
			continue
		}
		if err := commit(rec); err != nil {
			// Distinguish "already durable" (remove, no duplicate) from a
			// transient outage (leave the file for the next cycle).
			ok, derr := durable(rec.ID)
			if derr == nil && ok {
				o.removeLocked(path)
				log.Printf("event=outbox_replay_already_durable game=%s", rec.ID)
				continue
			}
			log.Printf("event=outbox_replay_deferred game=%s category=%s", rec.ID, categorizePersistError(err))
			continue
		}
		o.removeLocked(path)
		log.Printf("event=outbox_replay_committed game=%s", rec.ID)
	}
	persistHealth.setOutboxDepth(o.countLocked())
	persistHealth.setQuarantineDepth(o.quarantined)
}

func (o *outbox) removeLocked(path string) {
	if err := o.fs.Remove(path); err == nil {
		_ = o.fs.SyncDir(o.dir)
	}
}

func (o *outbox) quarantineLocked(path, name string) {
	dest := filepath.Join(o.quarantineDir, name)
	if err := o.fs.Rename(path, dest); err != nil {
		// Could not quarantine: preserve the source (retryable) and mark degraded.
		log.Printf("event=outbox_quarantine_error file=%q error=%q", name, err.Error())
		persistHealth.markOutboxDegraded()
		return
	}
	if err := o.fs.SyncDir(o.dir); err != nil { // source lost an entry
		log.Printf("event=outbox_quarantine_sync_error dir=source file=%q error=%q", name, err.Error())
		persistHealth.markOutboxDegraded()
	}
	if err := o.fs.SyncDir(o.quarantineDir); err != nil { // destination gained one
		log.Printf("event=outbox_quarantine_sync_error dir=dest file=%q error=%q", name, err.Error())
		persistHealth.markOutboxDegraded()
	}
	o.quarantined++
	persistHealth.setQuarantineDepth(o.quarantined)
	log.Printf("event=outbox_quarantined file=%q total=%d", name, o.quarantined)
}
