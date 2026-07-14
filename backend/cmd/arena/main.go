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
	opponent := flag.String("opponent", "all", "opponent to run: all, random, legacy, or greedy")
	flag.Parse()
	boards := []arena.Board{{Rows: 5, Cols: 5}, {Rows: 6, Cols: 6}, {Rows: 8, Cols: 8}}
	contender := arena.Tournament(*depth)
	mode := fmt.Sprintf("fixed-depth=%d", *depth)
	if *production {
		contender = arena.Production()
		mode = "production-budget"
	}
	if *opponent != "all" && *opponent != "random" && *opponent != "legacy" && *opponent != "greedy" {
		log.Fatalf("unknown opponent %q", *opponent)
	}
	legacyPassed, greedyPassed, complete := false, false, true
	for _, benchmark := range []struct {
		name    string
		factory arena.OpponentFactory
	}{
		{name: "random", factory: arena.Random},
		{name: "legacy", factory: arena.Legacy},
		{name: "greedy", factory: func(uint64) arena.Agent { return arena.Greedy }},
	} {
		if *opponent != "all" && *opponent != benchmark.name {
			continue
		}
		report, err := arena.Balanced(boards, *seeds, contender, benchmark.factory)
		if err != nil {
			log.Fatal(err)
		}
		fmt.Printf("tournament mode=%s opponent=%s boards=5x5,6x6,8x8 seeds=%d seats=balanced %s\n", mode, benchmark.name, *seeds, report)
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
