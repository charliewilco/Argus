package facebook

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
	ProviderID                   = "facebook"
	AuthorizationURL             = "https://www.facebook.com/v19.0/dialog/oauth"
	TokenURL                     = "https://graph.facebook.com/v19.0/oauth/access_token"
	ScopeEmail                   = "email"
	ScopePublicProfile           = "public_profile"
	ScopePagesManagePosts        = "pages_manage_posts"
	ScopePagesReadEngagement     = "pages_read_engagement"
	ScopeInstagramBasic          = "instagram_basic"
	ScopeInstagramContentPublish = "instagram_content_publish"
	HeaderSignature              = "X-Hub-Signature-256"
)

var (
	defaultScopes = []string{
		ScopeEmail,
		ScopePublicProfile,
		ScopePagesManagePosts,
		ScopePagesReadEngagement,
		ScopeInstagramBasic,
		ScopeInstagramContentPublish,
	}
	actions = []providerapi.Action{
		{Key: "create_post", Label: "Create Post"},
		{Key: "publish_to_page", Label: "Publish To Page"},
	}
)

type webhookPayload struct {
	Object string `json:"object"`
	Entry  []struct {
		Changes []struct {
			Field string `json:"field"`
		} `json:"changes"`
		Messaging []json.RawMessage `json:"messaging"`
	} `json:"entry"`
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
	if err := providerapi.ValidateHMACSHA256Hex(p.config.WebhookSecret, body, headers.Get(HeaderSignature), "sha256="); err != nil {
		return nil, fmt.Errorf("facebook: validate webhook signature: %w", err)
	}

	var payload webhookPayload
	if err := json.Unmarshal(body, &payload); err != nil {
		return nil, fmt.Errorf("facebook: decode webhook payload: %w", err)
	}

	eventType := detectEventType(payload)
	if eventType == "" {
		return nil, fmt.Errorf("facebook: unsupported webhook payload")
	}

	normalized := map[string]any{
		"object": payload.Object,
		"type":   eventType,
	}

	return &providerapi.WebhookEvent{
		TriggerKey: ProviderID + "." + eventType,
		Raw:        append([]byte(nil), body...),
		Payload:    payload,
		Normalized: normalized,
		ReceivedAt: time.Now().UTC(),
	}, nil
}

func (p *Provider) ExecuteAction(_ context.Context, _ *oauth2.Token, request providerapi.ActionRequest) (providerapi.ActionResult, error) {
	return providerapi.ActionResult{}, fmt.Errorf("facebook.ExecuteAction: %w %q", providerapi.ErrUnsupportedProviderAction, request.Action)
}

func detectEventType(payload webhookPayload) string {
	for _, entry := range payload.Entry {
		if len(entry.Messaging) > 0 {
			return "messages"
		}
		for _, change := range entry.Changes {
			switch change.Field {
			case "feed", "mention", "instagram_mentions":
				return change.Field
			}
		}
	}

	return ""
}
