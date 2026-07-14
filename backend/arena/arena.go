// Package arena runs reproducible headless Virus tournaments.
package arena

import (
	"fmt"
	"sort"
	"time"

	"virusgame/game"
)

type Agent func(game.State) (game.Action, bool)

type Board struct{ Rows, Cols int }
type OpponentFactory func(seed uint64) Agent

type Match struct {
	Rows, Cols int
	Agents     []Agent
	MaxActions int
}

type GameResult struct {
	Winner       game.Player
	Actions      int
	Decisions    int
	Eliminations int
	Illegal      int
	Maxed        bool
	Stalled      bool
	Latencies    [4][]time.Duration
	Elapsed      time.Duration
}

type Report struct {
	Games, Wins, Losses, Draws int
	Eliminations, Illegal      int
	Decisions                  int
	Maxed, Stalled             int
	Latencies                  []time.Duration
	Elapsed                    time.Duration
}

func Play(match Match) (GameResult, error) {
	if len(match.Agents) < 2 || len(match.Agents) > 4 {
		return GameResult{}, fmt.Errorf("need 2-4 agents")
	}
	if match.MaxActions <= 0 {
		match.MaxActions = match.Rows * match.Cols * 12
	}
	state, err := game.New(match.Rows, match.Cols, len(match.Agents))
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
		action, ok := match.Agents[player-1](state)
		result.Latencies[player-1] = append(result.Latencies[player-1], time.Since(decisionStart))
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
	return fmt.Sprintf("games=%d wins=%d losses=%d draws=%d win_rate=%.1f%% eliminations=%d illegal=%d maxed=%d stalled=%d decisions=%d p50=%s p95=%s max=%s decisions/s=%.1f",
		r.Games, r.Wins, r.Losses, r.Draws, r.WinRate(), r.Eliminations, r.Illegal, r.Maxed, r.Stalled,
		r.Decisions, r.Percentile(50), r.Percentile(95), r.MaxLatency(), r.Throughput())
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
