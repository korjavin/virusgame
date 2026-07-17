package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"os"
	"runtime"
	"time"

	"virusgame/arena"
	"virusgame/search"
)

func main() {
	seeds := flag.Int("seeds", 10, "random-baseline seeds per empty-board opening; deterministic opponents repeat the same game")
	depth := flag.Int("depth", 3, "deterministic action depth")
	production := flag.Bool("production", false, "use the deployed anytime search path and budget")
	nodeBudget := flag.Uint64("node-budget", 0, "deterministic equal-node budget without a wall deadline")
	opponent := flag.String("opponent", "all", "opponent to run: all, incumbent, random, legacy, greedy, base, or mobility")
	matrix := flag.String("matrix", "ci", "board matrix: ci or full (manual variable-size/time gate)")
	corpusPath := flag.String("corpus", "", "frozen strength corpus JSON; replaces repeated empty-board openings")
	corpusSplit := flag.String("corpus-split", "train", "frozen corpus split: train (default) or explicitly requested heldout")
	corpusTrack := flag.String("corpus-track", "", "optional corpus track")
	corpusBoard := flag.String("corpus-board", "", "optional exact board shard, e.g. 12x12")
	parallel := flag.Int("parallel", defaultParallelism(runtime.GOMAXPROCS(0)), "maximum concurrent board shards")
	jsonOutput := flag.Bool("json", false, "emit machine-readable corpus report")
	enforceGate := flag.Bool("enforce-corpus-gate", true, "hard-fail incumbent train superiority thresholds")
	flag.Parse()
	boards := []arena.Board{{Rows: 5, Cols: 5}, {Rows: 6, Cols: 6}, {Rows: 8, Cols: 8}}
	if *matrix == "full" {
		boards = []arena.Board{
			{Rows: 5, Cols: 5}, {Rows: 5, Cols: 10}, {Rows: 10, Cols: 5},
			{Rows: 8, Cols: 8}, {Rows: 10, Cols: 10}, {Rows: 15, Cols: 20},
			{Rows: 25, Cols: 25}, {Rows: 30, Cols: 30},
		}
	} else if *matrix != "ci" {
		log.Fatalf("unknown matrix %q", *matrix)
	}
	contender := arena.Tournament(*depth)
	telemetryContender := arena.TelemetryTournament(*depth)
	mode := fmt.Sprintf("fixed-depth=%d", *depth)
	if *production {
		contender = arena.Production()
		telemetryContender = arena.TelemetryProduction()
		mode = "production-budget"
	}
	if *nodeBudget > 0 {
		telemetryContender = arena.TelemetryNodeBudget(*nodeBudget, false)
		mode = fmt.Sprintf("node-budget=%d", *nodeBudget)
	}
	if *opponent != "all" && *opponent != "incumbent" && *opponent != "random" && *opponent != "legacy" && *opponent != "greedy" && *opponent != "base" && *opponent != "mobility" {
		log.Fatalf("unknown opponent %q", *opponent)
	}
	legacyPassed, greedyPassed, complete := false, false, true
	benchmarks := []struct {
		name    string
		factory arena.TelemetryOpponentFactory
	}{
		{name: "incumbent", factory: func(uint64) arena.TelemetryAgent {
			if *nodeBudget > 0 {
				return arena.TelemetryNodeBudget(*nodeBudget, true)
			}
			if *production {
				return arena.TelemetryFrozenProduction()
			}
			return arena.TelemetryFrozenTournament(*depth)
		}},
		{name: "random", factory: func(seed uint64) arena.TelemetryAgent { return arena.Instrument(arena.Random(seed)) }},
		{name: "legacy", factory: func(seed uint64) arena.TelemetryAgent { return arena.Instrument(arena.Legacy(seed)) }},
		{name: "greedy", factory: func(uint64) arena.TelemetryAgent { return arena.Instrument(arena.Greedy) }},
		{name: "base", factory: func(uint64) arena.TelemetryAgent { return arena.Instrument(arena.BaseAttacker) }},
		{name: "mobility", factory: func(uint64) arena.TelemetryAgent { return arena.Instrument(arena.MobilityAttacker) }},
	}
	if *corpusPath != "" {
		fixture, err := os.Open(*corpusPath)
		if err != nil {
			log.Fatal(err)
		}
		corpus, err := arena.DecodeCorpus(fixture)
		fixture.Close()
		if err != nil {
			log.Fatal(err)
		}
		for _, benchmark := range benchmarks {
			if *opponent != "all" && *opponent != benchmark.name {
				continue
			}
			rows, cols := 0, 0
			if *corpusBoard != "" {
				if _, err := fmt.Sscanf(*corpusBoard, "%dx%d", &rows, &cols); err != nil || rows < 2 || cols < 2 {
					log.Fatalf("invalid corpus board %q", *corpusBoard)
				}
			}
			progress := func(update arena.CorpusProgress) {
				fmt.Fprintf(os.Stderr, "progress board=%s games=%d\n", update.Board, update.Games)
			}
			var report arena.CorpusReport
			var err error
			if rows > 0 {
				report, err = arena.CompareCorpusFiltered(corpus, *corpusSplit, arena.CorpusFilter{Track: *corpusTrack, Rows: rows, Cols: cols}, progress,
					func() arena.TelemetryAgent { return telemetryContender }, func() arena.TelemetryAgent { return benchmark.factory(1) })
			} else if *parallel > 1 {
				var shardBoards []arena.Board
				seen := map[arena.Board]bool{}
				for _, testCase := range corpus.Cases {
					if testCase.Split == *corpusSplit && (*corpusTrack == "" || testCase.Track == *corpusTrack) && testCase.Track != "stress" {
						board := arena.Board{Rows: testCase.State.Rows(), Cols: testCase.State.Cols()}
						if !seen[board] {
							seen[board] = true
							shardBoards = append(shardBoards, board)
						}
					}
				}
				report, err = arena.CompareCorpusBoards(corpus, *corpusSplit, *corpusTrack, shardBoards, *parallel, progress,
					func() arena.TelemetryAgent { return telemetryContender }, func() arena.TelemetryAgent { return benchmark.factory(1) })
			} else {
				report, err = arena.CompareCorpusFiltered(corpus, *corpusSplit, arena.CorpusFilter{Track: *corpusTrack}, progress,
					func() arena.TelemetryAgent { return telemetryContender },
					func() arena.TelemetryAgent { return benchmark.factory(1) },
				)
			}
			if err != nil {
				log.Fatal(err)
			}
			if *jsonOutput {
				if err := json.NewEncoder(os.Stdout).Encode(report); err != nil {
					log.Fatal(err)
				}
			} else {
				fmt.Printf("corpus mode=%s opponent=%s %s\n", mode, benchmark.name, report)
			}
			if *enforceGate && benchmark.name == "incumbent" && *corpusSplit == "train" {
				if err := report.ValidateSuperiority(); err != nil {
					log.Fatal(err)
				}
			}
			if *jsonOutput {
				continue
			}
			for _, key := range report.SortedBuckets() {
				bucket := report.Buckets[key]
				interval := arena.Wilson95(bucket.Wins, bucket.Games)
				fmt.Printf("  bucket %s %s wilson95=[%.1f%%,%.1f%%]\n", key, bucket, interval.Low, interval.High)
			}
		}
		return
	}
	for _, benchmark := range benchmarks {
		if *opponent != "all" && *opponent != benchmark.name {
			continue
		}
		report, err := arena.CompareTelemetry(boards, *seeds, telemetryContender, benchmark.factory)
		if err != nil {
			log.Fatal(err)
		}
		fmt.Printf("tournament mode=%s matrix=%s opponent=%s boards=%d seeds=%d seats=balanced %s\n", mode, *matrix, benchmark.name, len(boards), *seeds, report)
		if report.Illegal != 0 || report.Maxed != 0 || report.Stalled != 0 {
			complete = false
		}
		// The anytime search saturates ProductionBudget by design (p50 reads
		// ~budget), so allow 50ms of stop-jitter above it instead of a hard
		// 600ms — vs-ai2.34 owner-authorized.
		latencyCap := search.ProductionBudget + 50*time.Millisecond
		switch benchmark.name {
		case "legacy":
			legacyPassed = report.WinRate() >= 85 && report.Percentile(95) <= latencyCap
		case "greedy":
			greedyPassed = report.WinRate() >= 75 && report.Percentile(95) <= latencyCap
		}
	}
	for players := 3; players <= 4; players++ {
		agents := make([]arena.Agent, players)
		agents[0] = contender
		for index := 1; index < players; index++ {
			agents[index] = arena.Greedy
		}
		result, err := arena.Play(arena.Match{Rows: 6, Cols: 6, Agents: agents})
		if err != nil {
			log.Fatal(err)
		}
		fmt.Printf("smoke players=%d winner=%d actions=%d eliminations=%d illegal=%d maxed=%v stalled=%v\n",
			players, result.Winner, result.Actions, result.Eliminations, result.Illegal, result.Maxed, result.Stalled)
		if result.Illegal != 0 || result.Maxed || result.Stalled {
			log.Fatalf("%d-player smoke failed", players)
		}
	}
	if *matrix == "full" {
		stress := []arena.Board{{Rows: 5, Cols: 50}, {Rows: 50, Cols: 5}, {Rows: 50, Cols: 50}}
		report, err := arena.Probe(stress, telemetryContender)
		if err != nil {
			log.Fatal(err)
		}
		fmt.Printf("stress mode=%s boards=5x50,50x5,50x50 %s\n", mode, report)
		if report.Illegal != 0 || report.Stalled != 0 || report.MaxLatency() > searchBudget(*production) {
			log.Fatalf("maximum-dimension stress failed: %s", report)
		}
	}
	passed := complete
	switch *opponent {
	case "all":
		passed = passed && legacyPassed && greedyPassed
	case "legacy":
		passed = passed && legacyPassed
	case "greedy":
		passed = passed && greedyPassed
	}
	if !passed {
		log.Fatalf("strength gate failed: complete=%v legacy=%v greedy=%v", complete, legacyPassed, greedyPassed)
	}
}

func defaultParallelism(cpus int) int {
	if cpus <= 1 {
		return 1
	}
	if cpus > 4 {
		return 3
	}
	return cpus - 1
}

func searchBudget(production bool) time.Duration {
	if production {
		return 850 * time.Millisecond
	}
	return 10 * time.Second
}
