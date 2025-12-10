package main

import (
    "log"
    "os"
)

func main() {
    backendURL := os.Getenv("BACKEND_URL")
    if backendURL == "" {
        backendURL = "ws://localhost:8080/ws"
    }

    bot := NewBot(backendURL)

    if err := bot.Connect(); err != nil {
        log.Fatal(err)
    }

    log.Println("Bot running... Press Ctrl+C to stop")
    bot.Run()
}
