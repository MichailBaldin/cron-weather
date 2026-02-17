package sender

import (
	"context"
	"errors"
	"testing"
)

type mockSender struct {
	sentMessages [][]string
	err          error
}

func (m *mockSender) Send(ctx context.Context, messages []string) error {
	// Имитируем реальное поведение: не отправляем пустые сообщения
	if len(messages) == 0 {
		return nil
	}
	m.sentMessages = append(m.sentMessages, messages)
	return m.err
}

func TestSenderInterface(t *testing.T) {
	ctx := context.Background()

	t.Run("successful send", func(t *testing.T) {
		sender := &mockSender{}
		messages := []string{"alert1", "alert2"}

		err := sender.Send(ctx, messages)
		if err != nil {
			t.Errorf("unexpected error: %v", err)
		}
		if len(sender.sentMessages) != 1 {
			t.Fatalf("expected 1 send, got %d", len(sender.sentMessages))
		}
		if len(sender.sentMessages[0]) != 2 {
			t.Errorf("expected 2 messages, got %d", len(sender.sentMessages[0]))
		}
	})

	t.Run("send with error", func(t *testing.T) {
		expectedErr := errors.New("network error")
		sender := &mockSender{err: expectedErr}
		messages := []string{"alert"}

		err := sender.Send(ctx, messages)
		if err == nil {
			t.Error("expected error, got nil")
		}
		if err != expectedErr {
			t.Errorf("expected %v, got %v", expectedErr, err)
		}
		// Даже при ошибке сообщение должно быть записано (если mock так устроен)
		if len(sender.sentMessages) != 1 {
			t.Errorf("expected send attempt, got %d", len(sender.sentMessages))
		}
	})

	t.Run("send empty messages", func(t *testing.T) {
		sender := &mockSender{}
		err := sender.Send(ctx, []string{})
		if err != nil {
			t.Errorf("unexpected error: %v", err)
		}
		if len(sender.sentMessages) != 0 {
			t.Error("expected no send for empty messages")
		}
	})
}
