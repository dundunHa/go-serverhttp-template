package storage

import (
	"database/sql"
	"log"

	_ "github.com/lib/pq"

	"go-serverhttp-template/internal/config"
)

var DB *sql.DB

func InitDB() (*sql.DB, error) {
	cfg, err := config.LoadConfig()
	if err != nil {
		return nil, err
	}

	db, err := sql.Open("postgres", cfg.DB.Mysql.Addr)
	if err != nil {
		return nil, err
	}
	if err := db.Ping(); err != nil {
		return nil, err
	}

	DB = db
	log.Println("PostgreSQL Database initialized successfully.")
	return db, nil
}
