package sender

import "context"

type Sender interface {
	Send(ctx context.Context, messages []string) error
}
