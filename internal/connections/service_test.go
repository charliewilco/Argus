package connections_test

import (
	"context"
	"errors"
	"net/http"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/charliewilco/argus/internal/connections"
	"github.com/charliewilco/argus/internal/oauth"
	"github.com/charliewilco/argus/providers"
	"golang.org/x/oauth2"
)

type connectionStoreStub struct {
	connectionByID     *connections.Connection
	connectionByTenant *connections.Connection
	getTenantID        string
	getConnectionID    string
	listTenantID       string
	listProviderID     string
	deleteTenantID     string
	deleteConnectionID string
	listResult         []*connections.Connection
}

func (s *connectionStoreStub) SaveConnection(_ context.Context, _ *connections.Connection) error {
	return errors.New("unexpected call")
}

func (s *connectionStoreStub) GetConnection(_ context.Context, tenantID, connectionID string) (*connections.Connection, error) {
	s.getTenantID = tenantID
	s.getConnectionID = connectionID
	if s.connectionByTenant == nil {
		return nil, errors.New("not found")
	}
	return s.connectionByTenant, nil
}

func (s *connectionStoreStub) GetConnectionByID(_ context.Context, _ string) (*connections.Connection, error) {
	if s.connectionByID == nil {
		return nil, errors.New("not found")
	}
	return s.connectionByID, nil
}

func (s *connectionStoreStub) ListConnections(_ context.Context, tenantID, providerID string) ([]*connections.Connection, error) {
	s.listTenantID = tenantID
	s.listProviderID = providerID
	return s.listResult, nil
}

func (s *connectionStoreStub) DeleteConnection(_ context.Context, tenantID, connectionID string) error {
	s.deleteTenantID = tenantID
	s.deleteConnectionID = connectionID
	return nil
}

type tokenReaderStub struct {
	cfg *oauth2.Config
}

func (s *tokenReaderStub) GetToken(_ context.Context, _, _ string, cfg *oauth2.Config) (*oauth2.Token, error) {
	s.cfg = cfg
	return &oauth2.Token{AccessToken: "token"}, nil
}

type providerRegistryStub struct {
	provider providers.Provider
}

func (s *providerRegistryStub) Get(_ string) (providers.Provider, error) {
	return s.provider, nil
}

type providerStub struct {
	config *oauth.Config
}

func (s providerStub) ID() string { return "github" }

func (s providerStub) Metadata() providers.Metadata { return providers.Metadata{ID: "github"} }

func (s providerStub) OAuthConfig() *oauth.Config { return s.config }

func (s providerStub) ParseWebhookEvent(_ http.Header, _ []byte) (*providers.WebhookEvent, error) {
	return nil, errors.New("unexpected call")
}

func (s providerStub) ExecuteAction(_ context.Context, _ *oauth2.Token, _ providers.ActionRequest) (providers.ActionResult, error) {
	return providers.ActionResult{}, errors.New("unexpected call")
}

func TestServiceListConnectionsScopesByTenant(t *testing.T) {
	t.Parallel()

	store := &connectionStoreStub{
		listResult: []*connections.Connection{{TenantID: "tenant_1", ConnectionID: "conn_1"}},
	}
	service, err := connections.NewService(store, &tokenReaderStub{}, &providerRegistryStub{provider: providerStub{}}, nil)
	require.NoError(t, err)

	values, err := service.ListConnections(context.Background(), "tenant_1", "github")
	require.NoError(t, err)
	require.Len(t, values, 1)
	require.Equal(t, "tenant_1", store.listTenantID)
	require.Equal(t, "github", store.listProviderID)
}

func TestServiceDeleteConnectionScopesByTenant(t *testing.T) {
	t.Parallel()

	store := &connectionStoreStub{
		connectionByTenant: &connections.Connection{
			TenantID:     "tenant_1",
			ConnectionID: "conn_1",
		},
	}
	service, err := connections.NewService(store, &tokenReaderStub{}, &providerRegistryStub{provider: providerStub{}}, nil)
	require.NoError(t, err)

	err = service.DeleteConnection(context.Background(), "tenant_1", "conn_1")
	require.NoError(t, err)
	require.Equal(t, "tenant_1", store.getTenantID)
	require.Equal(t, "conn_1", store.getConnectionID)
	require.Equal(t, "tenant_1", store.deleteTenantID)
	require.Equal(t, "conn_1", store.deleteConnectionID)
}

func TestServiceGetDecryptedTokenUsesProviderOAuthConfig(t *testing.T) {
	t.Parallel()

	store := &connectionStoreStub{
		connectionByTenant: &connections.Connection{
			TenantID:     "tenant_1",
			ConnectionID: "conn_1",
			Provider:     "github",
		},
	}
	tokenReader := &tokenReaderStub{}
	service, err := connections.NewService(store, tokenReader, &providerRegistryStub{
		provider: providerStub{
			config: &oauth.Config{
				ClientID:    "client-id",
				RedirectURL: "http://localhost/oauth/github/callback",
			},
		},
	}, nil)
	require.NoError(t, err)

	token, err := service.GetDecryptedToken(context.Background(), "tenant_1", "conn_1")
	require.NoError(t, err)
	require.Equal(t, "token", token.AccessToken)
	require.NotNil(t, tokenReader.cfg)
	require.Equal(t, "client-id", tokenReader.cfg.ClientID)
	require.Equal(t, "http://localhost/oauth/github/callback", tokenReader.cfg.RedirectURL)
}
