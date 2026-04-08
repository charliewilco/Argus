package stripe_test

import (
	"net/http"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/charliewilco/argus/providers"
	stripeprovider "github.com/charliewilco/argus/providers/stripe"
)

func TestProviderID(t *testing.T) {
	t.Parallel()

	provider := stripeprovider.New(stripeprovider.ProviderConfig{})
	require.Equal(t, "stripe", provider.ID())
}

func TestParseWebhookEventRejectsInvalidSignature(t *testing.T) {
	t.Parallel()

	provider := stripeprovider.New(stripeprovider.ProviderConfig{
		WebhookSecret: "secret",
	})

	headers := http.Header{}
	headers.Set("Stripe-Signature", "t=123,v1=bad")

	_, err := provider.ParseWebhookEvent(headers, []byte(`{"type":"invoice.paid"}`))
	require.ErrorIs(t, err, providers.ErrInvalidWebhookSignature)
}
