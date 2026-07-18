package main

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"virusgame/search"
)

// TestSPSAReproducible asserts the whole SPSA loop is a pure function of its
// config: two runs with the same seed produce byte-identical trace + summary.
// Everything downstream (node-budget agents incl. the OwnerBot rung, seeded
// openings, seeded perturbations) is deterministic and timing-free, and the
// parallel game workers fold + early-stop strictly in permutation order, so the
// JSON must match regardless of worker scheduling. Workers=4 exercises that
// parallel fold. A full cross-process proof at the overnight regime lives in
// results/README.md (two separate invocations, byte-identical output).
func TestSPSAReproducible(t *testing.T) {
	cfg := configRecord{Iters: 3, Openings: 2, FloorOpenings: 2, Nodes: 200, Seed: 1, Workers: 4}

	marshal := func() []byte {
		trace, summary, err := newOptimizer(cfg, false).run()
		if err != nil {
			t.Fatalf("run: %v", err)
		}
		b, err := json.Marshal(output{Config: cfg, Iterations: trace, Summary: summary})
		if err != nil {
			t.Fatalf("marshal: %v", err)
		}
		return b
	}

	first, second := marshal(), marshal()
	if !bytes.Equal(first, second) {
		t.Fatalf("SPSA trace not reproducible:\nfirst=%s\nsecond=%s", first, second)
	}
}

// TestLoadInitTheta covers both accepted warm-start forms (a full results JSON
// and a bare EvalParams map) and the degenerate-file rejection.
func TestLoadInitTheta(t *testing.T) {
	dir := t.TempDir()
	want := search.DefaultEvalParams()
	want.Connected = 42 // make it distinct from the default

	// Bare EvalParams map.
	bareMap := filepath.Join(dir, "bare.json")
	writeFile(t, bareMap, mustJSON(t, want))
	if got, err := loadInitTheta(bareMap); err != nil || got != want {
		t.Fatalf("bare map: got %+v err %v, want %+v", got, err, want)
	}

	// Full results JSON — summary.bestTheta must win.
	resultsFile := filepath.Join(dir, "results.json")
	writeFile(t, resultsFile, mustJSON(t, output{Summary: summaryRecord{BestTheta: want}}))
	if got, err := loadInitTheta(resultsFile); err != nil || got != want {
		t.Fatalf("results JSON: got %+v err %v, want %+v", got, err, want)
	}

	// Degenerate all-zero file is rejected.
	zeroFile := filepath.Join(dir, "zero.json")
	writeFile(t, zeroFile, mustJSON(t, search.EvalParams{}))
	if _, err := loadInitTheta(zeroFile); err == nil {
		t.Fatal("all-zero file: expected error, got nil")
	}
}

// TestWarmStartDefaultIsColdStart asserts warm-starting from the default vector
// reproduces a cold start exactly: the default maps to all-1.0 in scaled space.
func TestWarmStartDefaultIsColdStart(t *testing.T) {
	cfg := configRecord{Iters: 2, Openings: 2, FloorOpenings: 2, Nodes: 200, Seed: 1, Workers: 4}

	cold, _, err := newOptimizer(cfg, false).run()
	if err != nil {
		t.Fatalf("cold: %v", err)
	}
	o := newOptimizer(cfg, false)
	def := search.DefaultEvalParams()
	o.initTheta = &def
	warm, _, err := o.run()
	if err != nil {
		t.Fatalf("warm: %v", err)
	}
	if !bytes.Equal(mustJSON(t, cold), mustJSON(t, warm)) {
		t.Fatal("warm start from default diverged from cold start")
	}
}

func mustJSON(t *testing.T, v any) []byte {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	return b
}

func writeFile(t *testing.T, path string, b []byte) {
	t.Helper()
	if err := os.WriteFile(path, b, 0o644); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}
