package slack_test

import (
	"net/http"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/charliewilco/argus/providers"
	slackprovider "github.com/charliewilco/argus/providers/slack"
)

func TestProviderID(t *testing.T) {
	t.Parallel()

	provider := slackprovider.New(slackprovider.ProviderConfig{})
	require.Equal(t, "slack", provider.ID())
}

func TestParseWebhookEventRejectsInvalidSignature(t *testing.T) {
	t.Parallel()

	provider := slackprovider.New(slackprovider.ProviderConfig{
		WebhookSecret: "secret",
	})

	headers := http.Header{}
	headers.Set("X-Slack-Request-Timestamp", "123")
	headers.Set("X-Slack-Signature", "v0=bad")

	_, err := provider.ParseWebhookEvent(headers, []byte(`{"event":{"type":"message"}}`))
	require.ErrorIs(t, err, providers.ErrInvalidWebhookSignature)
}
