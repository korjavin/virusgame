package main

import (
	"go/ast"
	"go/importer"
	"go/parser"
	"go/token"
	"go/types"
	"math"
	"strings"
	"testing"
)

// synth builds a learnable dataset: score is a fixed linear function of a few
// input features, so a couple epochs must lower the loss.
func synth(n int) []Sample {
	var rng uint64 = 12345
	samples := make([]Sample, n)
	for i := 0; i < n; i++ {
		x := make([]float64, inputDim)
		// vary a handful of dims deterministically
		for _, d := range []int{0, 5, 19, 24, 38} {
			x[d] = float64(next(&rng)%10) - 5
		}
		score := 3*x[0] - 2*x[5] + 1.5*x[19] + 0.5*x[24]
		outcome := 1.0
		if score < 0 {
			outcome = -1
		}
		samples[i] = Sample{Input: x, Score: score, Outcome: outcome, Hash: uint32(i)}
	}
	return samples
}

func TestTrainLossDecreases(t *testing.T) {
	samples := synth(200)
	train, _ := split(samples)
	st := normStats(train)
	var rng uint64 = 7
	m := newMLP(inputDim, 32, &rng)
	a := newAdam(m, 0.01)

	epochs := 15
	losses := make([]float64, epochs)
	for e := 0; e < epochs; e++ {
		losses[e] = trainEpoch(m, a, train, st, 0.05)
	}
	for e := 1; e < epochs; e++ {
		if losses[e] >= losses[e-1] {
			t.Fatalf("loss not strictly decreasing at epoch %d: %.6f >= %.6f", e, losses[e], losses[e-1])
		}
	}
	if losses[epochs-1] > 0.5*losses[0] {
		t.Fatalf("loss barely moved: start %.6f end %.6f", losses[0], losses[epochs-1])
	}
}

func TestInt8RoundTrip(t *testing.T) {
	samples := synth(200)
	trained, err := Train(samples, 32, 40, 0.01, 0.05, 3, nil)
	if err != nil {
		t.Fatal(err)
	}
	qm := Quantize(trained)

	// Reference float prediction (de-normalized) vs the int8 forward pass.
	var maxErr, scale float64
	for _, s := range samples {
		ref := trained.Model.predict(s.Input)*trained.Stats.Std + trained.Stats.Mean
		got := qm.Predict(s.Input)
		if d := math.Abs(got - ref); d > maxErr {
			maxErr = d
		}
		if a := math.Abs(ref); a > scale {
			scale = a
		}
	}
	// Quantization tolerance: int8 has ~1/127 relative resolution per matrix;
	// allow a generous absolute band relative to the score range.
	tol := 0.05*scale + 1
	if maxErr > tol {
		t.Fatalf("int8 round-trip error %.4f exceeds tolerance %.4f (score range ~%.2f)", maxErr, tol, scale)
	}
}

// typeCheck parses and fully type-checks generated Go source. Parsing alone
// misses e.g. an int-inferred scale var breaking the stub's float arithmetic.
func typeCheck(t *testing.T, src string) {
	t.Helper()
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, "weights_out.go", src, parser.AllErrors)
	if err != nil {
		t.Fatalf("exported source does not parse: %v", err)
	}
	conf := types.Config{Importer: importer.Default()}
	if _, err := conf.Check("nnueweights", fset, []*ast.File{f}, nil); err != nil {
		t.Fatalf("exported source does not type-check: %v", err)
	}
}

func TestExportGoCompiles(t *testing.T) {
	samples := synth(50)
	trained, err := Train(samples, 8, 5, 0.01, 0.05, 1, nil)
	if err != nil {
		t.Fatal(err)
	}
	src := ExportGo(trained, "nnueweights")
	if !strings.Contains(src, "func Predict(") || !strings.Contains(src, "UNUSED BY PRODUCTION") {
		t.Fatal("exported source missing loader stub")
	}
	typeCheck(t, src)

	// Degenerate model (epochs=0): zero-init biases/W2 hit quantVec's scale=1
	// zero-guard, which prints whole-number scales — the case that must still
	// emit float64-typed vars and type-check.
	zero, err := Train(samples, 8, 0, 0.01, 0.05, 1, nil)
	if err != nil {
		t.Fatal(err)
	}
	typeCheck(t, ExportGo(zero, "nnueweights"))
}
