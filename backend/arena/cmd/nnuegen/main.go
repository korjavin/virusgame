// Command nnuegen generates the offline NNUE-lite training corpus: it samples
// positions, labels each with a deep search score + eventual game outcome, and
// writes deterministic JSONL shards. Output feeds tools/nnue-train (Stage 2);
// Stage 3 (int8 inference inside search) is a separate later bead.
//
// Positions come from three sources, interleaved per worker under a seeded
// xorshift64 stream (Task 3):
//   - "selfplay": self-contained telemetry-agent games; every intermediate
//     position is recorded and backfilled with the game's winner/placement.
//   - "corpus": owner-corpus replays (loaded via -corpus); positions carry the
//     replay's known winner.
//   - "ladder": RandomLegalOpening seeds; no completed game, sentinel outcome.
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
//	                             (ladder positions with no completed game).
//	                             Placement is the mover's finishing rank (1=won).
//	source         string        "selfplay" | "corpus" | "ladder"
package main

import (
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"

	"virusgame/arena"
	"virusgame/game"
	"virusgame/search"
)

// errNoDeepScore signals that ChooseNodeBudget could not produce a real deep
// score for a position: a terminal / no-legal-move state (ok=false), an
// opening-book hit, or a search that could not complete even depth 1 (both leave
// Depth 0 with a placeholder Score 0). Such positions must be skipped, not
// recorded as DeepScore 0 — a decided corpus terminal would otherwise get a
// contradictory label (score 0 alongside a real win/loss outcome), and opening
// positions (which the book short-circuits, so NNUE never evaluates them) would
// pin a false 0 target.
var errNoDeepScore = errors.New("no deep score for position")

// mateMagnitude is the floor for treating a deep score as a forced-mate result.
// search's mateScore is 1e9 (unexported); forced wins/losses back up ±(mateScore
// − ply), while a normal eval stays well under ~1e6. Half of mateScore cleanly
// separates the two without a fragile exact match.
const mateMagnitude = 500_000_000

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
// outcome is filled by the caller (self-play/corpus backfill winner+placement;
// ladder positions keep the zero sentinel).
func Label(state game.State, budget uint64, source string) (Record, error) {
	fingerprint, err := arena.StateFingerprint(state)
	if err != nil {
		return Record{}, err
	}
	result, ok := search.ChooseNodeBudget(state, budget)
	// Depth 0 means no search-derived score: an opening-book hit (Depth/Nodes 0,
	// Score 0) or a budget too small to finish depth 1. Both leave Score at the
	// placeholder 0; recording that as a label would poison the target.
	if !ok || result.Depth == 0 {
		return Record{}, errNoDeepScore
	}
	// A forced mate within the node budget backs up ±mateScore(~1e9) − ply, orders
	// of magnitude above the ~1e3–1e4 of a normal eval. A single such outlier blows
	// up the trainer's global z-score std and collapses every normal target into a
	// dead band, so drop mate-magnitude positions (search finds those anyway).
	if result.Score >= mateMagnitude || result.Score <= -mateMagnitude {
		return Record{}, errNoDeepScore
	}
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

// placement returns the mover's finishing rank given the game winner: 1 if the
// mover won, 2 otherwise. Self-play and corpus games here are 2-player.
func placement(mover, winner int) int {
	if winner == 0 {
		return 0
	}
	if mover == winner {
		return 1
	}
	return 2
}

// ShardWriter appends JSONL records to a per-worker shard file. Append-safe
// (O_APPEND) so -resume can continue an existing shard.
type ShardWriter struct {
	file *os.File
	buf  *bufio.Writer
	seen map[string]bool // dedupe by fingerprint
}

// NewShardWriter opens (creating/appending) shard-NNN.jsonl in dir for worker.
// seen pre-seeds the dedupe set (used by -resume to skip already-written
// fingerprints); pass nil for a fresh writer.
func NewShardWriter(dir string, worker int, seen map[string]bool) (*ShardWriter, error) {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, err
	}
	path := filepath.Join(dir, fmt.Sprintf("shard-%03d.jsonl", worker))
	file, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return nil, err
	}
	if seen == nil {
		seen = map[string]bool{}
	}
	return &ShardWriter{file: file, buf: bufio.NewWriter(file), seen: seen}, nil
}

// Write emits one record as a JSON line, skipping fingerprints already written.
// Returns true if the record was written.
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

// loadFingerprints scans every shard-*.jsonl in dir and returns the set of
// fingerprints already recorded, so -resume skips them.
// ponytail: O(total-records) startup scan + in-memory set; fine for the
// smoke/local runs. A production 1-5M run would want a compacted index file.
func loadFingerprints(dir string) (map[string]bool, error) {
	seen := map[string]bool{}
	shards, err := filepath.Glob(filepath.Join(dir, "shard-*.jsonl"))
	if err != nil {
		return nil, err
	}
	for _, path := range shards {
		file, err := os.Open(path)
		if err != nil {
			return nil, err
		}
		scanner := bufio.NewScanner(file)
		scanner.Buffer(make([]byte, 0, 1<<20), 1<<24)
		for scanner.Scan() {
			var record Record
			if err := json.Unmarshal(scanner.Bytes(), &record); err != nil {
				file.Close()
				return nil, fmt.Errorf("%s: %w", path, err)
			}
			seen[record.Fingerprint] = true
		}
		if err := scanner.Err(); err != nil {
			file.Close()
			return nil, err
		}
		file.Close()
	}
	return seen, nil
}

// truncatePartialRecord drops a torn trailing line from a shard. A killed writer
// flushes bufio in 4 KB chunks, so an interrupted run can leave the final record
// half-written (no trailing newline). Left in place it aborts the -resume scan
// (json.Unmarshal fails) and the next append concatenates onto the garbage.
// Truncating back to the last newline makes resume both scannable and append-safe;
// a file that is entirely one partial line becomes empty.
func truncatePartialRecord(path string) error {
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return err
	}
	if len(data) == 0 || data[len(data)-1] == '\n' {
		return nil // clean shard (or empty) — no torn tail
	}
	cut := bytes.LastIndexByte(data, '\n') + 1 // 0 when the whole file is one partial line
	return os.Truncate(path, int64(cut))
}

// cloneSet returns a shallow copy of s (nil for nil), so each worker mutates its
// own dedupe set without racing siblings.
//
// ponytail: per-worker dedupe only. Two workers can each emit the same
// fingerprint (one line per shard), so a multi-worker fresh run holds fewer
// unique positions than the printed total, and the trainer over-weights the
// overlap. Accepted here because global dedupe needs a shared locked set, which
// would forfeit the per-shard byte-determinism the acceptance criteria require
// (write order across workers becomes non-deterministic). Committed artifacts
// (smoke fixture, determinism test) run -workers 1, so they are unaffected; the
// -resume path already merges every shard's fingerprints into one global set.
// Upgrade path for the production run: assign each fingerprint to a fixed shard
// by hash%workers so dedupe is global yet still order-independent.
func cloneSet(s map[string]bool) map[string]bool {
	if s == nil {
		return nil
	}
	out := make(map[string]bool, len(s))
	for k := range s {
		out[k] = true
	}
	return out
}

// countLines returns the number of JSONL records in path, or 0 if it is absent.
func countLines(path string) (int, error) {
	file, err := os.Open(path)
	if os.IsNotExist(err) {
		return 0, nil
	}
	if err != nil {
		return 0, err
	}
	defer file.Close()
	count := 0
	scanner := bufio.NewScanner(file)
	scanner.Buffer(make([]byte, 0, 1<<20), 1<<24)
	for scanner.Scan() {
		count++
	}
	return count, scanner.Err()
}

// Config parameterizes a generation run so tests can drive it in-process.
type Config struct {
	Out        string
	Workers    int
	Positions  int
	Budget     uint64
	Seed       uint64
	Boards     []arena.Board
	CorpusPath string // owner-corpus manifest; "" disables the corpus source
	Resume     bool
}

func next(rng *uint64) uint64 {
	*rng ^= *rng << 13
	*rng ^= *rng >> 7
	*rng ^= *rng << 17
	return *rng
}

// roster is the fixed set of deterministic self-play agents. Self-play only
// needs the chosen action, so these are plain Agents (no telemetry).
func roster() []arena.Agent {
	budget := func(nodes uint64) arena.Agent {
		return func(state game.State) (game.Action, bool) {
			result, ok := search.ChooseNodeBudget(state, nodes)
			return result.Action, ok
		}
	}
	return []arena.Agent{
		budget(2000),
		budget(8000),
		arena.Tournament(2),
		arena.Greedy,
		arena.BaseAttacker,
		arena.MobilityAttacker,
	}
}

// corpusPosition carries a replay position plus its game's known winner.
type corpusPosition struct {
	state  game.State
	winner int
}

// loadCorpus reads the owner-corpus manifest and reconstructs every within-turn
// position from each replay fixture, deterministically ordered.
func loadCorpus(manifest string) ([]corpusPosition, error) {
	entries, err := arena.LoadOwnerCorpus(manifest)
	if err != nil {
		return nil, err
	}
	dir := filepath.Dir(manifest)
	var positions []corpusPosition
	for _, entry := range entries {
		fixture, err := os.Open(filepath.Join(dir, entry.FixtureName()))
		if err != nil {
			return nil, err
		}
		replay, _, err := arena.DecodeReplay(fixture)
		fixture.Close()
		if err != nil {
			return nil, err
		}
		points, err := arena.ReplayPositions(replay)
		if err != nil {
			return nil, err
		}
		// Deterministic order: sort points by (turn, afterActions).
		keys := make([]arena.ReplayPoint, 0, len(points))
		for point := range points {
			keys = append(keys, point)
		}
		// Unique map keys → a total order; stability is irrelevant.
		sort.Slice(keys, func(i, j int) bool {
			a, b := keys[i], keys[j]
			return a.Turn < b.Turn || (a.Turn == b.Turn && a.AfterActions < b.AfterActions)
		})
		for _, point := range keys {
			positions = append(positions, corpusPosition{state: points[point], winner: int(replay.Winner)})
		}
	}
	return positions, nil
}

// selfPlay plays one 2-player game between agentA (seat 1) and agentB (seat 2),
// recording every intermediate position and backfilling the game outcome.
func selfPlay(board arena.Board, budget uint64, agentA, agentB arena.Agent) []Record {
	state, err := game.New(board.Rows, board.Cols, 2)
	if err != nil {
		return nil
	}
	var records []Record
	maxPlies := board.Rows * board.Cols * 4
	for plies := 0; !state.GameOver() && plies < maxPlies; plies++ {
		record, err := Label(state, budget, "selfplay")
		if err == nil {
			records = append(records, record)
		}
		agent := agentA
		if state.CurrentPlayer() == 2 {
			agent = agentB
		}
		action, ok := agent(state)
		if !ok {
			break
		}
		next, err := state.Apply(action)
		if err != nil {
			break
		}
		state = next
	}
	winner := int(state.Winner())
	for i := range records {
		records[i].Outcome = Outcome{Winner: winner, Placement: placement(records[i].CurrentPlayer, winner)}
	}
	return records
}

// generateWorker samples up to target positions into worker's shard. seen is the
// worker's private dedupe set (a clone of the resume scan, or nil for a fresh run).
func generateWorker(cfg Config, worker, target int, corpus []corpusPosition, seen map[string]bool) (written int, retErr error) {
	existing := 0 // records already in this worker's shard (resume counts them toward target)
	if cfg.Resume {
		var err error
		existing, err = countLines(filepath.Join(cfg.Out, fmt.Sprintf("shard-%03d.jsonl", worker)))
		if err != nil {
			return 0, err
		}
	}
	writer, err := NewShardWriter(cfg.Out, worker, seen)
	if err != nil {
		return 0, err
	}
	// Close flushes the final buffered records; surface a flush error rather
	// than silently dropping the tail of the shard.
	defer func() {
		if cerr := writer.Close(); cerr != nil && retErr == nil {
			retErr = cerr
		}
	}()

	agents := roster()
	rng := (cfg.Seed + uint64(worker)*0x9e3779b97f4a7c15) | 1
	emit := func(record Record) error {
		if existing+written >= target {
			return nil
		}
		ok, err := writer.Write(record)
		if err != nil {
			return err
		}
		if ok {
			written++
		}
		return nil
	}

	for attempt := 0; existing+written < target && attempt < target*40; attempt++ {
		board := cfg.Boards[int(next(&rng)%uint64(len(cfg.Boards)))]
		switch next(&rng) % 3 {
		case 0: // self-play
			agentA := agents[int(next(&rng)%uint64(len(agents)))]
			agentB := agents[int(next(&rng)%uint64(len(agents)))]
			for _, record := range selfPlay(board, cfg.Budget, agentA, agentB) {
				if err := emit(record); err != nil {
					return written, err
				}
			}
		case 1: // ladder opening
			snapshot, err := arena.RandomLegalOpening(board.Rows, board.Cols, next(&rng))
			if err != nil {
				continue
			}
			state, err := game.FromSnapshot(snapshot)
			if err != nil {
				continue
			}
			record, err := Label(state, cfg.Budget, "ladder")
			if errors.Is(err, errNoDeepScore) {
				continue
			}
			if err != nil {
				return written, err
			}
			if err := emit(record); err != nil {
				return written, err
			}
		case 2: // owner-corpus replay position
			if len(corpus) == 0 {
				continue
			}
			position := corpus[int(next(&rng)%uint64(len(corpus)))]
			record, err := Label(position.state, cfg.Budget, "corpus")
			if errors.Is(err, errNoDeepScore) {
				continue // decided terminal position — no meaningful deep score
			}
			if err != nil {
				return written, err
			}
			record.Outcome = Outcome{Winner: position.winner, Placement: placement(record.CurrentPlayer, position.winner)}
			if err := emit(record); err != nil {
				return written, err
			}
		}
	}
	return written, nil
}

// Generate runs cfg across cfg.Workers shards and returns the total positions
// written. Each worker is deterministic in (seed, worker index), so two runs
// with the same config produce byte-identical shards.
func Generate(cfg Config) (int, error) {
	if len(cfg.Boards) == 0 {
		cfg.Boards = []arena.Board{{Rows: 8, Cols: 8}}
	}
	if cfg.Workers < 1 {
		cfg.Workers = 1
	}
	corpus, err := loadCorpus(cfg.CorpusPath)
	if err != nil {
		return 0, err
	}
	// Scan existing shards ONCE, up front — before any worker opens its shard
	// for append. Scanning inside each worker goroutine would race sibling
	// workers' concurrent writes (torn final line -> abort). Each worker gets a
	// private clone so per-shard dedupe writes don't race across workers.
	var resumeSeen map[string]bool
	if cfg.Resume {
		// Repair torn trailing records left by a killed writer before scanning or
		// counting, so both see clean shards and appends resume on a newline.
		shards, gerr := filepath.Glob(filepath.Join(cfg.Out, "shard-*.jsonl"))
		if gerr != nil {
			return 0, gerr
		}
		for _, shard := range shards {
			if terr := truncatePartialRecord(shard); terr != nil {
				return 0, terr
			}
		}
		resumeSeen, err = loadFingerprints(cfg.Out)
		if err != nil {
			return 0, err
		}
	}
	var wg sync.WaitGroup
	totals := make([]int, cfg.Workers)
	errs := make([]error, cfg.Workers)
	for worker := 0; worker < cfg.Workers; worker++ {
		target := cfg.Positions / cfg.Workers
		if worker < cfg.Positions%cfg.Workers {
			target++
		}
		wg.Add(1)
		go func(worker, target int) {
			defer wg.Done()
			totals[worker], errs[worker] = generateWorker(cfg, worker, target, corpus, cloneSet(resumeSeen))
		}(worker, target)
	}
	wg.Wait()
	total := 0
	for worker := 0; worker < cfg.Workers; worker++ {
		if errs[worker] != nil {
			return total, errs[worker]
		}
		total += totals[worker]
	}
	return total, nil
}

// parseBoards parses "8x8,12x12" into arena.Board specs.
func parseBoards(spec string) ([]arena.Board, error) {
	var boards []arena.Board
	for _, part := range strings.Split(spec, ",") {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		dims := strings.SplitN(part, "x", 2)
		if len(dims) != 2 {
			return nil, fmt.Errorf("bad board %q (want RxC)", part)
		}
		rows, err := strconv.Atoi(dims[0])
		if err != nil {
			return nil, fmt.Errorf("bad board %q: %w", part, err)
		}
		cols, err := strconv.Atoi(dims[1])
		if err != nil {
			return nil, fmt.Errorf("bad board %q: %w", part, err)
		}
		boards = append(boards, arena.Board{Rows: rows, Cols: cols})
	}
	if len(boards) == 0 {
		return nil, fmt.Errorf("no boards parsed from %q", spec)
	}
	return boards, nil
}

func main() {
	out := flag.String("out", "", "output shard directory (required)")
	workers := flag.Int("workers", 1, "number of parallel shard writers")
	positions := flag.Int("positions", 5000, "target total positions to sample")
	budget := flag.Uint64("budget", 2000, "node budget for deep-score labels")
	seed := flag.Uint64("seed", 1, "base seed for deterministic sampling")
	boards := flag.String("boards", "8x8", "comma-separated board sizes, e.g. 8x8,12x12")
	corpus := flag.String("corpus", "", "owner-corpus manifest path (enables the corpus source)")
	resume := flag.Bool("resume", false, "scan existing shards and skip fingerprints already present")
	flag.Parse()
	if *out == "" {
		fmt.Fprintln(os.Stderr, "-out is required")
		os.Exit(2)
	}
	parsedBoards, err := parseBoards(*boards)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(2)
	}
	total, err := Generate(Config{
		Out:        *out,
		Workers:    *workers,
		Positions:  *positions,
		Budget:     *budget,
		Seed:       *seed,
		Boards:     parsedBoards,
		CorpusPath: *corpus,
		Resume:     *resume,
	})
	if err != nil {
		panic(err)
	}
	fmt.Printf("wrote %d positions across %d shard(s) in %s\n", total, *workers, *out)
}
