// Command searchparitygen distills GoBot's SEARCH (not just the leaf eval) into
// a deterministic move-parity fixture for the Java port (bd nnue-trainer-0dj.2,
// Phase 1). It mirrors staticevalgen's diverse self-play position generator, but
// per position calls search.ChooseDepth(ctx, state, depth) for a couple of small
// fixed depths and emits the CHOSEN action + its score. ChooseDepth is
// deterministic and fully completed (no time/node cutoff, no opening book), so a
// faithful Java port must pick the same action and same integer score on every
// record — that is the whole acceptance gate.
//
// JSONL line:
//
//	{"board":[[{"owner":int,"kind":str}...]...],"player":int,"depth":int,
//	 "score":int,"action":{...},"movesLeft":int,"neutralUsed":[bool...]}
//
//	owner: 0 neutral, 1/2 players.  kind: EMPTY|NORMAL|FORTIFIED|NEUTRAL|BASE.
//	action for a Move:          {"type":"MOVE","target":{"row":r,"col":c}}
//	action for PlaceNeutrals:   {"type":"PLACE_NEUTRALS","pos1":{"row":r,"col":c},
//	                             "pos2":{"row":r,"col":c}}
//	  (pos1/pos2 = Go Action.Neutrals[0]/[1], the row/col encoding matches the
//	   Java MoveAction/PlaceNeutralsAction constructors exactly.)
//
// Hidden (non-board) State the search depends on but the board cannot encode —
// the Java port MUST reproduce these (same lesson as Phase 0's StaticEval):
// movesLeft feeds the tempo terms and stateHash; neutralUsed[p-1] feeds the
// per-player neutral bonus and stateHash. currentPlayer == the emitted "player".
package main

import (
	"bufio"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"time"

	"virusgame/arena"
	"virusgame/game"
	"virusgame/search"
)

var kindName = map[game.CellKind]string{
	game.Empty: "EMPTY", game.Normal: "NORMAL", game.Base: "BASE",
	game.Fortified: "FORTIFIED", game.Neutral: "NEUTRAL",
}

type cellJSON struct {
	Owner int    `json:"owner"`
	Kind  string `json:"kind"`
}

type posJSON struct {
	Row int `json:"row"`
	Col int `json:"col"`
}

type actionJSON struct {
	Type   string   `json:"type"`
	Target *posJSON `json:"target,omitempty"`
	Pos1   *posJSON `json:"pos1,omitempty"`
	Pos2   *posJSON `json:"pos2,omitempty"`
}

type record struct {
	Board       [][]cellJSON `json:"board"`
	Player      int          `json:"player"`
	Depth       int          `json:"depth,omitempty"`
	NodeLimit   int          `json:"nodeLimit,omitempty"`
	Score       int          `json:"score"`
	Action      actionJSON   `json:"action"`
	MovesLeft   int          `json:"movesLeft"`
	NeutralUsed []bool       `json:"neutralUsed"`
}

func encodeAction(a game.Action) actionJSON {
	if a.Kind == game.PlaceNeutrals {
		return actionJSON{
			Type: "PLACE_NEUTRALS",
			Pos1: &posJSON{Row: a.Neutrals[0].Row, Col: a.Neutrals[0].Col},
			Pos2: &posJSON{Row: a.Neutrals[1].Row, Col: a.Neutrals[1].Col},
		}
	}
	return actionJSON{Type: "MOVE", Target: &posJSON{Row: a.Target.Row, Col: a.Target.Col}}
}

func boardJSON(snap game.Snapshot) [][]cellJSON {
	board := make([][]cellJSON, snap.Rows)
	for r := 0; r < snap.Rows; r++ {
		board[r] = make([]cellJSON, snap.Cols)
		for c := 0; c < snap.Cols; c++ {
			cell := snap.Board[r][c]
			board[r][c] = cellJSON{Owner: int(cell.Owner), Kind: kindName[cell.Kind]}
		}
	}
	return board
}

// toRecords runs ChooseDepth at each requested depth and returns one record per
// depth whose search fully completed within the timeout. The timeout only gates
// SELECTION (a pathologically wide position — many neutral-placement pairs — is
// dropped); every KEPT record is a fully-completed deterministic search, so the
// Java port (which needs no timeout) reproduces it exactly.
func toRecords(state game.State, depths []int, timeout time.Duration) []record {
	snap := state.Snapshot()
	board := boardJSON(snap)
	player := int(state.CurrentPlayer())
	var out []record
	for _, d := range depths {
		ctx, cancel := context.WithTimeout(context.Background(), timeout)
		result, ok := search.ChooseDepth(ctx, state, d)
		cancel()
		if !ok {
			continue
		}
		out = append(out, record{
			Board: board, Player: player, Depth: d, Score: result.Score,
			Action: encodeAction(result.Action), MovesLeft: snap.MovesLeft,
			NeutralUsed: snap.NeutralUsed,
		})
	}
	return out
}

// toNodeBudgetRecord runs the deterministic node-limited iterative-deepening
// search (search.ChooseNodeBudget) — the same core the live Choose path uses,
// plus the opening book that ChooseDepth skips. Emitting its chosen action+score
// gives the Java port a deterministic live-path oracle (bd nnue-trainer-0dj.4).
func toNodeBudgetRecord(state game.State, limit uint64) (record, bool) {
	result, ok := search.ChooseNodeBudget(state, limit)
	if !ok {
		return record{}, false
	}
	snap := state.Snapshot()
	return record{
		Board:       boardJSON(snap),
		Player:      int(state.CurrentPlayer()),
		NodeLimit:   int(limit),
		Score:       result.Score,
		Action:      encodeAction(result.Action),
		MovesLeft:   snap.MovesLeft,
		NeutralUsed: snap.NeutralUsed,
	}, true
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

// selfPlay plays one game, emitting oracle records for every Nth non-terminal
// position (sampling keeps depth-5 search cost bounded while staying diverse).
func selfPlay(rows, cols int, agentA, agentB arena.Agent, depths []int, sample int, timeout time.Duration, w, wNode *bufio.Writer, nodeLimit uint64) int {
	state, err := game.New(rows, cols, 2)
	if err != nil {
		return 0
	}
	written := 0
	maxPlies := rows * cols * 4
	for plies := 0; !state.GameOver() && plies < maxPlies; plies++ {
		if plies%sample == 0 {
			for _, rec := range toRecords(state, depths, timeout) {
				if b, err := json.Marshal(rec); err == nil {
					w.Write(b)
					w.WriteByte('\n')
					written++
				}
			}
			if wNode != nil {
				if rec, ok := toNodeBudgetRecord(state, nodeLimit); ok {
					if b, err := json.Marshal(rec); err == nil {
						wNode.Write(b)
						wNode.WriteByte('\n')
					}
				}
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
	nodeOut := flag.String("nodebudget-out", "", "optional JSONL path for deterministic ChooseNodeBudget records")
	nodeLimit := flag.Uint64("nodelimit", 50000, "node budget for ChooseNodeBudget records")
	target := flag.Int("positions", 400, "approximate number of records to emit")
	sample := flag.Int("sample", 3, "emit records every Nth ply of a game")
	timeoutMs := flag.Int("timeout", 800, "per-position ChooseDepth wall-clock cap (ms); wider positions are skipped, kept records are fully completed")
	seed := flag.Uint64("seed", 1, "base seed")
	flag.Parse()
	if *out == "" {
		fmt.Fprintln(os.Stderr, "-out is required")
		os.Exit(2)
	}
	depths := []int{3, 5}
	timeout := time.Duration(*timeoutMs) * time.Millisecond
	f, err := os.Create(*out)
	if err != nil {
		panic(err)
	}
	defer f.Close()
	w := bufio.NewWriter(f)
	defer w.Flush()

	var wNode *bufio.Writer
	if *nodeOut != "" {
		fn, err := os.Create(*nodeOut)
		if err != nil {
			panic(err)
		}
		defer fn.Close()
		wNode = bufio.NewWriter(fn)
		defer wNode.Flush()
	}

	agents := roster()
	rng := *seed | 1
	written := 0
	for written < *target {
		a := agents[int(next(&rng)%uint64(len(agents)))]
		b := agents[int(next(&rng)%uint64(len(agents)))]
		written += selfPlay(12, 12, a, b, depths, *sample, timeout, w, wNode, *nodeLimit)
	}
	fmt.Printf("wrote %d records to %s\n", written, *out)
	if *nodeOut != "" {
		fmt.Printf("wrote node-budget records (limit %d) to %s\n", *nodeLimit, *nodeOut)
	}
}
