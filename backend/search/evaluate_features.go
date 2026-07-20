package search

// Feature indices for the explicit linear evaluation components.
//
// We only extract terms here that can be factored exactly into
// Feature * Weight without altering the arithmetic rounding of the original
// evaluation function. Terms such as the area-normalized material (which
// multiply by weight prior to integer division) and the executable cut
// reward (which divides dynamically based on the opponent's component size)
// cannot be factored cleanly, so they remain outside the vector as documented.
const (
	featBaseExits = iota
	featBaseOpenings
	featBaseAnchors
	featBaseThreatTempo
	featBaseClosed
	featNeutralUnused
	featMovesLeft
	numFeatures
)

// incumbentWeights defines the frozen weight vector for the dot product.
var incumbentWeights = [numFeatures]int{
	featBaseExits:       180,
	featBaseOpenings:    80,
	featBaseAnchors:     240,
	featBaseThreatTempo: -650,
	featBaseClosed:      -5000,
	featNeutralUnused:   20,
	featMovesLeft:       12,
}
