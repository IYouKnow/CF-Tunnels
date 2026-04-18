package main

import (
	"database/sql"
	"fmt"

	_ "github.com/glebarez/sqlite"
)

func main() {
	dbs := []string{"tunnels.db", "../tunnels.db"}
	
	for _, dbPath := range dbs {
		db, err := sql.Open("sqlite", dbPath+"?cache=shared")
		if err != nil {
			continue
		}
		
		// Drop and recreate tables
		db.Exec("DROP TABLE IF EXISTS tunnels")
		db.Exec("DROP TABLE IF EXISTS logs")
		db.Exec("DROP TABLE IF EXISTS ingress_rules")
		
		db.Exec(`
			CREATE TABLE tunnels (
				id INTEGER PRIMARY KEY AUTOINCREMENT,
				name TEXT UNIQUE NOT NULL,
				uuid TEXT,
				account_id TEXT,
				zone_id TEXT,
				subdomain TEXT,
				domain TEXT,
				dns_record_id TEXT,
				created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
				status TEXT DEFAULT 'stopped',
				pid INTEGER DEFAULT 0
			);
			CREATE TABLE logs (
				id INTEGER PRIMARY KEY AUTOINCREMENT,
				tunnel_id INTEGER,
				timestamp DATETIME DEFAULT CURRENT_TIMESTAMP,
				level TEXT,
				message TEXT,
				FOREIGN KEY(tunnel_id) REFERENCES tunnels(id) ON DELETE CASCADE
			);
			CREATE TABLE ingress_rules (
				id INTEGER PRIMARY KEY AUTOINCREMENT,
				tunnel_id INTEGER NOT NULL,
				hostname TEXT NOT NULL,
				path TEXT,
				service TEXT NOT NULL,
				protocol TEXT DEFAULT 'http',
				FOREIGN KEY(tunnel_id) REFERENCES tunnels(id) ON DELETE CASCADE
			);
		`)
		
		fmt.Printf("Reset: %s\n", dbPath)
		db.Close()
	}
	fmt.Println("Done - databases reset with fresh schema")
}