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
