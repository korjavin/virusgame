package main

import (
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
)

// buildSHA is injected at build time via -ldflags "-X main.buildSHA=<sha>" so a
// stale /last_games answer can be traced back to the exact deployed commit.
var buildSHA = "unknown"

// instanceID identifies this process across restarts and routed replicas. A
// mismatch between the value in a WS welcome and a /last_games header proves the
// endpoint and the game socket were served by different backend instances.
var instanceID = uuid.New().String()

// persistHealth records terminal-persistence outcomes so operators can tell a
// silent write failure from an empty backlog without touching the database.
//
// SECURITY: everything exposed publicly here (unauthenticated /diag and
// /last_games) is non-sensitive: build SHA, instance ID, an opaque random
// database UUID, a categorized error/status, timestamps, and queue depths.
// Absolute paths and verbatim error strings are recorded to the server logs
// only, never surfaced to clients.
var persistHealth persistHealthState

type persistHealthState struct {
	mu                sync.Mutex
	dbID              string // opaque UUID minted inside the mounted database
	outboxUnavailable bool   // init failed a durability step -> fail-closed
	outboxDegraded    bool   // a durability op (dir sync / promote / quarantine) failed
	outboxDepth       int
	quarantineDepth   int    // corrupt/unrecoverable records = lost games (persistent)
	errorCategory     string // categorized, safe to expose (never the raw error)
	lastErrorAt       time.Time
	lastSuccessID     string
	lastSuccessAt     time.Time
}

// setOutboxAvailable records the outcome of outbox initialization. A successful
// init clears any prior degraded flag (fresh, healthy spool); a failed init
// marks the outbox unavailable so admission fails closed.
func (s *persistHealthState) setOutboxAvailable(ok bool) {
	s.mu.Lock()
	s.outboxUnavailable = !ok
	if ok {
		s.outboxDegraded = false
	}
	s.mu.Unlock()
}

// markOutboxDegraded records that a durability operation (a directory sync,
// promotion, or quarantine move) failed. It is not swallowed: it surfaces as an
// unhealthy status until the next successful init.
func (s *persistHealthState) markOutboxDegraded() {
	s.mu.Lock()
	s.outboxDegraded = true
	s.mu.Unlock()
}

func (s *persistHealthState) setDBID(id string) {
	s.mu.Lock()
	s.dbID = id
	s.mu.Unlock()
}

func (s *persistHealthState) recordSuccess(gameID string) {
	s.mu.Lock()
	s.lastSuccessID = gameID
	s.lastSuccessAt = time.Now()
	s.mu.Unlock()
}

// recordFailure stores only a safe error category. The raw error is logged by
// the caller (server logs), never retained for public exposure.
func (s *persistHealthState) recordFailure(err error) {
	s.mu.Lock()
	s.errorCategory = categorizePersistError(err)
	s.lastErrorAt = time.Now()
	s.mu.Unlock()
}

func (s *persistHealthState) setOutboxDepth(depth int) {
	s.mu.Lock()
	s.outboxDepth = depth
	s.mu.Unlock()
}

func (s *persistHealthState) setQuarantineDepth(depth int) {
	s.mu.Lock()
	s.quarantineDepth = depth
	s.mu.Unlock()
}

// categorizePersistError maps an error to a stable, non-sensitive category so
// operators can distinguish an outage from a data bug without leaking details.
func categorizePersistError(err error) string {
	if err == nil {
		return ""
	}
	msg := strings.ToLower(err.Error())
	switch {
	case strings.Contains(msg, "not initialized"), strings.Contains(msg, "database is locked"),
		strings.Contains(msg, "no such file"), strings.Contains(msg, "unable to open"):
		return "db_unavailable"
	case strings.Contains(msg, "constraint"), strings.Contains(msg, "unique"):
		return "constraint"
	case strings.Contains(msg, "encode"), strings.Contains(msg, "marshal"), strings.Contains(msg, "history"):
		return "encode"
	case strings.Contains(msg, "disk"), strings.Contains(msg, "space"), strings.Contains(msg, "i/o"),
		strings.Contains(msg, "spool"), strings.Contains(msg, "outbox"):
		return "io"
	default:
		return "other"
	}
}

// provenance is the safe snapshot shared by /diag, /last_games headers, and the
// WS welcome message. It intentionally contains no path or raw error text.
type provenance struct {
	BuildSHA          string    `json:"build_sha"`
	InstanceID        string    `json:"instance_id"`
	DBID              string    `json:"db_id"`
	PersistStatus     string    `json:"persist_status"`
	OutboxDepth       int       `json:"outbox_depth"`
	QuarantineDepth   int       `json:"quarantine_depth"`
	LastErrorCategory string    `json:"last_error_category,omitempty"`
	LastErrorAt       time.Time `json:"last_error_at,omitempty"`
	LastSuccessID     string    `json:"last_success_game_id,omitempty"`
	LastSuccessAt     time.Time `json:"last_success_at,omitempty"`
}

func (s *persistHealthState) snapshot() provenance {
	s.mu.Lock()
	defer s.mu.Unlock()
	// Fail-closed init is the most severe (no durable custody at all). Quarantined
	// records are lost games and an unconfirmed durability op are both unhealthy;
	// a drainable backlog is merely degraded. None of these read as healthy.
	status := "ok"
	switch {
	case s.outboxUnavailable:
		status = "unavailable"
	case s.quarantineDepth > 0 || s.outboxDegraded:
		status = "unhealthy"
	case s.outboxDepth > 0:
		status = "degraded"
	}
	return provenance{
		BuildSHA:          buildSHA,
		InstanceID:        instanceID,
		DBID:              s.dbID,
		PersistStatus:     status,
		OutboxDepth:       s.outboxDepth,
		QuarantineDepth:   s.quarantineDepth,
		LastErrorCategory: s.errorCategory,
		LastErrorAt:       s.lastErrorAt,
		LastSuccessID:     s.lastSuccessID,
		LastSuccessAt:     s.lastSuccessAt,
	}
}

// setProvenanceHeaders binds instance/build/database identity to any response as
// headers. Additive headers keep existing JSON clients compatible.
func setProvenanceHeaders(w http.ResponseWriter, p provenance) {
	w.Header().Set("X-Build-SHA", p.BuildSHA)
	w.Header().Set("X-Instance-ID", p.InstanceID)
	w.Header().Set("X-DB-Id", p.DBID)
	w.Header().Set("X-Persist-Status", p.PersistStatus)
	w.Header().Set("X-Outbox-Depth", strconv.Itoa(p.OutboxDepth))
	w.Header().Set("X-Quarantine-Depth", strconv.Itoa(p.QuarantineDepth))
	if p.LastSuccessID != "" {
		w.Header().Set("X-Persist-Last-Success-Id", p.LastSuccessID)
		w.Header().Set("X-Persist-Last-Success-At", p.LastSuccessAt.UTC().Format(time.RFC3339))
	}
	if p.LastErrorCategory != "" {
		w.Header().Set("X-Persist-Last-Error-Category", p.LastErrorCategory)
		w.Header().Set("X-Persist-Last-Error-At", p.LastErrorAt.UTC().Format(time.RFC3339))
	}
}

func diagHandler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store")
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		p := persistHealth.snapshot()
		setProvenanceHeaders(w, p)
		_ = json.NewEncoder(w).Encode(p)
	})
}
