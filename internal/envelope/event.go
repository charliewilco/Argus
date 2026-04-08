package envelope

import "time"

type Event struct {
	ID           string
	TenantID     string
	ConnectionID string
	Provider     string
	TriggerKey   string
	Raw          []byte
	Normalized   map[string]any
	ReceivedAt   time.Time
}
