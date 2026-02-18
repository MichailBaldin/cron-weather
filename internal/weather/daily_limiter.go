package weather

import (
	"errors"
	"sync"
	"time"
)

var ErrDailyWeatherLimitExceeded = errors.New("openweather daily limit exceeded")

type DailyLimiter struct {
	mu    sync.Mutex
	day   string
	used  int
	limit int
	loc   *time.Location
}

func NewDailyLimiter(limit int, loc *time.Location) *DailyLimiter {
	if loc == nil {
		loc = time.UTC
	}
	return &DailyLimiter{
		limit: limit,
		loc:   loc,
	}
}

// Allow increments usage for the current day and reports whether the call is allowed.
func (l *DailyLimiter) Allow(now time.Time) (remaining int, ok bool) {
	l.mu.Lock()
	defer l.mu.Unlock()

	today := now.In(l.loc).Format("2006-01-02")
	if l.day != today {
		l.day = today
		l.used = 0
	}

	if l.used >= l.limit {
		return 0, false
	}

	l.used++
	return l.limit - l.used, true
}