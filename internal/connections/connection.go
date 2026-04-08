package connections

import (
	"time"

	"golang.org/x/oauth2"
)

type Connection struct {
	TenantID     string
	ConnectionID string
	Provider     string
	Token        *oauth2.Token
	Config       map[string]any
	CreatedAt    time.Time
}
