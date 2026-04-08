package providers_test

import (
	"testing"

	"github.com/charliewilco/argus/providers"
	"github.com/charliewilco/argus/providers/github"
	"github.com/charliewilco/argus/providers/spotify"
	"github.com/stretchr/testify/require"
)

func TestRegistryMetadataIsSorted(t *testing.T) {
	t.Parallel()

	registry, err := providers.NewRegistry(
		spotify.New(spotify.ProviderConfig{}),
		github.New(github.ProviderConfig{}),
	)
	require.NoError(t, err)

	metadata := registry.Metadata()
	require.Len(t, metadata, 2)
	require.Equal(t, "github", metadata[0].ID)
	require.Equal(t, "spotify", metadata[1].ID)
}
