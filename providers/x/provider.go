package x

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	providerapi "github.com/charliewilco/argus/providers"
	"golang.org/x/oauth2"
)

const (
	ProviderID         = "x"
	AuthorizationURL   = "https://twitter.com/i/oauth2/authorize"
	TokenURL           = "https://api.twitter.com/2/oauth2/token"
	ScopeTweetRead     = "tweet.read"
	ScopeTweetWrite    = "tweet.write"
	ScopeUsersRead     = "users.read"
	ScopeOfflineAccess = "offline.access"
	HeaderSignature    = "X-Twitter-Webhooks-Signature"
)

var (
	defaultScopes = []string{
		ScopeTweetRead,
		ScopeTweetWrite,
		ScopeUsersRead,
		ScopeOfflineAccess,
	}
	actions = []providerapi.Action{
		{Key: "create_tweet", Label: "Create Tweet"},
		{Key: "delete_tweet", Label: "Delete Tweet"},
		{Key: "send_direct_message", Label: "Send Direct Message"},
	}
)

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
	if err := validateSignature(p.config.WebhookSecret, body, headers.Get(HeaderSignature)); err != nil {
		return nil, fmt.Errorf("x: validate webhook signature: %w", err)
	}

	var payload map[string]json.RawMessage
	if err := json.Unmarshal(body, &payload); err != nil {
		return nil, fmt.Errorf("x: decode webhook payload: %w", err)
	}

	eventType := detectEventType(payload)
	if eventType == "" {
		return nil, fmt.Errorf("x: unsupported webhook payload")
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

func (p *Provider) ExecuteAction(_ context.Context, _ *oauth2.Token, request providerapi.ActionRequest) (providerapi.ActionResult, error) {
	return providerapi.ActionResult{}, fmt.Errorf("x.ExecuteAction: %w %q", providerapi.ErrUnsupportedProviderAction, request.Action)
}

func validateSignature(secret string, body []byte, signature string) error {
	if signature == "" {
		return providerapi.ErrMissingWebhookSignature
	}
	if err := providerapi.ValidateHMACSHA256Base64(secret, body, signature, "sha256="); err == nil {
		return nil
	}

	return providerapi.ValidateHMACSHA256Base64(secret, body, signature, "")
}

func detectEventType(payload map[string]json.RawMessage) string {
	for _, key := range []string{"tweet_create_events", "direct_message_events", "mention_events"} {
		raw, ok := payload[key]
		if !ok || len(raw) == 0 || string(raw) == "null" || string(raw) == "[]" {
			continue
		}

		return key
	}

	return ""
}
