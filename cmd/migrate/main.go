package main

import (
	"database/sql"
	"fmt"
	"log"
	"os"

	"github.com/joho/godotenv"
	_ "github.com/lib/pq"
)

func main() {
	godotenv.Load(".env")
	dsn := fmt.Sprintf("host=%s port=%s user=%s password=%s dbname=%s sslmode=disable",
		getenv("DB_HOST", "localhost"),
		getenv("DB_PORT", "5432"),
		getenv("DB_USER", "alphen"),
		getenv("DB_PASSWORD", "goldfutionz.124"),
		getenv("DB_NAME", "notelet"),
	)
	db, err := sql.Open("postgres", dsn)
	if err != nil {
		log.Fatal(err)
	}
	defer db.Close()

	_, err = db.Exec(`ALTER TABLE appuser ADD COLUMN IF NOT EXISTS is_central_staff BOOLEAN NOT NULL DEFAULT false`)
	if err != nil {
		log.Fatal("migrate:", err)
	}
	fmt.Println("OK: is_central_staff column ensured")

	// Verify
	var cnt int
	db.QueryRow(`SELECT COUNT(*) FROM appuser WHERE is_central_staff = true`).Scan(&cnt)
	fmt.Printf("Central staff accounts: %d\n", cnt)
}

func getenv(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}
