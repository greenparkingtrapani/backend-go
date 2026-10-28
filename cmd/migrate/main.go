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
	if os.Getenv("RAILWAY_ENVIRONMENT_NAME") != "staging" || os.Getenv("RAILWAY_PROJECT_ID") != "f74a2555-8202-40db-84f4-71df97ee4b58" || os.Getenv("RAILWAY_ENVIRONMENT_ID") != "e621aab4-c0d4-4b4c-99df-3f0df64dd4f4" {
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
