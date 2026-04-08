package uber_test

import (
	"net/http"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/charliewilco/argus/providers"
	uberprovider "github.com/charliewilco/argus/providers/uber"
)

func TestProviderID(t *testing.T) {
	t.Parallel()

	provider := uberprovider.New(uberprovider.ProviderConfig{})
	require.Equal(t, "uber", provider.ID())
}

func TestParseWebhookEventRejectsInvalidSignature(t *testing.T) {
	t.Parallel()

	provider := uberprovider.New(uberprovider.ProviderConfig{
		WebhookSecret: "secret",
	})

	headers := http.Header{}
	headers.Set("X-Uber-Signature", "bad")

	_, err := provider.ParseWebhookEvent(headers, []byte(`{"event_type":"requests.status_changed"}`))
	require.ErrorIs(t, err, providers.ErrInvalidWebhookSignature)
}
