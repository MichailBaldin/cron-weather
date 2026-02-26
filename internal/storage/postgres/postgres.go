// Package postgres implements the storage repository using PostgreSQL.
package postgres

import (
	"context"
	"embed"
	"errors"
	"fmt"
	"strconv"
	"time"

	"cron-weather/internal/domain"
	"cron-weather/internal/storage"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/jackc/pgx/v5/stdlib"
	"github.com/pressly/goose/v3"
)

//go:embed migrations/*.sql
var migrationsFS embed.FS

// PostgresRepo implements Repo using PostgreSQL.
type PostgresRepo struct {
	pool *pgxpool.Pool
}

// New creates a PostgresRepo and verifies the connection.
func New(ctx context.Context, dsn string) (*PostgresRepo, error) {
	if err := runMigrations(ctx, dsn); err != nil {
		return nil, err
	}

	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		return nil, fmt.Errorf("pgxpool new: %w", err)
	}

	ctxPing, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	if err := pool.Ping(ctxPing); err != nil {
		pool.Close()
		return nil, fmt.Errorf("pg ping: %w", err)
	}

	return &PostgresRepo{pool: pool}, nil
}

func runMigrations(ctx context.Context, dsn string) error {
	cfg, err := pgxpool.ParseConfig(dsn)
	if err != nil {
		return fmt.Errorf("parse dsn: %w", err)
	}

	db := stdlib.OpenDB(*cfg.ConnConfig)
	defer db.Close()

	ctxPing, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	if err := db.PingContext(ctxPing); err != nil {
		return fmt.Errorf("db ping: %w", err)
	}

	goose.SetBaseFS(migrationsFS)
	if err := goose.SetDialect("postgres"); err != nil {
		return fmt.Errorf("goose dialect: %w", err)
	}

	if err := goose.Up(db, "migrations"); err != nil {
		return fmt.Errorf("goose up: %w", err)
	}

	return nil
}

// Close releases database resources.
func (r *PostgresRepo) Close() {
	if r.pool != nil {
		r.pool.Close()
	}
}

var _ storage.Repo = (*PostgresRepo)(nil)

// ActiveSubscription ensures an active subscription for chatID and returns its ID.
func (r *PostgresRepo) ActiveSubscription(ctx context.Context, chatID int64) (string, error) {
	ownerRef := fmt.Sprintf("telegram:chat:%d", chatID)
	endpointKind := "telegram"
	endpointAddr := strconv.FormatInt(chatID, 10)

	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return "", fmt.Errorf("begin: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	endpointID, err := upsertEndpoint(ctx, tx, endpointKind, endpointAddr)
	if err != nil {
		return "", err
	}

	subID, err := upsertSubscription(ctx, tx, ownerRef)
	if err != nil {
		return "", err
	}

	_, err = tx.Exec(ctx, `
		INSERT INTO subscription_endpoints(endpoint_id, subscription_id)
		VALUES($1, $2)
		ON CONFLICT(endpoint_id) DO UPDATE
		SET subscription_id = EXCLUDED.subscription_id`,
		endpointID, subID)
	if err != nil {
		return "", fmt.Errorf("link endpoint->subscription: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return "", fmt.Errorf("commit: %w", err)
	}

	return subID, nil
}

// DeactivateSubscription marks the subscription for chatID as inactive.
func (r *PostgresRepo) DeactivateSubscription(ctx context.Context, chatID int64) error {
	ownerRef := fmt.Sprintf("telegram:chat:%d", chatID)

	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	var subID string
	err = tx.QueryRow(ctx, `SELECT id FROM subscriptions WHERE owner_ref=$1`, ownerRef).Scan(&subID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			// nothing to deactivate
			return nil
		}
		return fmt.Errorf("get subscription: %w", err)
	}

	_, err = tx.Exec(ctx, `UPDATE subscriptions SET active=false, updated_at=now() WHERE id=$1`, subID)
	if err != nil {
		return fmt.Errorf("deactivate subscription: %w", err)
	}

	_, err = tx.Exec(ctx, `UPDATE schedules SET active=false, updated_at=now() WHERE subscription_id=$1`, subID)
	if err != nil {
		return fmt.Errorf("deactivate schedules: %w", err)
	}

	return tx.Commit(ctx)
}

// CreateScheduler creates a new schedule for the given chat.
func (r *PostgresRepo) CreateScheduler(ctx context.Context, chatID int64, cronExpr string, startAt, endAt *time.Time) (string, error) {
	ownerRef := fmt.Sprintf("telegram:chat:%d", chatID)

	// require subscription to exist (and be active)
	var subID string
	err := r.pool.QueryRow(ctx, `SELECT id FROM subscriptions WHERE owner_ref=$1 AND active=true`, ownerRef).Scan(&subID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return "", fmt.Errorf("subscription not active")
		}
		return "", fmt.Errorf("get subscription: %w", err)
	}

	var scheduleID string
	err = r.pool.QueryRow(ctx, `
		INSERT INTO schedules(subscription_id, kind, expr, tz, starts_at, ends_at, active)
		VALUES($1, 'cron', $2, 'Europe/Vilnius', $3, $4, true)
		RETURNING id
	`, subID, cronExpr, startAt, endAt).Scan(&scheduleID)
	if err != nil {
		return "", fmt.Errorf("insert schedule: %w", err)
	}
	return scheduleID, nil
}

// StopScheduler deactivates a schedule owned by the given chat.
func (r *PostgresRepo) StopScheduler(ctx context.Context, chatID int64, schedulerID string) error {
	ownerRef := fmt.Sprintf("telegram:chat:%d", chatID)

	cmdTag, err := r.pool.Exec(ctx, `
		UPDATE schedules
		SET active=false, updated_at=now()
		WHERE id=$1
		  AND subscription_id = (SELECT id FROM subscriptions WHERE owner_ref=$2)
	`, schedulerID, ownerRef)
	if err != nil {
		return fmt.Errorf("stop schedule: %w", err)
	}
	if cmdTag.RowsAffected() == 0 {
		return fmt.Errorf("schedule not found")
	}
	return nil
}

// ListActiveSchedulers returns active schedules for the given chat.
func (r *PostgresRepo) ListActiveSchedulers(ctx context.Context, chatID int64) ([]domain.Scheduler, error) {
	ownerRef := fmt.Sprintf("telegram:chat:%d", chatID)

	rows, err := r.pool.Query(ctx, `
		SELECT sc.id, sc.expr, sc.starts_at, sc.ends_at, sc.active, sc.created_at
		FROM schedules sc
		JOIN subscriptions s ON s.id = sc.subscription_id
		WHERE s.owner_ref=$1 AND s.active=true AND sc.active=true
		ORDER BY sc.created_at ASC
	`, ownerRef)
	if err != nil {
		return nil, fmt.Errorf("query schedules: %w", err)
	}
	defer rows.Close()

	var out []domain.Scheduler
	for rows.Next() {
		var it domain.Scheduler
		var startAt, endAt *time.Time
		err := rows.Scan(&it.ID, &it.Expr, &startAt, &endAt, &it.IsActive, &it.CreatedAt)
		if err != nil {
			return nil, fmt.Errorf("scan: %w", err)
		}
		it.StartAt = startAt
		it.EndAt = endAt
		out = append(out, it)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("rows: %w", err)
	}
	return out, nil
}

// ListAllActiveSchedulers returns all active schedules with delivery targets for bootstrapping.
func (r *PostgresRepo) ListAllActiveSchedulers(ctx context.Context) ([]domain.SchedulerWithTarget, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT sc.id, sc.subscription_id, sc.kind, sc.expr, sc.tz, sc.starts_at, sc.ends_at, sc.active, sc.created_at,
		       s.owner_ref, s.lat, s.lon, s.active,
		       e.kind, e.address
		FROM schedules sc
		JOIN subscriptions s ON s.id = sc.subscription_id
		JOIN subscription_endpoints se ON se.subscription_id = s.id
		JOIN endpoints e ON e.id = se.endpoint_id
		WHERE s.active=true AND sc.active=true
		ORDER BY sc.created_at ASC
	`)
	if err != nil {
		return nil, fmt.Errorf("query all active schedules: %w", err)
	}
	defer rows.Close()

	var out []domain.SchedulerWithTarget
	for rows.Next() {
		var it domain.SchedulerWithTarget
		var startAt, endAt *time.Time
		err := rows.Scan(
			&it.Scheduler.ID,
			&it.Scheduler.SubscriptionID,
			&it.Scheduler.Kind,
			&it.Scheduler.Expr,
			&it.Scheduler.TZ,
			&startAt,
			&endAt,
			&it.Scheduler.IsActive,
			&it.Scheduler.CreatedAt,
			&it.Subscription.OwnerRef,
			&it.Subscription.Lat,
			&it.Subscription.Lon,
			&it.Subscription.IsActive,
			&it.Target.Kind,
			&it.Target.Address,
		)
		if err != nil {
			return nil, fmt.Errorf("scan: %w", err)
		}
		it.Scheduler.StartAt = startAt
		it.Scheduler.EndAt = endAt
		it.Subscription.ID = it.Scheduler.SubscriptionID
		out = append(out, it)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("rows: %w", err)
	}
	return out, nil
}

// GetActiveScheduler loads a single active schedule with its target for runtime registration.
func (r *PostgresRepo) GetActiveScheduler(ctx context.Context, schedulerID string) (domain.SchedulerWithTarget, error) {
	var it domain.SchedulerWithTarget
	var startAt, endAt *time.Time
	err := r.pool.QueryRow(ctx, `
		SELECT sc.id, sc.subscription_id, sc.kind, sc.expr, sc.tz, sc.starts_at, sc.ends_at, sc.active, sc.created_at,
		       s.owner_ref, s.lat, s.lon, s.active,
		       e.kind, e.address
		FROM schedules sc
		JOIN subscriptions s ON s.id = sc.subscription_id
		JOIN subscription_endpoints se ON se.subscription_id = s.id
		JOIN endpoints e ON e.id = se.endpoint_id
	WHERE sc.id=$1 AND s.active=true AND sc.active=true
	`, schedulerID).Scan(
		&it.Scheduler.ID,
		&it.Scheduler.SubscriptionID,
		&it.Scheduler.Kind,
		&it.Scheduler.Expr,
		&it.Scheduler.TZ,
		&startAt,
		&endAt,
		&it.Scheduler.IsActive,
		&it.Scheduler.CreatedAt,
		&it.Subscription.OwnerRef,
		&it.Subscription.Lat,
		&it.Subscription.Lon,
		&it.Subscription.IsActive,
		&it.Target.Kind,
		&it.Target.Address,
	)
	if err != nil {
		return domain.SchedulerWithTarget{}, fmt.Errorf("get active schedule: %w", err)
	}
	it.Scheduler.StartAt = startAt
	it.Scheduler.EndAt = endAt
	it.Subscription.ID = it.Scheduler.SubscriptionID
	return it, nil
}

// DeactivateScheduler marks a schedule as inactive.
func (r *PostgresRepo) DeactivateScheduler(ctx context.Context, schedulerID string) error {
	_, err := r.pool.Exec(ctx, `UPDATE schedules SET active=false, updated_at=now() WHERE id=$1`, schedulerID)
	if err != nil {
		return fmt.Errorf("deactivate schedule: %w", err)
	}
	return nil
}

// UpdateSchedulerNextRunAt updates the computed next run time for a schedule.
func (r *PostgresRepo) UpdateSchedulerNextRunAt(ctx context.Context, schedulerID string, nextRunAt *time.Time) error {
	_, err := r.pool.Exec(ctx, `UPDATE schedules SET next_run_at=$2, updated_at=now() WHERE id=$1`, schedulerID, nextRunAt)
	if err != nil {
		return fmt.Errorf("update next_run_at: %w", err)
	}
	return nil
}

// InsertRun records one schedule execution attempt.
func (r *PostgresRepo) InsertRun(ctx context.Context, subscriptionID, schedulerID string, scheduledFor time.Time, status, payload, errText string) error {
	// scheduler_id is not stored directly in schema; payload can carry it.
	if payload == "" {
		payload = fmt.Sprintf("scheduler_id=%s", schedulerID)
	}
	_, err := r.pool.Exec(ctx, `
		INSERT INTO runs(subscription_id, scheduled_for, finished_at, status, payload, error)
		VALUES($1, $2, now(), $3, $4, NULLIF($5, ''))
	`, subscriptionID, scheduledFor, status, payload, errText)
	if err != nil {
		return fmt.Errorf("insert run: %w", err)
	}
	return nil
}

// ReserveDailyUsage atomically reserves one API call for the subscription for the given day.
func (r *PostgresRepo) ReserveDailyUsage(ctx context.Context, subscriptionID string, day time.Time, limit int) (bool, int, error) {
	dayKey := day.UTC().Format("2006-01-02")
	var used int
	err := r.pool.QueryRow(ctx, `
		INSERT INTO daily_usage(day, subscription_id, used)
		VALUES($1::date, $2, 1)
		ON CONFLICT (day, subscription_id) DO UPDATE
		SET used = daily_usage.used + 1
		WHERE daily_usage.used < $3
		RETURNING used
	`, dayKey, subscriptionID, limit).Scan(&used)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return false, limit, nil
		}
		return false, 0, fmt.Errorf("reserve daily usage: %w", err)
	}
	return true, used, nil
}

// MarkAlertSent stores the alert fingerprint if it has not been sent yet.
// It returns true if the fingerprint was inserted (i.e. the alert is new).
func (r *PostgresRepo) MarkAlertSent(ctx context.Context, subscriptionID string, fingerprint string) (bool, error) {
	cmd, err := r.pool.Exec(ctx, `
		INSERT INTO sent_alerts(subscription_id, fingerprint)
		VALUES($1, $2)
		ON CONFLICT (subscription_id, fingerprint) DO NOTHING
	`, subscriptionID, fingerprint)
	if err != nil {
		return false, fmt.Errorf("mark alert sent: %w", err)
	}
	return cmd.RowsAffected() > 0, nil
}

// SetSubscriptionLocation updates coordinates for the chat subscription.
func (r *PostgresRepo) SetSubscriptionLocation(ctx context.Context, chatID int64, lat, lon float64) error {
	ownerRef := fmt.Sprintf("telegram:chat:%d", chatID)
	cmd, err := r.pool.Exec(ctx, `
		UPDATE subscriptions
		SET lat=$2, lon=$3, updated_at=now()
		WHERE owner_ref=$1
	`, ownerRef, lat, lon)
	if err != nil {
		return fmt.Errorf("set subscription location: %w", err)
	}
	if cmd.RowsAffected() == 0 {
		return fmt.Errorf("subscription not found")
	}
	return nil
}

func upsertEndpoint(ctx context.Context, tx pgx.Tx, kind, address string) (string, error) {
	var endpointID string

	err := tx.QueryRow(ctx, `
		INSERT INTO endpoints(kind, address)
		VALUES ($1, $2)
		ON CONFLICT (kind, address)
		DO UPDATE SET kind = EXCLUDED.kind
		RETURNING id;`,
		kind, address).Scan(&endpointID)
	if err != nil {
		return "", fmt.Errorf("upsert endpoint: %w", err)
	}
	return endpointID, nil
}

func upsertSubscription(ctx context.Context, tx pgx.Tx, ownerRef string) (string, error) {
	var subID string

	err := tx.QueryRow(ctx, `
		INSERT INTO subscriptions(owner_ref, active)
		VALUES($1, true)
		ON CONFLICT(owner_ref) DO UPDATE SET active = true, updated_at = now()
		RETURNING id`,
		ownerRef).Scan(&subID)
	if err != nil {
		return "", fmt.Errorf("upsert subscription: %w", err)
	}
	return subID, err
}
