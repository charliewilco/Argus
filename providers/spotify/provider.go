package spotify

import (
	"context"
	"fmt"
	"net/http"

	providerapi "github.com/charliewilco/argus/providers"
	"golang.org/x/oauth2"
)

const (
	ProviderID                    = "spotify"
	AuthorizationURL              = "https://accounts.spotify.com/authorize"
	TokenURL                      = "https://accounts.spotify.com/api/token"
	ScopeUserReadPlaybackState    = "user-read-playback-state"
	ScopeUserModifyPlaybackState  = "user-modify-playback-state"
	ScopeUserReadCurrentlyPlaying = "user-read-currently-playing"
	ScopePlaylistReadPrivate      = "playlist-read-private"
	ScopePlaylistModifyPublic     = "playlist-modify-public"
	ScopePlaylistModifyPrivate    = "playlist-modify-private"
	ScopeUserLibraryRead          = "user-library-read"
	ScopeUserLibraryModify        = "user-library-modify"
)

var (
	defaultScopes = []string{
		ScopeUserReadPlaybackState,
		ScopeUserModifyPlaybackState,
		ScopeUserReadCurrentlyPlaying,
		ScopePlaylistReadPrivate,
		ScopePlaylistModifyPublic,
		ScopePlaylistModifyPrivate,
		ScopeUserLibraryRead,
		ScopeUserLibraryModify,
	}
	actions = []providerapi.Action{
		{Key: "play", Label: "Play"},
		{Key: "pause", Label: "Pause"},
		{Key: "skip", Label: "Skip"},
		{Key: "add_to_playlist", Label: "Add To Playlist"},
		{Key: "save_track", Label: "Save Track"},
	}
)

type ProviderConfig = providerapi.ProviderConfig

type Provider struct {
	oauthConfig *oauth2.Config
}

func New(config ProviderConfig) *Provider {
	return &Provider{
		oauthConfig: &oauth2.Config{
			ClientID:     config.ClientID,
			ClientSecret: config.ClientSecret,
			RedirectURL:  providerapi.RedirectURL(config, ProviderID),
			Scopes:       providerapi.EffectiveScopes(config, defaultScopes),
			Endpoint: oauth2.Endpoint{
				AuthURL:  AuthorizationURL,
				TokenURL: TokenURL,
			},
		},
	}
}

func (p *Provider) ID() string {
	return ProviderID
}

func (p *Provider) Metadata() providerapi.Metadata {
	return providerapi.Metadata{
		ID:                ProviderID,
		AuthorizationURL:  AuthorizationURL,
		TokenURL:          TokenURL,
		Scopes:            providerapi.CloneStrings(p.oauthConfig.Scopes),
		Actions:           providerapi.CloneActions(actions),
		WebhooksSupported: false,
	}
}

func (p *Provider) OAuthConfig() *oauth2.Config {
	cfg := *p.oauthConfig
	cfg.Scopes = providerapi.CloneStrings(p.oauthConfig.Scopes)
	return &cfg
}

func (p *Provider) ParseWebhookEvent(headers http.Header, body []byte) (*providerapi.WebhookEvent, error) {
	return nil, fmt.Errorf("spotify: %w", providerapi.ErrWebhooksNotSupported)
}

func (p *Provider) ExecuteAction(_ context.Context, _ *oauth2.Token, request providerapi.ActionRequest) (providerapi.ActionResult, error) {
	return providerapi.ActionResult{}, fmt.Errorf("spotify.ExecuteAction: %w %q", providerapi.ErrUnsupportedProviderAction, request.Action)
}
