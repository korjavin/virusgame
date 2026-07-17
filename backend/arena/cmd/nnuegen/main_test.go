package main

import (
	"encoding/json"
	"reflect"
	"testing"
)

// TestRecordRoundTrip asserts a Record survives JSON marshal/unmarshal
// unchanged. Full in-process generation + determinism lands in Task 3.
func TestRecordRoundTrip(t *testing.T) {
	want := Record{
		Fingerprint:   "deadbeefcafef00d",
		Rows:          8,
		Cols:          8,
		CurrentPlayer: 1,
		Features: [4][]float64{
			{1, 0, 2.5, 3, 0, 0, 1, 0, 0, 0, 0, 0, 1, 0, 0, 4, 0, 1, 2},
			{0, 1, 1, 0, 0, 0, 0, 1, 0, 0, 0, 0, 1, 0, 0, 3, 1, 0, 0},
			nil,
			nil,
		},
		DeepScore: -42,
		Budget:    20000,
		Outcome:   Outcome{Winner: 1, Placement: 1},
		Source:    "selfplay",
	}
	encoded, err := json.Marshal(want)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var got Record
	if err := json.Unmarshal(encoded, &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if !reflect.DeepEqual(want, got) {
		t.Fatalf("round-trip mismatch:\n want %+v\n got  %+v", want, got)
	}
}
