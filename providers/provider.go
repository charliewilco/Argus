package providers

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/charliewilco/argus/internal/envelope"
	"golang.org/x/oauth2"
)

var (
	ErrProviderNotFound          = errors.New("providers: provider not found")
	ErrWebhooksNotSupported      = errors.New("providers: webhooks not supported")
	ErrInvalidWebhookSignature   = errors.New("providers: invalid webhook signature")
	ErrMissingWebhookSignature   = errors.New("providers: missing webhook signature")
	ErrWebhookSecretRequired     = errors.New("providers: webhook secret is required")
	ErrUnsupportedProviderAction = errors.New("providers: unsupported action")
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
	ID         string         `json:"id,omitempty"`
	TriggerKey string         `json:"triggerKey"`
	Raw        []byte         `json:"-"`
	Payload    any            `json:"payload,omitempty"`
	Normalized map[string]any `json:"normalized,omitempty"`
	ReceivedAt time.Time      `json:"receivedAt"`
}

type ActionRequest struct {
	Action       string
	ConnectionID string
	Config       map[string]any
	Event        *envelope.Event
}

type ActionResult struct {
	Provider string         `json:"provider"`
	Action   string         `json:"action"`
	Status   string         `json:"status"`
	Output   map[string]any `json:"output,omitempty"`
}

type Provider interface {
	ID() string
	Metadata() Metadata
	OAuthConfig() *oauth2.Config
	ParseWebhookEvent(headers http.Header, body []byte) (*WebhookEvent, error)
	ExecuteAction(ctx context.Context, token *oauth2.Token, request ActionRequest) (ActionResult, error)
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
