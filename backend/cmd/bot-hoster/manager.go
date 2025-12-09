package main

import (
	"fmt"
	"log"
	"sync"
)

type BotManager struct {
	config *Config
	bots   []*Bot
	mu     sync.RWMutex
}

func NewBotManager(config *Config) *BotManager {
	return &BotManager{
		config: config,
		bots:   make([]*Bot, 0, config.PoolSize),
	}
}

// Start initializes and connects all bots
func (m *BotManager) Start() error {
	log.Printf("Starting bot pool with size: %d", m.config.PoolSize)

	for i := 0; i < m.config.PoolSize; i++ {
		bot := NewBot(m.config.BackendURL, m)

		if err := bot.Connect(); err != nil {
			log.Printf("Failed to connect bot %d: %v (continuing with remaining bots)", i+1, err)
			continue
		}

		m.mu.Lock()
		m.bots = append(m.bots, bot)
		m.mu.Unlock()

		// Start bot message loop in goroutine
		go bot.Run()

		log.Printf("Bot %d/%d started", i+1, m.config.PoolSize)
	}

	m.mu.RLock()
	connectedCount := len(m.bots)
	m.mu.RUnlock()

	if connectedCount == 0 {
		return fmt.Errorf("no bots connected successfully")
	}

	log.Printf("Bot pool ready: %d/%d bots connected", connectedCount, m.config.PoolSize)
	return nil
}

// Stop gracefully shuts down all bots
func (m *BotManager) Stop() {
	log.Println("Stopping bot pool...")

	m.mu.Lock()
	defer m.mu.Unlock()

	for _, bot := range m.bots {
		bot.Disconnect()
	}

	log.Printf("All %d bots stopped", len(m.bots))
}

// GetStats returns current pool statistics
func (m *BotManager) GetStats() map[string]int {
	m.mu.RLock()
	defer m.mu.RUnlock()

	stats := map[string]int{
		"total":        len(m.bots),
		"idle":         0,
		"in_lobby":     0,
		"in_game":      0,
		"disconnected": 0,
	}

	for _, bot := range m.bots {
		bot.mu.RLock()
		state := bot.State
		bot.mu.RUnlock()

		switch state {
		case BotIdle:
			stats["idle"]++
		case BotInLobby:
			stats["in_lobby"]++
		case BotInGame:
			stats["in_game"]++
		case BotDisconnected:
			stats["disconnected"]++
		}
	}

	return stats
}
