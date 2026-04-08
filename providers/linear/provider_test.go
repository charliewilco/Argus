package linear_test

import (
	"net/http"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/charliewilco/argus/providers"
	linearprovider "github.com/charliewilco/argus/providers/linear"
)

func TestProviderID(t *testing.T) {
	t.Parallel()

	provider := linearprovider.New(linearprovider.ProviderConfig{})
	require.Equal(t, "linear", provider.ID())
}

func TestParseWebhookEventRejectsInvalidSignature(t *testing.T) {
	t.Parallel()

	provider := linearprovider.New(linearprovider.ProviderConfig{
		WebhookSecret: "secret",
	})

	headers := http.Header{}
	headers.Set("Linear-Signature", "bad")

	_, err := provider.ParseWebhookEvent(headers, []byte(`{"type":"Issue","action":"create"}`))
	require.ErrorIs(t, err, providers.ErrInvalidWebhookSignature)
}
