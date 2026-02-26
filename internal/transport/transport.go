// Package transport defines message delivery interfaces and payload types.
package transport

import "context"

// CronJob describes a scheduled job command received from a transport (Telegram).
type CronJob struct {
	ChatID  int64
	Command string
	Args    string
}

// Message is an outgoing message to be delivered to a chat.
type Message struct {
	ChatID int64
	Text   string
}

// Consumer provides incoming updates (e.g. Telegram updates) to the application.
type Consumer interface {
	Get(ctx context.Context) (<-chan CronJob, error)
}

// Producer delivers outgoing messages to a target transport.
type Producer interface {
	Send(ctx context.Context, msg Message) error
}
