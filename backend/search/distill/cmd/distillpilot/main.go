// Command distillpilot runs the offline teacher-preference distillation pilot's
// FIT phase: it labels generated train/tune states with the fixed-depth teacher,
// fits regularized integer weights toward the incumbent, and reports coverage,
// disagreement, terminal counts, train/tune ranking agreement (exact weighted
// search), per-board throughput, provenance, a measured <=2h budget derivation,
// and a predeclared seat-balanced panel of equal-depth weighted search (learned
// vs incumbent) plus greedy/base/mobility baselines.
//
// It never opens a strength corpus and never replaces production weights. The
// once-only frozen TEST evaluation is deliberately NOT run here: it is a separate
// distill.EvaluateTest call the architect invokes after freezing fresh test
// seeds. RunFit reports a NULL verdict when tune does not improve.
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"runtime"
	"time"

	"virusgame/arena"
	"virusgame/game"
	"virusgame/search"
	"virusgame/search/distill"
)

func main() {
	mode := flag.String("mode", "ci", "config preset: ci (fast smoke) or pilot (5/12/20)")
	workers := flag.Int("workers", max(1, runtime.GOMAXPROCS(0)-1), "parallel teacher-labeling workers (result is worker-count invariant)")
	teacherDepth := flag.Int("teacher-depth", 0, "override teacher fixed search depth (0 = preset)")
	panelSeeds := flag.Int("panel-seeds", 4, "seat-balanced panel seeds per board")
	panelDepth := flag.Int("panel-depth", 0, "override equal weighted-search panel depth (0 = preset)")
	maxStates := flag.Int("max-states", 0, "hard ceiling on labeled states (0 = none)")
	maxNodes := flag.Uint64("max-teacher-nodes", 0, "hard ceiling on total teacher nodes (0 = none)")
	deadline := flag.Duration("deadline", 0, "hard wall-time ceiling for labeling (0 = none)")
	budgetSeconds := flag.Float64("budget-seconds", 7200, "offline budget ceiling for full-run derivation")
	panelDeadline := flag.Duration("panel-deadline", 10*time.Minute, "hard wall-time bound on the whole panel")
	emitJSON := flag.Bool("json", false, "emit machine-readable JSON")
	flag.Parse()

	if *mode != "ci" && *mode != "pilot" {
		fmt.Fprintf(os.Stderr, "invalid -mode %q: want ci or pilot\n", *mode)
		os.Exit(2)
	}
	if *workers < 1 || *panelSeeds < 1 || *panelDepth < 0 || *teacherDepth < 0 || *maxStates < 0 || *budgetSeconds <= 0 || *deadline < 0 || *panelDeadline <= 0 {
		fmt.Fprintln(os.Stderr, "invalid flags: workers/panel-seeds>=1, teacher-depth/panel-depth/max-states>=0, budget-seconds/panel-deadline>0")
		os.Exit(2)
	}

	cfg := distill.CIConfig()
	if *mode == "pilot" {
		cfg = distill.PilotConfig()
	}
	if *teacherDepth > 0 {
		cfg.TeacherDepth = *teacherDepth
	}
	if *panelDepth > 0 {
		cfg.PanelDepth = *panelDepth
	}

	limits := distill.Limits{MaxStates: *maxStates, MaxTeacherNodes: *maxNodes}
	if *deadline > 0 {
		limits.Deadline = time.Now().Add(*deadline)
	}

	res, err := distill.RunFit(context.Background(), cfg, limits, *workers)
	if err != nil {
		fmt.Fprintf(os.Stderr, "distill fit failed: %v\n", err)
		os.Exit(1)
	}

	panelDepthUsed := cfg.PanelDepth
	if panelDepthUsed < 1 {
		panelDepthUsed = 1
	}
	// The panel is a separate approved-candidate phase: it runs only when the fit
	// produced an approved, non-truncated candidate. A null or measurement-only
	// run publishes no learned weights and no panel.
	runnable := res.Approved && !res.Truncated
	var panel []panelRow
	if runnable {
		pctx, cancel := context.WithTimeout(context.Background(), *panelDeadline)
		panel, err = runPanel(pctx, cfg, res.Weights, panelDepthUsed, *panelSeeds)
		cancel()
		if err != nil {
			fmt.Fprintf(os.Stderr, "panel failed: %v\n", err)
			os.Exit(1)
		}
	}

	if *emitJSON {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		_ = enc.Encode(map[string]any{"config": cfg, "fit": res, "panel": panel,
			"budget": deriveBudget(res, *budgetSeconds), "panel_depth": panelDepthUsed, "panel_ran": runnable})
		return
	}

	fmt.Printf("distill FIT mode=%s workers=%d teacher_depth=%d shallow_depth=%d\n", *mode, *workers, cfg.TeacherDepth, cfg.ShallowDepth)
	fmt.Printf("states: train=%d tune=%d dropped_duplicates=%d incomplete_teacher=%d truncated=%v labeled_nodes=%d\n",
		res.Train.States, res.Tune.States, res.Duplicates, res.Incomplete, res.Truncated, res.LabeledNodes)
	printSplit("train", res.Train)
	printSplit("tune", res.Tune)
	fmt.Printf("throughput: wall=%s states/s=%.1f\n", res.WallElapsed.Truncate(time.Millisecond), res.StatesPerSec)
	for _, b := range res.PerBoard {
		fmt.Printf("  board=%dx%d states=%d teacher_nodes=%d nodes/s=%.0f\n", b.Board.Rows, b.Board.Cols, b.States, b.TeacherNodes, b.NodesPerSec)
	}
	bd := deriveBudget(res, *budgetSeconds)
	fmt.Printf("budget: at %.1f states/s a %.0fs (<=2h) run fits ~%d labeled states (%.1fx this fit)\n",
		res.StatesPerSec, *budgetSeconds, bd.MaxStates, bd.ScaleVsFit)
	fmt.Printf("VERDICT: %s\n", res.Verdict)
	fmt.Printf("provenance: version=%s build=%q checksum=%s\n  config=%s dataset=%s labels=%s weights=%s\n",
		res.Provenance.Version, res.Provenance.Build, res.Provenance.Checksum,
		res.Provenance.Config, res.Provenance.Dataset, res.Provenance.Labels, res.Provenance.Weights)
	if res.Approved {
		fmt.Printf("candidate weights (scale=%d): %v\n", search.WeightScale, res.Weights)
	} else {
		fmt.Printf("no approved weights emitted (null tune); incumbent unchanged: %v\n", search.IncumbentWeights())
	}
	if runnable {
		fmt.Printf("panel (equal weighted search depth=%d, seat-balanced, seeds=%d):\n", panelDepthUsed, *panelSeeds)
		for _, p := range panel {
			fmt.Printf("  vs %-9s win=%.1f%% illegal=%d stalled=%d maxed=%d games=%d wilson95=[%.1f,%.1f]\n",
				p.Opponent, p.WinRate, p.Illegal, p.Stalled, p.Maxed, p.Games, p.Low, p.High)
		}
	} else {
		fmt.Println("panel: SKIPPED (not an approved, non-truncated candidate)")
	}
	fmt.Println("note: FIT/tune only; the frozen TEST verdict is a separate EvaluateTest call. Production weights are unchanged.")
}

func printSplit(name string, m distill.SplitMetrics) {
	fmt.Printf("%s: states=%d pairs=%d mean_legal=%.1f mean_candidates=%.1f candidate/legal=%.3f\n",
		name, m.States, m.Pairs, m.MeanLegal, m.MeanCandidates, m.CandidateCoverage)
	fmt.Printf("  terminal: forced_win=%d forced_loss=%d terminal_candidates=%d  teacher_vs_shallow=%.3f\n",
		m.ForcedWinStates, m.ForcedLossStates, m.TerminalCandidates, m.TeacherDisagreement)
	fmt.Printf("  agreement: learned=%.3f incumbent=%.3f  positional(learned=%.3f incumbent=%.3f) tie_fraction=%.3f\n",
		m.AgreementLearned, m.AgreementIncumbent, m.PositionalLearned, m.PositionalIncumbent, m.TieFractionLearned)
}

type budget struct {
	MaxStates  int
	ScaleVsFit float64
}

func deriveBudget(res distill.FitResult, seconds float64) budget {
	fitStates := res.Train.States + res.Tune.States
	maxStates := int(res.StatesPerSec * seconds)
	scale := 0.0
	if fitStates > 0 {
		scale = float64(maxStates) / float64(fitStates)
	}
	return budget{MaxStates: maxStates, ScaleVsFit: scale}
}

type panelRow struct {
	Opponent  string
	WinRate   float64
	Illegal   int
	Stalled   int
	Maxed     int
	Games     int
	Low, High float64
}

// runPanel plays the equal-depth weighted-search contender against the incumbent
// search and the baselines over an explicit, bounded board/seed grid. Each single
// game is the bounded unit: ctx is checked before every game AND the weighted
// agents are context-aware, so a cancelled deadline aborts the in-progress search
// (fallback moves) and stops the panel promptly, not merely between opponents.
// Any illegal or stalled decision is a hard failure, never a report-only footnote.
func runPanel(ctx context.Context, cfg distill.Config, learned search.WeightVector, depth, seeds int) ([]panelRow, error) {
	opponents := []struct {
		name    string
		factory arena.OpponentFactory
	}{
		{"incumbent", func(uint64) arena.Agent {
			return arena.Agent(distill.WeightedAgent(ctx, search.IncumbentWeights(), depth))
		}},
		{"greedy", func(uint64) arena.Agent { return arena.Greedy }},
		{"base", func(uint64) arena.Agent { return arena.BaseAttacker }},
		{"mobility", func(uint64) arena.Agent { return arena.MobilityAttacker }},
	}
	var rows []panelRow
	for _, opp := range opponents {
		var report arena.Report
		for bi, b := range cfg.Boards {
			for seed := 1; seed <= seeds; seed++ {
				for seat := 0; seat < 2; seat++ {
					if err := ctx.Err(); err != nil {
						return nil, fmt.Errorf("panel bound reached during %s: %w", opp.name, err)
					}
					agents := []arena.Agent{arena.Agent(distill.WeightedAgent(ctx, learned, depth)), opp.factory(uint64(bi*10_000 + seed))}
					if seat == 1 {
						agents[0], agents[1] = agents[1], agents[0]
					}
					result, err := arena.PlayContext(ctx, arena.Match{Rows: b.Rows, Cols: b.Cols, Agents: agents})
					if err != nil {
						return nil, fmt.Errorf("panel %s: %w", opp.name, err)
					}
					if result.Aborted {
						return nil, fmt.Errorf("panel bound reached mid-game during %s: %w", opp.name, ctx.Err())
					}
					report.Add(result, game.Player(seat+1))
				}
			}
		}
		if report.Illegal != 0 || report.Stalled != 0 {
			return nil, fmt.Errorf("panel %s produced illegal=%d stalled=%d", opp.name, report.Illegal, report.Stalled)
		}
		interval := arena.Wilson95(report.Wins, report.Games)
		rows = append(rows, panelRow{
			Opponent: opp.name, WinRate: report.WinRate(), Illegal: report.Illegal,
			Stalled: report.Stalled, Maxed: report.Maxed, Games: report.Games,
			Low: interval.Low, High: interval.High,
		})
	}
	return rows, nil
}
