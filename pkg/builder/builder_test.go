package builder

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/mxpv/podsync/pkg/model"
)

func TestRequiresKey(t *testing.T) {
	assert.True(t, RequiresKey(model.ProviderYoutube))
	assert.True(t, RequiresKey(model.ProviderVimeo))
	assert.True(t, RequiresKey(model.ProviderTwitch))
	assert.False(t, RequiresKey(model.ProviderSoundcloud))
	assert.False(t, RequiresKey(model.ProviderPatreon))
}

func TestMirrorsArtwork(t *testing.T) {
	assert.True(t, MirrorsArtwork(model.ProviderPatreon))
	assert.False(t, MirrorsArtwork(model.ProviderYoutube))
}
