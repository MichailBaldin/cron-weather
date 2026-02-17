package weather

import (
	"context"
	"log/slog"
	"testing"
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

func TestNewFetchJob(t *testing.T) {
	logger := slog.New(slog.DiscardHandler)

	t.Run("success with alerts", func(t *testing.T) {
		fetcher := &mockFetcher{
			alerts: []string{"alert1", "alert2"},
		}
		sdr := &mockSender{}
		job := NewFetchJob(fetcher, sdr, 0, 0)

		job(context.Background(), logger)

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
		job := NewFetchJob(fetcher, sdr, 0, 0)

		job(context.Background(), logger)

		if len(sdr.sentMessages) != 0 {
			t.Error("expected no send when no alerts")
		}
	})

	t.Run("fetcher error", func(t *testing.T) {
		fetcher := &mockFetcher{err: context.DeadlineExceeded}
		sdr := &mockSender{}
		job := NewFetchJob(fetcher, sdr, 0, 0)

		job(context.Background(), logger)

		if len(sdr.sentMessages) != 0 {
			t.Error("expected no send on fetcher error")
		}
	})

	t.Run("context cancelled", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		fetcher := &mockFetcher{alerts: []string{"alert"}}
		sdr := &mockSender{}
		job := NewFetchJob(fetcher, sdr, 0, 0)

		job(ctx, logger)

		if len(sdr.sentMessages) != 0 {
			t.Error("expected no send when context cancelled")
		}
	})
}
