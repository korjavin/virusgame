package main

import "testing"

func TestDefaultParallelismLeavesHeadroomAndCapsAtThree(t *testing.T) {
	tests := map[int]int{0: 1, 1: 1, 2: 1, 3: 2, 4: 3, 8: 3}
	for cpus, want := range tests {
		if got := defaultParallelism(cpus); got != want {
			t.Fatalf("cpus=%d got=%d want=%d", cpus, got, want)
		}
	}
}
