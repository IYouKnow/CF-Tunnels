package main

import (
	"database/sql"
	"fmt"
	"log"

	_ "github.com/glebarez/sqlite"
)

func main() {
	// Check both possible locations
	dbs := []string{"tunnels.db", "../tunnels.db"}
	
	for _, dbPath := range dbs {
		fmt.Printf("\n=== Checking %s ===\n", dbPath)
		db, err := sql.Open("sqlite", dbPath+"?cache=shared")
		if err != nil {
			log.Printf("Cannot open %s: %v", dbPath, err)
			continue
		}
		defer db.Close()

		rows, _ := db.Query("SELECT id, name, uuid, status FROM tunnels")
		var count int
		for rows.Next() {
			var id int
			var name, uuid, status string
			rows.Scan(&id, &name, &uuid, &status)
			fmt.Printf("ID: %d, Name: %s, UUID: %s, Status: %s\n", id, name, uuid, status)
			count++
		}
		fmt.Printf("Total tunnels: %d\n", count)
	}
}