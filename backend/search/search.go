// Package search chooses Virus actions using deterministic anytime search.
package search

import (
	"context"
	"runtime"
	"sort"
	"sync"
	"time"

	"virusgame/game"
)

const (
	// ProductionBudget is the single move-search budget used by both the
	// deployed bot and production-path strength benchmarks.
	ProductionBudget = 600 * time.Millisecond
	maxDepth         = 64
	infScore         = 1 << 60
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
	depth  int
	ply    int
	values [4]int
	bound  boundKind
}

type boundKind uint8

const (
	boundExact boundKind = iota
	boundLower
	boundUpper
)

type searcher struct {
	ctx                context.Context
	root               game.Player
	multi              bool
	table              map[uint64]tableEntry
	nodes, evaluations uint64
	nodeLimit          uint64
	eval               evalWorkspace
	pvs                bool
	bounded            bool
	productionOrder    bool
}

// ChooseNodeBudget performs deterministic iterative deepening without an
// implicit wall-clock deadline.
func ChooseNodeBudget(state game.State, limit uint64) (Result, bool) {
	return chooseNodeBudget(state, limit)
}

func chooseNodeBudget(state game.State, limit uint64) (Result, bool) {
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
	previousPV := fallback
	for depth := state.MovesLeft(); depth <= maxDepth; depth += 3 {
		s := newSearcher(ctx, state)
		s.pvs = true
		s.productionOrder = true
		workers := max(1, runtime.GOMAXPROCS(0)-2)
		result, complete := s.atDepthParallel(state, depth, workers, previousPV)
		totalNodes += s.nodes
		totalEvaluations += s.evaluations
		if !complete {
			break
		}
		best = result
		best.Depth = depth
		best.Nodes = totalNodes
		best.Evaluations = totalEvaluations
		previousPV = result.Action
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

type rootOutcome struct {
	action             game.Action
	ordinal            int
	score              int
	nodes, evaluations uint64
	complete           bool
}

// atDepthParallel evaluates root children independently. A result is
// publishable only when every child completes before the shared deadline.
func (s *searcher) atDepthParallel(state game.State, depth, workers int, pv game.Action) (Result, bool) {
	children, ok := s.orderedChildren(state)
	if !ok || len(children) == 0 {
		return Result{}, ok
	}
	children = preservingChildren(children, s.root)
	children = boundedChildren(children, 32)
	type rootChild struct {
		child
		ordinal int
	}
	roots := make([]rootChild, len(children))
	for i, child := range children {
		roots[i] = rootChild{child: child, ordinal: i}
	}
	for i := range roots {
		if roots[i].action == pv {
			roots[0], roots[i] = roots[i], roots[0]
			break
		}
	}
	if workers > len(roots) {
		workers = len(roots)
	}
	if workers < 1 {
		workers = 1
	}
	evaluate := func(root rootChild) rootOutcome {
		worker := newSearcher(s.ctx, state)
		worker.pvs = true
		worker.bounded = true
		worker.productionOrder = true
		var values [4]int
		var complete bool
		if worker.multi {
			values, complete = worker.maxN(root.state, depth-1, 1)
		} else {
			values[0], complete = worker.minimax(root.state, depth-1, -infScore, infScore, 1)
		}
		score := values[0]
		if worker.multi {
			score = values[worker.root-1]
		}
		return rootOutcome{action: root.action, ordinal: root.ordinal, score: score, nodes: worker.nodes, evaluations: worker.evaluations, complete: complete}
	}

	outcomes := make([]rootOutcome, 0, len(roots))
	first := evaluate(roots[0])
	outcomes = append(outcomes, first)
	if !first.complete {
		s.nodes += first.nodes
		s.evaluations += first.evaluations
		return Result{}, false
	}
	jobs := make(chan rootChild, len(roots)-1)
	results := make(chan rootOutcome, len(roots)-1)
	var wg sync.WaitGroup
	parallel := min(workers, len(roots)-1)
	for range parallel {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for root := range jobs {
				results <- evaluate(root)
			}
		}()
	}
	for _, root := range roots[1:] {
		jobs <- root
	}
	close(jobs)
	wg.Wait()
	close(results)
	for outcome := range results {
		outcomes = append(outcomes, outcome)
	}

	best := Result{Action: children[0].action, Score: -infScore}
	bestOrdinal := len(children)
	complete := true
	for _, outcome := range outcomes {
		s.nodes += outcome.nodes
		s.evaluations += outcome.evaluations
		if !outcome.complete {
			complete = false
			continue
		}
		if outcome.score > best.Score || outcome.score == best.Score && outcome.ordinal < bestOrdinal {
			best.Action, best.Score, bestOrdinal = outcome.action, outcome.score, outcome.ordinal
		}
	}
	if !complete {
		return Result{}, false
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
	alphaOriginal, betaOriginal := alpha, beta
	if entry, ok := s.table[key]; ok && entry.depth >= depth && entry.ply == ply {
		if !s.pvs || entry.bound == boundExact {
			return entry.values[0], true
		}
		if entry.bound == boundLower && entry.values[0] > alpha {
			alpha = entry.values[0]
		} else if entry.bound == boundUpper && entry.values[0] < beta {
			beta = entry.values[0]
		}
		if alpha >= beta {
			return entry.values[0], true
		}
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
	for index, child := range children {
		childAlpha, childBeta := alpha, beta
		if s.pvs && index > 0 {
			if maximizing {
				childBeta = alpha + 1
			} else {
				childAlpha = beta - 1
			}
		}
		score, ok := s.minimax(child.state, depth-1, childAlpha, childBeta, ply+1)
		if !ok {
			return 0, false
		}
		if s.pvs && index > 0 && ((maximizing && score > alpha && score < beta) || (!maximizing && score < beta && score > alpha)) {
			score, ok = s.minimax(child.state, depth-1, alpha, beta, ply+1)
			if !ok {
				return 0, false
			}
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
	if !s.pvs && !cut {
		var values [4]int
		values[0] = best
		s.table[key] = tableEntry{depth: depth, ply: ply, values: values}
	} else if s.pvs {
		bound := boundExact
		if best <= alphaOriginal {
			bound = boundUpper
		} else if best >= betaOriginal {
			bound = boundLower
		}
		var values [4]int
		values[0] = best
		s.table[key] = tableEntry{depth: depth, ply: ply, values: values, bound: bound}
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
	action                                          game.Action
	state                                           game.State
	order                                           int
	win, survives, eliminations, capture, continues int
	pressure                                        int
}

func (s *searcher) orderedChildren(state game.State) ([]child, bool) {
	var position game.Position
	var actions []game.Action
	if s.productionOrder {
		position = game.NewPosition(state)
		position.ForEachSearchAction(func(action game.Action) bool {
			actions = append(actions, action)
			return true
		})
	} else {
		actions = state.LegalActions()
	}
	children := make([]child, 0, len(actions))
	actor := state.CurrentPlayer()
	beforeActive := activeCount(state)
	for _, action := range actions {
		if !s.running() {
			return nil, false
		}
		target, _ := state.At(action.Target)
		var next game.State
		if s.productionOrder {
			next = position.ApplySearch(action).State()
		} else {
			var err error
			next, err = state.Apply(action)
			if err != nil {
				continue
			}
		}
		order := 0
		win := 0
		if next.GameOver() && next.Winner() == actor {
			order += 1_000_000
			win = 1
		}
		survives := 0
		if next.Active(actor) {
			survives = 1
		}
		eliminations := beforeActive - activeCount(next)
		order += eliminations * 100_000
		capture := 0
		if action.Kind == game.Move && target.Kind == game.Normal && target.Owner != actor {
			order += 10_000
			capture = 1
		}
		continues := 0
		if next.CurrentPlayer() == actor {
			order += 100
			continues = 1
		}
		pressure := 0
		if action.Kind == game.Move {
			pressure = -nearestEnemyBaseDistance(state, actor, action.Target)
		}
		children = append(children, child{action: action, state: next, order: order, win: win, survives: survives, eliminations: eliminations, capture: capture, continues: continues, pressure: pressure})
	}
	if s.productionOrder {
		sort.SliceStable(children, func(i, j int) bool { return betterProductionOrder(children[i], children[j]) })
	} else {
		sort.SliceStable(children, func(i, j int) bool { return children[i].order > children[j].order })
	}
	if s.bounded {
		children = boundedChildren(children, 12)
	}
	return children, true
}

// boundedChildren retains every tactically forcing action and caps only quiet
// alternatives. Stable input order remains the deterministic tie order.
func boundedChildren(children []child, quietLimit int) []child {
	selected := children[:0]
	quiet := 0
	hasSurvivor := false
	for _, child := range children {
		if child.survives != 0 {
			hasSurvivor = true
			break
		}
	}
	for _, child := range children {
		if hasSurvivor && child.survives == 0 {
			continue
		}
		forcing := child.survives != 0 && (child.win != 0 || child.eliminations > 0 || child.capture != 0)
		if forcing || quiet < quietLimit {
			selected = append(selected, child)
			if !forcing {
				quiet++
			}
		}
	}
	return selected
}

func betterProductionOrder(a, b child) bool {
	av := [...]int{a.win, a.survives, a.eliminations, a.capture, a.continues, a.pressure}
	bv := [...]int{b.win, b.survives, b.eliminations, b.capture, b.continues, b.pressure}
	for i := range av {
		if av[i] != bv[i] {
			return av[i] > bv[i]
		}
	}
	return false
}

func nearestEnemyBaseDistance(state game.State, actor game.Player, target game.Pos) int {
	best := state.Rows() + state.Cols()
	for player := game.Player(1); player <= 4; player++ {
		if player == actor || !state.Active(player) {
			continue
		}
		base, ok := playerBase(state, player)
		if !ok {
			continue
		}
		distance := abs(target.Row-base.Row) + abs(target.Col-base.Col)
		if distance < best {
			best = distance
		}
	}
	return best
}

func playerBase(state game.State, player game.Player) (game.Pos, bool) {
	for row := 0; row < state.Rows(); row++ {
		for col := 0; col < state.Cols(); col++ {
			cell, _ := state.At(game.Pos{Row: row, Col: col})
			if cell.Owner == player && cell.Kind == game.Base {
				return game.Pos{Row: row, Col: col}, true
			}
		}
	}
	return game.Pos{}, false
}

func abs(value int) int {
	if value < 0 {
		return -value
	}
	return value
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
