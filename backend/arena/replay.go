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
	SourceID    string       `json:"source_id"`
	Players     [2]string    `json:"players"`
	Rows        int          `json:"rows"`
	Cols        int          `json:"cols"`
	Winner      game.Player  `json:"winner"`
	Termination string       `json:"termination"`
	Turns       []ReplayTurn `json:"turns"`
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
	state, err := game.New(replay.Rows, replay.Cols, 2)
	if err != nil {
		return Replay{}, nil, fmt.Errorf("new replay game: %w", err)
	}
	states := make(map[int]game.State, len(replay.Turns))
	for index, turn := range replay.Turns {
		if turn.Number != index+1 || turn.Player != state.CurrentPlayer() {
			return Replay{}, nil, fmt.Errorf("turn %d: expected player %d, got %d", turn.Number, state.CurrentPlayer(), turn.Player)
		}
		for moveIndex, move := range turn.Actions {
			action, err := move.action()
			if err != nil {
				return Replay{}, nil, fmt.Errorf("turn %d action %d: %w", turn.Number, moveIndex+1, err)
			}
			state, err = state.Apply(action)
			if err != nil {
				return Replay{}, nil, fmt.Errorf("turn %d action %d: %w", turn.Number, moveIndex+1, err)
			}
		}
		states[turn.Number] = state
	}
	if !state.GameOver() || state.Winner() != replay.Winner {
		return Replay{}, nil, fmt.Errorf("final result: got over=%v winner=%d, want winner=%d", state.GameOver(), state.Winner(), replay.Winner)
	}
	return replay, states, nil
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
