package uber

import (
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	providerapi "github.com/charliewilco/argus/providers"
	"golang.org/x/oauth2"
)

const (
	ProviderID          = "uber"
	AuthorizationURL    = "https://login.uber.com/oauth/v2/authorize"
	TokenURL            = "https://login.uber.com/oauth/v2/token"
	ScopeProfile        = "profile"
	ScopeRequest        = "request"
	ScopeRequestReceipt = "request.receipt"
	ScopeHistory        = "history"
	HeaderSignature     = "X-Uber-Signature"
)

var (
	defaultScopes = []string{
		ScopeProfile,
		ScopeRequest,
		ScopeRequestReceipt,
		ScopeHistory,
	}
)

type webhookPayload struct {
	EventType string `json:"event_type"`
	Kind      string `json:"kind"`
}

type ProviderConfig = providerapi.ProviderConfig

type Provider struct {
	config      ProviderConfig
	oauthConfig *oauth2.Config
}

func New(config ProviderConfig) *Provider {
	return &Provider{
		config: config,
		oauthConfig: &oauth2.Config{
			ClientID:     config.ClientID,
			ClientSecret: config.ClientSecret,
			RedirectURL:  providerapi.RedirectURL(config, ProviderID),
			Scopes:       providerapi.EffectiveScopes(config, defaultScopes),
			Endpoint: oauth2.Endpoint{
				AuthURL:  AuthorizationURL,
				TokenURL: TokenURL,
			},
		},
	}
}

func (p *Provider) ID() string {
	return ProviderID
}

func (p *Provider) Metadata() providerapi.Metadata {
	return providerapi.Metadata{
		ID:                ProviderID,
		AuthorizationURL:  AuthorizationURL,
		TokenURL:          TokenURL,
		Scopes:            providerapi.CloneStrings(p.oauthConfig.Scopes),
		Actions:           nil,
		WebhooksSupported: true,
	}
}

func (p *Provider) OAuthConfig() *oauth2.Config {
	cfg := *p.oauthConfig
	cfg.Scopes = providerapi.CloneStrings(p.oauthConfig.Scopes)
	return &cfg
}

func (p *Provider) ParseWebhookEvent(headers http.Header, body []byte) (*providerapi.WebhookEvent, error) {
	if err := validateSignature(p.config.WebhookSecret, body, headers.Get(HeaderSignature)); err != nil {
		return nil, fmt.Errorf("uber: validate webhook signature: %w", err)
	}

	var payload webhookPayload
	if err := json.Unmarshal(body, &payload); err != nil {
		return nil, fmt.Errorf("uber: decode webhook payload: %w", err)
	}

	eventType := payload.EventType
	if eventType == "" {
		eventType = payload.Kind
	}
	if eventType == "" {
		return nil, fmt.Errorf("uber: missing event type")
	}

	normalized := map[string]any{"type": eventType}

	return &providerapi.WebhookEvent{
		TriggerKey: ProviderID + "." + eventType,
		Raw:        append([]byte(nil), body...),
		Payload:    payload,
		Normalized: normalized,
		ReceivedAt: time.Now().UTC(),
	}, nil
}

func validateSignature(secret string, body []byte, signature string) error {
	if signature == "" {
		return providerapi.ErrMissingWebhookSignature
	}
	if err := providerapi.ValidateHMACSHA256Hex(secret, body, signature, "sha256="); err == nil {
		return nil
	}

	return providerapi.ValidateHMACSHA256Hex(secret, body, signature, "")
}
