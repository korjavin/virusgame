package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"

	"virusgame/game"
	"virusgame/search"
)

// residualize rewrites shards so deepScore holds the RESIDUAL vs the frozen
// hand-tuned static eval: the net then only learns what the formula misses,
// and play-time eval = static + net (see search.nnueEvaluate residual mode).
func residualize(inDir, outDir string) {
	if err := os.MkdirAll(outDir, 0o755); err != nil {
		log.Fatal(err)
	}
	shards, _ := filepath.Glob(filepath.Join(inDir, "shard-*.jsonl"))
	total, skipped := 0, 0
	for _, sh := range shards {
		f, err := os.Open(sh)
		if err != nil {
			log.Fatal(err)
		}
		of, err := os.Create(filepath.Join(outDir, filepath.Base(sh)))
		if err != nil {
			log.Fatal(err)
		}
		w := bufio.NewWriter(of)
		sc := bufio.NewScanner(f)
		sc.Buffer(make([]byte, 1<<20), 1<<20)
		for sc.Scan() {
			var rec Record
			if err := json.Unmarshal(sc.Bytes(), &rec); err != nil {
				skipped++
				continue
			}
			state, err := game.FromSnapshot(rec.toSnapshot())
			if err != nil {
				skipped++
				continue
			}
			rec.DeepScore -= search.StaticEval(state, state.CurrentPlayer())
			nb, _ := json.Marshal(rec)
			w.Write(nb)
			w.WriteByte('\n')
			total++
		}
		w.Flush()
		of.Close()
		f.Close()
	}
	fmt.Printf("residualized %d records (%d skipped)\n", total, skipped)
}
