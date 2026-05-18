package storage

import (
	"database/sql"
	"errors"

	_ "github.com/lib/pq"
)

var ErrMissingDSN = errors.New("database dsn is empty")

func InitDB(dsn string) (*sql.DB, error) {
	if dsn == "" {
		return nil, ErrMissingDSN
	}

	db, err := sql.Open("postgres", dsn)
	if err != nil {
		return nil, err
	}
	if err := db.Ping(); err != nil {
		return nil, err
	}

	return db, nil
}
