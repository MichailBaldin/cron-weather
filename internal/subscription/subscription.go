package subscription

import "time"

type Subscription struct {
	ChatID   int64         `db:"chat_id"`
	Interval time.Duration `db:"interval"`
	StartAt  string        `db:"start_at"`
	Lat      float64       `db:"lat"`
	Lon      float64       `db:"lon"`
}

func (Subscription) TableName() string {
	return "subscriptions"
}
