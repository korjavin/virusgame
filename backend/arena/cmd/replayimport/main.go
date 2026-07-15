// Command replayimport converts a bounded /last_games response into immutable
// arena replay fixtures. Network fetching is deliberately outside this tool so
// the exact fetched bytes can be archived and reviewed before import.
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"virusgame/arena"
	"virusgame/game"
)

type recentResponse struct {
	Games []recentGame `json:"games"`
}

type recentGame struct {
	ID          string       `json:"id"`
	Player1     string       `json:"player1_name"`
	Player2     string       `json:"player2_name"`
	Termination string       `json:"termination"`
	Rows        int          `json:"rows"`
	Cols        int          `json:"cols"`
	Result      int          `json:"result"`
	PGN         []recentTurn `json:"pgn_content"`
}

type recentTurn struct {
	Turn   int          `json:"turn"`
	Player game.Player  `json:"player"`
	Moves  []recentMove `json:"moves"`
}

type recentMove struct {
	Type  string     `json:"type"`
	Row   int        `json:"row"`
	Col   int        `json:"col"`
	Cells []game.Pos `json:"cells"`
}

func main() {
	input := flag.String("input", "", "saved /last_games JSON response")
	output := flag.String("output-dir", "", "fixture output directory")
	flag.Parse()
	if *input == "" || *output == "" {
		panic("-input and -output-dir are required")
	}
	data, err := os.ReadFile(*input)
	if err != nil {
		panic(err)
	}
	var response recentResponse
	decoderError := json.Unmarshal(data, &response)
	if decoderError != nil {
		panic(decoderError)
	}
	for _, source := range response.Games {
		replay := arena.Replay{SourceID: source.ID, Players: [2]string{source.Player1, source.Player2}, Rows: source.Rows, Cols: source.Cols, Winner: game.Player(source.Result), Termination: source.Termination, ObservedTurns: len(source.PGN)}
		state, err := game.New(source.Rows, source.Cols, 2)
		if err != nil {
			panic(err)
		}
		for _, sourceTurn := range source.PGN {
			turn := arena.ReplayTurn{Number: sourceTurn.Turn, Player: sourceTurn.Player}
			for _, sourceMove := range sourceTurn.Moves {
				if state.GameOver() {
					replay.OmittedMoves++
					continue
				}
				move := arena.ReplayMove{Row: sourceMove.Row, Col: sourceMove.Col}
				var action game.Action
				switch sourceMove.Type {
				case "place", "attack":
					move.Kind = "move"
					action = game.Action{Kind: game.Move, Target: game.Pos{Row: move.Row, Col: move.Col}}
				case "neutral":
					move.Kind, move.Neutrals = "neutral", sourceMove.Cells
					if len(move.Neutrals) != 2 {
						panic(fmt.Sprintf("game %s: neutral needs two cells", source.ID))
					}
					action = game.Action{Kind: game.PlaceNeutrals, Neutrals: [2]game.Pos{move.Neutrals[0], move.Neutrals[1]}}
				default:
					panic(fmt.Sprintf("game %s: unknown move type %q", source.ID, sourceMove.Type))
				}
				state, err = state.Apply(action)
				if err != nil {
					panic(fmt.Sprintf("game %s turn %d: %v", source.ID, sourceTurn.Turn, err))
				}
				turn.Actions = append(turn.Actions, move)
			}
			if len(turn.Actions) != 0 {
				turn.Number = len(replay.Turns) + 1
				replay.Turns = append(replay.Turns, turn)
			}
		}
		encoded, err := json.MarshalIndent(replay, "", "  ")
		if err != nil {
			panic(err)
		}
		name := fmt.Sprintf("production-%dx%d-%s-%s.json", replay.Rows, replay.Cols, strings.ReplaceAll(replay.Termination, "_", "-"), replay.SourceID)
		if err := os.WriteFile(filepath.Join(*output, name), append(encoded, '\n'), 0o644); err != nil {
			panic(err)
		}
	}
}
