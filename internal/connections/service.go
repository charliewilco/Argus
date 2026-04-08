package connections

import (
	"context"
	"fmt"
	"time"

	"github.com/charliewilco/argus/providers"
	"golang.org/x/oauth2"
)

type connectionStore interface {
	SaveConnection(ctx context.Context, connection *Connection) error
	GetConnection(ctx context.Context, tenantID, connectionID string) (*Connection, error)
	GetConnectionByID(ctx context.Context, connectionID string) (*Connection, error)
	ListConnections(ctx context.Context, tenantID, providerID string) ([]*Connection, error)
	DeleteConnection(ctx context.Context, tenantID, connectionID string) error
}

type tokenReader interface {
	GetToken(ctx context.Context, tenantID, connectionID string, cfg *oauth2.Config) (*oauth2.Token, error)
}

type providerRegistry interface {
	Get(id string) (providers.Provider, error)
}

type Service struct {
	store     connectionStore
	oauth     tokenReader
	providers providerRegistry
	now       func() time.Time
}

func NewService(store connectionStore, tokenReader tokenReader, providers providerRegistry, now func() time.Time) (*Service, error) {
	if store == nil {
		return nil, fmt.Errorf("connections.NewService: store is required")
	}
	if tokenReader == nil {
		return nil, fmt.Errorf("connections.NewService: oauth manager is required")
	}
	if providers == nil {
		return nil, fmt.Errorf("connections.NewService: provider registry is required")
	}
	if now == nil {
		now = time.Now
	}

	return &Service{
		store:     store,
		oauth:     tokenReader,
		providers: providers,
		now:       now,
	}, nil
}

func (s *Service) CreateConnection(ctx context.Context, connection Connection) error {
	if connection.TenantID == "" {
		return fmt.Errorf("connections.CreateConnection: tenant ID is required")
	}
	if connection.ConnectionID == "" {
		return fmt.Errorf("connections.CreateConnection: connection ID is required")
	}
	if connection.Provider == "" {
		return fmt.Errorf("connections.CreateConnection: provider is required")
	}

	if connection.CreatedAt.IsZero() {
		connection.CreatedAt = s.now().UTC()
	} else {
		connection.CreatedAt = connection.CreatedAt.UTC()
	}

	if err := s.store.SaveConnection(ctx, &connection); err != nil {
		return fmt.Errorf("connections.CreateConnection: %w", err)
	}

	return nil
}

func (s *Service) GetConnection(ctx context.Context, tenantID, id string) (Connection, error) {
	connection, err := s.store.GetConnection(ctx, tenantID, id)
	if err != nil {
		return Connection{}, fmt.Errorf("connections.GetConnection: %w", err)
	}

	return *connection, nil
}

func (s *Service) ListConnections(ctx context.Context, tenantID, providerID string) ([]Connection, error) {
	connectionsList, err := s.store.ListConnections(ctx, tenantID, providerID)
	if err != nil {
		return nil, fmt.Errorf("connections.ListConnections: %w", err)
	}

	result := make([]Connection, 0, len(connectionsList))
	for _, connection := range connectionsList {
		if connection == nil {
			continue
		}
		result = append(result, *connection)
	}

	return result, nil
}

func (s *Service) DeleteConnection(ctx context.Context, tenantID, id string) error {
	connection, err := s.store.GetConnection(ctx, tenantID, id)
	if err != nil {
		return fmt.Errorf("connections.DeleteConnection: load connection: %w", err)
	}

	if err := s.store.DeleteConnection(ctx, connection.TenantID, connection.ConnectionID); err != nil {
		return fmt.Errorf("connections.DeleteConnection: %w", err)
	}

	return nil
}

func (s *Service) GetDecryptedToken(ctx context.Context, tenantID, id string) (*oauth2.Token, error) {
	connection, err := s.store.GetConnection(ctx, tenantID, id)
	if err != nil {
		return nil, fmt.Errorf("connections.GetDecryptedToken: load connection: %w", err)
	}

	provider, err := s.providers.Get(connection.Provider)
	if err != nil {
		return nil, fmt.Errorf("connections.GetDecryptedToken: resolve provider: %w", err)
	}

	cfg := provider.OAuthConfig()
	token, err := s.oauth.GetToken(ctx, connection.TenantID, connection.ConnectionID, cfg)
	if err != nil {
		return nil, fmt.Errorf("connections.GetDecryptedToken: %w", err)
	}

	return token, nil
}
