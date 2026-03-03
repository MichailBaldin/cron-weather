// Package storage defines repository interfaces used by the application.
package storage

import (
	"context"
	"time"

	"cron-weather/internal/domain"
)

// Repo defines persistence operations required by the application and scheduler runtime.
type Repo interface {
	ActiveSubscription(ctx context.Context, chatID int64) (string, error)
	DeactivateSubscription(ctx context.Context, chatID int64) error

	CreateScheduler(ctx context.Context, chatID int64, cronExpr string, tz string, startAt, endAt *time.Time) (string, error)
	StopScheduler(ctx context.Context, chatID int64, schedulerID string) error
	ListActiveSchedulers(ctx context.Context, chatID int64) ([]domain.Scheduler, error)

	// Runtime scheduler support
	ListAllActiveSchedulers(ctx context.Context) ([]domain.SchedulerWithTarget, error)
	GetActiveScheduler(ctx context.Context, schedulerID string) (domain.SchedulerWithTarget, error)
	DeactivateScheduler(ctx context.Context, schedulerID string) error
	UpdateSchedulerNextRunAt(ctx context.Context, schedulerID string, nextRunAt *time.Time) error
	InsertRun(ctx context.Context, subscriptionID, schedulerID string, scheduledFor time.Time, status, payload, errText string) error

	// Weather task support
	ReserveDailyUsage(ctx context.Context, subscriptionID string, day time.Time, limit int) (ok bool, used int, err error)
	MarkAlertSent(ctx context.Context, subscriptionID string, fingerprint string) (inserted bool, err error)
	SetSubscriptionLocation(ctx context.Context, chatID int64, lat, lon float64) error

	Close()
}
