package x_test

import (
	"net/http"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/charliewilco/argus/providers"
	xprovider "github.com/charliewilco/argus/providers/x"
)

func TestProviderID(t *testing.T) {
	t.Parallel()

	provider := xprovider.New(xprovider.ProviderConfig{})
	require.Equal(t, "x", provider.ID())
}

func TestParseWebhookEventRejectsInvalidSignature(t *testing.T) {
	t.Parallel()

	provider := xprovider.New(xprovider.ProviderConfig{
		WebhookSecret: "secret",
	})

	headers := http.Header{}
	headers.Set("X-Twitter-Webhooks-Signature", "sha256=bad")

	_, err := provider.ParseWebhookEvent(headers, []byte(`{"tweet_create_events":[{}]}`))
	require.ErrorIs(t, err, providers.ErrInvalidWebhookSignature)
}
