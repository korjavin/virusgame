package main

import (
	"database/sql"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"os"
	"time"

	_ "github.com/mattn/go-sqlite3"
)

func main() {
	dbPath := flag.String("db", "../data/games.db", "Path to SQLite database")
	flag.Parse()

	if _, err := os.Stat(*dbPath); os.IsNotExist(err) {
		log.Fatalf("Database not found at %s", *dbPath)
	}

	db, err := sql.Open("sqlite3", *dbPath)
	if err != nil {
		log.Fatalf("Failed to open database: %v", err)
	}
	defer db.Close()

	rows, err := db.Query(`
		SELECT id, started_at, ended_at, rows, cols,
		       player1_name, player2_name, player3_name, player4_name,
		       result, termination, pgn_content
		FROM games
		ORDER BY started_at DESC
	`)
	if err != nil {
		log.Fatalf("Failed to query games: %v", err)
	}
	defer rows.Close()

	count := 0
	for rows.Next() {
		var id string
		var startedAt, endedAt time.Time
		var r, c int
		var p1, p2, p3, p4 sql.NullString
		var result int
		var termination string
		var pgnContent string

		err = rows.Scan(&id, &startedAt, &endedAt, &r, &c,
			&p1, &p2, &p3, &p4,
			&result, &termination, &pgnContent)
		if err != nil {
			log.Fatalf("Failed to scan row: %v", err)
		}

		fmt.Printf("Game ID: %s\n", id)
		fmt.Printf("Time: %s - %s\n", startedAt.Format(time.RFC822), endedAt.Format(time.RFC822))
		fmt.Printf("Board: %dx%d\n", r, c)
		fmt.Printf("Players: %s vs %s", p1.String, p2.String)
		if p3.Valid && p3.String != "" {
			fmt.Printf(" vs %s", p3.String)
		}
		if p4.Valid && p4.String != "" {
			fmt.Printf(" vs %s", p4.String)
		}
		fmt.Printf("\n")
		fmt.Printf("Result: Winner %d (%s)\n", result, termination)

		fmt.Println("PGN Content (formatted):")
		var pgn interface{}
		if err := json.Unmarshal([]byte(pgnContent), &pgn); err == nil {
			formatted, _ := json.MarshalIndent(pgn, "", "  ")
			fmt.Println(string(formatted))
		} else {
			fmt.Println(pgnContent)
		}
		fmt.Println("--------------------------------------------------")
		count++
	}

	fmt.Printf("Total games found: %d\n", count)
}
