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
}

func LoadConfig() *Config {
	backendURL := getEnv("BACKEND_URL", "ws://localhost:8080/ws")
	poolSize, _ := strconv.Atoi(getEnv("BOT_POOL_SIZE", "10"))

	return &Config{
		BackendURL: backendURL,
		PoolSize:   poolSize,
		NamePrefix: getEnv("BOT_NAME_PREFIX", ""),
	}
}

func getEnv(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}
