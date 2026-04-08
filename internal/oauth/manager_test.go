package oauth_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"golang.org/x/oauth2"

	"github.com/charliewilco/argus/internal/connections"
	"github.com/charliewilco/argus/internal/oauth"
	sqlitestore "github.com/charliewilco/argus/internal/store/sqlite"
)

func TestBeginAuthStoresStateAndProducesPKCEURL(t *testing.T) {
	t.Parallel()

	manager, store := newManager(t)
	ctx := context.Background()

	cfg := &oauth2.Config{
		ClientID:    "client-id",
		RedirectURL: "http://localhost:8080/oauth/github/callback",
		Endpoint: oauth2.Endpoint{
			AuthURL: "https://example.com/oauth/authorize",
		},
		Scopes: []string{"repo"},
	}

	session, err := manager.BeginAuth(ctx, cfg, oauth.AuthorizationRequest{
		TenantID:     "tenant_1",
		ConnectionID: "conn_1",
		Provider:     "github",
	})
	require.NoError(t, err)
	require.NotEmpty(t, session.State)

	storedState, err := store.GetOAuthState(ctx, session.State)
	require.NoError(t, err)
	require.Equal(t, "github", storedState.Provider)

	authURL, err := url.Parse(session.AuthURL)
	require.NoError(t, err)
	query := authURL.Query()
	require.Equal(t, session.State, query.Get("state"))
	require.Equal(t, "S256", query.Get("code_challenge_method"))
	require.NotEmpty(t, query.Get("code_challenge"))
}

func TestExchangePersistsEncryptedToken(t *testing.T) {
	t.Parallel()

	manager, store := newManager(t)
	ctx := context.Background()

	var capturedVerifier string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, "/token", r.URL.Path)
		require.NoError(t, r.ParseForm())

		capturedVerifier = r.Form.Get("code_verifier")
		require.Equal(t, "authorization_code", r.Form.Get("grant_type"))
		require.Equal(t, "test-code", r.Form.Get("code"))

		writeTokenResponse(t, w, map[string]any{
			"access_token":  "access-token",
			"refresh_token": "refresh-token",
			"token_type":    "Bearer",
			"expires_in":    3600,
		})
	}))
	defer server.Close()

	cfg := &oauth2.Config{
		ClientID:     "client-id",
		ClientSecret: "client-secret",
		RedirectURL:  "http://localhost:8080/oauth/github/callback",
		Endpoint: oauth2.Endpoint{
			AuthURL:  "https://example.com/oauth/authorize",
			TokenURL: server.URL + "/token",
		},
	}

	session, err := manager.BeginAuth(ctx, cfg, oauth.AuthorizationRequest{
		TenantID:     "tenant_1",
		ConnectionID: "conn_1",
		Provider:     "github",
	})
	require.NoError(t, err)

	result, err := manager.Exchange(ctx, cfg, "test-code", session.State)
	require.NoError(t, err)
	require.Equal(t, "tenant_1", result.TenantID)
	require.Equal(t, "conn_1", result.ConnectionID)
	require.Equal(t, "access-token", result.Token.AccessToken)
	require.NotEmpty(t, capturedVerifier)

	secret, err := store.GetConnectionSecret(ctx, "tenant_1", "conn_1")
	require.NoError(t, err)
	require.NotContains(t, string(secret.Ciphertext), "access-token")

	_, err = store.GetOAuthState(ctx, session.State)
	require.Error(t, err)

	token, err := manager.GetToken(ctx, "tenant_1", "conn_1", nil)
	require.NoError(t, err)
	require.Equal(t, "access-token", token.AccessToken)
}

func TestGetTokenRefreshesNearExpiry(t *testing.T) {
	t.Parallel()

	manager, store := newManager(t)
	ctx := context.Background()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, "/token", r.URL.Path)
		require.NoError(t, r.ParseForm())
		require.Equal(t, "refresh_token", r.Form.Get("grant_type"))
		require.Equal(t, "refresh-token", r.Form.Get("refresh_token"))

		writeTokenResponse(t, w, map[string]any{
			"access_token":  "refreshed-access-token",
			"refresh_token": "refresh-token",
			"token_type":    "Bearer",
			"expires_in":    3600,
		})
	}))
	defer server.Close()

	cfg := &oauth2.Config{
		ClientID:     "client-id",
		ClientSecret: "client-secret",
		Endpoint: oauth2.Endpoint{
			TokenURL: server.URL + "/token",
		},
	}

	expiringToken := &oauth2.Token{
		AccessToken:  "old-access-token",
		RefreshToken: "refresh-token",
		TokenType:    "Bearer",
		Expiry:       time.Now().UTC().Add(2 * time.Minute),
	}
	require.NoError(t, store.SaveConnection(ctx, &connections.Connection{
		TenantID:     "tenant_1",
		ConnectionID: "conn_1",
		Provider:     "github",
		Config:       map[string]any{},
		CreatedAt:    time.Now().UTC(),
	}))
	require.NoError(t, manager.SaveToken(ctx, "tenant_1", "conn_1", expiringToken))

	token, err := manager.GetToken(ctx, "tenant_1", "conn_1", cfg)
	require.NoError(t, err)
	require.Equal(t, "refreshed-access-token", token.AccessToken)

	secret, err := store.GetConnectionSecret(ctx, "tenant_1", "conn_1")
	require.NoError(t, err)
	require.NotContains(t, string(secret.Ciphertext), "old-access-token")
	require.NotContains(t, string(secret.Ciphertext), "refreshed-access-token")
}

func TestGetTokenReturnsErrorWhenRefreshNeedsConfig(t *testing.T) {
	t.Parallel()

	manager, store := newManager(t)
	ctx := context.Background()

	require.NoError(t, store.SaveConnection(ctx, &connections.Connection{
		TenantID:     "tenant_1",
		ConnectionID: "conn_1",
		Provider:     "github",
		Config:       map[string]any{},
		CreatedAt:    time.Now().UTC(),
	}))
	require.NoError(t, manager.SaveToken(ctx, "tenant_1", "conn_1", &oauth2.Token{
		AccessToken:  "old-access-token",
		RefreshToken: "refresh-token",
		TokenType:    "Bearer",
		Expiry:       time.Now().UTC().Add(2 * time.Minute),
	}))

	_, err := manager.GetToken(ctx, "tenant_1", "conn_1", nil)
	require.EqualError(t, err, "oauth.GetToken: config is required to refresh token")
}

func newManager(t *testing.T) (*oauth.Manager, *sqlitestore.Store) {
	t.Helper()

	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "argus-oauth.db")
	store, err := sqlitestore.Open(ctx, "sqlite:"+path)
	require.NoError(t, err)
	t.Cleanup(func() {
		require.NoError(t, store.Close())
	})

	manager, err := oauth.NewManager(store, oauth.Options{
		SecretKey: "test-secret-key",
	})
	require.NoError(t, err)

	return manager, store
}

func writeTokenResponse(t *testing.T, w http.ResponseWriter, payload map[string]any) {
	t.Helper()

	w.Header().Set("Content-Type", "application/json")
	require.NoError(t, json.NewEncoder(w).Encode(payload))
}
