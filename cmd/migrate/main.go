package main

import (
	"context"
	"database/sql"
	"estacionamienti/internal/db"
	_ "github.com/lib/pq"
	"log"
	"os"
	"time"
)

func main() {
	// Fail closed: this command is authorized for staging only.
	if os.Getenv("RAILWAY_ENVIRONMENT_NAME") != "staging" {
		log.Fatal("migration requires RAILWAY_ENVIRONMENT_NAME=staging")
	}
	url := os.Getenv("DATABASE_URL")
	if url == "" {
		log.Fatal("DATABASE_URL is required")
	}
	database, err := sql.Open("postgres", url)
	if err != nil {
		log.Fatal("cannot open migration database")
	}
	defer database.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	if err := db.MigrateSiteContent(ctx, database); err != nil {
		log.Fatalf("site content migration failed: %v", err)
	}
	log.Print("site content migration completed")
}
