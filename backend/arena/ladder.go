package arena

import (
	"fmt"
	"math/rand"
	"runtime"

	"virusgame/game"
)

// ladderOrderSeed fixes the opening permutation so a sequential run is a pure
// function of (code, maxOpenings): same inputs => same game sequence, same stop
// point, same verdict. It matches the value the arena gate tests have always
// used; the test-side sequentialOrderSeed constant aliases it.
const ladderOrderSeed = 20260716

// SequentialResult is a Report plus the SPRT verdict against ThresholdPct.
type SequentialResult struct {
	Report
	ThresholdPct float64
	Stopped      bool
	Above        bool
}

// RandomLegalOpening plays ~8 pseudo-random legal plies from the empty
// rows x cols two-player board and returns the resulting non-terminal snapshot.
// The seed makes each opening distinct but reproducible. Balancing both seats
// over the shared position cancels any opening advantage. It generalizes the
// hardcoded-12x12 test helper to any board size.
func RandomLegalOpening(rows, cols int, seed uint64) (game.Snapshot, error) {
	const plies = 8
	rng := rand.New(rand.NewSource(int64(seed)))
	state, err := game.New(rows, cols, 2)
	if err != nil {
		return game.Snapshot{}, err
	}
	for ply := 0; ply < plies; ply++ {
		actions := state.LegalActions()
		if len(actions) == 0 || state.GameOver() {
			return game.Snapshot{}, fmt.Errorf("opening seed %d went terminal after %d plies", seed, ply)
		}
		next, err := state.Apply(actions[rng.Intn(len(actions))])
		if err != nil {
			return game.Snapshot{}, fmt.Errorf("opening seed %d: illegal random ply: %w", seed, err)
		}
		state = next
	}
	if state.GameOver() {
		return game.Snapshot{}, fmt.Errorf("opening seed %d is terminal", seed)
	}
	return state.Snapshot(), nil
}

// WilsonDecision is the SPRT-style stop rule: stop once games >= minGames and
// the Wilson 95% interval lies entirely on one side of thresholdPct. above
// reports which side. minGames guards against a premature stop on the first
// pair.
func WilsonDecision(wins, games int, thresholdPct float64, minGames int) (stop, above bool) {
	if games < minGames {
		return false, false
	}
	interval := Wilson95(wins, games)
	switch {
	case interval.Low > thresholdPct:
		return true, true
	case interval.High < thresholdPct:
		return true, false
	}
	return false, false
}

// openingPair holds both balanced-seat games played from one opening.
type openingPair struct {
	seat [2]GameResult
	err  error
}

// PlaySequentialOpenings plays both seats of each seeded opening in a
// fixed-seed permutation over [0,maxOpenings), accumulating a's report in
// permutation order (seat 0 then seat 1 of each opening) and stopping as soon
// as WilsonDecision resolves the matchup against thresholdPct — else at the
// opening cap. The verdict {Games,Wins,Stopped,Above} is byte-identical to a
// serial run for any order-independent (per-decision-deterministic) agents:
// openings are computed across a pool of `workers` goroutines but folded and
// decided strictly in permutation order. workers<=0 defaults to GOMAXPROCS.
// It returns an error on any illegal/stalled/maxed game.
//
// Determinism note: the node-budget and heuristic ladder agents (Greedy,
// BaseAttacker, MobilityAttacker, MobilityBaseAttacker, TelemetryNodeBudget,
// incumbent, CutSeeker) are pure functions of the position — goroutine-safe and
// order-independent — so parallel and serial runs agree exactly. Agents that
// carry RNG state across games (Random, and Legacy which wraps it) are neither
// goroutine-safe nor order-independent; run those at workers=1.
func PlaySequentialOpenings(rows, cols, maxOpenings int, thresholdPct float64, minGames int, a, b TelemetryAgent, workers int) (SequentialResult, error) {
	result := SequentialResult{ThresholdPct: thresholdPct}
	if maxOpenings <= 0 {
		return result, nil
	}
	if workers <= 0 {
		workers = runtime.GOMAXPROCS(0)
	}
	order := rand.New(rand.NewSource(ladderOrderSeed)).Perm(maxOpenings)

	playPair := func(idx int) openingPair {
		snapshot, err := RandomLegalOpening(rows, cols, uint64(idx)+1)
		if err != nil {
			return openingPair{err: err}
		}
		var pair openingPair
		for seat := 0; seat < 2; seat++ {
			agents := []TelemetryAgent{a, b}
			if seat == 1 {
				agents[0], agents[1] = agents[1], agents[0]
			}
			played, err := Play(Match{Rows: rows, Cols: cols, Initial: &snapshot, TelemetryAgents: agents})
			if err != nil {
				return openingPair{err: fmt.Errorf("opening %d seat %d: %w", idx, seat, err)}
			}
			if played.Illegal != 0 || played.Stalled || played.Maxed {
				return openingPair{err: fmt.Errorf("opening %d seat %d produced illegal/stalled/maxed game: %+v", idx, seat, played)}
			}
			pair.seat[seat] = played
		}
		return pair
	}

	// Process in windows of `workers`: launch a window in parallel, fold its
	// results strictly in permutation order, and short-circuit the instant the
	// SPRT verdict resolves. Extra openings in the stop window are discarded.
	for start := 0; start < maxOpenings; start += workers {
		end := min(start+workers, maxOpenings)
		chans := make([]chan openingPair, end-start)
		for i := start; i < end; i++ {
			ch := make(chan openingPair, 1)
			chans[i-start] = ch
			go func(pos int, ch chan openingPair) { ch <- playPair(order[pos]) }(i, ch)
		}
		for i := start; i < end; i++ {
			pair := <-chans[i-start]
			if pair.err != nil {
				return result, pair.err
			}
			result.Add(pair.seat[0], game.Player(1))
			result.Add(pair.seat[1], game.Player(2))
			if stop, above := WilsonDecision(result.Wins, result.Games, thresholdPct, minGames); stop {
				result.Stopped, result.Above = true, above
				return result, nil
			}
		}
	}
	return result, nil
}
