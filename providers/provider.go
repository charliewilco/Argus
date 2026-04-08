package providers

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"slices"

	"github.com/charliewilco/argus/internal/envelope"
	"golang.org/x/oauth2"
)

var ErrProviderNotFound = errors.New("providers: provider not found")

type ActionRequest struct {
	Action       string
	ConnectionID string
	Config       map[string]any
	Event        *envelope.Event
}

type ActionResult struct {
	Provider string         `json:"provider"`
	Action   string         `json:"action"`
	Status   string         `json:"status"`
	Output   map[string]any `json:"output,omitempty"`
}

type Provider interface {
	ID() string
	OAuthConfig() oauth2.Config
	ParseWebhookEvent(r *http.Request) (envelope.Event, error)
	ExecuteAction(ctx context.Context, token *oauth2.Token, request ActionRequest) (ActionResult, error)
}

type Registry struct {
	providers map[string]Provider
}

func NewRegistry(values ...Provider) (*Registry, error) {
	registry := &Registry{
		providers: make(map[string]Provider, len(values)),
	}

	for _, value := range values {
		if value == nil {
			return nil, fmt.Errorf("providers.NewRegistry: provider is required")
		}

		id := value.ID()
		if id == "" {
			return nil, fmt.Errorf("providers.NewRegistry: provider ID is required")
		}
		if _, ok := registry.providers[id]; ok {
			return nil, fmt.Errorf("providers.NewRegistry: duplicate provider %q", id)
		}

		registry.providers[id] = value
	}

	return registry, nil
}

func (r *Registry) Get(id string) (Provider, error) {
	if r == nil {
		return nil, fmt.Errorf("providers.Get: registry is required")
	}

	provider, ok := r.providers[id]
	if !ok {
		return nil, fmt.Errorf("providers.Get: provider %q: %w", id, ErrProviderNotFound)
	}

	return provider, nil
}

func (r *Registry) List() []Provider {
	if r == nil {
		return nil
	}

	keys := make([]string, 0, len(r.providers))
	for key := range r.providers {
		keys = append(keys, key)
	}
	slices.Sort(keys)

	providers := make([]Provider, 0, len(keys))
	for _, key := range keys {
		providers = append(providers, r.providers[key])
	}

	return providers
}
