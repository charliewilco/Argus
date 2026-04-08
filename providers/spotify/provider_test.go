package spotify_test

import (
	"net/http"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/charliewilco/argus/providers"
	spotifyprovider "github.com/charliewilco/argus/providers/spotify"
)

func TestProviderID(t *testing.T) {
	t.Parallel()

	provider := spotifyprovider.New(spotifyprovider.ProviderConfig{})
	require.Equal(t, "spotify", provider.ID())
}

func TestParseWebhookEventReturnsUnsupported(t *testing.T) {
	t.Parallel()

	provider := spotifyprovider.New(spotifyprovider.ProviderConfig{})

	_, err := provider.ParseWebhookEvent(http.Header{}, nil)
	require.ErrorIs(t, err, providers.ErrWebhooksNotSupported)
}
