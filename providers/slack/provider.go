package slack

import (
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	providerapi "github.com/charliewilco/argus/providers"
	"golang.org/x/oauth2"
)

const (
	ProviderID          = "slack"
	AuthorizationURL    = "https://slack.com/oauth/v2/authorize"
	TokenURL            = "https://slack.com/api/oauth.v2.access"
	ScopeChannelsRead   = "channels:read"
	ScopeChatWrite      = "chat:write"
	ScopeFilesWrite     = "files:write"
	ScopeUsersRead      = "users:read"
	ScopeReactionsWrite = "reactions:write"
	HeaderSignature     = "X-Slack-Signature"
	HeaderTimestamp     = "X-Slack-Request-Timestamp"
)

var (
	defaultScopes = []string{
		ScopeChannelsRead,
		ScopeChatWrite,
		ScopeFilesWrite,
		ScopeUsersRead,
		ScopeReactionsWrite,
	}
	actions = []providerapi.Action{
		{Key: "post_message", Label: "Post Message"},
		{Key: "upload_file", Label: "Upload File"},
		{Key: "add_reaction", Label: "Add Reaction"},
	}
)

type webhookPayload struct {
	Type  string `json:"type"`
	Event struct {
		Type string `json:"type"`
	} `json:"event"`
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
	signedPayload := []byte("v0:" + headers.Get(HeaderTimestamp) + ":" + string(body))
	if err := providerapi.ValidateHMACSHA256Hex(p.config.WebhookSecret, signedPayload, headers.Get(HeaderSignature), "v0="); err != nil {
		return nil, fmt.Errorf("slack: validate webhook signature: %w", err)
	}

	var payload webhookPayload
	if err := json.Unmarshal(body, &payload); err != nil {
		return nil, fmt.Errorf("slack: decode webhook payload: %w", err)
	}

	eventType := payload.Event.Type
	if eventType == "" {
		eventType = payload.Type
	}
	if eventType == "" {
		return nil, fmt.Errorf("slack: missing event type")
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
