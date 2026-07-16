package search

import (
	"os"
	"strconv"
)

// Throwaway sweep hooks (deleted before commit).
func init() {
	if v := os.Getenv("VS_AI2_33_FRAG"); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			fragilityCoef = n
		}
	}
	if v := os.Getenv("VS_AI2_32_MOBW"); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			mobilityWeight = n
		}
	}
	if v := os.Getenv("VS_AI2_32_DANGER"); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			strangulationDanger = n
		}
	}
	if v := os.Getenv("VS_AI2_34_SPACE"); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			spaceRaceCoef = n
		}
	}
}
