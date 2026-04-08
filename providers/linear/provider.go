package linear

import (
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	providerapi "github.com/charliewilco/argus/providers"
	"golang.org/x/oauth2"
)

const (
	ProviderID        = "linear"
	AuthorizationURL  = "https://linear.app/oauth/authorize"
	TokenURL          = "https://api.linear.app/oauth/token"
	ScopeRead         = "read"
	ScopeWrite        = "write"
	ScopeIssuesCreate = "issues:create"
	HeaderSignature   = "Linear-Signature"
)

var (
	defaultScopes = []string{
		ScopeRead,
		ScopeWrite,
		ScopeIssuesCreate,
	}
	actions = []providerapi.Action{
		{Key: "create_issue", Label: "Create Issue"},
		{Key: "update_issue", Label: "Update Issue"},
		{Key: "add_comment", Label: "Add Comment"},
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
	if err := providerapi.ValidateHMACSHA256Hex(p.config.WebhookSecret, body, headers.Get(HeaderSignature), ""); err != nil {
		return nil, fmt.Errorf("linear: validate webhook signature: %w", err)
	}

	var payload webhookPayload
	if err := json.Unmarshal(body, &payload); err != nil {
		return nil, fmt.Errorf("linear: decode webhook payload: %w", err)
	}
	if payload.Type == "" {
		return nil, fmt.Errorf("linear: missing event type")
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
