package update

import (
	"fmt"
	"time"

	log "github.com/sirupsen/logrus"

	"github.com/mxpv/podsync/pkg/builder"
	"github.com/mxpv/podsync/pkg/feed"
	"github.com/mxpv/podsync/pkg/model"
	"github.com/mxpv/podsync/pkg/patreon"
)

const (
	sessionExpiryWarning  = 30 * 24 * time.Hour
	sessionExpiryCritical = 7 * 24 * time.Hour
)

func (u *Manager) FeedHealth() map[string]feed.Health {
	u.healthMu.RLock()
	defer u.healthMu.RUnlock()

	result := make(map[string]feed.Health, len(u.health))
	for id, h := range u.health {
		result[id] = h
	}

	return result
}

func (u *Manager) recordHealth(feedID string, err error, warning string) {
	h := feed.Health{UpdatedAt: time.Now().UTC(), Warning: warning}
	if err != nil {
		h.Error = err.Error()
	}

	u.healthMu.Lock()
	defer u.healthMu.Unlock()

	if u.health == nil {
		u.health = make(map[string]feed.Health)
	}
	u.health[feedID] = h
}

func credentialsWarning(feedConfig *feed.Config, now time.Time) string {
	if feedConfig.CookiesFile == "" {
		return ""
	}

	info, err := builder.ParseURL(feedConfig.URL)
	if err != nil || info.Provider != model.ProviderPatreon {
		return ""
	}

	cookies, err := patreon.LoadCookiesFile(feedConfig.CookiesFile)
	if err != nil {
		return ""
	}

	expiry, ok := cookies.SessionExpiry()
	if !ok {
		return ""
	}

	left := expiry.Sub(now)
	logger := log.WithFields(log.Fields{"feed_id": feedConfig.ID, "expires": expiry.Format(time.RFC3339)})

	switch {
	case left <= 0:
		return ""
	case left <= sessionExpiryCritical:
		msg := fmt.Sprintf("patreon session cookie expires on %s, re-export cookies to %s",
			expiry.Format("2006-01-02"), feedConfig.CookiesFile)
		logger.Error(msg)
		return msg
	case left <= sessionExpiryWarning:
		logger.Warnf("patreon session cookie expires in %d day(s), re-export cookies to %s soon",
			int(left.Hours()/24), feedConfig.CookiesFile)
	}

	return ""
}

func runErrorHooks(feedConfig *feed.Config, episodeTitle string, err error) {
	if len(feedConfig.OnEpisodeDownloadError) == 0 {
		return
	}

	env := []string{
		"FEED_NAME=" + feedConfig.ID,
		"EPISODE_TITLE=" + episodeTitle,
		"ERROR_MESSAGE=" + err.Error(),
	}

	for i, hook := range feedConfig.OnEpisodeDownloadError {
		if hookErr := hook.Invoke(env); hookErr != nil {
			log.Errorf("failed to execute episode download error hook %d: %v", i+1, hookErr)
		} else {
			log.Infof("episode download error hook %d executed successfully", i+1)
		}
	}
}
