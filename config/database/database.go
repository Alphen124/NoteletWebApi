package database

import (
	"database/sql"
	"fmt"
	"log"
	"net/url"
	"os"
	"sync"

	_ "github.com/lib/pq"
)

var (
	dbInstance *sql.DB
	dbOnce     sync.Once
)

func getEnvOrDefault(key, defaultVal string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return defaultVal
}

// ConnectNoteletDB เชื่อมต่อฐานข้อมูล NoteLet (PostgreSQL singleton)
// รองรับทั้ง DATABASE_URL (Railway) และตัวแปรแยก DB_HOST/DB_PORT/...
func ConnectNoteletDB() *sql.DB {
	dbOnce.Do(func() {
		var psqlInfo string

		if dbURL := os.Getenv("DATABASE_URL"); dbURL != "" {
			// Railway / cloud providers inject DATABASE_URL
			// Force sslmode=require if not already set
			u, err := url.Parse(dbURL)
			if err != nil {
				log.Fatalf("Invalid DATABASE_URL: %v", err)
			}
			q := u.Query()
			if q.Get("sslmode") == "" {
				q.Set("sslmode", "require")
				u.RawQuery = q.Encode()
			}
			psqlInfo = u.String()
		} else {
			// Fallback to individual env vars (local dev)
			host := getEnvOrDefault("DB_HOST", "localhost")
			port := getEnvOrDefault("DB_PORT", "5432")
			user := getEnvOrDefault("DB_USER", "alphen")
			password := os.Getenv("DB_PASSWORD")
			if password == "" {
				log.Fatal("Either DATABASE_URL or DB_PASSWORD environment variable is required")
			}
			dbname := getEnvOrDefault("DB_NAME", "notelet")
			psqlInfo = fmt.Sprintf(
				"host=%s port=%s user=%s password=%s dbname=%s sslmode=disable client_encoding=UTF8",
				host, port, user, password, dbname,
			)
		}

		var err error
		dbInstance, err = sql.Open("postgres", psqlInfo)
		if err != nil {
			panic(fmt.Sprintf("Error opening database connection: %v", err))
		}

		if err = dbInstance.Ping(); err != nil {
			panic(fmt.Sprintf("Error pinging database: %v", err))
		}
	})

	return dbInstance
}

// GetNoteletDB ใช้สำหรับดึง instance ของ database
func GetNoteletDB() *sql.DB {
	if dbInstance == nil {
		panic("Database connection is not initialized - this should not happen")
	}
	return dbInstance
}
