package main

import (
	"log"
	"net/http"
	"os"
)

func main() {
	hub := newHub()
	go hub.run()

	http.HandleFunc("/ws", func(w http.ResponseWriter, r *http.Request) {
		serveWs(hub, w, r)
	})

	// Determine static files directory
	// In Docker: files are in /app
	// In development: files are in parent directory ../
	staticDir := "../"
	if _, err := os.Stat("/app/index.html"); err == nil {
		staticDir = "/app"
	}

	// Serve static files
	fs := http.FileServer(http.Dir(staticDir))
	http.Handle("/", fs)

	log.Println("Server starting on :8080")
	log.Printf("Serving static files from: %s", staticDir)
	err := http.ListenAndServe(":8080", nil)
	if err != nil {
		log.Fatal("ListenAndServe: ", err)
	}
}
