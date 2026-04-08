package github

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/charliewilco/argus/internal/envelope"
	"github.com/charliewilco/argus/providers"
	"golang.org/x/oauth2"
)

const providerID = "github"

type Config struct {
	ClientID      string
	ClientSecret  string
	BaseURL       string
	WebhookSecret string
}

type Provider struct {
	config Config
}

type pushEvent struct {
	Ref        string         `json:"ref"`
	Repository repositoryInfo `json:"repository"`
	Pusher     actorInfo      `json:"pusher"`
	HeadCommit commitInfo     `json:"head_commit"`
}

type pullRequestEvent struct {
	Action      string          `json:"action"`
	Number      int             `json:"number"`
	Repository  repositoryInfo  `json:"repository"`
	PullRequest pullRequestInfo `json:"pull_request"`
	Sender      actorInfo       `json:"sender"`
}

type issuesEvent struct {
	Action     string         `json:"action"`
	Repository repositoryInfo `json:"repository"`
	Issue      issueInfo      `json:"issue"`
	Sender     actorInfo      `json:"sender"`
}

type repositoryInfo struct {
	ID       int    `json:"id"`
	Name     string `json:"name"`
	FullName string `json:"full_name"`
	HTMLURL  string `json:"html_url"`
	Private  bool   `json:"private"`
}

type actorInfo struct {
	Name  string `json:"name,omitempty"`
	Login string `json:"login,omitempty"`
	Email string `json:"email,omitempty"`
}

type commitInfo struct {
	ID      string `json:"id"`
	Message string `json:"message"`
	URL     string `json:"url"`
}

type pullRequestInfo struct {
	ID      int    `json:"id"`
	Number  int    `json:"number"`
	Title   string `json:"title"`
	State   string `json:"state"`
	HTMLURL string `json:"html_url"`
}

type issueInfo struct {
	ID      int    `json:"id"`
	Number  int    `json:"number"`
	Title   string `json:"title"`
	State   string `json:"state"`
	HTMLURL string `json:"html_url"`
}

func NewProvider(config Config) (*Provider, error) {
	if config.BaseURL == "" {
		return nil, fmt.Errorf("github.NewProvider: base URL is required")
	}

	return &Provider{config: config}, nil
}

func (p *Provider) ID() string {
	return providerID
}

func (p *Provider) OAuthConfig() oauth2.Config {
	baseURL := strings.TrimRight(p.config.BaseURL, "/")

	return oauth2.Config{
		ClientID:     p.config.ClientID,
		ClientSecret: p.config.ClientSecret,
		RedirectURL:  baseURL + "/oauth/github/callback",
		Scopes: []string{
			"repo",
			"read:user",
			"user:email",
		},
		Endpoint: oauth2.Endpoint{
			AuthURL:  "https://github.com/login/oauth/authorize",
			TokenURL: "https://github.com/login/oauth/access_token",
		},
	}
}

func (p *Provider) ParseWebhookEvent(r *http.Request) (envelope.Event, error) {
	if r == nil {
		return envelope.Event{}, fmt.Errorf("github.ParseWebhookEvent: request is required")
	}

	payload, err := io.ReadAll(r.Body)
	if err != nil {
		return envelope.Event{}, fmt.Errorf("github.ParseWebhookEvent: read body: %w", err)
	}

	if err := p.validateSignature(payload, r.Header.Get("X-Hub-Signature-256")); err != nil {
		return envelope.Event{}, fmt.Errorf("github.ParseWebhookEvent: %w", err)
	}

	eventName := r.Header.Get("X-GitHub-Event")
	if eventName == "" {
		return envelope.Event{}, fmt.Errorf("github.ParseWebhookEvent: missing X-GitHub-Event header")
	}

	deliveryID := r.Header.Get("X-GitHub-Delivery")
	if deliveryID == "" {
		return envelope.Event{}, fmt.Errorf("github.ParseWebhookEvent: missing X-GitHub-Delivery header")
	}

	var normalized map[string]any
	switch eventName {
	case "push":
		var event pushEvent
		if err := json.Unmarshal(payload, &event); err != nil {
			return envelope.Event{}, fmt.Errorf("github.ParseWebhookEvent: decode push: %w", err)
		}
		normalized, err = toMap(event)
		if err != nil {
			return envelope.Event{}, fmt.Errorf("github.ParseWebhookEvent: normalize push: %w", err)
		}
	case "pull_request":
		var event pullRequestEvent
		if err := json.Unmarshal(payload, &event); err != nil {
			return envelope.Event{}, fmt.Errorf("github.ParseWebhookEvent: decode pull request: %w", err)
		}
		normalized, err = toMap(event)
		if err != nil {
			return envelope.Event{}, fmt.Errorf("github.ParseWebhookEvent: normalize pull request: %w", err)
		}
	case "issues":
		var event issuesEvent
		if err := json.Unmarshal(payload, &event); err != nil {
			return envelope.Event{}, fmt.Errorf("github.ParseWebhookEvent: decode issues: %w", err)
		}
		normalized, err = toMap(event)
		if err != nil {
			return envelope.Event{}, fmt.Errorf("github.ParseWebhookEvent: normalize issues: %w", err)
		}
	default:
		return envelope.Event{}, fmt.Errorf("github.ParseWebhookEvent: unsupported event %q", eventName)
	}

	normalized["event_name"] = eventName

	return envelope.Event{
		ID:         deliveryID,
		Provider:   providerID,
		TriggerKey: providerID + "." + eventName,
		Raw:        payload,
		Normalized: normalized,
	}, nil
}

func (p *Provider) ExecuteAction(_ context.Context, _ *oauth2.Token, request providers.ActionRequest) (providers.ActionResult, error) {
	if request.Action == "github.noop" {
		return providers.ActionResult{
			Provider: providerID,
			Action:   request.Action,
			Status:   "ok",
			Output:   request.Config,
		}, nil
	}

	return providers.ActionResult{}, fmt.Errorf("github.ExecuteAction: unsupported action %q", request.Action)
}

func (p *Provider) validateSignature(payload []byte, signature string) error {
	if p.config.WebhookSecret == "" {
		return fmt.Errorf("webhook secret is required")
	}
	if signature == "" {
		return fmt.Errorf("missing X-Hub-Signature-256 header")
	}
	if !strings.HasPrefix(signature, "sha256=") {
		return fmt.Errorf("invalid signature format")
	}

	mac := hmac.New(sha256.New, []byte(p.config.WebhookSecret))
	mac.Write(payload)
	expected := "sha256=" + hex.EncodeToString(mac.Sum(nil))
	if !hmac.Equal([]byte(expected), []byte(signature)) {
		return fmt.Errorf("signature mismatch")
	}

	return nil
}

func toMap(value any) (map[string]any, error) {
	data, err := json.Marshal(value)
	if err != nil {
		return nil, err
	}

	var normalized map[string]any
	if err := json.Unmarshal(data, &normalized); err != nil {
		return nil, err
	}

	return normalized, nil
}
