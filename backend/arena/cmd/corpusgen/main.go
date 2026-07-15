// Command corpusgen deterministically creates the frozen synthetic trajectory
// corpus. Its output is checked in; tests never regenerate or tune against it.
package main

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"sort"

	"virusgame/arena"
	"virusgame/game"
)

func main() {
	output := flag.String("output", "", "output corpus JSON")
	flag.Parse()
	if *output == "" {
		panic("-output is required")
	}
	type spec struct {
		rows, cols, players int
		track               string
	}
	specs := []spec{
		{5, 5, 2, "competitive_1v1"}, {8, 8, 2, "competitive_1v1"}, {10, 10, 2, "competitive_1v1"},
		{12, 12, 2, "competitive_1v1"}, {15, 20, 2, "competitive_1v1"}, {20, 15, 2, "competitive_1v1"}, {20, 20, 2, "competitive_1v1"},
		{12, 12, 3, "multiplayer"}, {20, 20, 4, "multiplayer"}, {28, 28, 3, "multiplayer"}, {28, 28, 4, "multiplayer"},
		{25, 25, 2, "stress"}, {30, 30, 2, "stress"},
	}
	corpus := arena.Corpus{Version: arena.CorpusVersion, Generator: "corpusgen-v1: xorshift64, prefer-empty, 18 actions", GeneratedUTC: "2026-07-15T16:00:00Z", GroupHashes: map[string]string{}}
	for specIndex, board := range specs {
		for groupIndex, split := range []string{"train", "heldout"} {
			seed := uint64(0x9e3779b97f4a7c15) + uint64(specIndex*1009+groupIndex*7919)
			corpus.Trajectories = append(corpus.Trajectories, generate(board.rows, board.cols, board.players, board.track, split, seed))
		}
	}
	members := map[string][]string{"train": {}, "heldout": {}}
	for _, trajectory := range corpus.Trajectories {
		for _, checkpoint := range trajectory.Checkpoints {
			members[trajectory.Split] = append(members[trajectory.Split], fmt.Sprintf("%s@%d:%s", trajectory.ID, checkpoint.AfterActions, checkpoint.Hash))
		}
	}
	for split, lines := range members {
		sort.Strings(lines)
		var joined string
		for _, line := range lines {
			joined += line + "\n"
		}
		sum := sha256.Sum256([]byte(joined))
		corpus.GroupHashes[split] = hex.EncodeToString(sum[:])
	}
	encoded, err := json.MarshalIndent(corpus, "", "  ")
	if err != nil {
		panic(err)
	}
	encoded = append(encoded, '\n')
	if err := os.WriteFile(*output, encoded, 0o644); err != nil {
		panic(err)
	}
}

func generate(rows, cols, players int, track, split string, seed uint64) arena.CorpusTrajectory {
	state, err := game.New(rows, cols, players)
	if err != nil {
		panic(err)
	}
	trajectory := arena.CorpusTrajectory{ID: fmt.Sprintf("generated-v1-%dx%d-p%d-%s-%s-%016x", rows, cols, players, track, split, seed), Split: split, Track: track, Seed: seed, Rows: rows, Cols: cols, Players: players}
	rng := seed
	actionCount := 18
	checkpointStart := 12
	if players > 2 {
		actionCount = players * 6
		checkpointStart = players*3 + 1
	}
	if track == "stress" {
		actionCount, checkpointStart = 12, 12
	}
	for step := 1; step <= actionCount; step++ {
		actions := state.LegalActions()
		var quiet []game.Action
		for _, action := range actions {
			if action.Kind != game.Move {
				continue
			}
			cell, _ := state.At(action.Target)
			if cell.Kind == game.Empty {
				quiet = append(quiet, action)
			}
		}
		if len(quiet) == 0 {
			for _, action := range actions {
				if action.Kind == game.Move {
					quiet = append(quiet, action)
				}
			}
		}
		if len(quiet) == 0 {
			panic(fmt.Sprintf("trajectory %s ended at action %d", trajectory.ID, step))
		}
		rng ^= rng << 13
		rng ^= rng >> 7
		rng ^= rng << 17
		action := quiet[int(rng%uint64(len(quiet)))]
		trajectory.Actions = append(trajectory.Actions, arena.ReplayMove{Kind: "move", Row: action.Target.Row, Col: action.Target.Col})
		state, err = state.Apply(action)
		if err != nil {
			panic(err)
		}
		if step < checkpointStart || track == "competitive_1v1" && step > 17 {
			continue
		}
		hash, err := arena.SnapshotHash(state.Snapshot())
		if err != nil {
			panic(err)
		}
		phase := "midgame"
		if step <= players*3+1 {
			phase = "opening"
		}
		strata := []string{phase}
		if hasNeutral(state) {
			strata = append(strata, "neutral_available")
		}
		if tactical(state) {
			strata = append(strata, "tactical")
		}
		if baseThreat(state) {
			strata = append(strata, "base_threat")
		}
		trajectory.Checkpoints = append(trajectory.Checkpoints, arena.CorpusCheckpoint{AfterActions: step, Phase: phase, Strata: strata, Hash: hash})
	}
	return trajectory
}

func hasNeutral(state game.State) bool {
	for _, action := range state.LegalActions() {
		if action.Kind == game.PlaceNeutrals {
			return true
		}
	}
	return false
}

func tactical(state game.State) bool {
	actor := state.CurrentPlayer()
	for _, action := range state.LegalActions() {
		if action.Kind == game.Move {
			cell, _ := state.At(action.Target)
			if cell.Kind == game.Normal && cell.Owner != actor {
				return true
			}
		}
	}
	return false
}

func baseThreat(state game.State) bool {
	actor := state.CurrentPlayer()
	bases := []game.Pos{{Row: 0, Col: 0}, {Row: state.Rows() - 1, Col: state.Cols() - 1}, {Row: 0, Col: state.Cols() - 1}, {Row: state.Rows() - 1, Col: 0}}
	for _, action := range state.LegalActions() {
		if action.Kind != game.Move {
			continue
		}
		cell, _ := state.At(action.Target)
		if cell.Kind == game.Normal && cell.Owner != actor {
			for player, base := range bases {
				if game.Player(player+1) != actor && state.Active(game.Player(player+1)) && abs(action.Target.Row-base.Row) <= 1 && abs(action.Target.Col-base.Col) <= 1 {
					return true
				}
			}
		}
	}
	return false
}

func abs(value int) int {
	if value < 0 {
		return -value
	}
	return value
}
