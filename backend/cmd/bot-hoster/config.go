package main

import (
	"os"
	"strconv"
)

type Config struct {
	BackendURL string
	PoolSize   int
	// NamePrefix is passed to the server so this hoster's bots are named e.g.
	// "Canary Bot 1234". Empty (default) keeps the plain "Bot 1234" names.
	NamePrefix string
	// Challenger turns this pool into a self-sparring generator: even-indexed
	// bots challenge idle odd-indexed (acceptor) peers, so the pool plays
	// bot-vs-bot games with no human/matchmaker. Default false (accept-only).
	Challenger bool
	// ExploreEpsilon: probability of playing a uniformly random legal move
	// instead of the search's best, per turn. Injects diversity into self-play
	// data (deterministic search otherwise replays identical games). 0 = off.
	ExploreEpsilon float64
}

func LoadConfig() *Config {
	backendURL := getEnv("BACKEND_URL", "ws://localhost:8080/ws")
	poolSize, _ := strconv.Atoi(getEnv("BOT_POOL_SIZE", "10"))
	epsilon, _ := strconv.ParseFloat(getEnv("BOT_EXPLORE_EPSILON", "0"), 64)

	return &Config{
		BackendURL:     backendURL,
		PoolSize:       poolSize,
		NamePrefix:     getEnv("BOT_NAME_PREFIX", ""),
		Challenger:     getEnv("BOT_CHALLENGER", "") == "true",
		ExploreEpsilon: epsilon,
	}
}

func getEnv(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}
