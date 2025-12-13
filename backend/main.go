package main

import (
	"log"
	"net/http"
	"os"
	"strings"
)

// noCacheMiddleware adds cache-busting headers for JS/CSS files
func noCacheMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Apply no-cache headers to JS and CSS files to prevent stale code
		if strings.HasSuffix(r.URL.Path, ".js") || strings.HasSuffix(r.URL.Path, ".css") {
			w.Header().Set("Cache-Control", "no-cache, no-store, must-revalidate")
			w.Header().Set("Pragma", "no-cache")
			w.Header().Set("Expires", "0")
		}
		next.ServeHTTP(w, r)
	})
}

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

	// Serve static files with no-cache headers to prevent browser caching issues
	fs := http.FileServer(http.Dir(staticDir))
	http.Handle("/", noCacheMiddleware(fs))

	log.Println("Server starting on :8080")
	log.Printf("Serving static files from: %s", staticDir)
	err := http.ListenAndServe(":8080", nil)
	if err != nil {
		log.Fatal("ListenAndServe: ", err)
	}
}
