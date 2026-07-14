// Package search chooses Virus actions using deterministic anytime search.
package search

import (
	"context"
	"sort"
	"time"

	"virusgame/game"
)

const (
	defaultThinkTime = 500 * time.Millisecond
	maxDepth         = 64
	infScore         = 1 << 60
)

type Result struct {
	Action game.Action
	Score  int
	Depth  int
	Nodes  uint64
}

type tableEntry struct {
	depth  int
	values [4]int
}

type searcher struct {
	ctx   context.Context
	root  game.Player
	multi bool
	table map[uint64]tableEntry
	nodes uint64
}

// ChooseDepth performs one deterministic, fully completed action-depth search.
// It is intended for reproducible benchmarks; production callers should use Choose.
func ChooseDepth(ctx context.Context, state game.State, depth int) (Result, bool) {
	if depth < 1 || depth > maxDepth || len(state.LegalActions()) == 0 {
		return Result{}, false
	}
	if ctx == nil {
		ctx = context.Background()
	}
	s := newSearcher(ctx, state)
	result, complete := s.atDepth(state, depth)
	if !complete {
		return Result{}, false
	}
	result.Depth = depth
	result.Nodes = s.nodes
	return result, true
}

// Choose returns the best action from the last fully completed iteration. If
// ctx has no deadline, a production-safe default deadline is applied.
func Choose(ctx context.Context, state game.State) (Result, bool) {
	actions := state.LegalActions()
	if len(actions) == 0 {
		return Result{}, false
	}
	if ctx == nil {
		ctx = context.Background()
	}
	if _, ok := ctx.Deadline(); !ok {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, defaultThinkTime)
		defer cancel()
	}

	best := Result{Action: actions[0]}
	totalNodes := uint64(0)
	for depth := 1; depth <= maxDepth; depth++ {
		s := newSearcher(ctx, state)
		result, complete := s.atDepth(state, depth)
		totalNodes += s.nodes
		if !complete {
			break
		}
		best = result
		best.Depth = depth
		best.Nodes = totalNodes
	}
	return best, true
}

func newSearcher(ctx context.Context, state game.State) *searcher {
	active := 0
	for player := game.Player(1); player <= 4; player++ {
		if state.Active(player) {
			active++
		}
	}
	return &searcher{
		ctx: ctx, root: state.CurrentPlayer(), multi: active > 2,
		table: make(map[uint64]tableEntry),
	}
}

func (s *searcher) atDepth(state game.State, depth int) (Result, bool) {
	children, ok := s.orderedChildren(state)
	if !ok || len(children) == 0 {
		return Result{}, ok
	}
	best := Result{Action: children[0].action, Score: -infScore}
	alpha, beta := -infScore, infScore
	for _, child := range children {
		var values [4]int
		var complete bool
		if s.multi {
			values, complete = s.maxN(child.state, depth-1)
		} else {
			values[0], complete = s.minimax(child.state, depth-1, alpha, beta)
		}
		if !complete {
			return Result{}, false
		}
		score := values[0]
		if s.multi {
			score = values[s.root-1]
		}
		if score > best.Score {
			best.Action, best.Score = child.action, score
		}
		if !s.multi && score > alpha {
			alpha = score
		}
	}
	return best, true
}

func (s *searcher) minimax(state game.State, depth, alpha, beta int) (int, bool) {
	if !s.running() {
		return 0, false
	}
	s.nodes++
	if depth == 0 || state.GameOver() {
		return evaluate(state, s.root), true
	}
	key := stateHash(state)
	if entry, ok := s.table[key]; ok && entry.depth >= depth {
		return entry.values[0], true
	}
	children, complete := s.orderedChildren(state)
	if !complete {
		return 0, false
	}
	if len(children) == 0 {
		return evaluate(state, s.root), true
	}

	maximizing := state.CurrentPlayer() == s.root
	best := infScore
	if maximizing {
		best = -infScore
	}
	cut := false
	for _, child := range children {
		score, ok := s.minimax(child.state, depth-1, alpha, beta)
		if !ok {
			return 0, false
		}
		if maximizing {
			if score > best {
				best = score
			}
			if best > alpha {
				alpha = best
			}
		} else {
			if score < best {
				best = score
			}
			if best < beta {
				beta = best
			}
		}
		if alpha >= beta {
			cut = true
			break
		}
	}
	if !cut {
		var values [4]int
		values[0] = best
		s.table[key] = tableEntry{depth: depth, values: values}
	}
	return best, true
}

func (s *searcher) maxN(state game.State, depth int) ([4]int, bool) {
	if !s.running() {
		return [4]int{}, false
	}
	s.nodes++
	if depth == 0 || state.GameOver() {
		return evaluateAll(state), true
	}
	key := stateHash(state)
	if entry, ok := s.table[key]; ok && entry.depth >= depth {
		return entry.values, true
	}
	children, complete := s.orderedChildren(state)
	if !complete {
		return [4]int{}, false
	}
	if len(children) == 0 {
		return evaluateAll(state), true
	}

	player := state.CurrentPlayer()
	var best [4]int
	best[player-1] = -infScore
	for _, child := range children {
		values, ok := s.maxN(child.state, depth-1)
		if !ok {
			return [4]int{}, false
		}
		if values[player-1] > best[player-1] {
			best = values
		}
	}
	s.table[key] = tableEntry{depth: depth, values: best}
	return best, true
}

type child struct {
	action game.Action
	state  game.State
	order  int
}

func (s *searcher) orderedChildren(state game.State) ([]child, bool) {
	actions := state.LegalActions()
	children := make([]child, 0, len(actions))
	actor := state.CurrentPlayer()
	beforeActive := activeCount(state)
	for _, action := range actions {
		if !s.running() {
			return nil, false
		}
		target, _ := state.At(action.Target)
		next, err := state.Apply(action)
		if err != nil {
			continue
		}
		order := 0
		if next.GameOver() && next.Winner() == actor {
			order += 1_000_000
		}
		order += (beforeActive - activeCount(next)) * 100_000
		if action.Kind == game.Move && target.Kind == game.Normal && target.Owner != actor {
			order += 10_000
		}
		if next.CurrentPlayer() == actor {
			order += 100
		}
		children = append(children, child{action: action, state: next, order: order})
	}
	sort.SliceStable(children, func(i, j int) bool { return children[i].order > children[j].order })
	return children, true
}

func (s *searcher) running() bool {
	select {
	case <-s.ctx.Done():
		return false
	default:
		return true
	}
}

func activeCount(state game.State) int {
	count := 0
	for player := game.Player(1); player <= 4; player++ {
		if state.Active(player) {
			count++
		}
	}
	return count
}

func stateHash(state game.State) uint64 {
	const prime = uint64(1099511628211)
	hash := uint64(1469598103934665603)
	add := func(value byte) {
		hash ^= uint64(value)
		hash *= prime
	}
	add(byte(state.Rows()))
	add(byte(state.Cols()))
	add(byte(state.CurrentPlayer()))
	add(byte(state.MovesLeft()))
	for player := game.Player(1); player <= 4; player++ {
		if state.Active(player) {
			add(byte(player) | 0x10)
		}
		if state.NeutralUsed(player) {
			add(byte(player) | 0x20)
		}
	}
	for row := 0; row < state.Rows(); row++ {
		for col := 0; col < state.Cols(); col++ {
			cell, _ := state.At(game.Pos{Row: row, Col: col})
			add(byte(cell.Owner)<<3 | byte(cell.Kind))
		}
	}
	return hash
}
