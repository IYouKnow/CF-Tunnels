package main

import (
	"database/sql"
	"fmt"

	_ "github.com/glebarez/sqlite"
)

func main() {
	db, err := sql.Open("sqlite", "tunnels.db?cache=shared")
	if err != nil {
		fmt.Println("Error:", err)
		return
	}
	defer db.Close()

	rows, err := db.Query("SELECT id, tunnel_id, timestamp, level, message FROM logs")
	if err != nil {
		fmt.Println("Query error:", err)
		return
	}
	defer rows.Close()

	count := 0
	fmt.Println("=== Logs in database ===")
	for rows.Next() {
		var id, tunnelID int
		var timestamp, level, message string
		if err := rows.Scan(&id, &tunnelID, &timestamp, &level, &message); err != nil {
			fmt.Println("Scan error:", err)
			continue
		}
		fmt.Printf("ID: %d, TunnelID: %d, Time: %s, Level: %s, Msg: %s\n", id, tunnelID, timestamp, level, message)
		count++
	}
	fmt.Printf("Total: %d logs\n", count)
}