package web

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/mxpv/podsync/pkg/db"
	"github.com/mxpv/podsync/pkg/feed"
)

type staticHealth map[string]feed.Health

func (s staticHealth) FeedHealth() map[string]feed.Health { return s }

func TestHealthCheck_FeedProblems(t *testing.T) {
	database, err := db.NewBadger(&db.Config{Dir: t.TempDir()})
	require.NoError(t, err)
	defer database.Close()

	srv := New(Config{}, &mockFileSystem{}, database)

	check := func() (int, HealthStatus) {
		rec := httptest.NewRecorder()
		srv.healthCheckHandler(rec, httptest.NewRequest(http.MethodGet, "/health", nil))

		var status HealthStatus
		require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &status))
		return rec.Code, status
	}

	code, status := check()
	assert.Equal(t, http.StatusOK, code, "no reporter configured")
	assert.Equal(t, "healthy", status.Status)

	now := time.Now()
	srv.SetHealthReporter(staticHealth{
		"ok":      {UpdatedAt: now},
		"patreon": {UpdatedAt: now, Error: "patreon: session expired"},
	})

	code, status = check()
	assert.Equal(t, http.StatusServiceUnavailable, code)
	assert.Equal(t, "unhealthy", status.Status)
	require.Contains(t, status.Feeds, "patreon")
	assert.NotContains(t, status.Feeds, "ok")
	assert.Equal(t, "patreon: session expired", status.Feeds["patreon"].Error)

	srv.SetHealthReporter(staticHealth{"warn": {UpdatedAt: now, Warning: "cookie expires soon"}})
	code, _ = check()
	assert.Equal(t, http.StatusServiceUnavailable, code, "warnings need attention too")
}
