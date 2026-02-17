package weather

import (
	"context"
	"errors"
	"log/slog"
	"testing"
)

type mockFetcher struct {
	messages []string
	err      error
}

func (m *mockFetcher) FetchAlerts(ctx context.Context, lat, lon float64) ([]string, error) {
	return m.messages, m.err
}

func TestNewFetchJob(t *testing.T) {
	logger := slog.Default()
	ctx := context.Background()

	t.Run("success with alerts", func(t *testing.T) {
		fetcher := &mockFetcher{messages: []string{"alert1", "alert2"}}
		job := NewFetchJob(fetcher, 0, 0)
		job(ctx, logger)
	})

	t.Run("success no alerts", func(t *testing.T) {
		fetcher := &mockFetcher{messages: []string{}}
		job := NewFetchJob(fetcher, 0, 0)
		job(ctx, logger)
	})

	t.Run("fetcher error", func(t *testing.T) {
		fetcher := &mockFetcher{err: errors.New("some error")}
		job := NewFetchJob(fetcher, 0, 0)
		job(ctx, logger)
	})

	t.Run("context cancelled", func(t *testing.T) {
		ctx, cancel := context.WithCancel(ctx)
		cancel()
		fetcher := &mockFetcher{messages: []string{"alert"}}
		job := NewFetchJob(fetcher, 0, 0)
		job(ctx, logger)
	})
}