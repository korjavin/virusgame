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
		if base == "production-motifs-v1.json" || base == "strength-corpus-v1.json" ||
			base == filepath.Base(OwnerCorpusManifest) {
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
	causalBadActions := map[string]ReplayMove{
		"post-fix-fd-t8-capturable-bridge":   {Kind: "move", Row: 4, Col: 4},
		"post-fix-e854-t8-capturable-bridge": {Kind: "move", Row: 3, Col: 3},
		"post-fix-e7b2-t8-capturable-bridge": {Kind: "move", Row: 3, Col: 3},
		"post-fix-6bf-t8-capturable-bridge":  {Kind: "move", Row: 7, Col: 6},
		"post-fix-6bf-t18-recurrent-bridge":  {Kind: "move", Row: 3, Col: 4},
		"post-fix-4558-t12-base-spine-cut":   {Kind: "move", Row: 2, Col: 1},
		"post-fix-4558-t14-base-spine-cut":   {Kind: "move", Row: 7, Col: 6},
		"post-fix-4558-t18-base-spine-cut":   {Kind: "move", Row: 3, Col: 1},
	}
	for _, moment := range manifest.Moments {
		want, ok := causalBadActions[moment.ID]
		if !ok {
			continue
		}
		if moment.Polarity != "negative" {
			t.Fatalf("%s causal bad action is not a negative predecision motif", moment.ID)
		}
		replay := replays[moment.SourceID]
		actions := replay.Turns[moment.Turn-1].Actions
		if moment.AfterActions < 0 || moment.AfterActions >= len(actions) {
			t.Fatalf("%s has no recorded next action after predecision point %d", moment.ID, moment.AfterActions)
		}
		got := actions[moment.AfterActions]
		if got.Kind != want.Kind || got.Row != want.Row || got.Col != want.Col {
			t.Fatalf("%s causal action=%+v, want %+v", moment.ID, got, want)
		}
		delete(causalBadActions, moment.ID)
	}
	if len(causalBadActions) != 0 {
		t.Fatalf("missing post-fix causal moments: %v", causalBadActions)
	}
	losingSequence := replays["4558d2fe-c22f-4940-8012-8f4f43fac728"].Turns[17].Actions
	wantLosingSequence := []ReplayMove{{Kind: "move", Row: 3, Col: 1}, {Kind: "move", Row: 2, Col: 0}, {Kind: "move", Row: 0, Col: 1}}
	if len(losingSequence) != len(wantLosingSequence) {
		t.Fatalf("4558 T18 actions=%+v, want %+v", losingSequence, wantLosingSequence)
	}
	for i, want := range wantLosingSequence {
		got := losingSequence[i]
		if got.Kind != want.Kind || got.Row != want.Row || got.Col != want.Col {
			t.Fatalf("4558 T18 action %d=%+v, want %+v", i+1, got, want)
		}
	}
	for _, moment := range manifest.Moments {
		if strings.HasPrefix(moment.SourceID, "836204cc-") {
			t.Fatalf("illegal-move protocol fixture used as strategic motif: %+v", moment)
		}
	}
	wantTags := []string{"consolidation", "backup_route", "useful_redundant_corridor", "thin_tendril", "base_rooted_vertex_disjoint_corridors", "min_cut_le_3", "attack_chain_counter_capture", "translated_motif", "reflected_motif", "capturable_base_halo", "preserved_empty_escape", "hardened_enemy_foothold", "opponent_base_siege_decision", "neutral_denial_candidate", "avoid_capturable_normal_foothold", "articulation_base_shell", "irreversible_attack", "capturable_placement", "neutral_denial_alternative"}
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
