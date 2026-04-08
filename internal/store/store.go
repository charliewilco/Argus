package store

import (
	"context"
	"errors"
	"time"

	"github.com/charliewilco/argus/internal/connections"
	"github.com/charliewilco/argus/internal/envelope"
	"github.com/charliewilco/argus/internal/pipeline"
)

var ErrNotFound = errors.New("store: not found")

type EventFilter struct {
	ConnectionID string
	Provider     string
	TriggerKey   string
	Since        *time.Time
	Until        *time.Time
	Limit        int
}

type ConnectionSecret struct {
	TenantID     string
	ConnectionID string
	Ciphertext   []byte
	UpdatedAt    time.Time
}

type OAuthState struct {
	ID           string
	TenantID     string
	ConnectionID string
	Provider     string
	CodeVerifier string
	CreatedAt    time.Time
	ExpiresAt    time.Time
}

type Store interface {
	SaveEvent(ctx context.Context, event *envelope.Event) error
	GetEvent(ctx context.Context, id string) (*envelope.Event, error)
	ListEvents(ctx context.Context, tenantID string, filter EventFilter) ([]*envelope.Event, error)

	SaveConnection(ctx context.Context, connection *connections.Connection) error
	GetConnection(ctx context.Context, tenantID, connectionID string) (*connections.Connection, error)

	SavePipeline(ctx context.Context, pipeline *pipeline.Pipeline) error
	GetPipeline(ctx context.Context, id string) (*pipeline.Pipeline, error)
	ListPipelines(ctx context.Context, tenantID string) ([]*pipeline.Pipeline, error)

	SaveConnectionSecret(ctx context.Context, secret ConnectionSecret) error
	GetConnectionSecret(ctx context.Context, tenantID, connectionID string) (*ConnectionSecret, error)

	SaveOAuthState(ctx context.Context, state OAuthState) error
	GetOAuthState(ctx context.Context, id string) (*OAuthState, error)
	DeleteOAuthState(ctx context.Context, id string) error

	Close() error
}
