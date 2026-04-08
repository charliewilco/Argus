package github

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
	ProviderID             = "github"
	AuthorizationURL       = "https://github.com/login/oauth/authorize"
	TokenURL               = "https://github.com/login/oauth/access_token"
	ScopeRepo              = "repo"
	ScopeReadUser          = "read:user"
	ScopeUserEmail         = "user:email"
	HeaderEvent            = "X-GitHub-Event"
	HeaderDeliveryID       = "X-GitHub-Delivery"
	HeaderWebhookSignature = "X-Hub-Signature-256"
)

var (
	defaultScopes = []string{
		ScopeRepo,
		ScopeReadUser,
		ScopeUserEmail,
	}
	actions = []providerapi.Action{
		{Key: "create_issue", Label: "Create Issue"},
		{Key: "create_comment", Label: "Create Comment"},
		{Key: "create_check_run", Label: "Create Check Run"},
	}
)

type ProviderConfig = providerapi.ProviderConfig
type Config = providerapi.ProviderConfig

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

func NewProvider(config Config) (*Provider, error) {
	if config.BaseURL == "" && config.RedirectURL == "" {
		return nil, fmt.Errorf("github.NewProvider: base URL or redirect URL is required")
	}

	return New(ProviderConfig(config)), nil
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
	if err := providerapi.ValidateHMACSHA256Hex(p.config.WebhookSecret, body, headers.Get(HeaderWebhookSignature), "sha256="); err != nil {
		return nil, fmt.Errorf("github: validate webhook signature: %w", err)
	}

	eventType := headers.Get(HeaderEvent)
	if eventType == "" {
		return nil, fmt.Errorf("github: missing %s header", HeaderEvent)
	}

	var payload map[string]any
	if len(body) > 0 {
		if err := json.Unmarshal(body, &payload); err != nil {
			return nil, fmt.Errorf("github: decode webhook payload: %w", err)
		}
	}

	triggerKey := ProviderID + "." + eventType
	if action, ok := payload["action"].(string); ok && action != "" {
		triggerKey += "." + action
	}

	return &providerapi.WebhookEvent{
		ID:         headers.Get(HeaderDeliveryID),
		TriggerKey: triggerKey,
		Raw:        append([]byte(nil), body...),
		Payload:    payload,
		Normalized: payload,
		ReceivedAt: time.Now().UTC(),
	}, nil
}

func (p *Provider) ExecuteAction(_ context.Context, _ *oauth2.Token, request providerapi.ActionRequest) (providerapi.ActionResult, error) {
	return providerapi.ActionResult{}, fmt.Errorf("github.ExecuteAction: %w %q", providerapi.ErrUnsupportedProviderAction, request.Action)
}
