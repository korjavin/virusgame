// Package arena runs reproducible headless Virus tournaments.
package arena

import (
	"fmt"
	"math"
	"sort"
	"time"

	"virusgame/game"
)

type Agent func(game.State) (game.Action, bool)

// DecisionTelemetry is emitted by search-aware agents. CompletedTurnDepth is
// the number of whole turns fully searched. SearchedRoot fields count the
// authoritative root candidates covered by the last completed iteration; they
// are zero when no iteration completed, even if an aborted iteration visited
// some candidates. LegalRoot fields always describe the authoritative set.
type DecisionTelemetry struct {
	Nodes                                   uint64
	Evaluations                             uint64
	CompletedTurnDepth                      int
	LegalRootActions, SearchedRootActions   int
	LegalRootNeutrals, SearchedRootNeutrals int
	BudgetShortfall                         bool
}

type TelemetryAgent func(game.State) (game.Action, DecisionTelemetry, bool)

type Board struct{ Rows, Cols int }
type OpponentFactory func(seed uint64) Agent
type TelemetryOpponentFactory func(seed uint64) TelemetryAgent

type Match struct {
	Rows, Cols int
	// Initial, when present, is validated by game.FromSnapshot and replaces the
	// empty board. It lets strength comparisons start from a frozen identical
	// position instead of replaying the same deterministic opening as a "seed".
	Initial         *game.Snapshot
	Agents          []Agent
	TelemetryAgents []TelemetryAgent
	MaxActions      int
}

type GameResult struct {
	Winner                                  game.Player
	Actions                                 int
	Decisions                               int
	Eliminations                            int
	Illegal                                 int
	Maxed                                   bool
	Stalled                                 bool
	Latencies                               [4][]time.Duration
	Nodes                                   [4]uint64
	Evaluations                             [4]uint64
	BudgetShortfalls                        [4]int
	LegalRootActions, SearchedRootActions   [4]int
	LegalRootNeutrals, SearchedRootNeutrals [4]int
	CompletedTurnDepth                      [4]int
	Elapsed                                 time.Duration
}

type Report struct {
	Games, Wins, Losses, Draws              int
	Eliminations, Illegal                   int
	Decisions                               int
	Maxed, Stalled                          int
	Latencies                               []time.Duration
	Nodes                                   uint64
	Evaluations                             uint64
	BudgetShortfalls                        int
	LegalRootActions, SearchedRootActions   int
	LegalRootNeutrals, SearchedRootNeutrals int
	CompletedTurnDepth                      int
	Elapsed                                 time.Duration
}

// Probe runs one instrumented decision per board. It is intended for maximum
// dimension legality/deadline stress, not as a competitive strength claim.
func Probe(boards []Board, agent TelemetryAgent) (Report, error) {
	var report Report
	for _, board := range boards {
		state, err := game.New(board.Rows, board.Cols, 2)
		if err != nil {
			return report, err
		}
		started := time.Now()
		action, telemetry, ok := agent(state)
		latency := time.Since(started)
		report.Games++
		report.Decisions++
		report.Elapsed += latency
		report.Latencies = append(report.Latencies, latency)
		report.Nodes += telemetry.Nodes
		report.Evaluations += telemetry.Evaluations
		report.LegalRootActions += telemetry.LegalRootActions
		report.SearchedRootActions += telemetry.SearchedRootActions
		report.LegalRootNeutrals += telemetry.LegalRootNeutrals
		report.SearchedRootNeutrals += telemetry.SearchedRootNeutrals
		if telemetry.BudgetShortfall {
			report.BudgetShortfalls++
		}
		if telemetry.CompletedTurnDepth > report.CompletedTurnDepth {
			report.CompletedTurnDepth = telemetry.CompletedTurnDepth
		}
		if !ok {
			report.Stalled++
			continue
		}
		if _, err := state.Apply(action); err != nil {
			report.Illegal++
		}
	}
	return report, nil
}

func Play(match Match) (GameResult, error) {
	agentCount := len(match.Agents)
	if len(match.TelemetryAgents) > 0 {
		agentCount = len(match.TelemetryAgents)
	}
	if agentCount < 2 || agentCount > 4 || len(match.Agents) > 0 && len(match.TelemetryAgents) > 0 {
		return GameResult{}, fmt.Errorf("need 2-4 agents")
	}
	if match.MaxActions <= 0 {
		match.MaxActions = match.Rows * match.Cols * 12
	}
	var state game.State
	var err error
	if match.Initial != nil {
		state, err = game.FromSnapshot(*match.Initial)
		if err == nil && len(match.Initial.Bases) != agentCount {
			err = fmt.Errorf("initial snapshot has %d players, need %d agents", len(match.Initial.Bases), agentCount)
		} else if err == nil && (state.Rows() != match.Rows || state.Cols() != match.Cols) {
			err = fmt.Errorf("initial snapshot dimensions %dx%d do not match %dx%d", state.Rows(), state.Cols(), match.Rows, match.Cols)
		}
	} else {
		state, err = game.New(match.Rows, match.Cols, agentCount)
	}
	if err != nil {
		return GameResult{}, err
	}
	result := GameResult{}
	started := time.Now()
	for !state.GameOver() && result.Actions < match.MaxActions {
		player := state.CurrentPlayer()
		legal := state.LegalActions()
		before := activeCount(state)
		decisionStart := time.Now()
		result.Decisions++
		var action game.Action
		var telemetry DecisionTelemetry
		var ok bool
		if len(match.TelemetryAgents) > 0 {
			action, telemetry, ok = match.TelemetryAgents[player-1](state)
		} else {
			action, ok = match.Agents[player-1](state)
		}
		result.Latencies[player-1] = append(result.Latencies[player-1], time.Since(decisionStart))
		result.Nodes[player-1] += telemetry.Nodes
		result.Evaluations[player-1] += telemetry.Evaluations
		result.LegalRootActions[player-1] += telemetry.LegalRootActions
		result.SearchedRootActions[player-1] += telemetry.SearchedRootActions
		result.LegalRootNeutrals[player-1] += telemetry.LegalRootNeutrals
		result.SearchedRootNeutrals[player-1] += telemetry.SearchedRootNeutrals
		if telemetry.BudgetShortfall {
			result.BudgetShortfalls[player-1]++
		}
		if telemetry.CompletedTurnDepth > result.CompletedTurnDepth[player-1] {
			result.CompletedTurnDepth[player-1] = telemetry.CompletedTurnDepth
		}
		if !ok {
			if len(legal) > 0 {
				result.Illegal++
			}
			result.Stalled = true
			break
		}
		next, err := state.Apply(action)
		if err != nil {
			result.Illegal++
			break
		}
		result.Actions++
		result.Eliminations += before - activeCount(next)
		state = next
	}
	result.Elapsed = time.Since(started)
	result.Winner = state.Winner()
	result.Maxed = !state.GameOver() && result.Actions >= match.MaxActions
	return result, nil
}

// Interval is a binomial confidence interval expressed as percentages.
type Interval struct{ Low, High float64 }

// Wilson95 returns the Wilson score interval for wins out of games. Draws are
// deliberately not counted as half-wins: the superior-engine gate is about
// demonstrated wins, and this conservative definition cannot hide draws.
func Wilson95(wins, games int) Interval {
	if games == 0 {
		return Interval{}
	}
	const z = 1.959963984540054
	n := float64(games)
	p := float64(wins) / n
	denominator := 1 + z*z/n
	center := (p + z*z/(2*n)) / denominator
	margin := z * math.Sqrt((p*(1-p)+z*z/(4*n))/n) / denominator
	return Interval{Low: 100 * (center - margin), High: 100 * (center + margin)}
}

// Compare runs a balanced incumbent comparison and gives the relationship an
// explicit name for CLI/report consumers.
func Compare(boards []Board, seeds int, contender Agent, incumbent OpponentFactory) (Report, error) {
	return Balanced(boards, seeds, contender, incumbent)
}

// CompareTelemetry is Compare with per-decision search instrumentation. Plain
// baselines can be adapted with Instrument.
func CompareTelemetry(boards []Board, seeds int, contender TelemetryAgent, incumbent TelemetryOpponentFactory) (Report, error) {
	var report Report
	for boardIndex, board := range boards {
		for seed := 1; seed <= seeds; seed++ {
			for seat := 0; seat < 2; seat++ {
				agents := []TelemetryAgent{contender, incumbent(uint64(boardIndex*10_000 + seed))}
				if seat == 1 {
					agents[0], agents[1] = agents[1], agents[0]
				}
				result, err := Play(Match{Rows: board.Rows, Cols: board.Cols, TelemetryAgents: agents})
				if err != nil {
					return report, err
				}
				report.Add(result, game.Player(seat+1))
			}
		}
	}
	return report, nil
}

// Balanced runs every board/seed twice with the contender in each seat.
func Balanced(boards []Board, seeds int, contender Agent, opponent OpponentFactory) (Report, error) {
	var report Report
	for boardIndex, board := range boards {
		for seed := 1; seed <= seeds; seed++ {
			for seat := 0; seat < 2; seat++ {
				agents := []Agent{contender, opponent(uint64(boardIndex*10_000 + seed))}
				if seat == 1 {
					agents[0], agents[1] = agents[1], agents[0]
				}
				result, err := Play(Match{Rows: board.Rows, Cols: board.Cols, Agents: agents})
				if err != nil {
					return report, err
				}
				report.Add(result, game.Player(seat+1))
			}
		}
	}
	return report, nil
}

func (r *Report) Add(result GameResult, focus game.Player) {
	r.Games++
	r.Eliminations += result.Eliminations
	r.Illegal += result.Illegal
	r.Decisions += result.Decisions
	r.Elapsed += result.Elapsed
	r.Latencies = append(r.Latencies, result.Latencies[focus-1]...)
	r.Nodes += result.Nodes[focus-1]
	r.Evaluations += result.Evaluations[focus-1]
	r.LegalRootActions += result.LegalRootActions[focus-1]
	r.SearchedRootActions += result.SearchedRootActions[focus-1]
	r.LegalRootNeutrals += result.LegalRootNeutrals[focus-1]
	r.SearchedRootNeutrals += result.SearchedRootNeutrals[focus-1]
	r.BudgetShortfalls += result.BudgetShortfalls[focus-1]
	if result.CompletedTurnDepth[focus-1] > r.CompletedTurnDepth {
		r.CompletedTurnDepth = result.CompletedTurnDepth[focus-1]
	}
	if result.Maxed {
		r.Maxed++
	}
	if result.Stalled {
		r.Stalled++
	}
	switch result.Winner {
	case focus:
		r.Wins++
	case 0:
		r.Draws++
	default:
		r.Losses++
	}
}

func (r Report) WinRate() float64 {
	if r.Games == 0 {
		return 0
	}
	return 100 * float64(r.Wins) / float64(r.Games)
}

func (r Report) Percentile(percent int) time.Duration {
	if len(r.Latencies) == 0 {
		return 0
	}
	values := append([]time.Duration(nil), r.Latencies...)
	sort.Slice(values, func(i, j int) bool { return values[i] < values[j] })
	index := (len(values)*percent + 99) / 100
	if index < 1 {
		index = 1
	}
	return values[index-1]
}

func (r Report) MaxLatency() time.Duration {
	return r.Percentile(100)
}

func (r Report) Throughput() float64 {
	if r.Elapsed <= 0 {
		return 0
	}
	return float64(r.Decisions) / r.Elapsed.Seconds()
}

func (r Report) String() string {
	return fmt.Sprintf("games=%d wins=%d losses=%d draws=%d win_rate=%.1f%% eliminations=%d illegal=%d maxed=%d stalled=%d decisions=%d nodes=%d completed_turn_depth=%d p50=%s p95=%s max=%s decisions/s=%.1f",
		r.Games, r.Wins, r.Losses, r.Draws, r.WinRate(), r.Eliminations, r.Illegal, r.Maxed, r.Stalled,
		r.Decisions, r.Nodes, r.CompletedTurnDepth, r.Percentile(50), r.Percentile(95), r.MaxLatency(), r.Throughput())
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
