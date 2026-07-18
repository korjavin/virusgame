package arena

import (
	"virusgame/game"
	"virusgame/nnuefeat"
)

// The NNUE-lite feature extractor moved to virusgame/nnuefeat (Stage 3,
// vs-ai2.56) so the search package can compute the same features for in-search
// inference without importing arena (arena imports search — a cycle). These
// aliases keep the offline generator/trainer and existing tests unchanged.

// PlayerFeatures is the shared per-player feature vector; see nnuefeat.
type PlayerFeatures = nnuefeat.PlayerFeatures

// NNUEFeatures computes the per-player feature vectors for a position.
func NNUEFeatures(state game.State) [4]PlayerFeatures { return nnuefeat.NNUEFeatures(state) }
