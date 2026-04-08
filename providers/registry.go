package providers

import (
	"fmt"
	"sort"
)

type Registry struct {
	providers map[string]Provider
}

func NewRegistry(items ...Provider) (*Registry, error) {
	registry := &Registry{
		providers: make(map[string]Provider, len(items)),
	}

	for _, item := range items {
		if err := registry.Register(item); err != nil {
			return nil, fmt.Errorf("providers.NewRegistry: %w", err)
		}
	}

	return registry, nil
}

func (r *Registry) Register(provider Provider) error {
	if provider == nil {
		return fmt.Errorf("providers.Register: provider is required")
	}

	id := provider.ID()
	if id == "" {
		return fmt.Errorf("providers.Register: provider ID is required")
	}
	if _, exists := r.providers[id]; exists {
		return fmt.Errorf("providers.Register: provider %q already registered", id)
	}

	r.providers[id] = provider
	return nil
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
	if r == nil || len(r.providers) == 0 {
		return nil
	}

	ids := make([]string, 0, len(r.providers))
	for id := range r.providers {
		ids = append(ids, id)
	}
	sort.Strings(ids)

	list := make([]Provider, 0, len(ids))
	for _, id := range ids {
		list = append(list, r.providers[id])
	}

	return list
}

func (r *Registry) Metadata() []Metadata {
	items := r.List()
	if len(items) == 0 {
		return nil
	}

	metadata := make([]Metadata, 0, len(items))
	for _, item := range items {
		metadata = append(metadata, item.Metadata())
	}

	return metadata
}
