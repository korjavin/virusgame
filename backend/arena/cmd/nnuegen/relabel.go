package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"sync"

	"virusgame/game"
	"virusgame/search"
)

// relabel rewrites shards with deepScore recomputed at a (deeper) node budget
// from the stored raw position. Positions and outcomes are untouched.
func relabel(inDir, outDir string, budget uint64, workers int) {
	if err := os.MkdirAll(outDir, 0o755); err != nil {
		log.Fatal(err)
	}
	shards, _ := filepath.Glob(filepath.Join(inDir, "shard-*.jsonl"))
	var wg sync.WaitGroup
	sem := make(chan struct{}, workers)
	for _, sh := range shards {
		wg.Add(1)
		go func(sh string) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()
			f, err := os.Open(sh)
			if err != nil {
				log.Fatal(err)
			}
			defer f.Close()
			of, err := os.Create(filepath.Join(outDir, filepath.Base(sh)))
			if err != nil {
				log.Fatal(err)
			}
			defer of.Close()
			w := bufio.NewWriter(of)
			defer w.Flush()
			sc := bufio.NewScanner(f)
			sc.Buffer(make([]byte, 1<<20), 1<<20)
			n := 0
			for sc.Scan() {
				var rec Record
				if err := json.Unmarshal(sc.Bytes(), &rec); err != nil {
					continue
				}
				state, err := game.FromSnapshot(rec.toSnapshot())
				if err != nil || state.GameOver() {
					continue
				}
				if result, ok := search.ChooseNodeBudget(state, budget); ok {
					rec.DeepScore = result.Score
					rec.Budget = budget
					nb, _ := json.Marshal(rec)
					w.Write(nb)
					w.WriteByte('\n')
					n++
					if n%1000 == 0 {
						fmt.Printf("%s: %d\n", filepath.Base(sh), n)
					}
				}
			}
			fmt.Printf("%s done: %d records\n", filepath.Base(sh), n)
		}(sh)
	}
	wg.Wait()
}
