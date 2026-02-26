// Package domain defines core domain models used across the application.
package domain

import "time"

// Scheduler defines a persisted cron schedule.
type Scheduler struct {
	ID string
	// SubscriptionID is the owner subscription (telegram chat is linked through endpoints).
	SubscriptionID string
	Kind           string
	Expr           string
	TZ             string
	StartAt        *time.Time
	EndAt          *time.Time
	IsActive       bool
	CreatedAt      time.Time
}

// SchedulerTarget describes where the scheduled job should deliver its output.
// For now only telegram endpoints are used.
type SchedulerTarget struct {
	Kind    string
	Address string
}

// SchedulerWithTarget is used by runtime scheduler to bootstrap jobs after restart.
type SchedulerWithTarget struct {
	Scheduler    Scheduler
	Subscription Subscription
	Target       SchedulerTarget
}
