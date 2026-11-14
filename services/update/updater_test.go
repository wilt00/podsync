package update

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/mxpv/podsync/pkg/builder"
	"github.com/mxpv/podsync/pkg/db"
	"github.com/mxpv/podsync/pkg/feed"
	"github.com/mxpv/podsync/pkg/fs"
	"github.com/mxpv/podsync/pkg/model"
	"github.com/mxpv/podsync/pkg/ytdl"
)

type fakeBuilder struct {
	episodes []*model.Episode
	err      error
}

func (b *fakeBuilder) Build(_ context.Context, cfg *feed.Config) (*model.Feed, error) {
	if b.err != nil {
		return nil, b.err
	}

	episodes := make([]*model.Episode, 0, len(b.episodes))
	for _, e := range b.episodes {
		c := *e
		episodes = append(episodes, &c)
	}

	return &model.Feed{
		ID:       cfg.ID,
		Title:    "Test feed",
		Format:   cfg.Format,
		Provider: model.ProviderPatreon,
		Episodes: episodes,
	}, nil
}

type fakeDownloader struct {
	errs      map[string]error
	downloads []string
}

func (d *fakeDownloader) Download(_ context.Context, _ *feed.Config, episode *model.Episode) (io.ReadCloser, error) {
	if err, ok := d.errs[episode.ID]; ok {
		return nil, err
	}
	d.downloads = append(d.downloads, episode.ID)
	return io.NopCloser(strings.NewReader("audio")), nil
}

func (d *fakeDownloader) PlaylistMetadata(context.Context, string) (ytdl.PlaylistMetadata, error) {
	return ytdl.PlaylistMetadata{}, nil
}

type testEnv struct {
	manager    *Manager
	db         *db.Badger
	builder    *fakeBuilder
	downloader *fakeDownloader
	feed       *feed.Config
	providers  []model.Provider
}

func newTestEnv(t *testing.T) *testEnv {
	t.Helper()

	database, err := db.NewBadger(&db.Config{Dir: t.TempDir()})
	require.NoError(t, err)
	t.Cleanup(func() { _ = database.Close() })

	storage, err := fs.NewLocal(t.TempDir(), false, false)
	require.NoError(t, err)

	feedConfig := &feed.Config{
		ID:       "amt",
		URL:      "https://www.patreon.com/collection/1349741",
		Format:   model.FormatAudio,
		PageSize: 10,
	}

	env := &testEnv{
		db:         database,
		builder:    &fakeBuilder{},
		downloader: &fakeDownloader{errs: map[string]error{}},
		feed:       feedConfig,
	}

	manager, err := NewUpdater(map[string]*feed.Config{"amt": feedConfig}, map[model.Provider]feed.KeyProvider{},
		"http://localhost", env.downloader, database, storage)
	require.NoError(t, err)

	manager.newBuilder = func(_ context.Context, provider model.Provider, _ string, _ builder.Downloader) (builder.Builder, error) {
		env.providers = append(env.providers, provider)
		return env.builder, nil
	}

	env.manager = manager
	return env
}

func (e *testEnv) episodes(t *testing.T) map[string]*model.Episode {
	t.Helper()

	result := map[string]*model.Episode{}
	require.NoError(t, e.db.WalkEpisodes(context.Background(), e.feed.ID, func(episode *model.Episode) error {
		result[episode.ID] = episode
		return nil
	}))
	return result
}

func episode(id string, unavailable bool) *model.Episode {
	return &model.Episode{
		ID:          id,
		Title:       "Episode " + id,
		PubDate:     time.Now().Add(-time.Hour),
		Status:      model.EpisodeNew,
		Unavailable: unavailable,
	}
}

func TestUpdate_KeylessProviderAndUnavailableEpisodes(t *testing.T) {
	env := newTestEnv(t)
	env.builder.episodes = []*model.Episode{episode("1", false), episode("2", true)}

	require.NoError(t, env.manager.Update(context.Background(), env.feed))
	assert.Equal(t, []model.Provider{model.ProviderPatreon}, env.providers)

	stored := env.episodes(t)
	require.Contains(t, stored, "1")
	assert.Equal(t, model.EpisodeDownloaded, stored["1"].Status)
	assert.NotContains(t, stored, "2", "inaccessible episodes must not be stored or downloaded")
	assert.Equal(t, []string{"1"}, env.downloader.downloads)

	assert.True(t, env.manager.FeedHealth()["amt"].Healthy())
}

func TestUpdate_AccessLostToDownloadedEpisode(t *testing.T) {
	env := newTestEnv(t)
	env.builder.episodes = []*model.Episode{episode("1", false)}
	require.NoError(t, env.manager.Update(context.Background(), env.feed))

	env.builder.episodes = []*model.Episode{episode("1", true), episode("3", false)}
	err := env.manager.Update(context.Background(), env.feed)
	require.Error(t, err)
	assert.True(t, errors.Is(err, model.ErrAccessLost))

	health := env.manager.FeedHealth()["amt"]
	assert.False(t, health.Healthy())
	assert.Contains(t, health.Error, "no longer accessible")

	assert.NotContains(t, env.episodes(t), "3", "nothing new is stored while access is lost")
}

func TestUpdate_BuilderUnauthorizedIsRecorded(t *testing.T) {
	env := newTestEnv(t)
	env.builder.err = model.ErrUnauthorized

	err := env.manager.Update(context.Background(), env.feed)
	require.Error(t, err)
	assert.True(t, errors.Is(err, model.ErrUnauthorized))
	assert.NotEmpty(t, env.manager.FeedHealth()["amt"].Error)

	env.builder.err = nil
	require.NoError(t, env.manager.Update(context.Background(), env.feed))
	assert.True(t, env.manager.FeedHealth()["amt"].Healthy(), "recovers once the cookies are fixed")
}

func TestUpdate_NotYetAvailableIsDeferred(t *testing.T) {
	env := newTestEnv(t)
	env.builder.episodes = []*model.Episode{episode("live", false)}
	env.downloader.errs["live"] = ytdl.ErrNotYetAvailable

	require.NoError(t, env.manager.Update(context.Background(), env.feed))

	stored := env.episodes(t)
	require.Contains(t, stored, "live")
	assert.Equal(t, model.EpisodeNew, stored["live"].Status, "upcoming streams stay queued, not failed")
	assert.True(t, env.manager.FeedHealth()["amt"].Healthy())

	delete(env.downloader.errs, "live")
	require.NoError(t, env.manager.Update(context.Background(), env.feed))
	assert.Equal(t, model.EpisodeDownloaded, env.episodes(t)["live"].Status, "downloads once the stream is over")
}

func TestUpdate_AccessDeniedDownloadWarns(t *testing.T) {
	env := newTestEnv(t)
	env.builder.episodes = []*model.Episode{episode("1", false)}
	env.downloader.errs["1"] = ytdl.ErrAccessDenied

	require.NoError(t, env.manager.Update(context.Background(), env.feed))

	assert.Equal(t, model.EpisodeError, env.episodes(t)["1"].Status)
	health := env.manager.FeedHealth()["amt"]
	assert.Empty(t, health.Error)
	assert.Contains(t, health.Warning, "denied access")
}

func TestUpdate_RequiredKeyMissing(t *testing.T) {
	env := newTestEnv(t)
	env.feed.URL = "https://www.youtube.com/channel/UC5XPnUk8Vvv_pWslhwom6Og"

	err := env.manager.Update(context.Background(), env.feed)
	require.Error(t, err)
	assert.Contains(t, err.Error(), `key provider "youtube" not loaded`)
}

// TestHookHelperProcess is not a real test: it is executed as a hook command by other tests.
func TestHookHelperProcess(t *testing.T) {
	if os.Getenv("PODSYNC_HOOK_HELPER") != "1" {
		return
	}

	args := os.Args
	for i, arg := range args {
		if arg == "--" {
			args = args[i+1:]
			break
		}
	}

	out := fmt.Sprintf("title=%s\nerror=%s\n", os.Getenv("EPISODE_TITLE"), os.Getenv("ERROR_MESSAGE"))
	if err := os.WriteFile(args[0], []byte(out), 0o600); err != nil {
		os.Exit(2)
	}
	os.Exit(0)
}

func TestUpdate_LostCredentialsRunErrorHooks(t *testing.T) {
	t.Setenv("PODSYNC_HOOK_HELPER", "1")

	for _, buildErr := range []error{model.ErrUnauthorized, model.ErrAccessLost} {
		t.Run(buildErr.Error(), func(t *testing.T) {
			env := newTestEnv(t)
			out := filepath.Join(t.TempDir(), "hook.txt")
			env.feed.OnEpisodeDownloadError = []*feed.ExecHook{
				{Command: []string{os.Args[0], "-test.run=^TestHookHelperProcess$", "--", out}},
			}
			env.builder.err = buildErr

			require.Error(t, env.manager.Update(context.Background(), env.feed))

			data, err := os.ReadFile(out)
			require.NoError(t, err, "error hook must run when credentials are lost")
			assert.Contains(t, string(data), "title=\n")
			assert.Contains(t, string(data), "error=update failed: "+buildErr.Error())
		})
	}
}

func TestUpdate_OtherBuildErrorsDoNotRunHooks(t *testing.T) {
	t.Setenv("PODSYNC_HOOK_HELPER", "1")

	env := newTestEnv(t)
	out := filepath.Join(t.TempDir(), "hook.txt")
	env.feed.OnEpisodeDownloadError = []*feed.ExecHook{
		{Command: []string{os.Args[0], "-test.run=^TestHookHelperProcess$", "--", out}},
	}
	env.builder.err = errors.New("api is down")

	require.Error(t, env.manager.Update(context.Background(), env.feed))
	assert.NoFileExists(t, out)
}
