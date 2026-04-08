package google

import (
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	providerapi "github.com/charliewilco/argus/providers"
	"golang.org/x/oauth2"
)

const (
	ProviderID          = "google"
	AuthorizationURL    = "https://accounts.google.com/o/oauth2/v2/auth"
	TokenURL            = "https://oauth2.googleapis.com/token"
	ScopeGmail          = "https://www.googleapis.com/auth/gmail.modify"
	ScopeCalendar       = "https://www.googleapis.com/auth/calendar"
	ScopeDrive          = "https://www.googleapis.com/auth/drive"
	ScopeSheets         = "https://www.googleapis.com/auth/spreadsheets"
	ScopeDocs           = "https://www.googleapis.com/auth/documents"
	HeaderResourceState = "X-Goog-Resource-State"
	HeaderChanged       = "X-Goog-Changed"
	HeaderChannelID     = "X-Goog-Channel-Id"
	HeaderResourceID    = "X-Goog-Resource-Id"
	HeaderMessageNumber = "X-Goog-Message-Number"
)

type NotificationEvent struct {
	ResourceState string
	Changed       []string
	ChannelID     string
	ResourceID    string
	MessageNumber int64
}

type ProviderConfig = providerapi.ProviderConfig

type Provider struct {
	oauthConfig *oauth2.Config
}

func New(config ProviderConfig) *Provider {
	return &Provider{
		oauthConfig: &oauth2.Config{
			ClientID:     config.ClientID,
			ClientSecret: config.ClientSecret,
			RedirectURL:  providerapi.RedirectURL(config, ProviderID),
			Scopes:       providerapi.EffectiveScopes(config, nil),
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
	resourceState := headers.Get(HeaderResourceState)
	if resourceState == "" {
		return nil, fmt.Errorf("google: missing %s header", HeaderResourceState)
	}

	var changed []string
	if rawChanged := headers.Get(HeaderChanged); rawChanged != "" {
		for _, item := range strings.Split(rawChanged, ",") {
			item = strings.TrimSpace(item)
			if item != "" {
				changed = append(changed, item)
			}
		}
	}

	messageNumber := int64(0)
	if rawMessageNumber := headers.Get(HeaderMessageNumber); rawMessageNumber != "" {
		parsed, err := strconv.ParseInt(rawMessageNumber, 10, 64)
		if err != nil {
			return nil, fmt.Errorf("google: parse %s: %w", HeaderMessageNumber, err)
		}
		messageNumber = parsed
	}

	payload := NotificationEvent{
		ResourceState: resourceState,
		Changed:       changed,
		ChannelID:     headers.Get(HeaderChannelID),
		ResourceID:    headers.Get(HeaderResourceID),
		MessageNumber: messageNumber,
	}

	normalized := map[string]any{
		"resource_state": payload.ResourceState,
		"changed":        payload.Changed,
		"channel_id":     payload.ChannelID,
		"resource_id":    payload.ResourceID,
		"message_number": payload.MessageNumber,
	}

	return &providerapi.WebhookEvent{
		TriggerKey: ProviderID + "." + payload.ResourceState,
		Raw:        append([]byte(nil), body...),
		Payload:    payload,
		Normalized: normalized,
		ReceivedAt: time.Now().UTC(),
	}, nil
}
