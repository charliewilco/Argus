package oauth

import (
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"time"

	"github.com/charliewilco/argus/internal/connections"
	"github.com/charliewilco/argus/internal/store"
	"golang.org/x/oauth2"
)

const (
	defaultStateTTL      = 10 * time.Minute
	defaultRefreshLeeway = 5 * time.Minute
)

var ErrExpiredState = errors.New("oauth: state expired")

type Options struct {
	SecretKey     string
	StateTTL      time.Duration
	RefreshLeeway time.Duration
	Now           func() time.Time
}

type Manager struct {
	store         store.Store
	cipher        cipher.AEAD
	now           func() time.Time
	stateTTL      time.Duration
	refreshLeeway time.Duration
}

type AuthorizationRequest struct {
	TenantID     string
	ConnectionID string
	Provider     string
}

type AuthorizationSession struct {
	AuthURL   string
	State     string
	ExpiresAt time.Time
}

type ExchangeResult struct {
	TenantID     string
	ConnectionID string
	Provider     string
	Token        *oauth2.Token
}

func NewManager(store store.Store, opts Options) (*Manager, error) {
	if store == nil {
		return nil, errors.New("oauth.NewManager: store is required")
	}
	if opts.SecretKey == "" {
		return nil, errors.New("oauth.NewManager: secret key is required")
	}

	block, err := aes.NewCipher(deriveKey(opts.SecretKey))
	if err != nil {
		return nil, fmt.Errorf("oauth.NewManager: create cipher: %w", err)
	}

	aead, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("oauth.NewManager: create AEAD: %w", err)
	}

	now := opts.Now
	if now == nil {
		now = time.Now
	}

	stateTTL := opts.StateTTL
	if stateTTL <= 0 {
		stateTTL = defaultStateTTL
	}

	refreshLeeway := opts.RefreshLeeway
	if refreshLeeway <= 0 {
		refreshLeeway = defaultRefreshLeeway
	}

	return &Manager{
		store:         store,
		cipher:        aead,
		now:           now,
		stateTTL:      stateTTL,
		refreshLeeway: refreshLeeway,
	}, nil
}

func (m *Manager) BeginAuth(ctx context.Context, cfg *oauth2.Config, request AuthorizationRequest) (*AuthorizationSession, error) {
	if cfg == nil {
		return nil, errors.New("oauth.BeginAuth: config is required")
	}
	if request.TenantID == "" || request.ConnectionID == "" || request.Provider == "" {
		return nil, errors.New("oauth.BeginAuth: tenant ID, connection ID, and provider are required")
	}

	stateID, err := randomString(32)
	if err != nil {
		return nil, fmt.Errorf("oauth.BeginAuth: generate state: %w", err)
	}
	codeVerifier, err := randomString(32)
	if err != nil {
		return nil, fmt.Errorf("oauth.BeginAuth: generate code verifier: %w", err)
	}

	now := m.now().UTC()
	expiresAt := now.Add(m.stateTTL)
	if err := m.store.SaveOAuthState(ctx, store.OAuthState{
		ID:           stateID,
		TenantID:     request.TenantID,
		ConnectionID: request.ConnectionID,
		Provider:     request.Provider,
		CodeVerifier: codeVerifier,
		CreatedAt:    now,
		ExpiresAt:    expiresAt,
	}); err != nil {
		return nil, fmt.Errorf("oauth.BeginAuth: save state: %w", err)
	}

	authURL := cfg.AuthCodeURL(
		stateID,
		oauth2.AccessTypeOffline,
		oauth2.S256ChallengeOption(codeVerifier),
	)

	return &AuthorizationSession{
		AuthURL:   authURL,
		State:     stateID,
		ExpiresAt: expiresAt,
	}, nil
}

func (m *Manager) Exchange(ctx context.Context, cfg *oauth2.Config, code, stateID string) (*ExchangeResult, error) {
	if cfg == nil {
		return nil, errors.New("oauth.Exchange: config is required")
	}
	if code == "" || stateID == "" {
		return nil, errors.New("oauth.Exchange: code and state are required")
	}

	stateRecord, err := m.store.GetOAuthState(ctx, stateID)
	if err != nil {
		return nil, fmt.Errorf("oauth.Exchange: load state: %w", err)
	}

	if m.now().UTC().After(stateRecord.ExpiresAt) {
		_ = m.store.DeleteOAuthState(ctx, stateRecord.ID)
		return nil, ErrExpiredState
	}

	token, err := cfg.Exchange(ctx, code, oauth2.VerifierOption(stateRecord.CodeVerifier))
	if err != nil {
		return nil, fmt.Errorf("oauth.Exchange: exchange code: %w", err)
	}

	if err := m.ensureConnection(ctx, stateRecord); err != nil {
		return nil, fmt.Errorf("oauth.Exchange: ensure connection: %w", err)
	}

	if err := m.SaveToken(ctx, stateRecord.TenantID, stateRecord.ConnectionID, token); err != nil {
		return nil, fmt.Errorf("oauth.Exchange: persist token: %w", err)
	}

	if err := m.store.DeleteOAuthState(ctx, stateRecord.ID); err != nil {
		return nil, fmt.Errorf("oauth.Exchange: delete state: %w", err)
	}

	return &ExchangeResult{
		TenantID:     stateRecord.TenantID,
		ConnectionID: stateRecord.ConnectionID,
		Provider:     stateRecord.Provider,
		Token:        token,
	}, nil
}

func (m *Manager) SaveToken(ctx context.Context, tenantID, connectionID string, token *oauth2.Token) error {
	if tenantID == "" || connectionID == "" {
		return errors.New("oauth.SaveToken: tenant ID and connection ID are required")
	}
	if token == nil {
		return errors.New("oauth.SaveToken: token is required")
	}

	tokenJSON, err := json.Marshal(token)
	if err != nil {
		return fmt.Errorf("oauth.SaveToken: marshal token: %w", err)
	}

	ciphertext, err := m.encrypt(tokenJSON)
	if err != nil {
		return fmt.Errorf("oauth.SaveToken: encrypt token: %w", err)
	}

	if err := m.store.SaveConnectionSecret(ctx, store.ConnectionSecret{
		TenantID:     tenantID,
		ConnectionID: connectionID,
		Ciphertext:   ciphertext,
		UpdatedAt:    m.now().UTC(),
	}); err != nil {
		return fmt.Errorf("oauth.SaveToken: save secret: %w", err)
	}

	return nil
}

func (m *Manager) GetToken(ctx context.Context, tenantID, connectionID string, cfg *oauth2.Config) (*oauth2.Token, error) {
	secret, err := m.store.GetConnectionSecret(ctx, tenantID, connectionID)
	if err != nil {
		return nil, fmt.Errorf("oauth.GetToken: load secret: %w", err)
	}

	plaintext, err := m.decrypt(secret.Ciphertext)
	if err != nil {
		return nil, fmt.Errorf("oauth.GetToken: decrypt token: %w", err)
	}

	var token oauth2.Token
	if err := json.Unmarshal(plaintext, &token); err != nil {
		return nil, fmt.Errorf("oauth.GetToken: unmarshal token: %w", err)
	}

	if !m.shouldRefresh(&token) || cfg == nil {
		return &token, nil
	}

	expiredToken := token
	expiredToken.Expiry = m.now().UTC().Add(-time.Minute)

	refreshed, err := cfg.TokenSource(ctx, &expiredToken).Token()
	if err != nil {
		return nil, fmt.Errorf("oauth.GetToken: refresh token: %w", err)
	}

	if err := m.SaveToken(ctx, tenantID, connectionID, refreshed); err != nil {
		return nil, fmt.Errorf("oauth.GetToken: persist refreshed token: %w", err)
	}

	return refreshed, nil
}

func (m *Manager) ensureConnection(ctx context.Context, stateRecord *store.OAuthState) error {
	_, err := m.store.GetConnection(ctx, stateRecord.TenantID, stateRecord.ConnectionID)
	if err == nil {
		return nil
	}
	if !errors.Is(err, store.ErrNotFound) {
		return err
	}

	return m.store.SaveConnection(ctx, &connections.Connection{
		TenantID:     stateRecord.TenantID,
		ConnectionID: stateRecord.ConnectionID,
		Provider:     stateRecord.Provider,
		Config:       map[string]any{},
		CreatedAt:    m.now().UTC(),
	})
}

func (m *Manager) shouldRefresh(token *oauth2.Token) bool {
	if token == nil || token.Expiry.IsZero() {
		return false
	}

	return !token.Expiry.After(m.now().UTC().Add(m.refreshLeeway))
}

func (m *Manager) encrypt(plaintext []byte) ([]byte, error) {
	nonce := make([]byte, m.cipher.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return nil, fmt.Errorf("oauth.encrypt: generate nonce: %w", err)
	}

	return m.cipher.Seal(nonce, nonce, plaintext, nil), nil
}

func (m *Manager) decrypt(ciphertext []byte) ([]byte, error) {
	nonceSize := m.cipher.NonceSize()
	if len(ciphertext) < nonceSize {
		return nil, errors.New("oauth.decrypt: ciphertext too short")
	}

	nonce := ciphertext[:nonceSize]
	payload := ciphertext[nonceSize:]

	plaintext, err := m.cipher.Open(nil, nonce, payload, nil)
	if err != nil {
		return nil, fmt.Errorf("oauth.decrypt: %w", err)
	}

	return plaintext, nil
}

func deriveKey(secret string) []byte {
	sum := sha256.Sum256([]byte(secret))
	return sum[:]
}

func randomString(size int) (string, error) {
	value := make([]byte, size)
	if _, err := io.ReadFull(rand.Reader, value); err != nil {
		return "", err
	}

	return base64.RawURLEncoding.EncodeToString(value), nil
}
