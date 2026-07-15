package arena

import (
	"encoding/json"
	"fmt"
	"io"
	"sort"
)

type MotifManifest struct {
	Version string        `json:"version"`
	Moments []MotifMoment `json:"moments"`
}

type MotifMoment struct {
	ID           string   `json:"id"`
	SourceID     string   `json:"source_id"`
	Hash         string   `json:"hash"`
	Polarity     string   `json:"polarity"`
	Pair         string   `json:"pair,omitempty"`
	Turn         int      `json:"turn"`
	AfterActions int      `json:"after_actions"`
	Tags         []string `json:"tags"`
}

// DecodeMotifs validates stable annotated production positions against their
// authoritative replay prefixes. Replays are keyed by SourceID.
func DecodeMotifs(reader io.Reader, replays map[string]Replay) (MotifManifest, error) {
	var manifest MotifManifest
	decoder := json.NewDecoder(reader)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&manifest); err != nil {
		return MotifManifest{}, fmt.Errorf("decode motifs: %w", err)
	}
	if err := decoder.Decode(&struct{}{}); err != io.EOF || manifest.Version != "production-motifs-v1" {
		return MotifManifest{}, fmt.Errorf("decode motifs: trailing content or version")
	}
	seen, pairs := map[string]bool{}, map[string]int{}
	for _, moment := range manifest.Moments {
		if moment.ID == "" || seen[moment.ID] || (moment.Polarity != "positive" && moment.Polarity != "negative" && moment.Polarity != "neutral") || len(moment.Tags) == 0 {
			return MotifManifest{}, fmt.Errorf("invalid motif %q", moment.ID)
		}
		seen[moment.ID] = true
		replay, ok := replays[moment.SourceID]
		if !ok {
			return MotifManifest{}, fmt.Errorf("motif %s: missing replay %s", moment.ID, moment.SourceID)
		}
		positions, err := ReplayPositions(replay)
		if err != nil {
			return MotifManifest{}, fmt.Errorf("motif %s: %w", moment.ID, err)
		}
		state, ok := positions[ReplayPoint{Turn: moment.Turn, AfterActions: moment.AfterActions}]
		if !ok {
			return MotifManifest{}, fmt.Errorf("motif %s: missing replay point", moment.ID)
		}
		hash, err := SnapshotHash(state.Snapshot())
		if err != nil || len(moment.Hash) != 16 || hash[:16] != moment.Hash {
			return MotifManifest{}, fmt.Errorf("motif %s: hash=%s want=%s err=%v", moment.ID, hash, moment.Hash, err)
		}
		if moment.Pair != "" {
			pairs[moment.Pair]++
		}
	}
	for pair, count := range pairs {
		if count != 2 {
			return MotifManifest{}, fmt.Errorf("motif pair %s has %d members", pair, count)
		}
	}
	sort.Slice(manifest.Moments, func(i, j int) bool { return manifest.Moments[i].ID < manifest.Moments[j].ID })
	return manifest, nil
}
