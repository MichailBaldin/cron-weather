package domain

// Subscription represents an owner of schedules.
// In current app it's 1:1 with a Telegram chat (via owner_ref and endpoint link).
type Subscription struct {
	ID       string
	OwnerRef string
	Lat      float64
	Lon      float64
	IsActive bool
}
