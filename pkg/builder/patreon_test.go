package builder

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/mxpv/podsync/pkg/feed"
	"github.com/mxpv/podsync/pkg/model"
	"github.com/mxpv/podsync/pkg/patreon"
)

func TestParseURL_Patreon(t *testing.T) {
	tests := []struct {
		url  string
		kind model.Type
		id   string
	}{
		{"https://www.patreon.com/answermethis", model.TypeChannel, "answermethis"},
		{"https://patreon.com/answermethis/", model.TypeChannel, "answermethis"},
		{"https://www.patreon.com/answermethis/posts", model.TypeChannel, "answermethis"},
		{"https://www.patreon.com/c/answermethis", model.TypeChannel, "answermethis"},
		{"https://www.patreon.com/cw/answermethis", model.TypeChannel, "answermethis"},
		{"www.patreon.com/cw/answermethis/posts", model.TypeChannel, "answermethis"},
		{"https://www.patreon.com/collection/1349741", model.TypePlaylist, "1349741"},
		{"https://www.patreon.com/collection/1545734?view=expanded", model.TypePlaylist, "1545734"},
	}

	for _, tst := range tests {
		t.Run(tst.url, func(t *testing.T) {
			info, err := ParseURL(tst.url)
			require.NoError(t, err)
			assert.Equal(t, model.ProviderPatreon, info.Provider)
			assert.Equal(t, tst.kind, info.LinkType)
			assert.Equal(t, tst.id, info.ItemID)
		})
	}
}

func TestParseURL_PatreonInvalid(t *testing.T) {
	for _, link := range []string{
		"https://www.patreon.com/",
		"https://www.patreon.com/posts/podsync-is-down-34630503",
		"https://www.patreon.com/answermethis/collections",
		"https://www.patreon.com/answermethis/about",
		"https://www.patreon.com/collection/",
		"https://www.patreon.com/collection/abc",
		"https://www.patreon.com/home",
		"https://www.patreon.com/cw/",
		"https://notpatreon.com/answermethis",
	} {
		t.Run(link, func(t *testing.T) {
			_, err := ParseURL(link)
			assert.Error(t, err)
		})
	}
}

func TestStripHTML(t *testing.T) {
	assert.Equal(t, "Hello & welcome\nSecond line", stripHTML("<p>Hello &amp; welcome</p><p>Second <b>line</b></p>"))
	assert.Equal(t, "", stripHTML(""))
}

const patreonTestCookies = "#HttpOnly_.patreon.com\tTRUE\t/\tTRUE\t1826288000\tsession_id\tgood\n"

func fakePatreonAPI(t *testing.T) (*httptest.Server, *url.Values) {
	t.Helper()

	var postsQuery url.Values
	mux := http.NewServeMux()
	mux.HandleFunc("/current_user", func(w http.ResponseWriter, r *http.Request) {
		if !strings.Contains(r.Header.Get("Cookie"), "session_id=good") {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		_, _ = w.Write([]byte(`{"data":{"id":"1","type":"user"}}`))
	})
	mux.HandleFunc("/campaigns", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"data":[{"id":"13359703","type":"campaign","attributes":{"name":"Answer Me This!",
			"summary":"<p>A podcast</p>","url":"https://www.patreon.com/answermethis","avatar_photo_url":"https://img/avatar.png"}}]}`))
	})
	mux.HandleFunc("/campaigns/13359703", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"data":{"id":"13359703","type":"campaign","attributes":{"name":"Answer Me This!",
			"avatar_photo_url":"https://img/avatar.png"}}}`))
	})
	mux.HandleFunc("/collection/1349741", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"data":{"id":"1349741","type":"collection","attributes":{"title":"Bonus bits",
			"description":"Extra bits","thumbnail":{"default_large":"https://img/collection.png"}},
			"relationships":{"campaign":{"data":{"id":"13359703","type":"campaign"}}}}}`))
	})
	mux.HandleFunc("/posts", func(w http.ResponseWriter, r *http.Request) {
		postsQuery = r.URL.Query()
		_, _ = w.Write([]byte(`{"data":[
			{"id":"174464677","type":"post","attributes":{"title":"AMT423 bonus bits","post_type":"audio_file",
			 "published_at":"2026-11-01T10:00:00.000+00:00","current_user_can_view":true,
			 "url":"https://www.patreon.com/posts/amt423-174464677","content":"<p>Bits</p>","post_file":{"duration":360.0}}},
			{"id":"174191584","type":"post","attributes":{"title":"Transcript","post_type":"image_file",
			 "published_at":"2026-10-30T10:00:00.000+00:00","current_user_can_view":true}},
			{"id":"173759134","type":"post","attributes":{"title":"Higher tier","post_type":"podcast",
			 "published_at":"2026-10-20T10:00:00.000+00:00","current_user_can_view":false,
			 "url":"https://www.patreon.com/posts/higher-173759134"}}
		],"links":{}}`))
	})

	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv, &postsQuery
}

func writeCookies(t *testing.T, content string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "cookies.txt")
	require.NoError(t, os.WriteFile(path, []byte(content), 0o600))
	return path
}

func TestPatreonBuilder_Collection(t *testing.T) {
	srv, postsQuery := fakePatreonAPI(t)
	b, err := NewPatreonBuilder(patreon.WithBaseURL(srv.URL))
	require.NoError(t, err)

	result, err := b.Build(context.Background(), &feed.Config{
		URL:         "https://www.patreon.com/collection/1349741",
		Format:      model.FormatAudio,
		PageSize:    10,
		CookiesFile: writeCookies(t, patreonTestCookies),
	})
	require.NoError(t, err)

	assert.Equal(t, "13359703", postsQuery.Get("filter[campaign_id]"))
	assert.Equal(t, "1349741", postsQuery.Get("filter[collection_id]"))

	assert.Equal(t, "Bonus bits", result.Title)
	assert.Equal(t, "Extra bits", result.Description)
	assert.Equal(t, "Answer Me This!", result.Author)
	assert.Equal(t, "https://img/collection.png", result.CoverArt)
	assert.Equal(t, "https://www.patreon.com/collection/1349741", result.ItemURL)
	assert.Equal(t, model.TypePlaylist, result.LinkType)

	require.Len(t, result.Episodes, 2, "image_file post is skipped")

	ep := result.Episodes[0]
	assert.Equal(t, "174464677", ep.ID)
	assert.Equal(t, "Bits", ep.Description)
	assert.Equal(t, int64(360), ep.Duration)
	assert.Equal(t, int64(360*patreonAudioBytesPerSecond), ep.Size)
	assert.Equal(t, "https://www.patreon.com/posts/amt423-174464677", ep.VideoURL)
	assert.Equal(t, "https://img/collection.png", ep.Thumbnail)
	assert.False(t, ep.Unavailable)

	assert.Equal(t, "173759134", result.Episodes[1].ID)
	assert.True(t, result.Episodes[1].Unavailable, "locked posts are reported, not dropped")
}

func TestPatreonBuilder_Campaign(t *testing.T) {
	srv, postsQuery := fakePatreonAPI(t)
	b, err := NewPatreonBuilder(patreon.WithBaseURL(srv.URL))
	require.NoError(t, err)

	result, err := b.Build(context.Background(), &feed.Config{
		URL:         "https://www.patreon.com/cw/answermethis",
		Format:      model.FormatVideo,
		CookiesFile: writeCookies(t, patreonTestCookies),
	})
	require.NoError(t, err)

	assert.Equal(t, "13359703", postsQuery.Get("filter[campaign_id]"))
	assert.Empty(t, postsQuery.Get("filter[collection_id]"))
	assert.Equal(t, "50", postsQuery.Get("page[count]"), "default page size")

	assert.Equal(t, "Answer Me This!", result.Title)
	assert.Equal(t, "A podcast", result.Description)
	assert.Equal(t, "https://img/avatar.png", result.CoverArt)
	assert.Equal(t, model.TypeChannel, result.LinkType)
	require.Len(t, result.Episodes, 2)
	assert.Equal(t, int64(360*patreonVideoBytesPerSecond), result.Episodes[0].Size)
}

func TestPatreonBuilder_ExpiredSession(t *testing.T) {
	srv, _ := fakePatreonAPI(t)
	b, err := NewPatreonBuilder(patreon.WithBaseURL(srv.URL))
	require.NoError(t, err)

	cookies := writeCookies(t, "#HttpOnly_.patreon.com\tTRUE\t/\tTRUE\t1826288000\tsession_id\texpired\n")
	_, err = b.Build(context.Background(), &feed.Config{
		URL:         "https://www.patreon.com/collection/1349741",
		CookiesFile: cookies,
	})
	require.Error(t, err)
	assert.True(t, errors.Is(err, model.ErrUnauthorized))
	assert.Contains(t, err.Error(), cookies)
}

func TestPatreonBuilder_NoSessionCookie(t *testing.T) {
	b, err := NewPatreonBuilder()
	require.NoError(t, err)

	_, err = b.Build(context.Background(), &feed.Config{
		URL:         "https://www.patreon.com/collection/1349741",
		CookiesFile: writeCookies(t, "www.patreon.com\tFALSE\t/\tTRUE\t0\tpatreon_locale_code\ten-US\n"),
	})
	require.Error(t, err)
	assert.True(t, errors.Is(err, model.ErrUnauthorized))
}

func TestPatreonBuilder_RereadsCookiesEveryBuild(t *testing.T) {
	srv, _ := fakePatreonAPI(t)
	b, err := NewPatreonBuilder(patreon.WithBaseURL(srv.URL))
	require.NoError(t, err)

	cfg := &feed.Config{
		URL:         "https://www.patreon.com/collection/1349741",
		CookiesFile: writeCookies(t, patreonTestCookies),
	}
	_, err = b.Build(context.Background(), cfg)
	require.NoError(t, err)

	require.NoError(t, os.WriteFile(cfg.CookiesFile, []byte("#HttpOnly_.patreon.com\tTRUE\t/\tTRUE\t1826288000\tsession_id\trevoked\n"), 0o600))
	_, err = b.Build(context.Background(), cfg)
	assert.True(t, errors.Is(err, model.ErrUnauthorized), "a replaced cookies file is used without restarting")

	require.NoError(t, os.WriteFile(cfg.CookiesFile, []byte(patreonTestCookies), 0o600))
	_, err = b.Build(context.Background(), cfg)
	assert.NoError(t, err)
}

func TestPatreonBuilder_MissingCookiesFile(t *testing.T) {
	b, err := NewPatreonBuilder()
	require.NoError(t, err)

	_, err = b.Build(context.Background(), &feed.Config{
		URL:         "https://www.patreon.com/collection/1349741",
		CookiesFile: filepath.Join(t.TempDir(), "missing.txt"),
	})
	assert.Error(t, err)
}
