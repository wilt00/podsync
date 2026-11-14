package update

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	log "github.com/sirupsen/logrus"
	logtest "github.com/sirupsen/logrus/hooks/test"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/mxpv/podsync/pkg/feed"
)

func TestCredentialsWarning(t *testing.T) {
	now := time.Date(2027, 11, 1, 0, 0, 0, 0, time.UTC)

	cookiesExpiringIn := func(d time.Duration) string {
		path := filepath.Join(t.TempDir(), "cookies.txt")
		line := fmt.Sprintf("#HttpOnly_.patreon.com\tTRUE\t/\tTRUE\t%d\tsession_id\tvalue\n", now.Add(d).Unix())
		require.NoError(t, os.WriteFile(path, []byte(line), 0o600))
		return path
	}

	patreonFeed := func(cookies string) *feed.Config {
		return &feed.Config{ID: "p", URL: "https://www.patreon.com/collection/1", CookiesFile: cookies}
	}

	assert.Empty(t, credentialsWarning(patreonFeed(""), now), "no cookies configured")
	assert.Empty(t, credentialsWarning(patreonFeed(cookiesExpiringIn(60*24*time.Hour)), now), "far from expiry")
	assert.Empty(t, credentialsWarning(patreonFeed(cookiesExpiringIn(20*24*time.Hour)), now), "only logged")
	assert.Contains(t, credentialsWarning(patreonFeed(cookiesExpiringIn(3*24*time.Hour)), now), "expires on 2027-11-04")
	assert.Empty(t, credentialsWarning(patreonFeed(cookiesExpiringIn(-time.Hour)), now), "expired is an update error")

	youtube := &feed.Config{URL: "https://www.youtube.com/channel/UC5XPnUk8Vvv_pWslhwom6Og", CookiesFile: cookiesExpiringIn(time.Hour)}
	assert.Empty(t, credentialsWarning(youtube, now), "only patreon session cookies are checked")
}

func TestCredentialsWarning_UnreadableCookiesLeftToBuilder(t *testing.T) {
	cfg := &feed.Config{URL: "https://www.patreon.com/collection/1", CookiesFile: filepath.Join(t.TempDir(), "missing.txt")}
	assert.Empty(t, credentialsWarning(cfg, time.Now()))
}

func TestCredentialsWarning_LogsWithinWarningPeriod(t *testing.T) {
	hook := logtest.NewGlobal()
	defer hook.Reset()

	now := time.Now()
	path := filepath.Join(t.TempDir(), "cookies.txt")
	line := fmt.Sprintf(".patreon.com\tTRUE\t/\tTRUE\t%d\tsession_id\tvalue\n", now.Add(20*24*time.Hour).Unix())
	require.NoError(t, os.WriteFile(path, []byte(line), 0o600))

	assert.Empty(t, credentialsWarning(&feed.Config{URL: "https://www.patreon.com/collection/1", CookiesFile: path}, now))
	require.NotNil(t, hook.LastEntry())
	assert.Equal(t, log.WarnLevel, hook.LastEntry().Level)
}
