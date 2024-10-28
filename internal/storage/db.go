package storage

import (
	"database/sql"
	"fmt"
	"time"

	"github.com/rs/zerolog/log"

	_ "github.com/lib/pq"
)

type Options struct {
	MaxOpenConns    int
	MaxIdleConns    int
	ConnMaxLifetime time.Duration
	ConnMaxIdleTime time.Duration
}

func InitDB(dsn string, opt Options) (*sql.DB, error) {
	if dsn == "" {
		return nil, fmt.Errorf("empty db dsn")
	}
	db, err := sql.Open("postgres", dsn)
	if err != nil {
		return nil, err
	}
	if opt.MaxOpenConns > 0 {
		db.SetMaxOpenConns(opt.MaxOpenConns)
	}
	if opt.MaxIdleConns > 0 {
		db.SetMaxIdleConns(opt.MaxIdleConns)
	}
	if opt.ConnMaxLifetime > 0 {
		db.SetConnMaxLifetime(opt.ConnMaxLifetime)
	}
	if opt.ConnMaxIdleTime > 0 {
		db.SetConnMaxIdleTime(opt.ConnMaxIdleTime)
	}
	if err := db.Ping(); err != nil {
		_ = db.Close()
		return nil, err
	}

	log.Info().Msg("PostgreSQL database initialized successfully")
	return db, nil
}
