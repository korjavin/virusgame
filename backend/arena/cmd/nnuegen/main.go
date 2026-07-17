// Command nnuegen generates the offline NNUE-lite training corpus: it samples
// positions, labels each with a deep search score + eventual game outcome, and
// writes deterministic JSONL shards. Output feeds tools/nnue-train (Stage 2);
// Stage 3 (int8 inference inside search) is a separate later bead.
//
// This file (Task 2) defines the on-disk record, the labeling pipeline, and the
// per-worker shard writer. The multi-source sampling (self-play / owner-corpus /
// ladder openings), workers, resume, and determinism harness land in Task 3.
//
// JSONL schema — one Record per line:
//
//	fingerprint    string        arena.StateFingerprint(state) — stable dedupe key
//	rows, cols     int           board dimensions
//	currentPlayer  int           seat to move (1-based)
//	features       [4][]float64  per-seat feature vectors (seat-1 indexed), each
//	                             the fixed-order arena.PlayerFeatures.Features()
//	                             slice; inactive seats are the zero-length nil
//	                             (JSON null). The trainer picks its own
//	                             perspective (mover vs opponents) from the full
//	                             4×K matrix — kept whole here so no perspective
//	                             decision is baked into the data.
//	deepScore      int           search.ChooseNodeBudget(state, budget).Score
//	budget         uint64        node budget used to produce deepScore
//	outcome        {winner int, placement int}  eventual game result for the
//	                             source game; winner 0 / placement 0 = unknown
//	                             (corpus/ladder positions with no completed game).
//	                             Placement is the mover's finishing rank (1=won).
//	source         string        "selfplay" | "corpus" | "ladder"
package main

import (
	"bufio"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"

	"virusgame/arena"
	"virusgame/game"
	"virusgame/search"
)

// Outcome is the eventual game result attached to a sampled position. Zero
// values (Winner 0, Placement 0) are the sentinel for "no completed game".
type Outcome struct {
	Winner    int `json:"winner"`
	Placement int `json:"placement"`
}

// Record is one JSONL line. See the package doc for field semantics.
type Record struct {
	Fingerprint   string       `json:"fingerprint"`
	Rows          int          `json:"rows"`
	Cols          int          `json:"cols"`
	CurrentPlayer int          `json:"currentPlayer"`
	Features      [4][]float64 `json:"features"`
	DeepScore     int          `json:"deepScore"`
	Budget        uint64       `json:"budget"`
	Outcome       Outcome      `json:"outcome"`
	Source        string       `json:"source"`
}

// Label builds a Record for a position: deep score from ChooseNodeBudget,
// features from arena.NNUEFeatures, dedupe key from arena.StateFingerprint. The
// outcome is filled by the caller (self-play backfills winner/placement; corpus
// and ladder positions keep the zero sentinel).
func Label(state game.State, budget uint64, source string) (Record, error) {
	fingerprint, err := arena.StateFingerprint(state)
	if err != nil {
		return Record{}, err
	}
	result, _ := search.ChooseNodeBudget(state, budget)
	feats := arena.NNUEFeatures(state)
	var features [4][]float64
	for seat := 0; seat < 4; seat++ {
		if state.Active(game.Player(seat + 1)) {
			features[seat] = feats[seat].Features()
		}
	}
	return Record{
		Fingerprint:   fingerprint,
		Rows:          state.Rows(),
		Cols:          state.Cols(),
		CurrentPlayer: int(state.CurrentPlayer()),
		Features:      features,
		DeepScore:     result.Score,
		Budget:        budget,
		Source:        source,
	}, nil
}

// ShardWriter appends JSONL records to a per-worker shard file. Append-safe
// (O_APPEND) so -resume in Task 3 can continue an existing shard.
type ShardWriter struct {
	file *os.File
	buf  *bufio.Writer
	seen map[string]bool // dedupe by fingerprint within this writer
}

// NewShardWriter opens (creating/appending) shard-NNN.jsonl in dir for worker.
func NewShardWriter(dir string, worker int) (*ShardWriter, error) {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, err
	}
	path := filepath.Join(dir, fmt.Sprintf("shard-%03d.jsonl", worker))
	file, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return nil, err
	}
	return &ShardWriter{file: file, buf: bufio.NewWriter(file), seen: map[string]bool{}}, nil
}

// Write emits one record as a JSON line, skipping fingerprints already written
// by this writer. Returns true if the record was written.
func (w *ShardWriter) Write(record Record) (bool, error) {
	if w.seen[record.Fingerprint] {
		return false, nil
	}
	encoded, err := json.Marshal(record)
	if err != nil {
		return false, err
	}
	if _, err := w.buf.Write(append(encoded, '\n')); err != nil {
		return false, err
	}
	w.seen[record.Fingerprint] = true
	return true, nil
}

// Close flushes and closes the shard file.
func (w *ShardWriter) Close() error {
	if err := w.buf.Flush(); err != nil {
		w.file.Close()
		return err
	}
	return w.file.Close()
}

func main() {
	out := flag.String("out", "", "output shard directory (required)")
	worker := flag.Int("worker", 0, "worker id (shard file suffix)")
	budget := flag.Uint64("budget", 20000, "node budget for deep-score labels")
	positions := flag.Int("positions", 300, "target positions to sample")
	seed := flag.Uint64("seed", 1, "base seed for deterministic sampling")
	rows := flag.Int("rows", 8, "board rows")
	cols := flag.Int("cols", 8, "board cols")
	flag.Parse()
	if *out == "" {
		fmt.Fprintln(os.Stderr, "-out is required")
		os.Exit(2)
	}

	writer, err := NewShardWriter(*out, *worker)
	if err != nil {
		panic(err)
	}
	defer writer.Close()

	// ponytail: Task 2 ships a single ladder-opening source so the core pipeline
	// is exercised end to end; self-play + owner-corpus sources, workers, and
	// -resume land in Task 3. Deterministic seed = base seed + worker index.
	rng := *seed + uint64(*worker)
	written := 0
	for attempt := 0; written < *positions && attempt < *positions*4; attempt++ {
		rng ^= rng << 13
		rng ^= rng >> 7
		rng ^= rng << 17
		snapshot, err := arena.RandomLegalOpening(*rows, *cols, rng)
		if err != nil {
			continue
		}
		state, err := game.FromSnapshot(snapshot)
		if err != nil {
			continue
		}
		record, err := Label(state, *budget, "ladder")
		if err != nil {
			panic(err)
		}
		ok, err := writer.Write(record)
		if err != nil {
			panic(err)
		}
		if ok {
			written++
		}
	}
	fmt.Printf("wrote %d positions to shard-%03d.jsonl\n", written, *worker)
}
