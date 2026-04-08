package github_test

import (
	"net/http"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/charliewilco/argus/providers"
	githubprovider "github.com/charliewilco/argus/providers/github"
)

func TestProviderID(t *testing.T) {
	t.Parallel()

	provider := githubprovider.New(githubprovider.ProviderConfig{})
	require.Equal(t, "github", provider.ID())
}

func TestParseWebhookEventRejectsInvalidSignature(t *testing.T) {
	t.Parallel()

	provider := githubprovider.New(githubprovider.ProviderConfig{
		WebhookSecret: "secret",
	})

	headers := http.Header{}
	headers.Set("X-Hub-Signature-256", "sha256=bad")
	headers.Set("X-GitHub-Event", "issues")

	_, err := provider.ParseWebhookEvent(headers, []byte(`{"action":"opened"}`))
	require.ErrorIs(t, err, providers.ErrInvalidWebhookSignature)
}
