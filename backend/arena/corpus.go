package arena

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"sort"
	"sync"

	"virusgame/game"
)

const CorpusVersion = "virusgame-strength-corpus-v1"

type Corpus struct {
	Version      string             `json:"version"`
	Generator    string             `json:"generator"`
	GeneratedUTC string             `json:"generated_utc"`
	Trajectories []CorpusTrajectory `json:"trajectories"`
	GroupHashes  map[string]string  `json:"group_hashes"`
	Cases        []CorpusCase       `json:"-"`
}

type CorpusTrajectory struct {
	ID          string             `json:"id"`
	Split       string             `json:"split"`
	Track       string             `json:"track"`
	Seed        uint64             `json:"seed"`
	Rows        int                `json:"rows"`
	Cols        int                `json:"cols"`
	Players     int                `json:"players"`
	Actions     []ReplayMove       `json:"actions"`
	Checkpoints []CorpusCheckpoint `json:"checkpoints"`
}

type CorpusCheckpoint struct {
	AfterActions int      `json:"after_actions"`
	Phase        string   `json:"phase"`
	Strata       []string `json:"strata"`
	Hash         string   `json:"hash"`
}

type CorpusCase struct {
	ID, Trajectory, Split, Track, Phase string
	Strata                              []string
	Seed                                uint64
	Players                             int
	State                               game.State
	Hash                                string
}

// DecodeCorpus validates the frozen action trajectories through State.Apply,
// reconstructs every checkpoint, verifies hashes and split checksums, and
// rejects duplicate positions or a trajectory appearing in both splits.
func DecodeCorpus(reader io.Reader) (Corpus, error) {
	var corpus Corpus
	decoder := json.NewDecoder(reader)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&corpus); err != nil {
		return Corpus{}, fmt.Errorf("decode corpus: %w", err)
	}
	if err := decoder.Decode(&struct{}{}); err != io.EOF {
		return Corpus{}, fmt.Errorf("decode corpus: trailing content")
	}
	if corpus.Version != CorpusVersion || corpus.Generator == "" || len(corpus.Trajectories) == 0 {
		return Corpus{}, fmt.Errorf("invalid corpus metadata")
	}
	seenIDs, seenHashes := map[string]bool{}, map[string]bool{}
	groupMembers := map[string][]string{"train": {}, "heldout": {}}
	for _, trajectory := range corpus.Trajectories {
		if seenIDs[trajectory.ID] || (trajectory.Split != "train" && trajectory.Split != "heldout") {
			return Corpus{}, fmt.Errorf("invalid or duplicate trajectory %q", trajectory.ID)
		}
		seenIDs[trajectory.ID] = true
		if trajectory.Track != "competitive_1v1" && trajectory.Track != "multiplayer" && trajectory.Track != "stress" {
			return Corpus{}, fmt.Errorf("trajectory %s: invalid track %q", trajectory.ID, trajectory.Track)
		}
		state, err := game.New(trajectory.Rows, trajectory.Cols, trajectory.Players)
		if err != nil {
			return Corpus{}, fmt.Errorf("trajectory %s: %w", trajectory.ID, err)
		}
		checkpoints := make(map[int]CorpusCheckpoint, len(trajectory.Checkpoints))
		for _, checkpoint := range trajectory.Checkpoints {
			if checkpoint.AfterActions < 1 || checkpoint.AfterActions > len(trajectory.Actions) || checkpoint.Hash == "" || checkpoints[checkpoint.AfterActions].Hash != "" {
				return Corpus{}, fmt.Errorf("trajectory %s: invalid checkpoint %d", trajectory.ID, checkpoint.AfterActions)
			}
			checkpoints[checkpoint.AfterActions] = checkpoint
		}
		materialized := 0
		for index, move := range trajectory.Actions {
			action, err := move.action()
			if err != nil {
				return Corpus{}, fmt.Errorf("trajectory %s action %d: %w", trajectory.ID, index+1, err)
			}
			state, err = state.Apply(action)
			if err != nil {
				return Corpus{}, fmt.Errorf("trajectory %s action %d: %w", trajectory.ID, index+1, err)
			}
			checkpoint, ok := checkpoints[index+1]
			if !ok {
				continue
			}
			materialized++
			hash, err := SnapshotHash(state.Snapshot())
			if err != nil || hash != checkpoint.Hash || seenHashes[hash] {
				return Corpus{}, fmt.Errorf("trajectory %s checkpoint %d: hash=%s want=%s duplicate=%v err=%v", trajectory.ID, index+1, hash, checkpoint.Hash, seenHashes[hash], err)
			}
			seenHashes[hash] = true
			id := fmt.Sprintf("%s@%d", trajectory.ID, index+1)
			corpus.Cases = append(corpus.Cases, CorpusCase{ID: id, Trajectory: trajectory.ID, Split: trajectory.Split, Track: trajectory.Track, Phase: checkpoint.Phase, Strata: append([]string(nil), checkpoint.Strata...), Seed: trajectory.Seed, Players: trajectory.Players, State: state, Hash: hash})
			groupMembers[trajectory.Split] = append(groupMembers[trajectory.Split], id+":"+hash)
		}
		if materialized != len(checkpoints) {
			return Corpus{}, fmt.Errorf("trajectory %s: materialized %d of %d checkpoints", trajectory.ID, materialized, len(checkpoints))
		}
	}
	for _, split := range []string{"train", "heldout"} {
		sort.Strings(groupMembers[split])
		sum := sha256.Sum256([]byte(joinLines(groupMembers[split])))
		got := hex.EncodeToString(sum[:])
		if corpus.GroupHashes[split] != got {
			return Corpus{}, fmt.Errorf("%s group hash=%s want=%s", split, got, corpus.GroupHashes[split])
		}
	}
	return corpus, nil
}

func SnapshotHash(snapshot game.Snapshot) (string, error) {
	encoded, err := json.Marshal(snapshot)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(encoded)
	return hex.EncodeToString(sum[:]), nil
}

func joinLines(lines []string) string {
	var result string
	for _, line := range lines {
		result += line + "\n"
	}
	return result
}

type CorpusReport struct {
	Split    string
	Overall  Report
	Buckets  map[string]Report
	Interval Interval
}

type CorpusFilter struct {
	Track      string
	Rows, Cols int
}
type CorpusProgress struct {
	Board string
	Games int
}

// CompareCorpus plays each identical snapshot with the contender rotated
// through every player seat. Factories create fresh agents for every suffix so
// caches or session state cannot leak between cases.
func CompareCorpus(corpus Corpus, split string, contender, incumbent func() TelemetryAgent) (CorpusReport, error) {
	return CompareCorpusFiltered(corpus, split, CorpusFilter{}, nil, contender, incumbent)
}

func CompareCorpusFiltered(corpus Corpus, split string, filter CorpusFilter, progress func(CorpusProgress), contender, incumbent func() TelemetryAgent) (CorpusReport, error) {
	report := CorpusReport{Split: split, Buckets: make(map[string]Report)}
	for _, testCase := range corpus.Cases {
		if testCase.Split != split || testCase.Track == "stress" || filter.Track != "" && testCase.Track != filter.Track || filter.Rows > 0 && (testCase.State.Rows() != filter.Rows || testCase.State.Cols() != filter.Cols) {
			continue
		}
		for seat := 0; seat < testCase.Players; seat++ {
			agents := make([]TelemetryAgent, testCase.Players)
			for player := range agents {
				agents[player] = incumbent()
			}
			agents[seat] = contender()
			snapshot := testCase.State.Snapshot()
			result, err := Play(Match{Rows: testCase.State.Rows(), Cols: testCase.State.Cols(), Initial: &snapshot, TelemetryAgents: agents})
			if err != nil {
				return report, fmt.Errorf("case %s seat %d: %w", testCase.ID, seat+1, err)
			}
			focus := game.Player(seat + 1)
			report.Overall.Add(result, focus)
			if progress != nil {
				progress(CorpusProgress{Board: fmt.Sprintf("%dx%d", testCase.State.Rows(), testCase.State.Cols()), Games: report.Overall.Games})
			}
			board := fmt.Sprintf("board=%dx%d", testCase.State.Rows(), testCase.State.Cols())
			seatKey := "seat=" + fmt.Sprint(seat+1)
			phase := "phase=" + testCase.Phase
			keys := []string{board, "track=" + testCase.Track, seatKey, phase, board + "/" + seatKey, board + "/" + seatKey + "/" + phase}
			for _, stratum := range testCase.Strata {
				keys = append(keys, "stratum="+stratum)
			}
			for _, key := range keys {
				bucket := report.Buckets[key]
				bucket.Add(result, focus)
				report.Buckets[key] = bucket
			}
		}
	}
	if report.Overall.Games == 0 {
		return report, fmt.Errorf("corpus split %q has no cases", split)
	}
	report.Interval = Wilson95(report.Overall.Wins, report.Overall.Games)
	return report, nil
}

// CompareCorpusBoards runs deterministic board shards with bounded parallelism
// and returns their exact additive aggregate.
func CompareCorpusBoards(corpus Corpus, split, track string, boards []Board, parallel int, progress func(CorpusProgress), contender, incumbent func() TelemetryAgent) (CorpusReport, error) {
	if parallel < 1 {
		parallel = 1
	}
	type item struct {
		index  int
		report CorpusReport
		err    error
	}
	jobs, results := make(chan int), make(chan item, len(boards))
	var wg sync.WaitGroup
	var progressMu sync.Mutex
	shardProgress := progress
	if progress != nil {
		shardProgress = func(update CorpusProgress) { progressMu.Lock(); defer progressMu.Unlock(); progress(update) }
	}
	for worker := 0; worker < parallel && worker < len(boards); worker++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for index := range jobs {
				b := boards[index]
				r, err := CompareCorpusFiltered(corpus, split, CorpusFilter{Track: track, Rows: b.Rows, Cols: b.Cols}, shardProgress, contender, incumbent)
				results <- item{index, r, err}
			}
		}()
	}
	go func() {
		for index := range boards {
			jobs <- index
		}
		close(jobs)
		wg.Wait()
		close(results)
	}()
	ordered := make([]CorpusReport, len(boards))
	for result := range results {
		if result.err != nil {
			return CorpusReport{}, result.err
		}
		ordered[result.index] = result.report
	}
	return AggregateCorpusReports(split, ordered...), nil
}

func AggregateCorpusReports(split string, shards ...CorpusReport) CorpusReport {
	result := CorpusReport{Split: split, Buckets: make(map[string]Report)}
	for _, shard := range shards {
		addReport(&result.Overall, shard.Overall)
		for key, report := range shard.Buckets {
			bucket := result.Buckets[key]
			addReport(&bucket, report)
			result.Buckets[key] = bucket
		}
	}
	result.Interval = Wilson95(result.Overall.Wins, result.Overall.Games)
	return result
}

func addReport(dst *Report, src Report) {
	dst.Games += src.Games
	dst.Wins += src.Wins
	dst.Losses += src.Losses
	dst.Draws += src.Draws
	dst.Eliminations += src.Eliminations
	dst.Illegal += src.Illegal
	dst.Decisions += src.Decisions
	dst.Maxed += src.Maxed
	dst.Stalled += src.Stalled
	dst.Nodes += src.Nodes
	dst.Evaluations += src.Evaluations
	dst.BudgetShortfalls += src.BudgetShortfalls
	dst.LegalRootActions += src.LegalRootActions
	dst.SearchedRootActions += src.SearchedRootActions
	dst.LegalRootNeutrals += src.LegalRootNeutrals
	dst.SearchedRootNeutrals += src.SearchedRootNeutrals
	if src.CompletedTurnDepth > dst.CompletedTurnDepth {
		dst.CompletedTurnDepth = src.CompletedTurnDepth
	}
	dst.Latencies = append(dst.Latencies, src.Latencies...)
	dst.Elapsed += src.Elapsed
}

func (r CorpusReport) ValidateSuperiority() error {
	if r.Overall.Illegal != 0 || r.Overall.Maxed != 0 || r.Overall.Stalled != 0 || r.Overall.WinRate() < 80 {
		return fmt.Errorf("overall superiority gate failed: %s", r)
	}
	for key, bucket := range r.Buckets {
		if boardSeatKey(key) && bucket.WinRate() < 70 {
			return fmt.Errorf("board/seat superiority gate failed: %s %s", key, bucket)
		}
	}
	return nil
}

func boardSeatKey(key string) bool {
	slash := 0
	for _, char := range key {
		if char == '/' {
			slash++
		}
	}
	return len(key) > 6 && key[:6] == "board=" && slash == 1
}

func (r CorpusReport) String() string {
	return fmt.Sprintf("split=%s %s wilson95=[%.1f%%,%.1f%%]", r.Split, r.Overall, r.Interval.Low, r.Interval.High)
}

func (r CorpusReport) SortedBuckets() []string {
	keys := make([]string, 0, len(r.Buckets))
	for key := range r.Buckets {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}
