package providers

import (
	"errors"
	"net/http"
	"strings"
	"time"

	"golang.org/x/oauth2"
)

var (
	ErrWebhooksNotSupported    = errors.New("providers: webhooks not supported")
	ErrInvalidWebhookSignature = errors.New("providers: invalid webhook signature")
	ErrMissingWebhookSignature = errors.New("providers: missing webhook signature")
	ErrWebhookSecretRequired   = errors.New("providers: webhook secret is required")
)

type ProviderConfig struct {
	BaseURL       string
	RedirectURL   string
	ClientID      string
	ClientSecret  string
	WebhookSecret string
	Scopes        []string
}

type Action struct {
	Key   string `json:"key"`
	Label string `json:"label"`
}

type Metadata struct {
	ID                string   `json:"id"`
	AuthorizationURL  string   `json:"authorizationURL"`
	TokenURL          string   `json:"tokenURL"`
	Scopes            []string `json:"scopes"`
	Actions           []Action `json:"actions"`
	WebhooksSupported bool     `json:"webhooksSupported"`
}

type WebhookEvent struct {
	TriggerKey string         `json:"triggerKey"`
	Raw        []byte         `json:"-"`
	Payload    any            `json:"payload,omitempty"`
	Normalized map[string]any `json:"normalized,omitempty"`
	ReceivedAt time.Time      `json:"receivedAt"`
}

type Provider interface {
	ID() string
	Metadata() Metadata
	OAuthConfig() *oauth2.Config
	ParseWebhookEvent(headers http.Header, body []byte) (*WebhookEvent, error)
}

func EffectiveScopes(config ProviderConfig, defaults []string) []string {
	if len(config.Scopes) > 0 {
		return CloneStrings(config.Scopes)
	}

	return CloneStrings(defaults)
}

func RedirectURL(config ProviderConfig, providerID string) string {
	if config.RedirectURL != "" {
		return config.RedirectURL
	}
	if config.BaseURL == "" {
		return ""
	}

	return strings.TrimRight(config.BaseURL, "/") + "/oauth/" + providerID + "/callback"
}

func CloneStrings(values []string) []string {
	if len(values) == 0 {
		return nil
	}

	cloned := make([]string, len(values))
	copy(cloned, values)
	return cloned
}

func CloneActions(values []Action) []Action {
	if len(values) == 0 {
		return nil
	}

	cloned := make([]Action, len(values))
	copy(cloned, values)
	return cloned
}
