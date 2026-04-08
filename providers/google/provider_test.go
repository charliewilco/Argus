package google_test

import (
	"net/http"
	"testing"

	"github.com/stretchr/testify/require"

	googleprovider "github.com/charliewilco/argus/providers/google"
)

func TestProviderID(t *testing.T) {
	t.Parallel()

	provider := googleprovider.New(googleprovider.ProviderConfig{})
	require.Equal(t, "google", provider.ID())
}

func TestParseWebhookEventParsesHeaders(t *testing.T) {
	t.Parallel()

	provider := googleprovider.New(googleprovider.ProviderConfig{})

	headers := http.Header{}
	headers.Set("X-Goog-Channel-ID", "channel_123")
	headers.Set("X-Goog-Resource-ID", "resource_123")
	headers.Set("X-Goog-Resource-URI", "https://www.googleapis.com/drive/v3/files/1")
	headers.Set("X-Goog-Resource-State", "update")
	headers.Set("X-Goog-Changed", "content,permissions")
	headers.Set("X-Goog-Message-Number", "9")

	event, err := provider.ParseWebhookEvent(headers, nil)
	require.NoError(t, err)
	require.Equal(t, "google.update", event.TriggerKey)
	require.Equal(t, []string{"content", "permissions"}, event.Normalized["changed"])
}
