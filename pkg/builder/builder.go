package builder

import (
	"context"

	"github.com/mxpv/podsync/pkg/feed"
	"github.com/pkg/errors"

	"github.com/mxpv/podsync/pkg/model"
)

type Builder interface {
	Build(ctx context.Context, cfg *feed.Config) (*model.Feed, error)
}

func RequiresKey(provider model.Provider) bool {
	switch provider {
	case model.ProviderSoundcloud, model.ProviderPatreon:
		return false
	default:
		return true
	}
}

// MirrorsArtwork reports whether a provider's artwork URLs expire, so podsync must serve its own copies.
func MirrorsArtwork(provider model.Provider) bool {
	return provider == model.ProviderPatreon
}

func New(ctx context.Context, provider model.Provider, key string, downloader Downloader) (Builder, error) {
	switch provider {
	case model.ProviderYoutube:
		return NewYouTubeBuilder(key, downloader)
	case model.ProviderVimeo:
		return NewVimeoBuilder(ctx, key)
	case model.ProviderSoundcloud:
		return NewSoundcloudBuilder()
	case model.ProviderTwitch:
		return NewTwitchBuilder(key)
	case model.ProviderPatreon:
		return NewPatreonBuilder()
	default:
		return nil, errors.Errorf("unsupported provider %q", provider)
	}
}
