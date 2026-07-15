package arena

import (
	"encoding/json"
	"fmt"
	"io"

	"virusgame/game"
)

// Replay is the compact, engine-independent form of a recorded game. It can
// be checked into testdata without database timestamps or user metadata.
type Replay struct {
	SourceID      string       `json:"source_id"`
	Players       [2]string    `json:"players"`
	Rows          int          `json:"rows"`
	Cols          int          `json:"cols"`
	Winner        game.Player  `json:"winner"`
	Termination   string       `json:"termination"`
	ObservedTurns int          `json:"observed_turns,omitempty"`
	OmittedMoves  int          `json:"omitted_moves,omitempty"`
	Turns         []ReplayTurn `json:"turns"`
}

type ReplayTurn struct {
	Number  int          `json:"turn"`
	Player  game.Player  `json:"player"`
	Actions []ReplayMove `json:"actions"`
}

type ReplayMove struct {
	Kind     string     `json:"kind"`
	Row      int        `json:"row,omitempty"`
	Col      int        `json:"col,omitempty"`
	Neutrals []game.Pos `json:"neutrals,omitempty"`
}

type ReplayPoint struct {
	Turn, AfterActions int
}

// DecodeReplay validates and replays every action, returning immutable states
// after each complete recorded turn. A malformed or divergent fixture fails at
// the first action instead of silently producing a different position.
func DecodeReplay(reader io.Reader) (Replay, map[int]game.State, error) {
	var replay Replay
	decoder := json.NewDecoder(reader)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&replay); err != nil {
		return Replay{}, nil, fmt.Errorf("decode replay: %w", err)
	}
	if err := decoder.Decode(&struct{}{}); err != io.EOF {
		return Replay{}, nil, fmt.Errorf("decode replay: trailing content")
	}
	positions, err := ReplayPositions(replay)
	if err != nil {
		return Replay{}, nil, err
	}
	states := make(map[int]game.State, len(replay.Turns))
	for _, turn := range replay.Turns {
		states[turn.Number] = positions[ReplayPoint{Turn: turn.Number, AfterActions: len(turn.Actions)}]
	}
	state := states[len(replay.Turns)]
	if err := validateReplayResult(replay, state); err != nil {
		return Replay{}, nil, err
	}
	return replay, states, nil
}

// ReplayPositions reconstructs every within-turn action boundary through the
// authoritative rules. It supports tactical before/after annotations without
// copying brittle board snapshots into fixtures.
func ReplayPositions(replay Replay) (map[ReplayPoint]game.State, error) {
	state, err := game.New(replay.Rows, replay.Cols, 2)
	if err != nil {
		return nil, fmt.Errorf("new replay game: %w", err)
	}
	positions := make(map[ReplayPoint]game.State)
	for index, turn := range replay.Turns {
		if turn.Number != index+1 || turn.Player != state.CurrentPlayer() {
			return nil, fmt.Errorf("turn %d: expected player %d, got %d", turn.Number, state.CurrentPlayer(), turn.Player)
		}
		positions[ReplayPoint{Turn: turn.Number}] = state
		for moveIndex, move := range turn.Actions {
			action, err := move.action()
			if err != nil {
				return nil, fmt.Errorf("turn %d action %d: %w", turn.Number, moveIndex+1, err)
			}
			state, err = state.Apply(action)
			if err != nil {
				return nil, fmt.Errorf("turn %d action %d: %w", turn.Number, moveIndex+1, err)
			}
			positions[ReplayPoint{Turn: turn.Number, AfterActions: moveIndex + 1}] = state
		}
	}
	return positions, nil
}

func validateReplayResult(replay Replay, state game.State) error {
	if replay.ObservedTurns != 0 && replay.ObservedTurns < len(replay.Turns) || replay.OmittedMoves > 0 && replay.ObservedTurns <= len(replay.Turns) {
		return fmt.Errorf("final result: inconsistent observed turns or omitted moves")
	}
	if replay.Winner < 1 || replay.Winner > 2 {
		return fmt.Errorf("final result: invalid winner %d", replay.Winner)
	}
	switch replay.Termination {
	case "no_moves":
		if !state.GameOver() || state.Winner() != replay.Winner {
			return fmt.Errorf("final result: got over=%v winner=%d, want no_moves winner=%d", state.GameOver(), state.Winner(), replay.Winner)
		}
	case "resign", "resignation", "timeout", "disconnect", "illegal_move":
		// These are server/protocol outcomes, not board-rule terminal states. The
		// legal prefix must replay exactly, but fabricating State.GameOver would
		// make an illegal move look like a strategic elimination.
	default:
		return fmt.Errorf("final result: unknown termination %q", replay.Termination)
	}
	return nil
}

func (move ReplayMove) action() (game.Action, error) {
	switch move.Kind {
	case "move":
		return game.Action{Kind: game.Move, Target: game.Pos{Row: move.Row, Col: move.Col}}, nil
	case "neutral":
		if len(move.Neutrals) != 2 {
			return game.Action{}, fmt.Errorf("neutral action needs two cells")
		}
		return game.Action{Kind: game.PlaceNeutrals, Neutrals: [2]game.Pos{move.Neutrals[0], move.Neutrals[1]}}, nil
	default:
		return game.Action{}, fmt.Errorf("unknown action kind %q", move.Kind)
	}
}
