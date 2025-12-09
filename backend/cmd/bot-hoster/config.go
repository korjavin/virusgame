package main

import (
    "os"
    "strconv"
)

type Config struct {
    BackendURL  string
    PoolSize    int
}

func LoadConfig() *Config {
    backendURL := getEnv("BACKEND_URL", "ws://localhost:8080/ws")
    poolSize, _ := strconv.Atoi(getEnv("BOT_POOL_SIZE", "10"))

    return &Config{
        BackendURL: backendURL,
        PoolSize:   poolSize,
    }
}

func getEnv(key, defaultValue string) string {
    if value := os.Getenv(key); value != "" {
        return value
    }
    return defaultValue
}
