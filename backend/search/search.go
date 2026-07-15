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
	Action               game.Action
	Score                int
	Depth                int
	Nodes                uint64
	Evaluations          uint64
	BudgetExhausted      bool
	SearchComplete       bool
	CompletedTurnDepth   int
	Workers              int
	RootLegal            int
	RootSelected         int
	RootCompleted        int
	RootLegalNeutrals    int
	RootSelectedNeutrals int
	IterationsStarted    int
	IterationsCompleted  int
	Elapsed              time.Duration
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
	contactSeen        [2500]bool
	contactQueue       [2500]int16
}

const (
	rootOptionalLimit     = 32
	interiorOptionalLimit = 12
	multiOptionalLimit    = 8
	// Avoid spending the remaining deadline on a predictably explosive
	// iteration. This is admission control, not a node cutoff: a started
	// iteration still completes or is discarded atomically.
	maxProjectedIterationNodes = 20_000
)

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
	iterationsStarted, iterationsCompleted := 0, 0
	for depth := state.MovesLeft(); depth <= maxDepth && nodes < limit; depth += 3 {
		iterationsStarted++
		s := newSearcher(context.Background(), state)
		s.nodeLimit = limit - nodes
		result, complete := s.atDepth(state, depth)
		nodes += s.nodes
		evaluations += s.evaluations
		if !complete {
			break
		}
		best = result
		iterationsCompleted++
		best.Depth = depth
		best.CompletedTurnDepth = completedTurns(state.MovesLeft(), depth)
	}
	best.Nodes, best.Evaluations = nodes, evaluations
	best.BudgetExhausted = nodes >= limit
	best.SearchComplete = best.Depth > 0 && best.Depth+3 > maxDepth
	best.Workers = 1
	best.IterationsStarted, best.IterationsCompleted = iterationsStarted, iterationsCompleted
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
	result.CompletedTurnDepth = completedTurns(state.MovesLeft(), depth)
	result.Workers = 1
	result.IterationsStarted = 1
	result.IterationsCompleted = 1
	result.Nodes = s.nodes
	result.Evaluations = s.evaluations
	return result, true
}

// Choose returns the best action from the last fully completed iteration. If
// ctx has no deadline, a production-safe default deadline is applied.
func Choose(ctx context.Context, state game.State) (Result, bool) {
	started := time.Now()
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
	// Leave enough time for the caller to serialize and send the action.
	if deadline, ok := ctx.Deadline(); ok {
		searchDeadline := deadline.Add(-25 * time.Millisecond)
		if time.Until(searchDeadline) <= 0 {
			return Result{Action: fallback, Workers: 1, Elapsed: time.Since(started)}, true
		} else {
			var cancel context.CancelFunc
			ctx, cancel = context.WithDeadline(ctx, searchDeadline)
			defer cancel()
		}
	}

	best := Result{Action: fallback, Workers: 1}
	totalNodes, totalEvaluations := uint64(0), uint64(0)
	previousPV := fallback
	iterationsStarted := 0
	iterationsCompleted := 0
	var previousIteration time.Duration
	var priorIteration time.Duration
	var previousIterationNodes, priorIterationNodes uint64
	for depth := state.MovesLeft(); depth <= maxDepth; depth += 3 {
		if previousIteration > 0 {
			growth := 5.0
			if priorIteration > 0 && float64(previousIteration)/float64(priorIteration) > growth {
				growth = float64(previousIteration) / float64(priorIteration)
			}
			estimate := time.Duration(float64(previousIteration) * growth)
			if deadline, ok := ctx.Deadline(); ok && time.Until(deadline) < estimate {
				break
			}
			nodeGrowth := 5.0
			if priorIterationNodes > 0 && float64(previousIterationNodes)/float64(priorIterationNodes) > nodeGrowth {
				nodeGrowth = float64(previousIterationNodes) / float64(priorIterationNodes)
			}
			if float64(previousIterationNodes)*nodeGrowth > maxProjectedIterationNodes {
				break
			}
		}
		iterationsStarted++
		iterationStarted := time.Now()
		s := newSearcher(ctx, state)
		workers := max(1, runtime.GOMAXPROCS(0)-2)
		result, complete := s.atDepthParallel(state, depth, workers, previousPV)
		totalNodes += s.nodes
		totalEvaluations += s.evaluations
		if !complete {
			break
		}
		priorIteration, previousIteration = previousIteration, time.Since(iterationStarted)
		priorIterationNodes, previousIterationNodes = previousIterationNodes, s.nodes
		best = result
		iterationsCompleted++
		best.Depth = depth
		best.CompletedTurnDepth = completedTurns(state.MovesLeft(), depth)
		best.IterationsStarted = iterationsStarted
		best.IterationsCompleted = iterationsCompleted
		best.Nodes = totalNodes
		best.Evaluations = totalEvaluations
		previousPV = result.Action
	}
	best.IterationsStarted = iterationsStarted
	best.Nodes, best.Evaluations = totalNodes, totalEvaluations
	best.SearchComplete = best.Depth > 0 && best.Depth+3 > maxDepth
	best.Elapsed = time.Since(started)
	return best, true
}

func completedTurns(movesLeft, depth int) int {
	if depth < movesLeft {
		return 0
	}
	return 1 + (depth-movesLeft)/3
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
		table: make(map[uint64]tableEntry), pvs: true,
	}
}

func (s *searcher) atDepth(state game.State, depth int) (Result, bool) {
	children, legal, legalNeutrals, ok := s.orderedChildren(game.NewPosition(state), true)
	if !ok || len(children) == 0 {
		return Result{}, ok
	}
	children = preservingChildren(children, s.root)
	best := Result{Action: children[0].action, Score: -infScore, RootLegal: legal, RootSelected: len(children)}
	best.RootSelectedNeutrals = countNeutralChildren(children)
	best.RootLegalNeutrals = legalNeutrals
	bestOrdinal := int(^uint(0) >> 1)
	for _, child := range children {
		var values [4]int
		var complete bool
		if s.multi {
			values, complete = s.maxN(child.position, depth-1, 1)
		} else {
			values[0], complete = s.minimax(child.position, depth-1, -infScore, infScore, 1)
		}
		if !complete {
			return Result{}, false
		}
		best.RootCompleted++
		score := values[0]
		if s.multi {
			score = values[s.root-1]
		}
		if score > best.Score || (score == best.Score && child.ordinal < bestOrdinal) {
			best.Action, best.Score, bestOrdinal = child.action, score, child.ordinal
		}
	}
	return best, true
}

type rootOutcome struct {
	child              child
	score              int
	nodes, evaluations uint64
	complete           bool
}

// atDepthParallel evaluates independent root children with full windows. It
// publishes no partial iteration: callers keep the previous completed result
// unless every selected child reports completion.
func (s *searcher) atDepthParallel(state game.State, depth, workers int, pv game.Action) (Result, bool) {
	children, legal, legalNeutrals, ok := s.orderedChildren(game.NewPosition(state), true)
	if !ok || len(children) == 0 {
		return Result{}, ok
	}
	children = preservingChildren(children, s.root)
	if workers > len(children) {
		workers = len(children)
	}
	if workers < 1 {
		workers = 1
	}
	// Search the previous PV first, without changing its stable root ordinal.
	for i := range children {
		if children[i].action == pv {
			children[0], children[i] = children[i], children[0]
			break
		}
	}
	evaluate := func(c child) rootOutcome {
		worker := newSearcher(s.ctx, state)
		worker.pvs = s.pvs
		var values [4]int
		var complete bool
		if worker.multi {
			values, complete = worker.maxN(c.position, depth-1, 1)
		} else {
			values[0], complete = worker.minimax(c.position, depth-1, -infScore, infScore, 1)
		}
		score := values[0]
		if worker.multi {
			score = values[worker.root-1]
		}
		return rootOutcome{child: c, score: score, nodes: worker.nodes, evaluations: worker.evaluations, complete: complete}
	}
	outcomes := make([]rootOutcome, 0, len(children))
	first := evaluate(children[0])
	outcomes = append(outcomes, first)
	if !first.complete {
		s.nodes += first.nodes
		s.evaluations += first.evaluations
		return Result{}, false
	}
	if len(children) > 1 {
		jobs := make(chan child, len(children)-1)
		results := make(chan rootOutcome, len(children)-1)
		var wg sync.WaitGroup
		parallel := min(workers, len(children)-1)
		for range parallel {
			wg.Add(1)
			go func() {
				defer wg.Done()
				for c := range jobs {
					results <- evaluate(c)
				}
			}()
		}
		for _, c := range children[1:] {
			jobs <- c
		}
		close(jobs)
		wg.Wait()
		close(results)
		for result := range results {
			outcomes = append(outcomes, result)
		}
	}
	actualWorkers := 1
	if len(children) > 1 {
		actualWorkers = min(workers, len(children)-1)
	}
	best := Result{Action: children[0].action, Score: -infScore, Workers: actualWorkers, RootLegal: legal, RootSelected: len(children), RootCompleted: len(outcomes)}
	best.RootSelectedNeutrals = countNeutralChildren(children)
	best.RootLegalNeutrals = legalNeutrals
	complete := len(outcomes) == len(children)
	bestOrdinal := int(^uint(0) >> 1)
	for _, outcome := range outcomes {
		s.nodes += outcome.nodes
		s.evaluations += outcome.evaluations
		if !outcome.complete {
			complete = false
			continue
		}
		if outcome.score > best.Score || (outcome.score == best.Score && outcome.child.ordinal < bestOrdinal) {
			best.Action, best.Score, bestOrdinal = outcome.child.action, outcome.score, outcome.child.ordinal
		}
	}
	if !complete {
		return Result{}, false
	}
	return best, true
}

func countNeutralChildren(children []child) int {
	count := 0
	for _, c := range children {
		if c.action.Kind == game.PlaceNeutrals {
			count++
		}
	}
	return count
}

func (s *searcher) minimax(position game.Position, depth, alpha, beta, ply int) (int, bool) {
	state := position.State()
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
		switch entry.bound {
		case boundExact:
			return entry.values[0], true
		case boundLower:
			if entry.values[0] > alpha {
				alpha = entry.values[0]
			}
		case boundUpper:
			if entry.values[0] < beta {
				beta = entry.values[0]
			}
		}
		if alpha >= beta {
			return entry.values[0], true
		}
	}
	children, _, _, complete := s.orderedChildren(position, false)
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
	for index, child := range children {
		childAlpha, childBeta := alpha, beta
		if s.pvs && index > 0 {
			if maximizing {
				childBeta = alpha + 1
			} else {
				childAlpha = beta - 1
			}
		}
		score, ok := s.minimax(child.position, depth-1, childAlpha, childBeta, ply+1)
		if !ok {
			return 0, false
		}
		if s.pvs && index > 0 && ((maximizing && score > alpha && score < beta) || (!maximizing && score < beta && score > alpha)) {
			score, ok = s.minimax(child.position, depth-1, alpha, beta, ply+1)
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
			break
		}
	}
	bound := boundExact
	if best <= alphaOriginal {
		bound = boundUpper
	} else if best >= betaOriginal {
		bound = boundLower
	}
	var values [4]int
	values[0] = best
	s.table[key] = tableEntry{depth: depth, ply: ply, values: values, bound: bound}
	return best, true
}

func (s *searcher) maxN(position game.Position, depth, ply int) ([4]int, bool) {
	state := position.State()
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
	children, _, _, complete := s.orderedChildren(position, false)
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
		values, ok := s.maxN(child.position, depth-1, ply+1)
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
	position := game.NewPosition(state)
	actor := state.CurrentPlayer()
	var first, preserving game.Action
	found, foundPreserving := false, false
	position.ForEachSearchAction(func(action game.Action) bool {
		if !found {
			first, found = action, true
		}
		if position.ApplySearch(action).State().Active(actor) {
			preserving, foundPreserving = action, true
			return false
		}
		return true
	})
	if foundPreserving {
		return preserving, true
	}
	// Search candidates are deliberately bounded. On the rare path where none
	// preserves the actor, exhaust the authoritative set before accepting
	// immediate elimination; fallback safety is a harder invariant than speed.
	position.ForEachLegalAction(func(action game.Action) bool {
		if !found {
			first, found = action, true
		}
		next, err := state.Apply(action)
		if err == nil && next.Active(actor) {
			preserving, foundPreserving = action, true
			return false
		}
		return true
	})
	if foundPreserving {
		return preserving, true
	}
	return first, found
}

func preservingChildren(children []child, actor game.Player) []child {
	for _, candidate := range children {
		if candidate.position.State().Active(actor) {
			kept := children[:0]
			for _, child := range children {
				if child.position.State().Active(actor) {
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
	action   game.Action
	position game.Position
	order    actionOrder
	ordinal  int
}

type actionOrder struct {
	win, survives, eliminations, capture, continues, pressure int
}

func (s *searcher) orderedChildren(position game.Position, root bool) ([]child, int, int, bool) {
	state := position.State()
	children := make([]child, 0, 32)
	actor := state.CurrentPlayer()
	beforeActive := activeCount(state)
	legal := 0
	legalNeutrals := 0

	var contactDetected bool
	var hasPreservingOutward bool
	var actorBase game.Pos
	var hasBase bool

	if root {
		contactDetected = hasContact(state, actor, &s.contactSeen, &s.contactQueue)
		if !contactDetected {
			actorBase, hasBase = state.Base(actor)
			incompleteSearch := false
			position.ForEachLegalAction(func(a game.Action) bool {
				if !s.running() {
					incompleteSearch = true
					return false
				}
				if a.Kind == game.PlaceNeutrals {
					return true
				}
				// Check if it is a halo Move targeting an empty cell
				isHaloMove := false
				if hasBase && adjacent(a.Target, actorBase) {
					if cell, ok := state.At(a.Target); ok && cell.Kind == game.Empty {
						isHaloMove = true
					}
				}
				if isHaloMove {
					return true
				}
				// Apply search on non-halo Move
				nextPos := position.ApplySearch(a)
				if nextPos.State().Active(actor) {
					hasPreservingOutward = true
					return false // stop enumeration (found one!)
				}
				return true
			})
			if incompleteSearch {
				return nil, legal, legalNeutrals, false
			}
		}
	}

	completed := true
	position.ForEachSearchAction(func(action game.Action) bool {
		legal++
		if action.Kind == game.PlaceNeutrals {
			legalNeutrals++
		}
		if !s.running() {
			completed = false
			return false
		}
		target, _ := state.At(action.Target)
		nextPosition := position.ApplySearch(action)
		next := nextPosition.State()
		order := actionOrder{}
		if next.GameOver() && next.Winner() == actor {
			order.win = 1
		}
		if next.Active(actor) {
			order.survives = 1
		}
		order.eliminations = beforeActive - activeCount(next)
		if action.Kind == game.Move && target.Kind == game.Normal && target.Owner != actor {
			order.capture = 1
		}
		if next.CurrentPlayer() == actor {
			order.continues = 1
		}
		if action.Kind == game.Move {
			order.pressure = -nearestEnemyBaseDistance(state, actor, action.Target)
		}

		if root && !contactDetected && hasPreservingOutward {
			// A dominated candidate is only a Move targeting an Empty cell in own-base 8-neighbor halo
			isDominatedHaloMove := false
			if action.Kind == game.Move && hasBase && adjacent(action.Target, actorBase) {
				if cell, ok := state.At(action.Target); ok && cell.Kind == game.Empty {
					isDominatedHaloMove = true
				}
			}
			if isDominatedHaloMove {
				return true // suppress it!
			}
		}

		children = append(children, child{action: action, position: nextPosition, order: order, ordinal: legal - 1})
		return true
	})
	if !completed {
		return nil, legal, legalNeutrals, false
	}
	sort.SliceStable(children, func(i, j int) bool { return betterOrder(children[i].order, children[j].order) })
	// Retain every forcing action. Only quiet survivors compete for bounded
	// optional slots; if any survivor exists, self-eliminating actions cannot
	// displace the preserving fallback.
	limit := interiorOptionalLimit
	if root {
		limit = rootOptionalLimit
	} else if s.multi {
		limit = multiOptionalLimit
	}
	selected := children[:0]
	optional := 0
	hasSurvivor := false
	for _, c := range children {
		if c.order.survives != 0 {
			hasSurvivor = true
			break
		}
	}
	for _, c := range children {
		if hasSurvivor && c.order.survives == 0 {
			continue
		}
		forcing := c.order.survives != 0 && (c.order.win != 0 || c.order.eliminations > 0 || c.order.capture != 0)
		if forcing || (c.order.survives != 0 && optional < limit) || (!hasSurvivor && optional < limit) {
			selected = append(selected, c)
			if !forcing {
				optional++
			}
		}
	}
	return selected, legal, legalNeutrals, true
}

func betterOrder(a, b actionOrder) bool {
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
		base, ok := state.Base(player)
		if !ok {
			continue
		}
		d := abs(target.Row-base.Row) + abs(target.Col-base.Col)
		if d < best {
			best = d
		}
	}
	return best
}

func abs(v int) int {
	if v < 0 {
		return -v
	}
	return v
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

func adjacent(a, b game.Pos) bool {
	row := a.Row - b.Row
	if row < 0 {
		row = -row
	}
	col := a.Col - b.Col
	if col < 0 {
		col = -col
	}
	return row <= 1 && col <= 1 && a != b
}

func hasContact(state game.State, actor game.Player, seen *[2500]bool, queue *[2500]int16) bool {
	rows, cols := state.Rows(), state.Cols()
	size := rows * cols
	if size > 2500 {
		return true
	}
	clear(seen[:size])

	actorBase, ok := state.Base(actor)
	if !ok {
		return false
	}
	baseCell, ok := state.At(actorBase)
	if !ok || baseCell.Owner != actor || baseCell.Kind != game.Base {
		return false
	}

	baseIdx := actorBase.Row*cols + actorBase.Col
	seen[baseIdx] = true
	queue[0] = int16(baseIdx)
	head, tail := 0, 1

	for head < tail {
		curr := int(queue[head])
		head++

		currPos := game.Pos{Row: curr / cols, Col: curr % cols}

		for r := currPos.Row - 1; r <= currPos.Row+1; r++ {
			for c := currPos.Col - 1; c <= currPos.Col+1; c++ {
				if r < 0 || r >= rows || c < 0 || c >= cols || (r == currPos.Row && c == currPos.Col) {
					continue
				}
				neighbor := game.Pos{Row: r, Col: c}
				nIdx := r*cols + c
				nCell, _ := state.At(neighbor)

				if nCell.Owner != actor && nCell.Owner != 0 && (nCell.Kind == game.Normal || nCell.Kind == game.Fortified) {
					return true
				}

				if !seen[nIdx] && nCell.Owner == actor {
					seen[nIdx] = true
					queue[tail] = int16(nIdx)
					tail++
				}
			}
		}
	}
	return false
}
