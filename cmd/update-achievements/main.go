package main

import (
	"database/sql"
	"log"
	"os"

	_ "modernc.org/sqlite"

	"github.com/ad/go-telegram-quest/internal/db"
)

func main() {
	dbPath := os.Getenv("DB_PATH")
	if dbPath == "" {
		dbPath = "./quest.db"
	}

	database, err := sql.Open("sqlite", dbPath+"?_journal_mode=WAL&_busy_timeout=5000")
	if err != nil {
		log.Fatalf("Failed to open database: %v", err)
	}
	defer database.Close()

	log.Println("Updating achievements...")
	if err := db.UpdateAchievements(database); err != nil {
		log.Fatalf("Failed to update achievements: %v", err)
	}

	log.Println("Achievements updated successfully!")
}
