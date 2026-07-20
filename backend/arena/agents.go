package arena

import (
	"context"

	"virusgame/game"
	"virusgame/search"
	"virusgame/search/incumbent"
)

// Instrument adapts a baseline that has no search counters.
func Instrument(agent Agent) TelemetryAgent {
	return func(state game.State) (game.Action, DecisionTelemetry, bool) {
		action, ok := agent(state)
		return action, DecisionTelemetry{}, ok
	}
}

// TelemetryNodeBudget compares the current engine with the independently
// frozen incumbent under the same deterministic node ceiling.
func TelemetryNodeBudget(nodes uint64, frozen bool) TelemetryAgent {
	return func(state game.State) (game.Action, DecisionTelemetry, bool) {
		if frozen {
			result, ok := incumbent.ChooseNodeBudget(state, nodes)
			legal, searched, neutrals, searchedNeutrals := rootCoverage(state, result.Depth)
			return result.Action, DecisionTelemetry{Nodes: result.Nodes, Evaluations: result.Evaluations, CompletedTurnDepth: completedTurns(state.MovesLeft(), result.Depth), LegalRootActions: legal, SearchedRootActions: searched, LegalRootNeutrals: neutrals, SearchedRootNeutrals: searchedNeutrals, BudgetShortfall: !result.BudgetExhausted && !result.SearchComplete}, ok
		}
		result, ok := search.ChooseNodeBudget(state, nodes)
		telemetry := currentTelemetry(result)
		telemetry.BudgetShortfall = !result.BudgetExhausted && !result.SearchComplete
		return result.Action, telemetry, ok
	}
}

func rootCoverage(state game.State, completedDepth int) (legal, searched, neutrals, searchedNeutrals int) {
	actions := state.LegalActions()
	for _, action := range actions {
		if action.Kind == game.PlaceNeutrals {
			neutrals++
		}
	}
	legal = len(actions)
	if completedDepth > 0 {
		searched, searchedNeutrals = legal, neutrals
	}
	return
}

func Tournament(depth int) Agent {
	return func(state game.State) (game.Action, bool) {
		result, ok := search.ChooseDepth(context.Background(), state, depth)
		return result.Action, ok
	}
}

// TelemetryTournament is the fixed-depth CI agent with deterministic search
// counters. CompletedTurnDepth is a conservative lower bound derived from the
// root's remaining actions in the current turn.
func TelemetryTournament(depth int) TelemetryAgent {
	return func(state game.State) (game.Action, DecisionTelemetry, bool) {
		result, ok := search.ChooseDepth(context.Background(), state, depth)
		return result.Action, currentTelemetry(result), ok
	}
}

func TelemetryFrozenTournament(depth int) TelemetryAgent {
	return func(state game.State) (game.Action, DecisionTelemetry, bool) {
		result, ok := incumbent.ChooseDepth(context.Background(), state, depth)
		legal, searched, neutrals, searchedNeutrals := rootCoverage(state, result.Depth)
		return result.Action, DecisionTelemetry{Nodes: result.Nodes, Evaluations: result.Evaluations, CompletedTurnDepth: completedTurns(state.MovesLeft(), result.Depth), LegalRootActions: legal, SearchedRootActions: searched, LegalRootNeutrals: neutrals, SearchedRootNeutrals: searchedNeutrals}, ok
	}
}

// Production exercises the exact anytime search path and deadline used by the
// deployed bot. Keep deterministic Tournament agents for reproducible CI.
func Production() Agent {
	return func(state game.State) (game.Action, bool) {
		ctx, cancel := context.WithTimeout(context.Background(), search.ProductionBudget)
		defer cancel()
		result, ok := search.Choose(ctx, state)
		return result.Action, ok
	}
}

func TelemetryProduction() TelemetryAgent {
	return func(state game.State) (game.Action, DecisionTelemetry, bool) {
		ctx, cancel := context.WithTimeout(context.Background(), search.ProductionBudget)
		defer cancel()
		result, ok := search.Choose(ctx, state)
		return result.Action, currentTelemetry(result), ok
	}
}

func currentTelemetry(result search.Result) DecisionTelemetry {
	searched, searchedNeutrals := 0, 0
	if result.Depth > 0 {
		searched = result.RootSelected
		searchedNeutrals = result.RootSelectedNeutrals
	}
	return DecisionTelemetry{Nodes: result.Nodes, Evaluations: result.Evaluations,
		CompletedTurnDepth: result.CompletedTurnDepth, LegalRootActions: result.RootLegal,
		SearchedRootActions: searched, LegalRootNeutrals: result.RootLegalNeutrals,
		SearchedRootNeutrals: searchedNeutrals, Workers: result.Workers, RootCompleted: result.RootCompleted,
		IterationsStarted: result.IterationsStarted, IterationsCompleted: result.IterationsCompleted,
		SearchElapsed: result.Elapsed}
}

func TelemetryFrozenProduction() TelemetryAgent {
	return func(state game.State) (game.Action, DecisionTelemetry, bool) {
		ctx, cancel := context.WithTimeout(context.Background(), search.ProductionBudget)
		defer cancel()
		result, ok := incumbent.Choose(ctx, state)
		legal, searched, neutrals, searchedNeutrals := rootCoverage(state, result.Depth)
		return result.Action, DecisionTelemetry{Nodes: result.Nodes, Evaluations: result.Evaluations, CompletedTurnDepth: completedTurns(state.MovesLeft(), result.Depth), LegalRootActions: legal, SearchedRootActions: searched, LegalRootNeutrals: neutrals, SearchedRootNeutrals: searchedNeutrals}, ok
	}
}

func completedTurns(movesLeft, actionDepth int) int {
	if actionDepth < movesLeft {
		return 0
	}
	return 1 + (actionDepth-movesLeft)/3
}

func Random(seed uint64) Agent {
	if seed == 0 {
		seed = 1
	}
	return func(state game.State) (game.Action, bool) {
		actions := state.LegalActions()
		if len(actions) == 0 {
			return game.Action{}, false
		}
		seed ^= seed << 13
		seed ^= seed >> 7
		seed ^= seed << 17
		return actions[int(seed%uint64(len(actions)))], true
	}
}

// Legacy is a frozen approximation of the retired engine's dominant behavior:
// take a capture when available, otherwise choose a seeded legal expansion.
func Legacy(seed uint64) Agent {
	random := Random(seed)
	return func(state game.State) (game.Action, bool) {
		for _, action := range state.LegalActions() {
			if action.Kind != game.Move {
				continue
			}
			cell, _ := state.At(action.Target)
			if cell.Kind == game.Normal && cell.Owner != state.CurrentPlayer() {
				return action, true
			}
		}
		return random(state)
	}
}

// Greedy chooses the best immediate win, elimination, capture, fortification,
// mobility, and opponent-base pressure outcome with stable board-order ties.
func Greedy(state game.State) (game.Action, bool) {
	actions := state.LegalActions()
	if len(actions) == 0 {
		return game.Action{}, false
	}
	actor := state.CurrentPlayer()
	best, bestScore := actions[0], -1<<60
	beforeActive := activeCount(state)
	for _, action := range actions {
		target, _ := state.At(action.Target)
		next, err := state.Apply(action)
		if err != nil {
			continue
		}
		score := (beforeActive-activeCount(next))*100_000 + immediateMobility(next, actor)*20
		if next.GameOver() && next.Winner() == actor {
			score += 1_000_000
		}
		if action.Kind == game.PlaceNeutrals {
			score -= 1_000
		} else if target.Kind == game.Normal && target.Owner != actor {
			score += 10_000
		}
		score -= opponentBaseDistance(state, actor, action.Target)
		if score > bestScore {
			best, bestScore = action, score
		}
	}
	return best, true
}

// BaseAttacker is a tactical baseline that values immediate captures and a
// direct route to the opponent base. It is deliberately simpler than search,
// but punishes engines that expand without defending their base chain.
func BaseAttacker(state game.State) (game.Action, bool) {
	actions := state.LegalActions()
	if len(actions) == 0 {
		return game.Action{}, false
	}
	actor := state.CurrentPlayer()
	best, bestScore := actions[0], -1<<60
	for _, action := range actions {
		if action.Kind == game.PlaceNeutrals {
			continue
		}
		target, _ := state.At(action.Target)
		next, err := state.Apply(action)
		if err != nil {
			continue
		}
		score := -100 * opponentBaseDistance(state, actor, action.Target)
		if target.Kind == game.Normal && target.Owner != actor {
			score += 20_000
		}
		if next.GameOver() && next.Winner() == actor {
			score += 1_000_000
		}
		if score > bestScore {
			best, bestScore = action, score
		}
	}
	return best, true
}

// MobilityAttacker is a tactical baseline that chooses the successor with the
// fewest legal replies for the opponent, then prefers captures. It exposes
// shallow cut and confinement weaknesses without sharing production search.
func MobilityAttacker(state game.State) (game.Action, bool) {
	actions := state.LegalActions()
	if len(actions) == 0 {
		return game.Action{}, false
	}
	actor := state.CurrentPlayer()
	best, bestScore := actions[0], -1<<60
	for _, action := range actions {
		next, err := state.Apply(action)
		if err != nil {
			continue
		}
		score := immediateMobility(next, actor)
		if next.CurrentPlayer() != actor {
			score -= 50 * len(next.LegalActions())
		}
		target, _ := state.At(action.Target)
		if action.Kind == game.Move && target.Kind == game.Normal && target.Owner != actor {
			score += 10_000
		}
		if next.GameOver() && next.Winner() == actor {
			score += 1_000_000
		}
		if score > bestScore {
			best, bestScore = action, score
		}
	}
	return best, true
}

func immediateMobility(state game.State, player game.Player) int {
	if state.CurrentPlayer() == player {
		return len(state.LegalActions())
	}
	count := 0
	for row := 0; row < state.Rows(); row++ {
		for col := 0; col < state.Cols(); col++ {
			cell, _ := state.At(game.Pos{Row: row, Col: col})
			if cell.Owner == player {
				count++
			}
		}
	}
	return count
}

func opponentBaseDistance(state game.State, player game.Player, pos game.Pos) int {
	best := state.Rows() + state.Cols()
	for opponent := game.Player(1); opponent <= 4; opponent++ {
		if opponent == player || !state.Active(opponent) {
			continue
		}
		base := basePosition(state, opponent)
		distance := abs(pos.Row-base.Row) + abs(pos.Col-base.Col)
		if distance < best {
			best = distance
		}
	}
	return best
}

func basePosition(state game.State, player game.Player) game.Pos {
	switch player {
	case 1:
		return game.Pos{Row: 0, Col: 0}
	case 2:
		return game.Pos{Row: state.Rows() - 1, Col: state.Cols() - 1}
	case 3:
		return game.Pos{Row: 0, Col: state.Cols() - 1}
	default:
		return game.Pos{Row: state.Rows() - 1, Col: 0}
	}
}

func abs(value int) int {
	if value < 0 {
		return -value
	}
	return value
}
