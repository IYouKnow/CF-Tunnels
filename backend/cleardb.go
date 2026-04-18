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
		
		db.Exec("DELETE FROM tunnels")
		db.Exec("DELETE FROM logs")
		db.Exec("DELETE FROM ingress_rules")
		fmt.Printf("Cleared: %s\n", dbPath)
		db.Close()
	}
	fmt.Println("Done - all databases cleared")
}