package stripe

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	providerapi "github.com/charliewilco/argus/providers"
	"golang.org/x/oauth2"
)

const (
	ProviderID       = "stripe"
	AuthorizationURL = "https://connect.stripe.com/oauth/authorize"
	TokenURL         = "https://connect.stripe.com/oauth/token"
	ScopeReadWrite   = "read_write"
	HeaderSignature  = "Stripe-Signature"
)

var (
	defaultScopes = []string{ScopeReadWrite}
	actions       = []providerapi.Action{
		{Key: "create_payment_intent", Label: "Create Payment Intent"},
		{Key: "create_customer", Label: "Create Customer"},
		{Key: "cancel_subscription", Label: "Cancel Subscription"},
	}
)

type webhookPayload struct {
	Type string `json:"type"`
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
		Actions:           providerapi.CloneActions(actions),
		WebhooksSupported: true,
	}
}

func (p *Provider) OAuthConfig() *oauth2.Config {
	cfg := *p.oauthConfig
	cfg.Scopes = providerapi.CloneStrings(p.oauthConfig.Scopes)
	return &cfg
}

func (p *Provider) ParseWebhookEvent(headers http.Header, body []byte) (*providerapi.WebhookEvent, error) {
	if err := validateSignature(p.config.WebhookSecret, headers.Get(HeaderSignature), body); err != nil {
		return nil, fmt.Errorf("stripe: validate webhook signature: %w", err)
	}

	var payload webhookPayload
	if err := json.Unmarshal(body, &payload); err != nil {
		return nil, fmt.Errorf("stripe: decode webhook payload: %w", err)
	}
	if payload.Type == "" {
		return nil, fmt.Errorf("stripe: missing event type")
	}

	normalized := map[string]any{"type": payload.Type}

	return &providerapi.WebhookEvent{
		TriggerKey: ProviderID + "." + payload.Type,
		Raw:        append([]byte(nil), body...),
		Payload:    payload,
		Normalized: normalized,
		ReceivedAt: time.Now().UTC(),
	}, nil
}

func validateSignature(secret, signatureHeader string, body []byte) error {
	if secret == "" {
		return providerapi.ErrWebhookSecretRequired
	}
	if signatureHeader == "" {
		return providerapi.ErrMissingWebhookSignature
	}

	var timestamp string
	var signatures []string

	for _, part := range strings.Split(signatureHeader, ",") {
		key, value, ok := strings.Cut(strings.TrimSpace(part), "=")
		if !ok {
			continue
		}

		switch key {
		case "t":
			timestamp = value
		case "v1":
			signatures = append(signatures, value)
		}
	}

	if timestamp == "" || len(signatures) == 0 {
		return providerapi.ErrInvalidWebhookSignature
	}

	expected := providerapi.ComputeHMACSHA256Hex(secret, []byte(timestamp+"."+string(body)))
	for _, candidate := range signatures {
		if candidate == expected {
			return nil
		}
	}

	return providerapi.ErrInvalidWebhookSignature
}
