package update

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/mxpv/podsync/pkg/feed"
	"github.com/mxpv/podsync/pkg/model"
)

type imageServer struct {
	*httptest.Server

	mu       sync.Mutex
	requests map[string]int
	failing  bool
}

func newImageServer(t *testing.T) *imageServer {
	t.Helper()

	s := &imageServer{requests: map[string]int{}}
	s.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		s.mu.Lock()
		s.requests[r.URL.Path]++
		failing := s.failing
		s.mu.Unlock()

		switch {
		case failing:
			w.WriteHeader(http.StatusInternalServerError)
		case r.URL.Path == "/text":
			w.Header().Set("Content-Type", "text/html")
			_, _ = w.Write([]byte("<html></html>"))
		default:
			w.Header().Set("Content-Type", "image/jpeg; charset=binary")
			_, _ = w.Write([]byte("image " + r.URL.Path))
		}
	}))
	t.Cleanup(s.Close)
	return s
}

func (s *imageServer) count(path string) int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.requests[path]
}

func (s *imageServer) setFailing(failing bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.failing = failing
}

func (e *testEnv) artworkFiles(t *testing.T) []string {
	t.Helper()

	matches, err := filepath.Glob(filepath.Join(e.dataDir, e.feed.ID, artworkDir, "*"))
	require.NoError(t, err)
	for i, match := range matches {
		matches[i] = artworkDir + "/" + filepath.Base(match)
	}
	return matches
}

func withThumbnail(e *model.Episode, url string) *model.Episode {
	e.Thumbnail = url
	return e
}

func TestUpdate_MirrorsArtwork(t *testing.T) {
	images := newImageServer(t)
	env := newTestEnv(t)

	env.builder.coverArt = images.URL + "/cover.png?token-hash=a&token-time=1"
	env.builder.episodes = []*model.Episode{
		withThumbnail(episode("1", false), images.URL+"/one.jpeg?token-hash=a&token-time=1"),
		withThumbnail(episode("2", false), images.URL+"/cover.png?token-hash=b&token-time=1"),
		withThumbnail(episode("3", false), images.URL+"/no-extension"),
	}
	require.NoError(t, env.manager.Update(context.Background(), env.feed))

	stored, err := env.db.GetFeed(context.Background(), env.feed.ID)
	require.NoError(t, err)
	episodes := env.episodes(t)

	assert.Regexp(t, `^artwork/[0-9a-f]{32}\.png$`, stored.CoverArtFile)
	assert.Regexp(t, `^artwork/[0-9a-f]{32}\.jpg$`, episodes["1"].ThumbnailFile)
	assert.Equal(t, stored.CoverArtFile, episodes["2"].ThumbnailFile, "signing parameters don't change the file")
	assert.Regexp(t, `^artwork/[0-9a-f]{32}\.jpg$`, episodes["3"].ThumbnailFile, "extension comes from the content type")
	assert.Len(t, env.artworkFiles(t), 3)
	assert.Equal(t, 1, images.count("/cover.png"))

	data, err := os.ReadFile(filepath.Join(env.dataDir, env.feed.ID, filepath.FromSlash(episodes["1"].ThumbnailFile)))
	require.NoError(t, err)
	assert.Equal(t, "image /one.jpeg", string(data))

	env.builder.coverArt = images.URL + "/cover.png?token-hash=c&token-time=2"
	require.NoError(t, env.manager.Update(context.Background(), env.feed))
	assert.Equal(t, 1, images.count("/cover.png"), "stored images are not downloaded again")
	assert.Equal(t, 1, images.count("/one.jpeg"))

	xml, err := os.ReadFile(filepath.Join(env.dataDir, env.feed.ID+".xml"))
	require.NoError(t, err)
	assert.Contains(t, string(xml), "http://localhost/amt/"+stored.CoverArtFile)
	assert.Contains(t, string(xml), "http://localhost/amt/"+episodes["1"].ThumbnailFile)
	assert.NotContains(t, string(xml), "token-time")
}

func TestUpdate_ArtworkFailureFallsBackToRemoteURL(t *testing.T) {
	images := newImageServer(t)
	env := newTestEnv(t)

	thumbnail := images.URL + "/one.jpg"
	env.builder.episodes = []*model.Episode{
		withThumbnail(episode("1", false), thumbnail),
		withThumbnail(episode("2", false), images.URL+"/text"),
	}

	images.setFailing(true)
	require.NoError(t, env.manager.Update(context.Background(), env.feed))
	assert.Empty(t, env.episodes(t)["1"].ThumbnailFile)
	assert.Empty(t, env.artworkFiles(t))

	xml, err := os.ReadFile(filepath.Join(env.dataDir, env.feed.ID+".xml"))
	require.NoError(t, err)
	assert.Contains(t, string(xml), thumbnail)

	images.setFailing(false)
	require.NoError(t, env.manager.Update(context.Background(), env.feed))
	episodes := env.episodes(t)
	assert.NotEmpty(t, episodes["1"].ThumbnailFile, "retried on the next update")
	assert.Empty(t, episodes["2"].ThumbnailFile, "non-image responses are rejected")
	assert.Len(t, env.artworkFiles(t), 1)
}

func TestUpdate_UnusedArtworkIsDeleted(t *testing.T) {
	images := newImageServer(t)
	env := newTestEnv(t)
	env.feed.Clean = &feed.Cleanup{KeepLast: 1}
	env.downloader.errs["4"] = assert.AnError

	newest := episode("1", false)
	oldest := episode("2", false)
	oldest.PubDate = newest.PubDate.Add(-48 * time.Hour)
	shared := episode("3", false)
	shared.PubDate = newest.PubDate.Add(-24 * time.Hour)

	env.builder.coverArt = images.URL + "/cover.png"
	env.builder.episodes = []*model.Episode{
		withThumbnail(newest, images.URL+"/newest.jpg"),
		withThumbnail(oldest, images.URL+"/oldest.jpg"),
		withThumbnail(shared, images.URL+"/cover.png"),
		withThumbnail(episode("4", false), images.URL+"/never-downloaded.jpg"),
	}
	require.NoError(t, env.manager.Update(context.Background(), env.feed))

	episodes := env.episodes(t)
	assert.Equal(t, model.EpisodeCleaned, episodes["2"].Status)
	assert.Empty(t, episodes["2"].ThumbnailFile)
	assert.Equal(t, model.EpisodeCleaned, episodes["3"].Status)

	stored, err := env.db.GetFeed(context.Background(), env.feed.ID)
	require.NoError(t, err)
	assert.ElementsMatch(t, []string{stored.CoverArtFile, episodes["1"].ThumbnailFile, episodes["4"].ThumbnailFile},
		env.artworkFiles(t), "cleaned episodes' artwork is deleted unless the cover still uses it")

	env.builder.episodes = env.builder.episodes[:3]
	require.NoError(t, env.manager.Update(context.Background(), env.feed))
	assert.NotContains(t, env.episodes(t), "4")
	assert.ElementsMatch(t, []string{stored.CoverArtFile, episodes["1"].ThumbnailFile}, env.artworkFiles(t),
		"artwork of removed episodes is deleted")

	env.builder.coverArt = images.URL + "/new-cover.png"
	require.NoError(t, env.manager.Update(context.Background(), env.feed))
	stored, err = env.db.GetFeed(context.Background(), env.feed.ID)
	require.NoError(t, err)
	assert.ElementsMatch(t, []string{stored.CoverArtFile, episodes["1"].ThumbnailFile}, env.artworkFiles(t),
		"a replaced cover is deleted")
}
