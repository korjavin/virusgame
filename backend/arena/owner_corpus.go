package arena

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"sort"
	"strings"

	"virusgame/game"
)

// FixtureName is the established immutable-fixture filename for a game.
func FixtureName(rows, cols int, termination, sourceID string) string {
	return fmt.Sprintf("production-%dx%d-%s-%s.json", rows, cols, strings.ReplaceAll(termination, "_", "-"), sourceID)
}

// OwnerCorpusManifest is the pinned index of the owner-loss regression corpus:
// every 1v1 game a human won against the bot, harvested from /last_games. Each
// human win is a proven hole. The manifest is the data-form allowlist and
// fingerprint pin (kept out of Go source so `replayimport -fetch` can extend it
// in one command without editing test code). Entries are sorted by SourceID for
// deterministic output.
const OwnerCorpusManifest = "testdata/owner-corpus.json"

// OwnerCorpusEntry pins one harvested human-win fixture: its identity and the
// reconstructed terminal-position fingerprint (position, not moves — robust to
// eval changes, sensitive to rules/replay drift).
type OwnerCorpusEntry struct {
	SourceID            string    `json:"source_id"`
	Players             [2]string `json:"players"`
	Rows                int       `json:"rows"`
	Cols                int       `json:"cols"`
	Termination         string    `json:"termination"`
	TerminalFingerprint string    `json:"terminal_fingerprint"`
}

// FixtureName is the established testdata name for this entry.
func (e OwnerCorpusEntry) FixtureName() string {
	return FixtureName(e.Rows, e.Cols, e.Termination, e.SourceID)
}

// LoadOwnerCorpus reads the manifest, returning an empty slice if it does not
// exist yet.
func LoadOwnerCorpus(path string) ([]OwnerCorpusEntry, error) {
	data, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var entries []OwnerCorpusEntry
	if err := json.Unmarshal(data, &entries); err != nil {
		return nil, err
	}
	return entries, nil
}

// SaveOwnerCorpus writes the manifest sorted by SourceID with a trailing
// newline — deterministic regardless of harvest order.
func SaveOwnerCorpus(path string, entries []OwnerCorpusEntry) error {
	sort.Slice(entries, func(i, j int) bool { return entries[i].SourceID < entries[j].SourceID })
	encoded, err := json.MarshalIndent(entries, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, append(encoded, '\n'), 0o644)
}

// StateFingerprint is the stable position fingerprint used to pin terminal
// states (sha256 of the wire snapshot, first 8 bytes hex). Test helpers and the
// harvest tool share it so pinned hashes are computed identically.
func StateFingerprint(state game.State) (string, error) {
	encoded, err := json.Marshal(state.Snapshot())
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(encoded)
	return hex.EncodeToString(sum[:8]), nil
}
