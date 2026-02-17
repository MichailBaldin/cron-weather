package storage

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"cron-weather/internal/subscription"

	_ "github.com/mattn/go-sqlite3"
)

type Repository interface {
	GetAll(ctx context.Context) ([]subscription.Subscription, error)
	Add(ctx context.Context, sub subscription.Subscription) error
	Remove(ctx context.Context, chatID int64) error
	Close() error
}

type SQLiteRepository struct {
	db *sql.DB
}

func NewSQLiteRepository(dbPath string) (*SQLiteRepository, error) {
	db, err := sql.Open("sqlite3", dbPath)
	if err != nil {
		return nil, fmt.Errorf("failed to open DB: %w", err)
	}
	if err := db.Ping(); err != nil {
		return nil, fmt.Errorf("failed to ping DB: %w", err)
	}
	if err := createTable(db); err != nil {
		return nil, err
	}
	return &SQLiteRepository{db: db}, nil
}

func createTable(db *sql.DB) error {
	query := `
	CREATE TABLE IF NOT EXISTS subscriptions (
		chat_id INTEGER PRIMARY KEY,
		interval INTEGER NOT NULL, -- храним в наносекундах
		start_at TEXT NOT NULL DEFAULT '',
		lat REAL NOT NULL,
		lon REAL NOT NULL
	);`
	_, err := db.Exec(query)
	if err != nil {
		return fmt.Errorf("failed to create table: %w", err)
	}
	return nil
}

func (r *SQLiteRepository) GetAll(ctx context.Context) ([]subscription.Subscription, error) {
	rows, err := r.db.QueryContext(ctx, "SELECT chat_id, interval, start_at, lat, lon FROM subscriptions")
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var subs []subscription.Subscription
	for rows.Next() {
		var s subscription.Subscription
		var intervalNanos int64
		if err := rows.Scan(&s.ChatID, &intervalNanos, &s.StartAt, &s.Lat, &s.Lon); err != nil {
			return nil, err
		}
		s.Interval = time.Duration(intervalNanos)
		subs = append(subs, s)
	}
	return subs, rows.Err()
}

func (r *SQLiteRepository) Add(ctx context.Context, sub subscription.Subscription) error {
	_, err := r.db.ExecContext(ctx,
		"INSERT OR REPLACE INTO subscriptions (chat_id, interval, start_at, lat, lon) VALUES (?, ?, ?, ?, ?)",
		sub.ChatID, int64(sub.Interval), sub.StartAt, sub.Lat, sub.Lon,
	)
	return err
}

func (r *SQLiteRepository) Remove(ctx context.Context, chatID int64) error {
	_, err := r.db.ExecContext(ctx, "DELETE FROM subscriptions WHERE chat_id = ?", chatID)
	return err
}

func (r *SQLiteRepository) Close() error {
	return r.db.Close()
}