package model

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestEpisode_UnavailableIsNotPersisted(t *testing.T) {
	data, err := json.Marshal(&Episode{ID: "1", Unavailable: true})
	require.NoError(t, err)
	assert.NotContains(t, string(data), "navailable")

	var decoded Episode
	require.NoError(t, json.Unmarshal([]byte(`{"id":"1","Unavailable":true}`), &decoded))
	assert.False(t, decoded.Unavailable)
}
