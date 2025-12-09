package main

import (
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"
)

func main() {
	log.Println("=== Bot-Hoster Service Starting ===")

	config := LoadConfig()
	log.Printf("Configuration:")
	log.Printf("  Backend URL: %s", config.BackendURL)
	log.Printf("  Pool Size: %d", config.PoolSize)

	manager := NewBotManager(config)

	// Start bot pool
	if err := manager.Start(); err != nil {
		log.Fatalf("Failed to start bot manager: %v", err)
	}

	log.Println("=== Bot-Hoster Service Running ===")

	// Print stats periodically
	statsTicker := time.NewTicker(30 * time.Second)
	go func() {
		for range statsTicker.C {
			stats := manager.GetStats()
			log.Printf("Pool stats: Total=%d, Idle=%d, InLobby=%d, InGame=%d, Disconnected=%d",
				stats["total"], stats["idle"], stats["in_lobby"], stats["in_game"], stats["disconnected"])
		}
	}()

	// Wait for interrupt signal
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)
	<-sigChan

	log.Println("=== Shutdown Signal Received ===")
	statsTicker.Stop()
	manager.Stop()
	log.Println("=== Bot-Hoster Service Stopped ===")
}
