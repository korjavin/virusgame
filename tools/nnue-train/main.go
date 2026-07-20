// Command nnue-train trains a small MLP on the NNUE-lite JSONL shards produced
// by backend/arena/cmd/nnuegen and exports int8-quantized weights as Go source.
// It is Stage 2 of Plan B2 (data + training); Stage 3 (pure-Go int8 inference
// wired into search) is a separate later bead that ADOPTS the exported weights
// and the loader stub emitted here.
//
// Stdlib only — its own module (virusgame-nnue-train), no dependency on the
// backend module and no new backend deps.
//
// Pipeline:
//   - load every shard-*.jsonl under -data (record schema mirrors nnuegen).
//   - build a fixed 4×K input vector per position (inactive seats zero-padded).
//   - z-score-normalize the deep score into the regression target.
//   - deterministic 90/10 train/val split by fingerprint hash.
//   - train a 2-layer MLP (input → hidden → 1) with Adam + MSE, plus a small
//     game-outcome auxiliary term.
//   - per epoch report train loss, val loss, and Spearman rank correlation.
//   - export int8-quantized weights (symmetric per-matrix) to -export as Go
//     source, with a pure-Go forward-pass loader stub for Stage 3.
package main

import (
	"bufio"
	"encoding/json"
	"flag"
	"fmt"
	"hash/fnv"
	"math"
	"os"
	"path/filepath"
	"sort"
)

// featuresPerSeat is the fixed width of arena.PlayerFeatures.Features().
// Kept as a constant here so the input width is stable even if some shard has
// only inactive seats; asserted against the data at load time. vs-ai2.56 grew
// this 19 → 26 (owner-profile features); bump it in lockstep with the extractor.
const featuresPerSeat = 26

const seats = 4

// inputDim is the flattened 4×K input width.
const inputDim = seats * featuresPerSeat

// Outcome mirrors the nnuegen on-disk Outcome.
// ponytail: local struct copy of the nnuegen record schema — duplication over a
// cross-module import keeps this trainer dependency-free and separate from the
// backend module. Keep in sync with backend/arena/cmd/nnuegen/main.go.
type Outcome struct {
	Winner    int `json:"winner"`
	Placement int `json:"placement"`
}

// Record mirrors one nnuegen JSONL line. See nnuegen's package doc for field
// semantics.
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

// Sample is a training row: flattened input, regression target (raw deep score),
// and the game-outcome auxiliary signal (+1 won, -1 lost, 0 unknown/masked).
type Sample struct {
	Input   []float64
	Score   float64
	Outcome float64 // +1/-1, or 0 when unknown (auxiliary term masked)
	Hash    uint32  // fingerprint hash, for the deterministic split
}

// input flattens a record's 4×K feature matrix, zero-padding inactive seats.
func (r Record) input() ([]float64, error) {
	vec := make([]float64, inputDim)
	for seat := 0; seat < seats; seat++ {
		f := r.Features[seat]
		if len(f) == 0 {
			continue // inactive seat: leave zeros
		}
		if len(f) != featuresPerSeat {
			return nil, fmt.Errorf("seat %d has %d features, want %d", seat, len(f), featuresPerSeat)
		}
		copy(vec[seat*featuresPerSeat:], f)
	}
	return vec, nil
}

// outcomeSignal maps the mover's placement to a signed auxiliary target.
func outcomeSignal(o Outcome) float64 {
	switch o.Placement {
	case 1:
		return 1 // mover won
	case 2:
		return -1 // mover lost
	default:
		return 0 // unknown → masked
	}
}

// loadShards reads every shard-*.jsonl under dir into Samples.
func loadShards(dir string) ([]Sample, error) {
	shards, err := filepath.Glob(filepath.Join(dir, "shard-*.jsonl"))
	if err != nil {
		return nil, err
	}
	if len(shards) == 0 {
		return nil, fmt.Errorf("no shard-*.jsonl files in %s", dir)
	}
	sort.Strings(shards)
	var samples []Sample
	for _, path := range shards {
		file, err := os.Open(path)
		if err != nil {
			return nil, err
		}
		scanner := bufio.NewScanner(file)
		scanner.Buffer(make([]byte, 0, 1<<20), 1<<24)
		for scanner.Scan() {
			var rec Record
			if err := json.Unmarshal(scanner.Bytes(), &rec); err != nil {
				file.Close()
				return nil, fmt.Errorf("%s: %w", path, err)
			}
			// Mate-magnitude labels dwarf positional residuals (std explodes,
			// z-norm crushes real signal to zero — measured on deep labels).
			// The net cannot infer forced mates from static features anyway.
			if rec.DeepScore > 200000 || rec.DeepScore < -200000 {
				continue
			}
			vec, err := rec.input()
			if err != nil {
				file.Close()
				return nil, fmt.Errorf("%s: %w", path, err)
			}
			samples = append(samples, Sample{
				Input:   vec,
				Score:   float64(rec.DeepScore),
				Outcome: outcomeSignal(rec.Outcome),
				Hash:    hashString(rec.Fingerprint),
			})
		}
		if err := scanner.Err(); err != nil {
			file.Close()
			return nil, err
		}
		file.Close()
	}
	return samples, nil
}

func hashString(s string) uint32 {
	h := fnv.New32a()
	h.Write([]byte(s))
	return h.Sum32()
}

// Stats holds the target normalization (z-score) recovered from the train set.
type Stats struct {
	Mean, Std float64
}

func normStats(samples []Sample) Stats {
	if len(samples) == 0 {
		return Stats{Std: 1}
	}
	var sum float64
	for _, s := range samples {
		sum += s.Score
	}
	mean := sum / float64(len(samples))
	var varSum float64
	for _, s := range samples {
		d := s.Score - mean
		varSum += d * d
	}
	std := math.Sqrt(varSum / float64(len(samples)))
	if std < 1e-9 {
		std = 1
	}
	return Stats{Mean: mean, Std: std}
}

func (st Stats) norm(score float64) float64 { return (score - st.Mean) / st.Std }

// MLP is a 2-layer network: input → hidden (tanh) → 1 (linear).
type MLP struct {
	In, Hidden int
	W1         [][]float64 // Hidden × In
	B1         []float64   // Hidden
	W2         []float64   // Hidden (output weights)
	B2         float64
}

func newMLP(in, hidden int, rng *uint64) *MLP {
	m := &MLP{In: in, Hidden: hidden, B1: make([]float64, hidden), W2: make([]float64, hidden)}
	m.W1 = make([][]float64, hidden)
	// He-ish init scaled by fan-in; deterministic from the seed.
	scale1 := math.Sqrt(2.0 / float64(in))
	for h := 0; h < hidden; h++ {
		m.W1[h] = make([]float64, in)
		for i := 0; i < in; i++ {
			m.W1[h][i] = randNorm(rng) * scale1
		}
	}
	scale2 := math.Sqrt(2.0 / float64(hidden))
	for h := 0; h < hidden; h++ {
		m.W2[h] = randNorm(rng) * scale2
	}
	return m
}

// forward returns the network output and the hidden activations (for backprop).
func (m *MLP) forward(x []float64) (float64, []float64) {
	hid := make([]float64, m.Hidden)
	for h := 0; h < m.Hidden; h++ {
		z := m.B1[h]
		w := m.W1[h]
		for i := 0; i < m.In; i++ {
			z += w[i] * x[i]
		}
		hid[h] = math.Tanh(z)
	}
	out := m.B2
	for h := 0; h < m.Hidden; h++ {
		out += m.W2[h] * hid[h]
	}
	return out, hid
}

// predict returns just the output.
func (m *MLP) predict(x []float64) float64 {
	out, _ := m.forward(x)
	return out
}

// Adam holds per-parameter moment estimates, laid out to mirror MLP.
type adam struct {
	mW1, vW1 [][]float64
	mB1, vB1 []float64
	mW2, vW2 []float64
	mB2, vB2 float64
	t        int
	lr, b1, b2, eps float64
}

func newAdam(m *MLP, lr float64) *adam {
	a := &adam{lr: lr, b1: 0.9, b2: 0.999, eps: 1e-8}
	a.mW1 = zerosLike(m.W1)
	a.vW1 = zerosLike(m.W1)
	a.mB1 = make([]float64, m.Hidden)
	a.vB1 = make([]float64, m.Hidden)
	a.mW2 = make([]float64, m.Hidden)
	a.vW2 = make([]float64, m.Hidden)
	return a
}

func zerosLike(w [][]float64) [][]float64 {
	out := make([][]float64, len(w))
	for i := range w {
		out[i] = make([]float64, len(w[i]))
	}
	return out
}

// trainEpoch runs one pass over samples, returning the mean training loss.
// auxWeight scales the game-outcome auxiliary term.
func trainEpoch(m *MLP, a *adam, samples []Sample, st Stats, auxWeight float64) float64 {
	// bias-correction terms; recomputed each optimization step (per sample).
	var bc1, bc2 float64
	upd := func(g float64, mm, vv *float64) float64 {
		*mm = a.b1*(*mm) + (1-a.b1)*g
		*vv = a.b2*(*vv) + (1-a.b2)*g*g
		mHat := *mm / bc1
		vHat := *vv / bc2
		return a.lr * mHat / (math.Sqrt(vHat) + a.eps)
	}
	var lossSum float64
	for _, s := range samples {
		// Adam timestep advances once per update step, so bias correction
		// tracks the number of moment updates (not the epoch count).
		a.t++
		bc1 = 1 - math.Pow(a.b1, float64(a.t))
		bc2 = 1 - math.Pow(a.b2, float64(a.t))
		target := st.norm(s.Score)
		out, hid := m.forward(s.Input)
		// primary MSE on normalized score
		dScore := out - target
		loss := dScore * dScore
		// auxiliary: nudge output toward outcome sign when known
		dAux := 0.0
		if s.Outcome != 0 {
			dAux = auxWeight * (out - s.Outcome)
			loss += auxWeight * (out - s.Outcome) * (out - s.Outcome)
		}
		lossSum += loss
		// dLoss/dOut
		gOut := 2*dScore + 2*dAux
		// hidden layer grads first, using the output weights as of the forward
		// pass (before the output-layer step below mutates W2).
		for h := 0; h < m.Hidden; h++ {
			// dLoss/dhid = gOut * W2[h]; tanh' = 1 - hid^2
			gHid := gOut * m.W2[h] * (1 - hid[h]*hid[h])
			m.B1[h] -= upd(gHid, &a.mB1[h], &a.vB1[h])
			w := m.W1[h]
			mw, vw := a.mW1[h], a.vW1[h]
			for i := 0; i < m.In; i++ {
				gW1 := gHid * s.Input[i]
				w[i] -= upd(gW1, &mw[i], &vw[i])
			}
		}
		// output layer grads
		m.B2 -= upd(gOut, &a.mB2, &a.vB2)
		for h := 0; h < m.Hidden; h++ {
			gW2 := gOut * hid[h]
			m.W2[h] -= upd(gW2, &a.mW2[h], &a.vW2[h])
		}
	}
	return lossSum / float64(len(samples))
}

// valLoss returns the mean normalized-score MSE over samples (no aux term).
func valLoss(m *MLP, samples []Sample, st Stats) float64 {
	if len(samples) == 0 {
		return 0
	}
	var sum float64
	for _, s := range samples {
		d := m.predict(s.Input) - st.norm(s.Score)
		sum += d * d
	}
	return sum / float64(len(samples))
}

// spearman returns the Spearman rank correlation between predictions and deep
// scores on samples.
func spearman(m *MLP, samples []Sample) float64 {
	n := len(samples)
	if n < 2 {
		return 0
	}
	preds := make([]float64, n)
	scores := make([]float64, n)
	for i, s := range samples {
		preds[i] = m.predict(s.Input)
		scores[i] = s.Score
	}
	rp := ranks(preds)
	rs := ranks(scores)
	return pearson(rp, rs)
}

// ranks returns fractional ranks (ties averaged).
func ranks(v []float64) []float64 {
	n := len(v)
	idx := make([]int, n)
	for i := range idx {
		idx[i] = i
	}
	sort.Slice(idx, func(a, b int) bool { return v[idx[a]] < v[idx[b]] })
	r := make([]float64, n)
	for i := 0; i < n; {
		j := i
		for j < n && v[idx[j]] == v[idx[i]] {
			j++
		}
		avg := float64(i+j-1)/2 + 1 // average rank (1-based)
		for k := i; k < j; k++ {
			r[idx[k]] = avg
		}
		i = j
	}
	return r
}

func pearson(a, b []float64) float64 {
	n := float64(len(a))
	var sa, sb float64
	for i := range a {
		sa += a[i]
		sb += b[i]
	}
	ma, mb := sa/n, sb/n
	var cov, va, vb float64
	for i := range a {
		da, db := a[i]-ma, b[i]-mb
		cov += da * db
		va += da * da
		vb += db * db
	}
	if va == 0 || vb == 0 {
		return 0
	}
	return cov / math.Sqrt(va*vb)
}

// split partitions samples into train/val by fingerprint hash (deterministic:
// hash%10 == 0 → validation, ~10%).
func split(samples []Sample) (train, val []Sample) {
	for _, s := range samples {
		if s.Hash%10 == 0 {
			val = append(val, s)
		} else {
			train = append(train, s)
		}
	}
	if len(train) == 0 { // tiny datasets: don't starve training
		return samples, nil
	}
	return train, val
}

// randNorm draws a standard-normal via Box-Muller from the xorshift stream.
func randNorm(rng *uint64) float64 {
	u1 := (float64(next(rng)>>11) + 1) / float64(1<<53)
	u2 := float64(next(rng)>>11) / float64(1<<53)
	return math.Sqrt(-2*math.Log(u1)) * math.Cos(2*math.Pi*u2)
}

func next(rng *uint64) uint64 {
	*rng ^= *rng << 13
	*rng ^= *rng >> 7
	*rng ^= *rng << 17
	return *rng
}

// Trained bundles a network with the normalization it was trained under.
type Trained struct {
	Model *MLP
	Stats Stats
}

// Train runs the full training loop and returns the trained model. It prints a
// per-epoch report to w.
func Train(samples []Sample, hidden, epochs int, lr, auxWeight float64, seed uint64, w *os.File) (*Trained, error) {
	if len(samples) == 0 {
		return nil, fmt.Errorf("no samples")
	}
	train, val := split(samples)
	st := normStats(train)
	rng := seed | 1
	m := newMLP(inputDim, hidden, &rng)
	a := newAdam(m, lr)
	for e := 1; e <= epochs; e++ {
		tl := trainEpoch(m, a, train, st, auxWeight)
		vl := valLoss(m, val, st)
		sp := spearman(m, val)
		if w != nil {
			fmt.Fprintf(w, "epoch %3d  train_loss %.6f  val_loss %.6f  spearman %.4f\n", e, tl, vl, sp)
		}
	}
	return &Trained{Model: m, Stats: st}, nil
}

func main() {
	data := flag.String("data", "", "directory of shard-*.jsonl (required)")
	export := flag.String("export", "weights_out.go", "output Go source for int8 weights ('-' for stdout)")
	pkg := flag.String("package", "nnueweights", "package name for the exported weights file")
	hidden := flag.Int("hidden", 48, "hidden layer width")
	epochs := flag.Int("epochs", 100, "training epochs")
	lr := flag.Float64("lr", 0.001, "Adam learning rate")
	aux := flag.Float64("aux", 0.05, "game-outcome auxiliary loss weight")
	seed := flag.Uint64("seed", 1, "init/shuffle seed")
	flag.Parse()
	if *data == "" {
		fmt.Fprintln(os.Stderr, "-data is required")
		os.Exit(2)
	}
	samples, err := loadShards(*data)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	fmt.Printf("loaded %d samples from %s\n", len(samples), *data)
	trained, err := Train(samples, *hidden, *epochs, *lr, *aux, *seed, os.Stdout)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	if err := checkFinite(trained); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	src := ExportGo(trained, *pkg)
	if *export == "-" {
		fmt.Print(src)
		return
	}
	if err := os.WriteFile(*export, []byte(src), 0o644); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	fmt.Printf("wrote int8 weights to %s (package %s)\n", *export, *pkg)
}
