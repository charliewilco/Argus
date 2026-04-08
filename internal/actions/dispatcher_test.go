package actions_test

import (
	"context"
	"errors"
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/charliewilco/argus/internal/actions"
	"github.com/charliewilco/argus/internal/connections"
	"github.com/charliewilco/argus/internal/envelope"
	"github.com/charliewilco/argus/internal/oauth"
	"github.com/charliewilco/argus/providers"
)

type connectionStub struct {
	connection connections.Connection
	token      oauth.Token
}

func (s *connectionStub) GetConnection(_ context.Context, _ string) (connections.Connection, error) {
	return s.connection, nil
}

func (s *connectionStub) GetDecryptedToken(_ context.Context, _ string) (*oauth.Token, error) {
	return &s.token, nil
}

type providerStub struct{}

func (providerStub) ID() string { return "github" }

func (providerStub) OAuthConfig() oauth.Config { return oauth.Config{} }

func (providerStub) ParseWebhookEvent(_ *http.Request) (envelope.Event, error) {
	return envelope.Event{}, errors.New("not implemented")
}

func (providerStub) ExecuteAction(_ context.Context, _ *oauth.Token, _ providers.ActionRequest) (providers.ActionResult, error) {
	return providers.ActionResult{}, nil
}

func TestDispatcherReturnsTypedErrorForExpiredToken(t *testing.T) {
	t.Parallel()

	registry, err := providers.NewRegistry(providerStub{})
	require.NoError(t, err)

	dispatcher, err := actions.NewDispatcher(&connectionStub{
		connection: connections.Connection{
			ConnectionID: "conn_1",
			Provider:     "github",
		},
		token: oauth.Token{
			AccessToken: "expired",
			Expiry:      time.Date(2026, 4, 7, 11, 59, 0, 0, time.UTC),
		},
	}, registry, func() time.Time {
		return time.Date(2026, 4, 7, 12, 0, 0, 0, time.UTC)
	})
	require.NoError(t, err)

	_, err = dispatcher.Dispatch(context.Background(), map[string]any{
		"action": "github.noop",
	}, "conn_1", nil)
	require.Error(t, err)
	require.ErrorIs(t, err, actions.ErrTokenExpired)

	var expiredErr *actions.ExpiredTokenError
	require.ErrorAs(t, err, &expiredErr)
	require.Equal(t, "conn_1", expiredErr.ConnectionID)
}
