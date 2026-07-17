package main

import (
	"bytes"
	"encoding/json"
	"testing"
)

// TestSPSAReproducible asserts the whole SPSA loop is a pure function of its
// config: two runs with the same seed produce byte-identical trace + summary.
// Everything downstream (node-budget agents, seeded openings, seeded
// perturbations) is deterministic and timing-free, so the JSON must match.
func TestSPSAReproducible(t *testing.T) {
	cfg := configRecord{Iters: 2, Openings: 2, FloorOpenings: 2, Nodes: 200, Seed: 1, Workers: 4}

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
