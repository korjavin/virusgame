package arena

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestProductionMotifsAreFrozenAnnotatedReplayPositions(t *testing.T) {
	replays := make(map[string]Replay)
	paths, err := filepath.Glob("testdata/*.json")
	if err != nil {
		t.Fatal(err)
	}
	for _, path := range paths {
		base := filepath.Base(path)
		if base == "production-motifs-v1.json" || base == "strength-corpus-v1.json" {
			continue
		}
		fixture, err := os.Open(path)
		if err != nil {
			t.Fatal(err)
		}
		replay, _, decodeErr := DecodeReplay(fixture)
		fixture.Close()
		if decodeErr != nil {
			t.Fatalf("%s: %v", path, decodeErr)
		}
		replays[replay.SourceID] = replay
	}
	fixture, err := os.Open("testdata/production-motifs-v1.json")
	if err != nil {
		t.Fatal(err)
	}
	manifest, err := DecodeMotifs(fixture, replays)
	fixture.Close()
	if err != nil {
		t.Fatal(err)
	}
	wantTags := []string{"consolidation", "backup_route", "useful_redundant_corridor", "thin_tendril", "base_rooted_vertex_disjoint_corridors", "min_cut_le_3", "attack_chain_counter_capture", "translated_motif", "reflected_motif", "capturable_base_halo", "preserved_empty_escape", "hardened_enemy_foothold", "opponent_base_siege_decision", "neutral_denial_candidate", "avoid_capturable_normal_foothold"}
	allTags := ""
	polarities := map[string]int{}
	for _, moment := range manifest.Moments {
		polarities[moment.Polarity]++
		allTags += " " + strings.Join(moment.Tags, " ")
	}
	for _, tag := range wantTags {
		if !strings.Contains(allTags, tag) {
			t.Fatalf("motif manifest lacks %q: %s", tag, allTags)
		}
	}
	if len(manifest.Moments) < 12 || polarities["positive"] == 0 || polarities["negative"] == 0 {
		t.Fatalf("motif polarity coverage is incomplete: moments=%d polarities=%v", len(manifest.Moments), polarities)
	}
}
