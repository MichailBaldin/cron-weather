package weather

import (
	"context"
	"errors"
	"log/slog"
	"testing"

	"cron-weather/internal/sender"
	"cron-weather/internal/subscription"
)

type mockFetcher struct {
	alerts []string
	err    error
}

func (m *mockFetcher) FetchAlerts(ctx context.Context, lat, lon float64) ([]string, error) {
	if ctx.Err() != nil {
		return nil, ctx.Err()
	}
	return m.alerts, m.err
}

type mockSender struct {
	sentMessages [][]string
	err          error
}

func (m *mockSender) Send(ctx context.Context, messages []string) error {
	m.sentMessages = append(m.sentMessages, messages)
	return m.err
}

func TestNewFetchJobForSubscription(t *testing.T) {
	logger := slog.New(slog.DiscardHandler)
	ctx := context.Background()

	baseSub := subscription.Subscription{
		ChatID:   123,
		Interval: 0,
		StartAt:  "",
		Lat:      55.75,
		Lon:      37.62,
	}

	t.Run("success with alerts", func(t *testing.T) {
		fetcher := &mockFetcher{
			alerts: []string{"alert1", "alert2"},
		}
		sdr := &mockSender{}
		factory := func(chatID int64) (sender.Sender, error) {
			if chatID != baseSub.ChatID {
				t.Errorf("unexpected chatID: %d", chatID)
			}
			return sdr, nil
		}

		jobFunc, err := NewFetchJobForSubscription(fetcher, baseSub, factory, logger)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		jobFunc(ctx, logger)

		if len(sdr.sentMessages) != 1 {
			t.Fatalf("expected 1 send, got %d", len(sdr.sentMessages))
		}
		if len(sdr.sentMessages[0]) != 2 {
			t.Errorf("expected 2 messages, got %d", len(sdr.sentMessages[0]))
		}
	})

	t.Run("no alerts", func(t *testing.T) {
		fetcher := &mockFetcher{alerts: []string{}}
		sdr := &mockSender{}
		factory := func(chatID int64) (sender.Sender, error) {
			return sdr, nil
		}

		jobFunc, err := NewFetchJobForSubscription(fetcher, baseSub, factory, logger)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		jobFunc(ctx, logger)

		if len(sdr.sentMessages) != 0 {
			t.Error("expected no send when no alerts")
		}
	})

	t.Run("fetcher error", func(t *testing.T) {
		fetcher := &mockFetcher{err: errors.New("some error")}
		sdr := &mockSender{}
		factory := func(chatID int64) (sender.Sender, error) {
			return sdr, nil
		}

		jobFunc, err := NewFetchJobForSubscription(fetcher, baseSub, factory, logger)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		jobFunc(ctx, logger)

		if len(sdr.sentMessages) != 0 {
			t.Error("expected no send on fetcher error")
		}
	})

	t.Run("context cancelled", func(t *testing.T) {
		fetcher := &mockFetcher{alerts: []string{"alert"}}
		sdr := &mockSender{}
		factory := func(chatID int64) (sender.Sender, error) {
			return sdr, nil
		}

		jobFunc, err := NewFetchJobForSubscription(fetcher, baseSub, factory, logger)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		cancelledCtx, cancel := context.WithCancel(ctx)
		cancel()
		jobFunc(cancelledCtx, logger)

		if len(sdr.sentMessages) != 0 {
			t.Error("expected no send when context cancelled")
		}
	})

	t.Run("sender factory error", func(t *testing.T) {
		fetcher := &mockFetcher{alerts: []string{"alert"}}
		factory := func(chatID int64) (sender.Sender, error) {
			return nil, errors.New("factory error")
		}

		_, err := NewFetchJobForSubscription(fetcher, baseSub, factory, logger)
		if err == nil {
			t.Error("expected error from factory, got nil")
		}
	})

	t.Run("dedup same alerts on next run", func(t *testing.T) {
		fetcher := &mockFetcher{
			alerts: []string{"alert1", "alert2"},
		}
		sdr := &mockSender{}
		factory := func(chatID int64) (sender.Sender, error) {
			return sdr, nil
		}

		jobFunc, err := NewFetchJobForSubscription(fetcher, baseSub, factory, logger)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		// First run sends alerts.
		jobFunc(ctx, logger)
		// Second run gets same alerts, but should not send again.
		jobFunc(ctx, logger)

		if len(sdr.sentMessages) != 1 {
			t.Fatalf("expected 1 send due to dedup, got %d", len(sdr.sentMessages))
		}
	})
}
