// Package sender defines message delivery interfaces and implementations
package sender

import "context"

// Sender delivers a batch of alert messages to some destination.
// Implementations should treat an empty batch as a no-op.
type Sender interface {
	Send(ctx context.Context, messages []string) error
}
