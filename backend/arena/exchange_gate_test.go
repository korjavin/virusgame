package arena

import (
	"os"
	"testing"

	"virusgame/game"
	"virusgame/search"
)

// vs-ai2.47 constructed exchange gate. Two 12x12 positions with the bot (P2) to
// move at a deterministic node budget. It documents the STANDING exchange-ratio
// blindness: (a) the bot takes a negative capture that leaves >=2 of its own
// normals capturable next turn (the 1-for-N), while (b) it correctly takes a
// favorable capture that severs >=2 enemy cells for at most one exposed cell.
// The vs-ai2.47 static-eval sweep could NOT flip (a) without breaking (b) — see
// the negative-result comment in search/evaluate.go and the Task-4 sweep data in
// docs/plans/20260717-vs-ai2.47-exchange-ratio.md. A future fix (likely
// quiescence in search) flips the (a) assertion to "declined".
//
// The negative scenario uses the anchor's real mechanism (see
// exchange_evidence_test.go): the bot advances INTO contact — here by capturing a
// bridge cell that reconnects a forward group — and the eval prices that
// reconnect at full material while pricing the enabled retaliation at a small
// proxy. A quiet placement leaves the forward group disconnected and safe.

// exchangeNodeBudget is the deterministic ceiling for the gate. Both scenarios
// are stable across the 20k-100k band; 30k reaches depth ~4-6 without a wall clock.
const exchangeNodeBudget = 30_000

// connectedMask returns which cells belong to a player's connected component
// (base + 8-connected own cells). connectedComponent already returns counts;
// the classifier needs the mask itself to test capturability.
func connectedMask(state game.State, player game.Player) []bool {
	rows, cols := state.Rows(), state.Cols()
	seen := make([]bool, rows*cols)
	base := basePosition(state, player)
	if cell, _ := state.At(base); cell.Owner != player {
		return seen
	}
	queue := []game.Pos{base}
	seen[base.Row*cols+base.Col] = true
	for len(queue) > 0 {
		cur := queue[0]
		queue = queue[1:]
		for dr := -1; dr <= 1; dr++ {
			for dc := -1; dc <= 1; dc++ {
				n := game.Pos{Row: cur.Row + dr, Col: cur.Col + dc}
				if n.Row < 0 || n.Row >= rows || n.Col < 0 || n.Col >= cols || seen[n.Row*cols+n.Col] {
					continue
				}
				if cell, _ := state.At(n); cell.Owner == player {
					seen[n.Row*cols+n.Col] = true
					queue = append(queue, n)
				}
			}
		}
	}
	return seen
}

// capturableBy counts victim's CONNECTED normal cells adjacent to attacker's
// connected territory — the cells the attacker can convert next turn, and the
// same signal the eval prices via m.threatened (which also requires connectivity).
func capturableBy(state game.State, victim, attacker game.Player) int {
	amask := connectedMask(state, attacker)
	vmask := connectedMask(state, victim)
	rows, cols := state.Rows(), state.Cols()
	count := 0
	for _, pos := range ownedCells(state, victim) {
		cell, _ := state.At(pos)
		if cell.Kind != game.Normal || !vmask[pos.Row*cols+pos.Col] {
			continue
		}
	adjacent:
		for dr := -1; dr <= 1; dr++ {
			for dc := -1; dc <= 1; dc++ {
				n := game.Pos{Row: pos.Row + dr, Col: pos.Col + dc}
				if n.Row < 0 || n.Row >= rows || n.Col < 0 || n.Col >= cols {
					continue
				}
				if amask[n.Row*cols+n.Col] {
					count++
					break adjacent
				}
			}
		}
	}
	return count
}

// exchangeMetrics classifies a chosen action by applying it and counting deltas,
// so the same builders/classifier serve the before and after assertions.
type exchangeMetrics struct {
	capturedEnemy int // enemy owned cells removed by the action (a capture => 1)
	exposedAfter  int // own connected normals capturable by the enemy after the action
	enemySevered  int // enemy owned cells disconnected from their base after the action
}

func classifyExchange(state game.State, action game.Action, bot game.Player) (exchangeMetrics, error) {
	enemy := game.Player(3 - bot)
	after, err := state.Apply(action)
	if err != nil {
		return exchangeMetrics{}, err
	}
	connected, owned := connectedComponent(after, enemy)
	return exchangeMetrics{
		capturedEnemy: len(ownedCells(state, enemy)) - len(ownedCells(after, enemy)),
		exposedAfter:  capturableBy(after, bot, enemy),
		enemySevered:  owned - connected,
	}, nil
}

func exchangeBoard() [][]game.Cell {
	b := make([][]game.Cell, 12)
	for r := range b {
		b[r] = make([]game.Cell, 12)
	}
	return b
}

func putCells(b [][]game.Cell, owner game.Player, cells ...[2]int) {
	for _, c := range cells {
		b[c[0]][c[1]] = game.Cell{Owner: owner, Kind: game.Normal}
	}
}

func exchangeSnapshot(b [][]game.Cell) game.Snapshot {
	b[0][0] = game.Cell{Owner: 1, Kind: game.Base}
	b[11][11] = game.Cell{Owner: 2, Kind: game.Base}
	return game.Snapshot{
		Rows: 12, Cols: 12, Board: b,
		Bases:       []game.Pos{{Row: 0, Col: 0}, {Row: 11, Col: 11}},
		Active:      []bool{true, true},
		NeutralUsed: []bool{true, true},
		Current:     2, MovesLeft: 3,
	}
}

// scenarioNegativeExchange is the declinable 1-for-N (anchor mechanism). The bot
// (P2) has a DISCONNECTED forward group (5,8),(5,9),(5,10) pinned under a P1 wall
// (4,8),(4,9),(4,10). The sole bridge to the bot body is the P1 cell La=(6,8).
// Capturing La reconnects the forward group — a big eval lure (+connected,
// -disconnected, +captured normal) — but makes all three cells connected and
// capturable by P1 next turn. A quiet placement leaves them disconnected and safe.
func scenarioNegativeExchange() game.Snapshot {
	b := exchangeBoard()
	putCells(b, 1, [2]int{1, 1}, [2]int{2, 2}, [2]int{3, 3}, [2]int{3, 4}, [2]int{3, 5}, [2]int{3, 6}, [2]int{3, 7})
	putCells(b, 1, [2]int{4, 8}, [2]int{4, 9}, [2]int{4, 10}, [2]int{5, 7}, [2]int{6, 8}) // wall + bridge La=(6,8)
	putCells(b, 2, [2]int{10, 10}, [2]int{9, 9}, [2]int{8, 8}, [2]int{7, 8})              // spine touching La
	putCells(b, 2, [2]int{5, 8}, [2]int{5, 9}, [2]int{5, 10})                             // disconnected forward group
	return exchangeSnapshot(b)
}

// scenarioFavorableExchange is the 2-for-1 the fix must preserve. The P1
// articulation cell C=(5,5) is the sole link to a 2-cell pocket (4,6),(5,7).
// The bot reaches C from (6,6): capturing it severs both pocket cells while
// exposing no bot normal — a clear gain that must stay taken before and after.
func scenarioFavorableExchange() game.Snapshot {
	b := exchangeBoard()
	putCells(b, 1, [2]int{1, 1}, [2]int{2, 2}, [2]int{3, 3}, [2]int{4, 4}, [2]int{5, 5}, [2]int{4, 6}, [2]int{5, 7})
	putCells(b, 2, [2]int{10, 10}, [2]int{9, 9}, [2]int{8, 8}, [2]int{7, 7}, [2]int{6, 6})
	return exchangeSnapshot(b)
}

// TestExchangeGate asserts the CURRENT eval's behavior: (a) the bot TAKES the
// negative capture (>=2 own cells left exposed), (b) the bot TAKES the favorable
// 2-for-1. A future exchange-aware fix flips (a) to the declined behavior
// (exposedAfter <= 1) and must keep (b) taken.
//
//	VS_EXCHANGE=1 go test ./arena -run TestExchangeGate -v
func TestExchangeGate(t *testing.T) {
	if os.Getenv("VS_EXCHANGE") != "1" {
		t.Skip("set VS_EXCHANGE=1 to run the constructed exchange gate")
	}
	const bot = game.Player(2)

	t.Run("negative_1_for_n_taken_before_fix", func(t *testing.T) {
		state, err := game.FromSnapshot(scenarioNegativeExchange())
		if err != nil {
			t.Fatalf("snapshot invalid: %v", err)
		}
		res, ok := search.ChooseNodeBudget(state, exchangeNodeBudget)
		if !ok {
			t.Fatal("bot produced no move")
		}
		m, err := classifyExchange(state, res.Action, bot)
		if err != nil {
			t.Fatalf("apply chosen action: %v", err)
		}
		t.Logf("negative: target=%v captured=%d exposedAfter=%d (want capture leaving >=2 exposed)",
			res.Action.Target, m.capturedEnemy, m.exposedAfter)
		if m.capturedEnemy < 1 || m.exposedAfter < 2 {
			t.Errorf("before-fix: got captured=%d exposedAfter=%d, want a capture leaving >=2 exposed (the negative 1-for-N)",
				m.capturedEnemy, m.exposedAfter)
		}
	})

	t.Run("favorable_2_for_1_taken", func(t *testing.T) {
		state, err := game.FromSnapshot(scenarioFavorableExchange())
		if err != nil {
			t.Fatalf("snapshot invalid: %v", err)
		}
		res, ok := search.ChooseNodeBudget(state, exchangeNodeBudget)
		if !ok {
			t.Fatal("bot produced no move")
		}
		m, err := classifyExchange(state, res.Action, bot)
		if err != nil {
			t.Fatalf("apply chosen action: %v", err)
		}
		t.Logf("favorable: target=%v captured=%d severed=%d exposedAfter=%d (want capture severing >=2 for <=1 exposed)",
			res.Action.Target, m.capturedEnemy, m.enemySevered, m.exposedAfter)
		if m.capturedEnemy < 1 || m.enemySevered < 2 || m.exposedAfter > 1 {
			t.Errorf("got captured=%d severed=%d exposedAfter=%d, want the favorable 2-for-1 (sever >=2, expose <=1)",
				m.capturedEnemy, m.enemySevered, m.exposedAfter)
		}
	})
}
