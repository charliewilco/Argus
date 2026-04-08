package github_test

import (
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	githubprovider "github.com/charliewilco/argus/providers/github"
)

func TestParseWebhookEventRejectsInvalidSignature(t *testing.T) {
	t.Parallel()

	provider, err := githubprovider.NewProvider(githubprovider.Config{
		BaseURL:       "http://localhost:8080",
		WebhookSecret: "secret",
	})
	require.NoError(t, err)

	request := httptest.NewRequest("POST", "/webhooks/github", strings.NewReader(`{"ref":"refs/heads/main"}`))
	request.Header.Set("X-GitHub-Event", "push")
	request.Header.Set("X-GitHub-Delivery", "delivery_1")
	request.Header.Set("X-Hub-Signature-256", "sha256=deadbeef")

	_, err = provider.ParseWebhookEvent(request)
	require.Error(t, err)
	require.Contains(t, err.Error(), "signature mismatch")
}
