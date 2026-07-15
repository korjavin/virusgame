// Command replayinspect prints stable hashes for every recorded action boundary.
package main

import (
	"fmt"
	"os"
	"sort"

	"virusgame/arena"
)

func main() {
	for _, path := range os.Args[1:] {
		fixture, err := os.Open(path)
		if err != nil {
			panic(err)
		}
		replay, _, err := arena.DecodeReplay(fixture)
		fixture.Close()
		if err != nil {
			panic(err)
		}
		positions, err := arena.ReplayPositions(replay)
		if err != nil {
			panic(err)
		}
		points := make([]arena.ReplayPoint, 0, len(positions))
		for point := range positions {
			points = append(points, point)
		}
		sort.Slice(points, func(i, j int) bool {
			return points[i].Turn < points[j].Turn || points[i].Turn == points[j].Turn && points[i].AfterActions < points[j].AfterActions
		})
		for _, point := range points {
			hash, _ := arena.SnapshotHash(positions[point].Snapshot())
			fmt.Printf("%s T%d.%d %s\n", replay.SourceID, point.Turn, point.AfterActions, hash[:16])
		}
	}
}
