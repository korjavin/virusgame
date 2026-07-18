package arena

import "virusgame/game"

// nnuefeatures.go is the arena-side mirror of the cheap per-player quantities
// that backend/search/evaluate.go computes in analyzeWithConnectivity /
// spaceRace / articulationPointsInto. Those functions are UNEXPORTED and the
// frozen search package is off limits (vs-ai2.56 architect directive), so this
// file reproduces the same computations using only the public game.State API.
// The output is a fixed-order feature vector per player for the offline
// NNUE-lite data generator + trainer; it is NOT wired into search.
//
// Coverage vs evaluate.go (honest enumeration):
//
//	COVERED (per player, same definitions as analyzeWithConnectivity):
//	  normal, fortified, connected, disconnected, mobility, captures,
//	  baseExits, baseOpenings, baseAnchors, baseThreat, threatened,
//	  threatenedLoss, threatTempo, sealed-base flag, neutral-unused flag,
//	  moves-left tempo, plus the Voronoi space-race first-reach count
//	  (spaceRace) and, from the articulation analysis, the cut COUNT and the
//	  MAX single-cut cutLoss (downstream subtree lost when one articulation
//	  cell is captured).
//
//	vs-ai2.56 OWNER-PROFILE ADDITIONS (per player, see nnueStructural):
//	  (a) threat-gated own-cut-risk: ThreatenedCuts, MinCutThreatDist
//	  (b) forward advance + openness: MinEnemyBaseDist, FrontOpenness
//	  (c) front width / chain potential: FrontWidth, ChainReach
//	  (d) severable mass, normalized: SeverableFrac (opponent-side severable mass
//	      is already present as each opponent seat's own MaxCutLoss/SeverableFrac
//	      in the 4×K matrix).
//
//	OMITTED, and why acceptable for a learned feature vector:
//	  - The cross-player PREDATORY-CUT term (evaluate.go's second pass, where
//	    one player scores against an OPPONENT's articulation cells adjacent to
//	    its own territory). It is a pairwise interaction between two players'
//	    analyses; a per-player vector cannot hold it. The learner instead sees
//	    each player's own Articulation/MaxCutLoss and can approximate the
//	    fragility signal; the exact predatory bonus is a Stage-3 concern.
//	  - The exact per-cell threatenedLoss WEIGHTING as folded into the final
//	    ratio()/normalized() score (denominators max(1,connected), area, owned,
//	    the threatTempo multiplier, spaceRaceWeight, etc.). We emit the raw
//	    integer tallies and let the trainer learn its own weights/scales; the
//	    hand-set EvalParams are exactly what the network is meant to replace.
//	  - The final self-minus-mean-opponent aggregation and the sealed-base /
//	    mate special-cases in evaluateAllWithWorkspace: those are score
//	    assembly, not features. We surface the raw sealed-base flag and let the
//	    trainer combine perspectives.
//
// ponytail: recomputes connectivity/articulation/space per NNUEFeatures call
// with fresh allocations (no shared evalWorkspace). Boards are small and this
// is offline tooling, so buffer reuse is not worth the complexity; add pooling
// only if profiling the full production run shows it matters.

// PlayerFeatures is the fixed-order per-player feature vector. Field order is
// stable and is the order Features() flattens; do not reorder without bumping
// the data-generation schema.
type PlayerFeatures struct {
	Normal         int
	Fortified      int
	Connected      int
	Disconnected   int
	Mobility       int
	Captures       int
	BaseExits      int
	BaseOpenings   int
	BaseAnchors    int
	BaseThreat     int
	Threatened     int
	ThreatenedLoss int
	ThreatTempo    int
	Articulation   int  // number of articulation (cut) cells in own territory
	MaxCutLoss     int  // largest downstream subtree lost on a single cut
	SpaceRace      int  // Voronoi first-reach empty-cell count
	SealedBase     bool // baseExits+baseOpenings == 0
	NeutralUnused  bool // player has not yet spent its neutral placement
	MovesLeftTempo int  // state.MovesLeft() if this seat is to move, else 0

	// vs-ai2.56 owner-profile features. See the file doc for the "why". All are
	// per-player; NNUEFeatures emits a vector for every active seat, so the
	// opponent-side of each (severable mass, cut risk, advance) is already in the
	// 4×K matrix — no cross-player term is baked in here.
	ThreatenedCuts   int     // (a) own articulation cells adjacent to enemy connected territory (threat-gated count)
	MinCutThreatDist int     // (a) min Chebyshev dist from an own articulation cell to the nearest enemy stone; rows+cols = none
	MinEnemyBaseDist int     // (b) min Chebyshev dist from an own connected cell to the nearest enemy base; rows+cols = none
	FrontOpenness    int     // (b) distinct empty cells adjacent to own frontier cells
	FrontWidth       int     // (c) size of the largest contiguous own-frontier group (8-connected)
	ChainReach       int     // (c) largest enemy-Normal cluster in capture-contact with own territory
	SeverableFrac    float64 // (d) MaxCutLoss / max(1,Connected) — single-cut loss normalized by own mass
}

// nnueFeatureCount is the length of the Features() flat vector.
const nnueFeatureCount = 26

// Features flattens the struct into a stable-order float64 slice for the
// generator/trainer. Bool flags map to 1.0/0.0.
func (f PlayerFeatures) Features() []float64 {
	b := func(v bool) float64 {
		if v {
			return 1
		}
		return 0
	}
	return []float64{
		float64(f.Normal), float64(f.Fortified), float64(f.Connected),
		float64(f.Disconnected), float64(f.Mobility), float64(f.Captures),
		float64(f.BaseExits), float64(f.BaseOpenings), float64(f.BaseAnchors),
		float64(f.BaseThreat), float64(f.Threatened), float64(f.ThreatenedLoss),
		float64(f.ThreatTempo), float64(f.Articulation), float64(f.MaxCutLoss),
		float64(f.SpaceRace), b(f.SealedBase), b(f.NeutralUnused),
		float64(f.MovesLeftTempo),
		float64(f.ThreatenedCuts), float64(f.MinCutThreatDist),
		float64(f.MinEnemyBaseDist), float64(f.FrontOpenness),
		float64(f.FrontWidth), float64(f.ChainReach), f.SeverableFrac,
	}
}

// NNUEFeatures computes the per-player feature vectors for a position, indexed
// by seat-1. Inactive seats keep the zero value.
func NNUEFeatures(state game.State) [4]PlayerFeatures {
	var out [4]PlayerFeatures
	rows, cols := state.Rows(), state.Cols()
	cells := make([]game.Cell, rows*cols)
	for row := 0; row < rows; row++ {
		for col := 0; col < cols; col++ {
			cells[row*cols+col], _ = state.At(game.Pos{Row: row, Col: col})
		}
	}
	var connected [4][]bool
	for player := game.Player(1); player <= 4; player++ {
		if state.Active(player) {
			connected[player-1] = nnueConnected(state, cells, player)
		}
	}
	space := nnueSpaceRace(state, cells, connected)
	for player := game.Player(1); player <= 4; player++ {
		if !state.Active(player) {
			continue
		}
		out[player-1] = nnueAnalyze(state, player, cells, connected, space[player-1])
	}
	return out
}

// nnueConnected mirrors connectedCellsInto: BFS over cells owned by player,
// seeded from the player's base (only if the base cell is intact).
func nnueConnected(state game.State, cells []game.Cell, player game.Player) []bool {
	cols := state.Cols()
	seen := make([]bool, len(cells))
	base := basePosition(state, player)
	baseIndex := base.Row*cols + base.Col
	cell := cells[baseIndex]
	if cell.Owner != player || cell.Kind != game.Base {
		return seen
	}
	seen[baseIndex] = true
	queue := []int{baseIndex}
	for head := 0; head < len(queue); head++ {
		index := queue[head]
		pos := game.Pos{Row: index / cols, Col: index % cols}
		for _, next := range neighbors8(state, pos) {
			ni := next.Row*cols + next.Col
			if !seen[ni] && cells[ni].Owner == player {
				seen[ni] = true
				queue = append(queue, ni)
			}
		}
	}
	return seen
}

// nnueArticulation mirrors articulationPointsInto: Tarjan over the player's
// connected subgraph, returning the cut-cell mask and per-cell cutLoss (the
// cell itself plus every downstream component severed from the base). Only cut
// cells that are the player's own Normals survive (base/fortified are anchors,
// not losable cuts) — same post-filter as evaluate.go.
func nnueArticulation(state game.State, player game.Player, cells []game.Cell, connected []bool) (articulation []bool, cutLoss []int) {
	cols := state.Cols()
	size := len(connected)
	articulation = make([]bool, size)
	cutLoss = make([]int, size)
	discovery := make([]int, size)
	low := make([]int, size)
	parent := make([]int, size)
	subtree := make([]int, size)
	for i := range parent {
		parent[i] = -1
	}
	time := 0
	var visit func(int)
	visit = func(index int) {
		time++
		discovery[index], low[index] = time, time
		subtree[index] = 1
		children := 0
		pos := game.Pos{Row: index / cols, Col: index % cols}
		for _, next := range neighbors8(state, pos) {
			ni := next.Row*cols + next.Col
			if !connected[ni] {
				continue
			}
			if discovery[ni] == 0 {
				children++
				parent[ni] = index
				visit(ni)
				subtree[index] += subtree[ni]
				if low[ni] < low[index] {
					low[index] = low[ni]
				}
				if parent[index] == -1 && children > 1 || parent[index] != -1 && low[ni] >= discovery[index] {
					articulation[index] = true
					cutLoss[index] += subtree[ni]
				}
			} else if ni != parent[index] && discovery[ni] < low[index] {
				low[index] = discovery[ni]
			}
		}
	}
	base := basePosition(state, player)
	baseIndex := base.Row*cols + base.Col
	if baseIndex >= 0 && baseIndex < size && connected[baseIndex] {
		visit(baseIndex)
	}
	for index, live := range connected {
		if live && discovery[index] == 0 {
			visit(index)
		}
	}
	for index := range articulation {
		if !articulation[index] {
			continue
		}
		cell := cells[index]
		if cell.Kind != game.Normal || cell.Owner != player {
			articulation[index] = false
			cutLoss[index] = 0
		} else {
			cutLoss[index]++ // capturing the cut cell loses the cell itself too
		}
	}
	return articulation, cutLoss
}

// nnueSpaceRace mirrors spaceRace: one shared multi-source BFS over empty cells
// seeded from every active player's connected territory; each empty cell goes
// to its nearest owner, equidistant cells are contested (nobody).
func nnueSpaceRace(state game.State, cells []game.Cell, connected [4][]bool) [4]int {
	cols := state.Cols()
	size := len(cells)
	const contested = -2
	dist := make([]int, size)
	owner := make([]int, size)
	for i := range dist {
		dist[i] = -1
		owner[i] = -1
	}
	queue := make([]int, 0, size)
	for p := 0; p < 4; p++ {
		if connected[p] == nil {
			continue
		}
		for i := 0; i < size; i++ {
			if connected[p][i] {
				dist[i] = 0
				owner[i] = p
				queue = append(queue, i)
			}
		}
	}
	for head := 0; head < len(queue); head++ {
		idx := queue[head]
		d, o := dist[idx], owner[idx]
		pos := game.Pos{Row: idx / cols, Col: idx % cols}
		for _, n := range neighbors8(state, pos) {
			ni := n.Row*cols + n.Col
			if cells[ni].Kind != game.Empty {
				continue
			}
			if dist[ni] == -1 {
				dist[ni] = d + 1
				owner[ni] = o
				queue = append(queue, ni)
			} else if dist[ni] == d+1 && owner[ni] != o && owner[ni] != contested {
				owner[ni] = contested
			}
		}
	}
	var counts [4]int
	for i := 0; i < size; i++ {
		if cells[i].Kind == game.Empty && owner[i] >= 0 {
			counts[owner[i]]++
		}
	}
	return counts
}

// nnueAnalyze mirrors analyzeWithConnectivity's per-player tallies.
func nnueAnalyze(state game.State, player game.Player, cells []game.Cell, connected [4][]bool, spaceReach int) PlayerFeatures {
	cols := state.Cols()
	own := connected[player-1]
	articulation, cutLoss := nnueArticulation(state, player, cells, own)
	var f PlayerFeatures
	f.SpaceRace = spaceReach
	f.NeutralUnused = !state.NeutralUsed(player)
	if state.CurrentPlayer() == player {
		f.MovesLeftTempo = state.MovesLeft()
	}
	for _, cut := range articulation {
		if cut {
			f.Articulation++
		}
	}
	for _, loss := range cutLoss {
		if loss > f.MaxCutLoss {
			f.MaxCutLoss = loss
		}
	}

	targets := make([]bool, len(cells))
	for row := 0; row < state.Rows(); row++ {
		for col := 0; col < cols; col++ {
			pos := game.Pos{Row: row, Col: col}
			index := row*cols + col
			cell := cells[index]
			if cell.Owner == player {
				switch cell.Kind {
				case game.Normal:
					f.Normal++
				case game.Fortified:
					f.Fortified++
				}
				if own[index] {
					f.Connected++
				} else {
					f.Disconnected++
				}
			}
			if own[index] && cell.Kind == game.Normal && nnueThreatened(state, index, player, connected) {
				f.Threatened++
				if articulation[index] {
					f.ThreatenedLoss += cutLoss[index]
				}
			}
			if !own[index] {
				continue
			}
			for _, n := range neighbors8(state, pos) {
				ti := n.Row*cols + n.Col
				target := cells[ti]
				if !targets[ti] && (target.Kind == game.Empty || target.Kind == game.Normal && target.Owner != player) {
					targets[ti] = true
					f.Mobility++
					if target.Kind == game.Normal {
						f.Captures++
					}
				}
			}
		}
	}

	base := basePosition(state, player)
	baseIndex := base.Row*cols + base.Col
	for _, n := range neighbors8(state, base) {
		index := n.Row*cols + n.Col
		cell := cells[index]
		switch {
		case cell.Owner == player && own[index]:
			f.BaseExits++
			if cell.Kind == game.Fortified {
				f.BaseAnchors++
			}
		case cell.Kind == game.Empty:
			f.BaseOpenings++
		case cell.Kind == game.Normal && cell.Owner != player:
			f.BaseOpenings++
			if nnueThreatened(state, baseIndex, player, connected) {
				f.BaseThreat++
			}
		}
	}
	f.ThreatTempo = nnueThreatTempo(state, player)
	f.SealedBase = f.BaseExits+f.BaseOpenings == 0
	nnueStructural(state, player, cells, own, articulation, connected, &f)
	return f
}

// nnueStructural computes the vs-ai2.56 owner-profile features (a)-(d) on top of
// the analyzeWithConnectivity tallies already in f. Boards are small and this is
// offline tooling, so distances are plain O(cells) Chebyshev scans rather than
// BFS.
// ponytail: O(cells²) worst case per player (articulation × enemy stones). Fine
// for 8x8–12x12 offline generation; switch to a multi-source BFS if a much
// larger board ever enters the corpus.
func nnueStructural(state game.State, player game.Player, cells []game.Cell, own, articulation []bool, connected [4][]bool, f *PlayerFeatures) {
	cols := state.Cols()
	far := state.Rows() + state.Cols() // "unreachable" sentinel, above any Chebyshev distance
	cheb := func(a, b game.Pos) int {
		dr, dc := a.Row-b.Row, a.Col-b.Col
		if dr < 0 {
			dr = -dr
		}
		if dc < 0 {
			dc = -dc
		}
		return max(dr, dc)
	}

	// (a) threat-gated own-cut-risk: unguarded tendrils only matter when an enemy
	// can bite them, so count articulation cells that touch enemy territory and
	// measure how close the nearest enemy stone is to any cut cell.
	f.MinCutThreatDist = far
	for i, cut := range articulation {
		if !cut {
			continue
		}
		if nnueThreatened(state, i, player, connected) {
			f.ThreatenedCuts++
		}
		cutPos := game.Pos{Row: i / cols, Col: i % cols}
		for j, c := range cells {
			if c.Owner != 0 && c.Owner != player {
				if d := cheb(cutPos, game.Pos{Row: j / cols, Col: j % cols}); d < f.MinCutThreatDist {
					f.MinCutThreatDist = d
				}
			}
		}
	}

	// (b) forward advance + openness. Frontier = own connected cell adjacent to an
	// empty cell; advance = nearest own cell to an enemy base; openness = empty
	// cells the front can expand into.
	enemyBases := make([]game.Pos, 0, 3)
	for opp := game.Player(1); opp <= 4; opp++ {
		if opp != player && state.Active(opp) {
			enemyBases = append(enemyBases, basePosition(state, opp))
		}
	}
	f.MinEnemyBaseDist = far
	frontier := make([]bool, len(cells))
	openSeen := make([]bool, len(cells))
	for i := range cells {
		if !own[i] {
			continue
		}
		pos := game.Pos{Row: i / cols, Col: i % cols}
		for _, b := range enemyBases {
			if d := cheb(pos, b); d < f.MinEnemyBaseDist {
				f.MinEnemyBaseDist = d
			}
		}
		for _, n := range neighbors8(state, pos) {
			ni := n.Row*cols + n.Col
			if cells[ni].Kind == game.Empty {
				frontier[i] = true
				if !openSeen[ni] {
					openSeen[ni] = true
					f.FrontOpenness++
				}
			}
		}
	}

	// (c) front width + chain potential.
	f.FrontWidth = nnueLargestGroup(state, frontier)
	f.ChainReach = nnueChainReach(state, player, cells, own)

	// (d) severable mass normalized: material misleads (the thrown-win lesson), so
	// scale the biggest single-cut loss by own connected mass.
	if f.Connected > 0 {
		f.SeverableFrac = float64(f.MaxCutLoss) / float64(f.Connected)
	}
}

// nnueLargestGroup returns the size of the largest 8-connected component of the
// masked cells (used for the contiguous frontier span).
func nnueLargestGroup(state game.State, mask []bool) int {
	cols := state.Cols()
	seen := make([]bool, len(mask))
	best := 0
	for start := range mask {
		if !mask[start] || seen[start] {
			continue
		}
		seen[start] = true
		queue := []int{start}
		count := 0
		for head := 0; head < len(queue); head++ {
			idx := queue[head]
			count++
			pos := game.Pos{Row: idx / cols, Col: idx % cols}
			for _, n := range neighbors8(state, pos) {
				ni := n.Row*cols + n.Col
				if mask[ni] && !seen[ni] {
					seen[ni] = true
					queue = append(queue, ni)
				}
			}
		}
		if count > best {
			best = count
		}
	}
	return best
}

// nnueChainReach returns the size of the largest 8-connected cluster of enemy
// Normal cells that is in capture-contact with the player's territory — a cheap
// proxy for the longest capture chain reachable from the current front.
func nnueChainReach(state game.State, player game.Player, cells []game.Cell, own []bool) int {
	cols := state.Cols()
	enemyNormal := func(i int) bool {
		c := cells[i]
		return c.Kind == game.Normal && c.Owner != 0 && c.Owner != player
	}
	seen := make([]bool, len(cells))
	best := 0
	for start := range cells {
		if !enemyNormal(start) || seen[start] {
			continue
		}
		seen[start] = true
		queue := []int{start}
		count := 0
		contact := false
		for head := 0; head < len(queue); head++ {
			idx := queue[head]
			count++
			pos := game.Pos{Row: idx / cols, Col: idx % cols}
			for _, n := range neighbors8(state, pos) {
				ni := n.Row*cols + n.Col
				if own[ni] {
					contact = true
				}
				if enemyNormal(ni) && !seen[ni] {
					seen[ni] = true
					queue = append(queue, ni)
				}
			}
		}
		if contact && count > best {
			best = count
		}
	}
	return best
}

// nnueThreatened mirrors threatenedByConnected: is the cell at index adjacent
// to any opponent's connected territory?
func nnueThreatened(state game.State, index int, player game.Player, connected [4][]bool) bool {
	cols := state.Cols()
	pos := game.Pos{Row: index / cols, Col: index % cols}
	for opponent := game.Player(1); opponent <= 4; opponent++ {
		if opponent == player || !state.Active(opponent) || connected[opponent-1] == nil {
			continue
		}
		for _, n := range neighbors8(state, pos) {
			if connected[opponent-1][n.Row*cols+n.Col] {
				return true
			}
		}
	}
	return false
}

// nnueThreatTempo mirrors threatTempo.
func nnueThreatTempo(state game.State, player game.Player) int {
	if state.CurrentPlayer() == player {
		return max(1, 4-state.MovesLeft())
	}
	return max(1, state.MovesLeft())
}
