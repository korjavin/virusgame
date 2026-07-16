package distill_test

import (
	"context"
	"reflect"
	"testing"

	"virusgame/search/distill"
)

// External-package proof: outside the distill package, FrozenCandidate is opaque.
// The only value external code can name is the zero token — its fields are
// unexported, so they cannot be set or mutated (uncommenting either line below
// fails to compile). A zero token is untrusted, so EvaluateTest rejects it and no
// external caller can forge a token to authorize the once-only holdout.
func TestFrozenCandidateOpaqueExternally(t *testing.T) {
	// Automated guarantee that no externally-settable field exists.
	ty := reflect.TypeOf(distill.FrozenCandidate{})
	for i := 0; i < ty.NumField(); i++ {
		if ty.Field(i).IsExported() {
			t.Fatalf("FrozenCandidate exposes settable field %q; it must be fully opaque", ty.Field(i).Name)
		}
	}

	var zero distill.FrozenCandidate
	// zero.weights = search.IncumbentWeights() // compile error: unexported field
	// zero.provenance = distill.Provenance{}   // compile error: unexported field

	// Only read-only copy accessors are reachable; they expose no mutable state.
	_ = zero.Weights()
	_ = zero.Provenance()

	cfg := distill.CIConfig()
	if _, err := distill.EvaluateTest(context.Background(), cfg, []uint64{9001, 9002}, zero, distill.Limits{}, 2); err == nil {
		t.Fatal("external zero candidate must be rejected by EvaluateTest")
	}
}
