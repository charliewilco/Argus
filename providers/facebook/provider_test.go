package facebook_test

import (
	"net/http"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/charliewilco/argus/providers"
	facebookprovider "github.com/charliewilco/argus/providers/facebook"
)

func TestProviderID(t *testing.T) {
	t.Parallel()

	provider := facebookprovider.New(facebookprovider.ProviderConfig{})
	require.Equal(t, "facebook", provider.ID())
}

func TestParseWebhookEventRejectsInvalidSignature(t *testing.T) {
	t.Parallel()

	provider := facebookprovider.New(facebookprovider.ProviderConfig{
		WebhookSecret: "secret",
	})

	headers := http.Header{}
	headers.Set("X-Hub-Signature-256", "sha256=bad")

	_, err := provider.ParseWebhookEvent(headers, []byte(`{"entry":[{"changes":[{"field":"feed"}]}]}`))
	require.ErrorIs(t, err, providers.ErrInvalidWebhookSignature)
}
