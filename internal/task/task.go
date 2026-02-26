// Package task defines the task runner abstraction executed by the scheduler runtime.
package task

import (
	"context"
	"time"

	"cron-weather/internal/domain"
)

// Input is passed from scheduler runtime to a task.
type Input struct {
	Scheduler    domain.Scheduler
	Subscription domain.Subscription
	Target       domain.SchedulerTarget
	ScheduledFor time.Time
}

// Result is returned by a task and then delivered to the endpoint.
type Result struct {
	Messages []string
	Payload  string
}

// Runner executes one scheduled task.
// Scheduler runtime is responsible for "when"; Runner is responsible for "what".
type Runner interface {
	Run(ctx context.Context, in Input) (Result, error)
}
