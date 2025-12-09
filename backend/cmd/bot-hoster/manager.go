package main

import "log"

type BotManager struct {
    config *Config
    // TODO: Add bot pool
}

func NewBotManager(config *Config) *BotManager {
    return &BotManager{
        config: config,
    }
}

func (m *BotManager) Start() error {
    log.Println("BotManager.Start() - TODO: Implement in Task 3")
    return nil
}

func (m *BotManager) Stop() {
    log.Println("BotManager.Stop() - TODO: Implement in Task 3")
}
