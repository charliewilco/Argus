package actions

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/charliewilco/argus/internal/connections"
	"github.com/charliewilco/argus/internal/envelope"
	"github.com/charliewilco/argus/providers"
	"golang.org/x/oauth2"
)

var ErrTokenExpired = errors.New("actions: token expired")

type connectionReader interface {
	GetConnection(ctx context.Context, id string) (connections.Connection, error)
	GetDecryptedToken(ctx context.Context, id string) (*oauth2.Token, error)
}

type providerRegistry interface {
	Get(id string) (providers.Provider, error)
}

type ExpiredTokenError struct {
	ConnectionID string
	ExpiredAt    time.Time
}

func (e *ExpiredTokenError) Error() string {
	if e == nil {
		return ErrTokenExpired.Error()
	}

	if e.ExpiredAt.IsZero() {
		return fmt.Sprintf("actions.Dispatch: connection %q: %v", e.ConnectionID, ErrTokenExpired)
	}

	return fmt.Sprintf("actions.Dispatch: connection %q expired at %s", e.ConnectionID, e.ExpiredAt.UTC().Format(time.RFC3339))
}

func (e *ExpiredTokenError) Unwrap() error {
	return ErrTokenExpired
}

type Dispatcher struct {
	connections connectionReader
	providers   providerRegistry
	now         func() time.Time
}

func NewDispatcher(connections connectionReader, providers providerRegistry, now func() time.Time) (*Dispatcher, error) {
	if connections == nil {
		return nil, fmt.Errorf("actions.NewDispatcher: connections service is required")
	}
	if providers == nil {
		return nil, fmt.Errorf("actions.NewDispatcher: provider registry is required")
	}
	if now == nil {
		now = time.Now
	}

	return &Dispatcher{
		connections: connections,
		providers:   providers,
		now:         now,
	}, nil
}

func (d *Dispatcher) Dispatch(ctx context.Context, stepConfig map[string]any, connectionID string, event *envelope.Event) (providers.ActionResult, error) {
	if connectionID == "" {
		return providers.ActionResult{}, fmt.Errorf("actions.Dispatch: connection ID is required")
	}

	connection, err := d.connections.GetConnection(ctx, connectionID)
	if err != nil {
		return providers.ActionResult{}, fmt.Errorf("actions.Dispatch: load connection: %w", err)
	}

	token, err := d.connections.GetDecryptedToken(ctx, connectionID)
	if err != nil {
		return providers.ActionResult{}, fmt.Errorf("actions.Dispatch: decrypt token: %w", err)
	}
	if token != nil && !token.Expiry.IsZero() && !token.Expiry.After(d.now().UTC()) {
		return providers.ActionResult{}, &ExpiredTokenError{
			ConnectionID: connectionID,
			ExpiredAt:    token.Expiry.UTC(),
		}
	}

	provider, err := d.providers.Get(connection.Provider)
	if err != nil {
		return providers.ActionResult{}, fmt.Errorf("actions.Dispatch: resolve provider: %w", err)
	}

	actionName, ok := stepConfig["action"].(string)
	if !ok || actionName == "" {
		return providers.ActionResult{}, fmt.Errorf("actions.Dispatch: action is required")
	}

	result, err := provider.ExecuteAction(ctx, token, providers.ActionRequest{
		Action:       actionName,
		ConnectionID: connectionID,
		Config:       stepConfig,
		Event:        event,
	})
	if err != nil {
		return providers.ActionResult{}, fmt.Errorf("actions.Dispatch: %w", err)
	}

	return result, nil
}
