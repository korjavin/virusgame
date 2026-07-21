// Command staticevalgen distills GoBot's STATIC evaluation for NNUE warm-start.
// It self-plays diverse 12x12 games (roster agents at varied budgets + random
// legal openings), and for every non-terminal intermediate position emits one
// JSONL line: the board (owner/kind per cell, in the string form the Python
// import_games.map_board_to_features expects) + the mover + StaticEval(state,
// mover). Deep search is NOT used — the label is the leaf eval only, which is
// the whole point (deep-search labels launder the teacher through lookahead).
//
// JSONL line: {"board":[[{"owner":int,"kind":str}...]...],"player":int,"score":int}
//   owner: 0 neutral, 1/2 players.  kind: EMPTY|NORMAL|FORTIFIED|NEUTRAL|BASE.
// Mate-magnitude positions are skipped so they don't blow up normalization.
package main

import (
	"bufio"
	"encoding/json"
	"flag"
	"fmt"
	"os"

	"virusgame/arena"
	"virusgame/game"
	"virusgame/search"
)

const mateMagnitude = 500_000_000

var kindName = map[game.CellKind]string{
	game.Empty: "EMPTY", game.Normal: "NORMAL", game.Base: "BASE",
	game.Fortified: "FORTIFIED", game.Neutral: "NEUTRAL",
}

type cellJSON struct {
	Owner int    `json:"owner"`
	Kind  string `json:"kind"`
}

type record struct {
	Board  [][]cellJSON `json:"board"`
	Player int          `json:"player"`
	Score  int          `json:"score"`
}

func toRecord(state game.State) (record, bool) {
	player := state.CurrentPlayer()
	score := search.StaticEval(state, player)
	if score >= mateMagnitude || score <= -mateMagnitude {
		return record{}, false
	}
	snap := state.Snapshot()
	board := make([][]cellJSON, snap.Rows)
	for r := 0; r < snap.Rows; r++ {
		board[r] = make([]cellJSON, snap.Cols)
		for c := 0; c < snap.Cols; c++ {
			cell := snap.Board[r][c]
			board[r][c] = cellJSON{Owner: int(cell.Owner), Kind: kindName[cell.Kind]}
		}
	}
	return record{Board: board, Player: int(player), Score: score}, true
}

func next(rng *uint64) uint64 {
	*rng ^= *rng << 13
	*rng ^= *rng >> 7
	*rng ^= *rng << 17
	return *rng
}

func roster() []arena.Agent {
	budget := func(nodes uint64) arena.Agent {
		return func(state game.State) (game.Action, bool) {
			r, ok := search.ChooseNodeBudget(state, nodes)
			return r.Action, ok
		}
	}
	return []arena.Agent{
		budget(2000), budget(8000), arena.Tournament(2),
		arena.Greedy, arena.BaseAttacker, arena.MobilityAttacker,
	}
}

// selfPlay plays one game, emitting every non-terminal position it passes through.
func selfPlay(rows, cols int, agentA, agentB arena.Agent, w *bufio.Writer) int {
	state, err := game.New(rows, cols, 2)
	if err != nil {
		return 0
	}
	written := 0
	maxPlies := rows * cols * 4
	for plies := 0; !state.GameOver() && plies < maxPlies; plies++ {
		if rec, ok := toRecord(state); ok {
			if b, err := json.Marshal(rec); err == nil {
				w.Write(b)
				w.WriteByte('\n')
				written++
			}
		}
		agent := agentA
		if state.CurrentPlayer() == 2 {
			agent = agentB
		}
		action, ok := agent(state)
		if !ok {
			break
		}
		nextState, err := state.Apply(action)
		if err != nil {
			break
		}
		state = nextState
	}
	return written
}

func main() {
	out := flag.String("out", "", "output JSONL path (required)")
	target := flag.Int("positions", 20000, "approximate number of positions to emit")
	seed := flag.Uint64("seed", 1, "base seed")
	flag.Parse()
	if *out == "" {
		fmt.Fprintln(os.Stderr, "-out is required")
		os.Exit(2)
	}
	f, err := os.Create(*out)
	if err != nil {
		panic(err)
	}
	defer f.Close()
	w := bufio.NewWriter(f)
	defer w.Flush()

	agents := roster()
	rng := *seed | 1
	written := 0
	for written < *target {
		switch next(&rng) % 4 {
		case 0, 1, 2: // self-play from the start (most positions)
			a := agents[int(next(&rng)%uint64(len(agents)))]
			b := agents[int(next(&rng)%uint64(len(agents)))]
			written += selfPlay(12, 12, a, b, w)
		case 3: // seeded random opening, then finish with two random-ish agents
			snap, err := arena.RandomLegalOpening(12, 12, next(&rng))
			if err != nil {
				continue
			}
			state, err := game.FromSnapshot(snap)
			if err != nil {
				continue
			}
			if rec, ok := toRecord(state); ok {
				if b, err := json.Marshal(rec); err == nil {
					w.Write(b)
					w.WriteByte('\n')
					written++
				}
			}
		}
	}
	fmt.Printf("wrote %d positions to %s\n", written, *out)
}
