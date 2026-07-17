// Package search chooses Virus actions using deterministic anytime search.
package search

import (
	"context"
	"sort"
	"time"

	"virusgame/game"
)

const (
	// ProductionBudget is the single move-search budget used by both the
	// deployed bot and production-path strength benchmarks.
	ProductionBudget = 1000 * time.Millisecond
	maxDepth         = 64
	infScore         = 1 << 60
)

// TT bound flags for fail-soft alpha-beta stores.
const (
	flagExact uint8 = iota
	flagLower
	flagUpper
)

type Result struct {
	Action          game.Action
	Score           int
	Depth           int
	Nodes           uint64
	Evaluations     uint64
	BudgetExhausted bool
	SearchComplete  bool
}

type tableEntry struct {
	depth      int
	ply        int
	flag       uint8
	bestAction game.Action
	values     [4]int
}

type searcher struct {
	ctx                context.Context
	root               game.Player
	multi              bool
	table              map[uint64]tableEntry
	nodes, evaluations uint64
	nodeLimit          uint64
	eval               evalWorkspace
}

// ChooseNodeBudget performs deterministic iterative deepening without an
// implicit wall-clock deadline.
func ChooseNodeBudget(state game.State, limit uint64) (Result, bool) {
	return chooseNodeBudget(state, limit)
}

func chooseNodeBudget(state game.State, limit uint64) (Result, bool) {
	if result, ok := openingBookResult(state); ok {
		return result, true
	}
	fallback, ok := preservingFallback(state)
	if !ok || limit == 0 {
		return Result{}, false
	}
	best := Result{Action: fallback}
	var nodes, evaluations uint64
	for depth := 1; depth <= maxDepth && nodes < limit; depth++ {
		s := newSearcher(context.Background(), state)
		s.nodeLimit = limit - nodes
		result, complete := s.atDepth(state, depth)
		nodes += s.nodes
		evaluations += s.evaluations
		if !complete {
			break
		}
		best = result
		best.Depth = depth
	}
	best.Nodes, best.Evaluations = nodes, evaluations
	best.BudgetExhausted = nodes >= limit
	best.SearchComplete = best.Depth == maxDepth
	return best, true
}

// ChooseDepth performs one deterministic, fully completed action-depth search.
// It is intended for reproducible benchmarks; production callers should use Choose.
func ChooseDepth(ctx context.Context, state game.State, depth int) (Result, bool) {
	if depth < 1 || depth > maxDepth {
		return Result{}, false
	}
	fallback, ok := preservingFallback(state)
	if !ok {
		return Result{}, false
	}
	if ctx == nil {
		ctx = context.Background()
	}
	s := newSearcher(ctx, state)
	result, complete := s.atDepth(state, depth)
	if !complete {
		return Result{Action: fallback}, false
	}
	result.Depth = depth
	result.Nodes = s.nodes
	result.Evaluations = s.evaluations
	return result, true
}

// Choose returns the best action from the last fully completed iteration. If
// ctx has no deadline, a production-safe default deadline is applied.
func Choose(ctx context.Context, state game.State) (Result, bool) {
	if result, ok := openingBookResult(state); ok {
		return result, true
	}
	fallback, ok := preservingFallback(state)
	if !ok {
		return Result{}, false
	}
	if ctx == nil {
		ctx = context.Background()
	}
	if _, ok := ctx.Deadline(); !ok {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, ProductionBudget)
		defer cancel()
	}

	best := Result{Action: fallback}
	totalNodes, totalEvaluations := uint64(0), uint64(0)
	for depth := 1; depth <= maxDepth; depth++ {
		s := newSearcher(ctx, state)
		result, complete := s.atDepth(state, depth)
		totalNodes += s.nodes
		totalEvaluations += s.evaluations
		if !complete {
			break
		}
		best = result
		best.Depth = depth
		best.Nodes = totalNodes
		best.Evaluations = totalEvaluations
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
	children = preservingChildren(children, s.root)
	best := Result{Action: children[0].action, Score: -infScore}
	alpha, beta := -infScore, infScore
	for _, child := range children {
		var values [4]int
		var complete bool
		if s.multi {
			values, complete = s.maxN(child.state, depth-1, 1)
		} else {
			values[0], complete = s.minimax(child.state, depth-1, alpha, beta, 1)
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

func (s *searcher) minimax(state game.State, depth, alpha, beta, ply int) (int, bool) {
	if !s.running() {
		return 0, false
	}
	s.nodes++
	if state.GameOver() {
		return terminalScore(state, s.root, ply), true
	}
	if depth == 0 {
		s.evaluations++
		return evaluateWithWorkspace(state, s.root, &s.eval), true
	}
	key := stateHash(state)
	if entry, ok := s.table[key]; ok && entry.depth >= depth && entry.ply == ply {
		return entry.values[0], true
	}
	children, complete := s.orderedChildren(state)
	if !complete {
		return 0, false
	}
	if len(children) == 0 {
		s.evaluations++
		return evaluateWithWorkspace(state, s.root, &s.eval), true
	}

	maximizing := state.CurrentPlayer() == s.root
	best := infScore
	if maximizing {
		best = -infScore
	}
	cut := false
	for _, child := range children {
		score, ok := s.minimax(child.state, depth-1, alpha, beta, ply+1)
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
		s.table[key] = tableEntry{depth: depth, ply: ply, values: values}
	}
	return best, true
}

func (s *searcher) maxN(state game.State, depth, ply int) ([4]int, bool) {
	if !s.running() {
		return [4]int{}, false
	}
	s.nodes++
	if state.GameOver() {
		return terminalScores(state, ply), true
	}
	if depth == 0 {
		s.evaluations++
		return evaluateAllWithWorkspace(state, &s.eval), true
	}
	key := stateHash(state)
	if entry, ok := s.table[key]; ok && entry.depth >= depth && entry.ply == ply {
		return entry.values, true
	}
	children, complete := s.orderedChildren(state)
	if !complete {
		return [4]int{}, false
	}
	if len(children) == 0 {
		s.evaluations++
		return evaluateAllWithWorkspace(state, &s.eval), true
	}

	player := state.CurrentPlayer()
	var best [4]int
	best[player-1] = -infScore
	for _, child := range children {
		values, ok := s.maxN(child.state, depth-1, ply+1)
		if !ok {
			return [4]int{}, false
		}
		if values[player-1] > best[player-1] {
			best = values
		}
	}
	s.table[key] = tableEntry{depth: depth, ply: ply, values: best}
	return best, true
}

// preservingFallback is deliberately independent of the search context. Even
// an already-canceled caller gets a legal action that does not immediately
// eliminate the actor whenever such an action exists.
func preservingFallback(state game.State) (game.Action, bool) {
	actions := state.LegalActions()
	if len(actions) == 0 {
		return game.Action{}, false
	}
	actor := state.CurrentPlayer()
	for _, action := range actions {
		next, err := state.Apply(action)
		if err == nil && next.Active(actor) {
			return action, true
		}
	}
	return actions[0], true
}

func preservingChildren(children []child, actor game.Player) []child {
	for _, candidate := range children {
		if candidate.state.Active(actor) {
			kept := children[:0]
			for _, child := range children {
				if child.state.Active(actor) {
					kept = append(kept, child)
				}
			}
			return kept
		}
	}
	return children
}

func terminalScore(state game.State, player game.Player, ply int) int {
	if state.Winner() == player {
		return mateScore - ply
	}
	return -mateScore + ply
}

func terminalScores(state game.State, ply int) [4]int {
	var scores [4]int
	for player := game.Player(1); player <= 4; player++ {
		scores[player-1] = terminalScore(state, player, ply)
	}
	return scores
}

type child struct {
	action game.Action
	state  game.State
	order  int
}

func (s *searcher) orderedChildren(state game.State) ([]child, bool) {
	pos := game.NewPosition(state)
	actor := state.CurrentPlayer()
	beforeActive := activeCount(state)
	var children []child
	stopped := false
	pos.ForEachSearchAction(func(action game.Action) bool {
		if !s.running() {
			stopped = true
			return false
		}
		target, _ := state.At(action.Target)
		next := pos.ApplySearch(action).State()
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
		return true
	})
	if stopped {
		return nil, false
	}
	sort.SliceStable(children, func(i, j int) bool { return children[i].order > children[j].order })
	return children, true
}

func (s *searcher) running() bool {
	if s.nodeLimit > 0 && s.nodes >= s.nodeLimit {
		return false
	}
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
