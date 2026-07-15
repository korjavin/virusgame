package main

import (
	"flag"
	"fmt"
	"log"
	"time"

	"virusgame/arena"
)

func main() {
	seeds := flag.Int("seeds", 10, "fixed seeds per board and seat")
	depth := flag.Int("depth", 3, "deterministic action depth")
	production := flag.Bool("production", false, "use the deployed anytime search path and budget")
	opponent := flag.String("opponent", "all", "opponent to run: all, random, legacy, greedy, base, or mobility")
	matrix := flag.String("matrix", "ci", "board matrix: ci or full (manual variable-size/time gate)")
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
	if *opponent != "all" && *opponent != "random" && *opponent != "legacy" && *opponent != "greedy" && *opponent != "base" && *opponent != "mobility" {
		log.Fatalf("unknown opponent %q", *opponent)
	}
	legacyPassed, greedyPassed, complete := false, false, true
	for _, benchmark := range []struct {
		name    string
		factory arena.TelemetryOpponentFactory
	}{
		{name: "random", factory: func(seed uint64) arena.TelemetryAgent { return arena.Instrument(arena.Random(seed)) }},
		{name: "legacy", factory: func(seed uint64) arena.TelemetryAgent { return arena.Instrument(arena.Legacy(seed)) }},
		{name: "greedy", factory: func(uint64) arena.TelemetryAgent { return arena.Instrument(arena.Greedy) }},
		{name: "base", factory: func(uint64) arena.TelemetryAgent { return arena.Instrument(arena.BaseAttacker) }},
		{name: "mobility", factory: func(uint64) arena.TelemetryAgent { return arena.Instrument(arena.MobilityAttacker) }},
	} {
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
		switch benchmark.name {
		case "legacy":
			legacyPassed = report.WinRate() >= 85 && report.Percentile(95) <= 600*time.Millisecond
		case "greedy":
			greedyPassed = report.WinRate() >= 75 && report.Percentile(95) <= 600*time.Millisecond
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

func searchBudget(production bool) time.Duration {
	if production {
		return 850 * time.Millisecond
	}
	return 10 * time.Second
}
