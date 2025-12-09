package main

import (
    "log"
    "os"
    "os/signal"
    "syscall"
)

func main() {
    log.Println("Starting bot-hoster service...")

    config := LoadConfig()
    manager := NewBotManager(config)

    // Start bot pool
    if err := manager.Start(); err != nil {
        log.Fatalf("Failed to start bot manager: %v", err)
    }

    log.Printf("Bot-hoster started with %d bots connected to %s",
        config.PoolSize, config.BackendURL)

    // Wait for interrupt signal
    sigChan := make(chan os.Signal, 1)
    signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)
    <-sigChan

    log.Println("Shutting down bot-hoster...")
    manager.Stop()
    log.Println("Bot-hoster stopped")
}
