// Package storage contains subscription persistence and in-memory cache.
package storage

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"cron-weather/internal/subscription"

	_ "github.com/mattn/go-sqlite3"
)

// Repository stores subscriptions.
type Repository interface {
	GetAll(ctx context.Context) ([]subscription.Subscription, error)
	Add(ctx context.Context, sub subscription.Subscription) error
	Remove(ctx context.Context, chatID int64) error
	Close() error
}

// SQLiteRepository stores subscriptions in SQLite.
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

// createTable ensures required schema exists.
func createTable(db *sql.DB) error {
	subscriptions := `
	CREATE TABLE IF NOT EXISTS subscriptions (
		chat_id INTEGER PRIMARY KEY,
		interval INTEGER NOT NULL,
		start_at TEXT NOT NULL DEFAULT '',
		lat REAL NOT NULL,
		lon REAL NOT NULL
	);`

	sentAlerts := `
	CREATE TABLE IF NOT EXISTS sent_alerts (
		chat_id INTEGER NOT NULL,
		fingerprint TEXT NOT NULL,
		sent_at INTEGER NOT NULL,
		PRIMARY KEY (chat_id, fingerprint)
	);`

	if _, err := db.Exec(subscriptions); err != nil {
		return fmt.Errorf("failed to create subscriptions table: %w", err)
	}
	if _, err := db.Exec(sentAlerts); err != nil {
		return fmt.Errorf("failed to create sent_alerts table: %w", err)
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

type SentAlertsRepository interface {
	WasSent(ctx context.Context, chatID int64, fingerprint string) (bool, error)
	MarkSent(ctx context.Context, chatID int64, fingerprint string, sentAt time.Time) error
}

func (r *SQLiteRepository) WasSent(ctx context.Context, chatID int64, fingerprint string) (bool, error) {
	var exists int
	err := r.db.QueryRowContext(ctx,
		"SELECT 1 FROM sent_alerts WHERE chat_id = ? AND fingerprint = ? LIMIT 1",
		chatID, fingerprint,
	).Scan(&exists)

	if err == sql.ErrNoRows {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return true, nil
}

func (r *SQLiteRepository) MarkSent(ctx context.Context, chatID int64, fingerprint string, sentAt time.Time) error {
	_, err := r.db.ExecContext(ctx,
		"INSERT OR IGNORE INTO sent_alerts (chat_id, fingerprint, sent_at) VALUES (?, ?, ?)",
		chatID, fingerprint, sentAt.Unix(),
	)
	return err
}