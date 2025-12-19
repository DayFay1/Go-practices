package database

import (
	"context"
	"database/sql"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib"
)

var (
	sqlOpen   = sql.Open
	timeNow   = time.Now
	timeSleep = time.Sleep
)

func NewPostgres(dsn string) (*sql.DB, error) {
	db, err := sqlOpen("pgx", dsn)
	if err != nil {
		return nil, err
	}

	db.SetMaxOpenConns(10)
	db.SetMaxIdleConns(5)
	db.SetConnMaxLifetime(time.Hour)

	deadline := timeNow().Add(15 * time.Second)
	for {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		err := db.PingContext(ctx)
		cancel()
		if err == nil {
			break
		}
		if timeNow().After(deadline) {
			_ = db.Close()
			return nil, err
		}
		timeSleep(500 * time.Millisecond)
	}

	return db, nil
}
