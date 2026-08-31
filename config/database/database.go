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

func getEnvAny(keys ...string) string {
	for _, key := range keys {
		if v := os.Getenv(key); v != "" {
			return v
		}
	}
	return ""
}

func redactURL(raw string) string {
	if raw == "" {
		return raw
	}
	u, err := url.Parse(raw)
	if err != nil {
		return raw
	}
	if u.User != nil {
		if _, hasPassword := u.User.Password(); hasPassword {
			u.User = url.UserPassword(u.User.Username(), "***REDACTED***")
		} else {
			u.User = url.User(u.User.Username())
		}
	}
	return u.String()
}

// ConnectNoteletDB เชื่อมต่อฐานข้อมูล NoteLet (PostgreSQL singleton)
// รองรับทั้ง DATABASE_URL/POSTGRES_URL (Render/Railway) และ DB_HOST/DB_PORT/DB_USER/...
func ConnectNoteletDB() *sql.DB {
	dbOnce.Do(func() {
		var psqlInfo string

		dbURL := getEnvAny("DATABASE_URL", "POSTGRES_URL")
		if dbURL != "" {
			u, err := url.Parse(dbURL)
			if err != nil {
				log.Fatalf("Invalid DATABASE_URL: %v | value=%s", err, redactURL(dbURL))
			}
			q := u.Query()
			if q.Get("sslmode") == "" {
				q.Set("sslmode", "require")
				u.RawQuery = q.Encode()
			}
			psqlInfo = u.String()
		} else {
			host := getEnvAny("DB_HOST", "POSTGRES_HOST", "localhost")
			port := getEnvAny("DB_PORT", "POSTGRES_PORT", "5432")
			user := getEnvAny("DB_USER", "DB_USERNAME", "POSTGRES_USER", "postgres")
			password := getEnvAny("DB_PASSWORD", "POSTGRES_PASSWORD")
			if password == "" {
				log.Fatal("No database password found. Set DATABASE_URL or DB_PASSWORD/POSTGRES_PASSWORD for the new Render/Postgres instance")
			}
			dbname := getEnvAny("DB_NAME", "POSTGRES_DB", "notelet")
			psqlInfo = fmt.Sprintf(
				"host=%s port=%s user=%s password=%s dbname=%s sslmode=disable client_encoding=UTF8",
				host, port, user, password, dbname,
			)
		}

		var err error
		dbInstance, err = sql.Open("postgres", psqlInfo)
		if err != nil {
			panic(fmt.Sprintf("Error opening database connection: %v | dsn=%s", err, redactURL(psqlInfo)))
		}

		if err = dbInstance.Ping(); err != nil {
			panic(fmt.Sprintf("Error pinging database: %v | dsn=%s", err, redactURL(psqlInfo)))
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
