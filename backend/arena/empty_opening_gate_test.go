package arena

import (
	"fmt"
	"os"
	"sort"
	"testing"

	"virusgame/game"
)

// namedOpening is a seat-1 first turn (three move targets) played from the empty
// 12x12 board. From an otherwise empty board two deterministic node-budget
// agents play a single fixed line, so variety comes entirely from the opening:
// each namedOpening seeds a distinct reply family.
type namedOpening struct {
	name  string
	moves []game.Pos
}

// emptyOpeningLines enumerates a small family of distinct, legal seat-1 first
// turns from base (0,0): the human's signature 1,1/2,2/3,3 diagonal tendril,
// edge hugs, a compact corner block, wide fronts, and elbows. buildOpening
// re-validates every line against the live rules, so an illegal typo or a rules
// change fails loudly rather than silently dropping coverage.
func emptyOpeningLines() []namedOpening {
	return []namedOpening{
		{"diagonal-1,1/2,2/3,3", []game.Pos{{Row: 1, Col: 1}, {Row: 2, Col: 2}, {Row: 3, Col: 3}}},
		{"top-edge-hug", []game.Pos{{Row: 0, Col: 1}, {Row: 0, Col: 2}, {Row: 0, Col: 3}}},
		{"left-edge-hug", []game.Pos{{Row: 1, Col: 0}, {Row: 2, Col: 0}, {Row: 3, Col: 0}}},
		{"corner-block", []game.Pos{{Row: 0, Col: 1}, {Row: 1, Col: 0}, {Row: 1, Col: 1}}},
		{"wide-front-fan", []game.Pos{{Row: 1, Col: 1}, {Row: 0, Col: 2}, {Row: 2, Col: 0}}},
		{"deep-diagonal-elbow", []game.Pos{{Row: 1, Col: 1}, {Row: 2, Col: 2}, {Row: 2, Col: 3}}},
		{"diagonal-then-lateral", []game.Pos{{Row: 1, Col: 1}, {Row: 2, Col: 2}, {Row: 1, Col: 3}}},
		{"diagonal-then-down", []game.Pos{{Row: 1, Col: 1}, {Row: 2, Col: 2}, {Row: 3, Col: 2}}},
	}
}

// buildOpening applies a seat-1 first turn to the empty 12x12 board and returns
// the resulting snapshot, with player 2 to move. It fails on any illegal move.
func buildOpening(t *testing.T, line namedOpening) game.Snapshot {
	t.Helper()
	state, err := game.New(12, 12, 2)
	if err != nil {
		t.Fatal(err)
	}
	for i, pos := range line.moves {
		next, err := state.Apply(game.Action{Kind: game.Move, Target: pos})
		if err != nil {
			t.Fatalf("opening %q move %d %+v illegal: %v", line.name, i, pos, err)
		}
		state = next
	}
	if state.GameOver() || state.CurrentPlayer() != 2 {
		t.Fatalf("opening %q did not hand a live turn to player 2: over=%v player=%d",
			line.name, state.GameOver(), state.CurrentPlayer())
	}
	return state.Snapshot()
}

// decisive reports whether the Wilson 95% interval for wins/games excludes the
// 50% coin-flip line — the point past which more games cannot flip the verdict.
// It is the early-stopping trigger: a deterministic, order-stable SPRT-lite.
func decisive(wins, games int) bool {
	iv := Wilson95(wins, games)
	return iv.High < 50 || iv.Low > 50
}

// playOpeningFamilyGate plays engine vs opponent from both seats of every
// opening in order, stopping early once at least minOpenings openings are in and
// the engine's Wilson 95% interval has become decisive. It returns the engine's
// report and the number of openings actually played. Illegal/stalled/maxed games
// are fatal — a maxed game would otherwise deflate both rates as a silent draw.
func playOpeningFamilyGate(t *testing.T, label string, openings []namedOpening, engine, opponent TelemetryAgent, minOpenings int) (Report, int) {
	t.Helper()
	var report Report
	played := 0
	for _, line := range openings {
		snapshot := buildOpening(t, line)
		for seat := 0; seat < 2; seat++ {
			agents := []TelemetryAgent{engine, opponent}
			if seat == 1 {
				agents[0], agents[1] = agents[1], agents[0]
			}
			result, err := Play(Match{Rows: 12, Cols: 12, Initial: &snapshot, TelemetryAgents: agents})
			if err != nil {
				t.Fatalf("%s %q seat %d: %v", label, line.name, seat, err)
			}
			if result.Illegal != 0 || result.Stalled || result.Maxed {
				t.Fatalf("%s %q seat %d illegal/stalled/maxed: %+v", label, line.name, seat, result)
			}
			report.Add(result, game.Player(seat+1))
		}
		played++
		if played >= minOpenings && decisive(report.Wins, report.Games) {
			break
		}
	}
	return report, played
}

// botReplyCells returns the bot's owned normal/fortified cells (its reply
// footprint, base excluded) in stable board order.
func botReplyCells(state game.State, bot game.Player) []game.Pos {
	var cells []game.Pos
	for row := 0; row < state.Rows(); row++ {
		for col := 0; col < state.Cols(); col++ {
			cell, _ := state.At(game.Pos{Row: row, Col: col})
			if cell.Owner == bot && cell.Kind != game.Base {
				cells = append(cells, game.Pos{Row: row, Col: col})
			}
		}
	}
	return cells
}

// playOneTurn drives the current player through one full turn.
func playOneTurn(t *testing.T, state game.State, agent Agent) game.State {
	t.Helper()
	actor := state.CurrentPlayer()
	for state.CurrentPlayer() == actor && !state.GameOver() {
		action, ok := agent(state)
		if !ok {
			t.Fatalf("agent stalled at %+v", state.Snapshot())
		}
		next, err := state.Apply(action)
		if err != nil {
			t.Fatalf("illegal action %+v: %v", action, err)
		}
		state = next
	}
	return state
}

// TestBotOpeningReplyInvariance records the bot's first-turn reply to every
// human opening in the family. The red-team probe found the reply is
// opening-invariant — the same width-1 diagonal (10,10/9,9/8,8) regardless of
// how the human opened — which is why the enumerated opening variety produces
// near-identical bot lines. It is a documented part of the baseline picture and
// the anchor for the vs-ai2.39 jitter work; reported, not hard-asserted.
//
//	VS_EMPTY=1 go test ./arena -run TestBotOpeningReplyInvariance -v
func TestBotOpeningReplyInvariance(t *testing.T) {
	if os.Getenv("VS_EMPTY") != "1" {
		t.Skip("set VS_EMPTY=1 to run the bot opening-reply invariance report")
	}
	const nodes = 1000
	bot := nodeBudgetPlainAgent(nodes, false)
	openings := emptyOpeningLines()
	replies := map[string]bool{}
	t.Logf("| human opening | bot reply cells |")
	t.Logf("|---|---|")
	for _, line := range openings {
		snapshot := buildOpening(t, line)
		state, err := game.FromSnapshot(snapshot)
		if err != nil {
			t.Fatal(err)
		}
		state = playOneTurn(t, state, bot)
		reply := botReplyCells(state, 2)
		sort.Slice(reply, func(i, j int) bool { return lessPos(reply[i], reply[j]) })
		key := fmt.Sprint(reply)
		replies[key] = true
		t.Logf("| %s | %v |", line.name, reply)
	}
	t.Logf("distinct bot replies across %d openings: %d (invariant=%v)", len(openings), len(replies), len(replies) == 1)
}

// TestFromEmptyOpeningGate is the vs-ai2.40 from-empty measurement: deterministic
// node-budget (N=1000) balanced-seat 12x12 games starting from the real empty
// board, with variety injected as a family of seat-1 opening lines. It measures
// both the live eval (candidate) and the byte-frozen incumbent against the
// strangler baselines CutSeeker and MobilityAttacker, with vs-ai2.37-style early
// stopping (decisive Wilson interval) wired in. It is a MEASUREMENT — it fails
// only on illegal/stalled/maxed games, never on strength — because its whole
// point is to expose how weak the current eval is from empty. The numbers it
// logs are the vs-ai2.38 before-picture baseline table.
//
// Reproduce:
//
//	VS_EMPTY=1 go test ./arena -run TestFromEmptyOpeningGate -v -timeout 60m
func TestFromEmptyOpeningGate(t *testing.T) {
	if os.Getenv("VS_EMPTY") != "1" {
		t.Skip("set VS_EMPTY=1 to run the from-empty opening gate")
	}
	const nodes = 1000
	minOpenings := 4
	openings := emptyOpeningLines()
	engines := []struct {
		name  string
		agent TelemetryAgent
	}{
		{"candidate", TelemetryNodeBudget(nodes, false)},
		{"incumbent", TelemetryNodeBudget(nodes, true)},
	}
	opponents := []struct {
		name  string
		agent TelemetryAgent
	}{
		{"CutSeeker", Instrument(CutSeeker)},
		{"MobilityAttacker", Instrument(MobilityAttacker)},
	}
	t.Logf("| engine | opponent | openings | wins/games | win%% | wilson95 | early-stopped |")
	t.Logf("|---|---|---|---|---|---|---|")
	for _, engine := range engines {
		for _, opponent := range opponents {
			report, played := playOpeningFamilyGate(t, engine.name+" vs "+opponent.name, openings, engine.agent, opponent.agent, minOpenings)
			interval := Wilson95(report.Wins, report.Games)
			t.Logf("| %s | %s | %d | %d/%d | %.1f%% | [%.1f%%, %.1f%%] | %v |",
				engine.name, opponent.name, played, report.Wins, report.Games, report.WinRate(),
				interval.Low, interval.High, played < len(openings))
		}
	}
}
