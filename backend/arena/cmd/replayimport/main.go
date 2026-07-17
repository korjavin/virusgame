// Command replayimport converts a bounded /last_games response into immutable
// arena replay fixtures.
//
// Two modes:
//
//	-input file        import from a saved /last_games response (bytes archived
//	                   and reviewed before import).
//	-fetch N           harvest live: fetch /last_games?limit=N (N in {5,10,20}),
//	                   keep only human-won 1v1 games (result==1, no player3),
//	                   dedupe against existing fixtures, write each as a testdata
//	                   anchor, and pin it in the owner-loss corpus manifest.
//
// Every human win against the bot is a proven hole; -fetch harvests them all in
// one command. Output is deterministic: only new games are written and the
// manifest is sorted.
package main

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strconv"

	"virusgame/arena"
	"virusgame/game"
)

const defaultLastGamesURL = "https://vs.wandergeek.org/last_games"

type recentResponse struct {
	Games []recentGame `json:"games"`
}

type recentGame struct {
	ID          string       `json:"id"`
	Player1     string       `json:"player1_name"`
	Player2     string       `json:"player2_name"`
	Player3     string       `json:"player3_name"`
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
	output := flag.String("output-dir", "arena/testdata", "fixture output directory")
	fetch := flag.Int("fetch", 0, "harvest live from /last_games?limit=N (N must be 5, 10, or 20)")
	endpoint := flag.String("url", defaultLastGamesURL, "/last_games endpoint for -fetch")
	flag.Parse()

	if *output == "" {
		panic("-output-dir is required")
	}
	if (*input == "") == (*fetch == 0) {
		panic("exactly one of -input or -fetch is required")
	}

	if *fetch != 0 {
		harvest(*fetch, *endpoint, *output)
		return
	}
	importFile(*input, *output)
}

// importFile is the archived-bytes path: it writes every game in the saved
// response as a fixture (unfiltered), preserving the original behaviour.
func importFile(input, output string) {
	data, err := os.ReadFile(input)
	if err != nil {
		panic(err)
	}
	response := decode(data)
	for _, source := range response.Games {
		replay, err := reconstruct(source)
		if err != nil {
			panic(err)
		}
		writeFixture(output, replay)
	}
}

// harvest fetches the live feed and adds every new human-won 1v1 game to the
// owner-loss regression corpus.
func harvest(limit int, endpoint, output string) {
	if limit != 5 && limit != 10 && limit != 20 {
		panic("-fetch limit must be 5, 10, or 20")
	}
	response := decode(fetchLastGames(endpoint, limit))

	manifestPath := filepath.Join(output, filepath.Base(arena.OwnerCorpusManifest))
	corpus, err := arena.LoadOwnerCorpus(manifestPath)
	if err != nil {
		panic(err)
	}
	have := map[string]bool{}
	for _, entry := range corpus {
		have[entry.SourceID] = true
	}

	added := 0
	for _, source := range response.Games {
		if source.Result != 1 || source.Player3 != "" || have[source.ID] {
			continue // not a fresh human-won 1v1 game
		}
		replay, err := reconstruct(source)
		if err != nil {
			fmt.Fprintf(os.Stderr, "skip %s: reconstruct: %v\n", source.ID, err)
			continue
		}
		fingerprint, err := terminalFingerprint(replay)
		if err != nil {
			fmt.Fprintf(os.Stderr, "skip %s: %v\n", source.ID, err)
			continue
		}
		writeFixture(output, replay)
		corpus = append(corpus, arena.OwnerCorpusEntry{
			SourceID:            replay.SourceID,
			Players:             replay.Players,
			Rows:                replay.Rows,
			Cols:                replay.Cols,
			Termination:         replay.Termination,
			TerminalFingerprint: fingerprint,
		})
		have[source.ID] = true
		added++
		fmt.Printf("added %s (%s beat %s, %s, %d turns, terminal=%s)\n",
			replay.SourceID, replay.Players[0], replay.Players[1], replay.Termination, len(replay.Turns), fingerprint)
	}
	if err := arena.SaveOwnerCorpus(manifestPath, corpus); err != nil {
		panic(err)
	}
	fmt.Printf("owner-loss corpus: %d games (%d new)\n", len(corpus), added)
}

func fetchLastGames(endpoint string, limit int) []byte {
	query := url.Values{"limit": {strconv.Itoa(limit)}}
	resp, err := http.Get(endpoint + "?" + query.Encode())
	if err != nil {
		panic(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		panic(fmt.Sprintf("%s: HTTP %d", endpoint, resp.StatusCode))
	}
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		panic(err)
	}
	return body
}

func decode(data []byte) recentResponse {
	var response recentResponse
	if err := json.Unmarshal(data, &response); err != nil {
		panic(err)
	}
	return response
}

// reconstruct replays one recorded game through the authoritative rules and
// returns its immutable fixture form. It returns an error (rather than
// panicking) so the harvester can skip a game that no longer replays cleanly.
func reconstruct(source recentGame) (arena.Replay, error) {
	replay := arena.Replay{SourceID: source.ID, Players: [2]string{source.Player1, source.Player2}, Rows: source.Rows, Cols: source.Cols, Winner: game.Player(source.Result), Termination: source.Termination, ObservedTurns: len(source.PGN)}
	state, err := game.New(source.Rows, source.Cols, 2)
	if err != nil {
		return arena.Replay{}, err
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
					return arena.Replay{}, fmt.Errorf("game %s: neutral needs two cells", source.ID)
				}
				action = game.Action{Kind: game.PlaceNeutrals, Neutrals: [2]game.Pos{move.Neutrals[0], move.Neutrals[1]}}
			default:
				return arena.Replay{}, fmt.Errorf("game %s: unknown move type %q", source.ID, sourceMove.Type)
			}
			state, err = state.Apply(action)
			if err != nil {
				return arena.Replay{}, fmt.Errorf("game %s turn %d: %w", source.ID, sourceTurn.Turn, err)
			}
			turn.Actions = append(turn.Actions, move)
		}
		if len(turn.Actions) != 0 {
			turn.Number = len(replay.Turns) + 1
			replay.Turns = append(replay.Turns, turn)
		}
	}
	return replay, nil
}

// terminalFingerprint validates the reconstructed replay through DecodeReplay
// (so a broken game is never written) and pins its terminal position.
func terminalFingerprint(replay arena.Replay) (string, error) {
	encoded, err := json.Marshal(replay)
	if err != nil {
		return "", err
	}
	decoded, states, err := arena.DecodeReplay(bytes.NewReader(encoded))
	if err != nil {
		return "", fmt.Errorf("decode: %w", err)
	}
	return arena.StateFingerprint(states[len(decoded.Turns)])
}

func writeFixture(output string, replay arena.Replay) {
	encoded, err := json.MarshalIndent(replay, "", "  ")
	if err != nil {
		panic(err)
	}
	name := arena.FixtureName(replay.Rows, replay.Cols, replay.Termination, replay.SourceID)
	if err := os.WriteFile(filepath.Join(output, name), append(encoded, '\n'), 0o644); err != nil {
		panic(err)
	}
}
