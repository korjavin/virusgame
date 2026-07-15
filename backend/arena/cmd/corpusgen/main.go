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
	corpus := arena.Corpus{Version: arena.CorpusVersion, Generator: "corpusgen-v1: xorshift64, two phase-directed 1v1 families; bounded multiplayer/stress", GeneratedUTC: "2026-07-15T16:00:00Z", GroupHashes: map[string]string{}}
	for specIndex, board := range specs {
		for groupIndex, split := range []string{"train", "heldout"} {
			families := 1
			if board.track == "competitive_1v1" {
				families = 2
			}
			for family := 0; family < families; family++ {
				seed := uint64(0x9e3779b97f4a7c15) + uint64(specIndex*1009+groupIndex*7919+family*104729)
				if board.track == "competitive_1v1" {
					corpus.Trajectories = append(corpus.Trajectories, generateCompetitive(board.rows, board.cols, split, family, seed))
				} else {
					corpus.Trajectories = append(corpus.Trajectories, generateBounded(board.rows, board.cols, board.players, board.track, split, seed))
				}
			}
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

func generateCompetitive(rows, cols int, split string, family int, seed uint64) arena.CorpusTrajectory {
	state, err := game.New(rows, cols, 2)
	if err != nil {
		panic(err)
	}
	trajectory := arena.CorpusTrajectory{ID: fmt.Sprintf("generated-v1-%dx%d-p2-competitive-%s-family%d-%016x", rows, cols, split, family, seed), Split: split, Track: "competitive_1v1", Seed: seed, Rows: rows, Cols: cols, Players: 2}
	checkpointAt := map[int]bool{}
	contact, base := false, false
	rng := seed
	route := family
	if split == "heldout" {
		route += 2
	}
	maxActions := rows * cols * 4
	for len(trajectory.Actions) < maxActions && !state.GameOver() && !base {
		actionCount := len(trajectory.Actions)
		if !contact && tactical(state) && !checkpointAt[actionCount] {
			addCheckpoint(&trajectory, state, actionCount, "contact_consolidation", []string{"contact", "tactical", "consolidation_candidate", "attack_chain_counter_capture"})
			checkpointAt[actionCount], contact = true, true
		}
		if !base && baseThreat(state) && !checkpointAt[actionCount] {
			addCheckpoint(&trajectory, state, actionCount, "tactical_base_threat", []string{"tactical", "base_threat", "base_rooted_cut", "min_cut_le_3"})
			checkpointAt[actionCount], base = true, true
			break
		}
		action, ok := directedAction(state, route, &rng)
		if !ok {
			break
		}
		trajectory.Actions = append(trajectory.Actions, arena.ReplayMove{Kind: "move", Row: action.Target.Row, Col: action.Target.Col})
		state, err = state.Apply(action)
		if err != nil {
			panic(err)
		}
		actionCount++
		opening := family == 0 && actionCount >= 1 && actionCount <= 3 || family == 1 && actionCount >= 4 && actionCount <= 6
		if opening {
			strata := []string{"opening"}
			if hasNeutral(state) {
				strata = append(strata, "neutral_available")
			}
			addCheckpoint(&trajectory, state, actionCount, "opening", strata)
			checkpointAt[actionCount] = true
		}
	}
	if !contact || !base {
		panic(fmt.Sprintf("trajectory %s incomplete: contact=%v base=%v actions=%d over=%v", trajectory.ID, contact, base, len(trajectory.Actions), state.GameOver()))
	}
	return trajectory
}

func addCheckpoint(trajectory *arena.CorpusTrajectory, state game.State, after int, phase string, strata []string) {
	hash, err := arena.SnapshotHash(state.Snapshot())
	if err != nil {
		panic(err)
	}
	trajectory.Checkpoints = append(trajectory.Checkpoints, arena.CorpusCheckpoint{AfterActions: after, Phase: phase, Strata: strata, Hash: hash})
}

func directedAction(state game.State, route int, rng *uint64) (game.Action, bool) {
	actor := state.CurrentPlayer()
	targetBase := game.Pos{Row: state.Rows() - 1, Col: state.Cols() - 1}
	if actor == 2 {
		targetBase = game.Pos{}
	}
	bestScore := -int(^uint(0)>>1) - 1
	var best game.Action
	found := false
	for _, action := range state.LegalActions() {
		if action.Kind != game.Move {
			continue
		}
		cell, _ := state.At(action.Target)
		distance := abs(action.Target.Row-targetBase.Row) + abs(action.Target.Col-targetBase.Col)
		delta := action.Target.Row*(state.Cols()-1) - action.Target.Col*(state.Rows()-1)
		diagonal := abs(delta)
		score := -1000 * distance
		switch route {
		case 0:
			score -= 2000 * diagonal
		case 1:
			score += 2000 * delta
		case 2:
			score -= 2000 * delta
		case 3:
			if actor == 1 {
				score += 2000 * delta
			} else {
				score -= 2000 * delta
			}
		}
		if cell.Kind == game.Normal && cell.Owner != actor {
			score += 500_000
		}
		if abs(action.Target.Row-targetBase.Row) <= 1 && abs(action.Target.Col-targetBase.Col) <= 1 {
			score += 1_000_000
		}
		*rng ^= *rng << 13
		*rng ^= *rng >> 7
		*rng ^= *rng << 17
		// Seeded jitter is deliberately large enough to make train and heldout
		// choose different legal route families while the phase-directed distance,
		// capture, and base-threat terms still dominate the overall objective.
		score += int(*rng & 0xfff)
		if !found || score > bestScore {
			best, bestScore, found = action, score, true
		}
	}
	return best, found
}

func generateBounded(rows, cols, players int, track, split string, seed uint64) arena.CorpusTrajectory {
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
