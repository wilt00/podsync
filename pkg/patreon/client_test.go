package patreon

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const testCookies = "# Netscape HTTP Cookie File\n" +
	"www.patreon.com\tFALSE\t/\tTRUE\t1826288000\tpatreon_device_id\tdevice\n" +
	"#HttpOnly_.patreon.com\tTRUE\t/\tTRUE\t1826288000\tsession_id\tsecret-session\n" +
	".example.com\tTRUE\t/\tFALSE\t1826288000\tother\tdo-not-send\n" +
	"\n" +
	"malformed line\n"

func TestParseCookies(t *testing.T) {
	cookies, err := ParseCookies(strings.NewReader(testCookies))
	require.NoError(t, err)
	require.Len(t, cookies, 2)

	session, ok := cookies.Session()
	require.True(t, ok)
	assert.Equal(t, "secret-session", session.Value)
	assert.True(t, session.HttpOnly)

	expiry, ok := cookies.SessionExpiry()
	require.True(t, ok)
	assert.Equal(t, time.Unix(1826288000, 0).UTC(), expiry)

	assert.Equal(t, "patreon_device_id=device; session_id=secret-session", cookies.Header())
}

func TestParseCookies_NoSession(t *testing.T) {
	cookies, err := ParseCookies(strings.NewReader("www.patreon.com\tFALSE\t/\tTRUE\t0\tpatreon_locale_code\ten-US\n"))
	require.NoError(t, err)

	_, ok := cookies.Session()
	assert.False(t, ok)

	_, ok = cookies.SessionExpiry()
	assert.False(t, ok)
}

func TestParseCookies_WindowsLineEndings(t *testing.T) {
	cookies, err := ParseCookies(strings.NewReader(strings.ReplaceAll(testCookies, "\n", "\r\n")))
	require.NoError(t, err)

	session, ok := cookies.Session()
	require.True(t, ok)
	assert.Equal(t, "secret-session", session.Value)
}

func TestParseCookies_SessionWithoutExpiry(t *testing.T) {
	cookies, err := ParseCookies(strings.NewReader(".patreon.com\tTRUE\t/\tTRUE\t0\tsession_id\tbrowser-session\n"))
	require.NoError(t, err)

	_, ok := cookies.Session()
	assert.True(t, ok)

	_, ok = cookies.SessionExpiry()
	assert.False(t, ok)
}

func fakePatreon(t *testing.T) *httptest.Server {
	t.Helper()

	mux := http.NewServeMux()

	loggedIn := func(r *http.Request) bool {
		return strings.Contains(r.Header.Get("Cookie"), "session_id=secret-session")
	}

	mux.HandleFunc("/current_user", func(w http.ResponseWriter, r *http.Request) {
		if !loggedIn(r) {
			w.WriteHeader(http.StatusUnauthorized)
			_, _ = w.Write([]byte(`{"errors":[{"code":1,"status":"401"}]}`))
			return
		}
		_, _ = w.Write([]byte(`{"data":{"id":"42","type":"user","attributes":{"full_name":"Test"}}}`))
	})

	mux.HandleFunc("/campaigns", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("filter[vanity]") != "answermethis" {
			_, _ = w.Write([]byte(`{"data":[]}`))
			return
		}
		_, _ = w.Write([]byte(`{"data":[{"id":"13359703","type":"campaign","attributes":{
			"name":"Answer Me This!","summary":"<p>Podcast</p>","url":"https://www.patreon.com/answermethis",
			"avatar_photo_url":"https://img/avatar.png","image_url":"https://img/banner.png"}}]}`))
	})

	mux.HandleFunc("/campaigns/13359703", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"data":{"id":"13359703","type":"campaign","attributes":{"name":"Answer Me This!"}}}`))
	})

	mux.HandleFunc("/collection/1349741", func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "campaign", r.URL.Query().Get("include"))
		_, _ = w.Write([]byte(`{"data":{"id":"1349741","type":"collection",
			"attributes":{"title":"Bonus bits","description":"Extra bits","thumbnail":{"original":"https://img/orig.png","default_large":"https://img/large.png"}},
			"relationships":{"campaign":{"data":{"id":"13359703","type":"campaign"}}}}}`))
	})

	mux.HandleFunc("/posts", func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		assert.Equal(t, "13359703", q.Get("filter[campaign_id]"))
		assert.Equal(t, "-published_at", q.Get("sort"))
		assert.Equal(t, "1.0", q.Get("json-api-version"))

		canView := "false"
		if loggedIn(r) {
			canView = "true"
		}

		if q.Get("page[cursor]") == "" {
			next := "http://" + r.Host + "/posts?" + q.Encode() + "&page%5Bcursor%5D=2"
			_, _ = w.Write([]byte(`{"data":[
				{"id":"174464677","type":"post","attributes":{"title":"AMT423 bonus bits","post_type":"audio_file",
				 "published_at":"2026-11-01T10:00:00.000+00:00","current_user_can_view":` + canView + `,
				 "url":"https://www.patreon.com/posts/amt423-174464677","content":"<p>Hello</p>",
				 "post_file":{"duration":360.0},"image":{"url":"https://img/p1.png","large_url":"https://img/p1-large.png"}}},
				{"id":"161670826","type":"post","attributes":{"title":"Petty Problems #6","post_type":"video_embed",
				 "published_at":"2026-06-21T10:00:00.000+00:00","current_user_can_view":true,
				 "url":"https://www.patreon.com/posts/petty-161670826","content":null,"teaser_text":"Teaser",
				 "post_file":null,"embed":{"url":"https://youtu.be/G5NpC7COe24"}}}
			],"links":{"next":"` + next + `"}}`))
			return
		}

		_, _ = w.Write([]byte(`{"data":[
			{"id":"171091754","type":"post","attributes":{"title":"AMT422 bonus bits","post_type":"audio_file",
			 "published_at":"2026-10-01T10:00:00.000+00:00","current_user_can_view":true,"url":""}}
		],"links":{}}`))
	})

	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv
}

func testClient(t *testing.T, srv *httptest.Server, cookies string) *Client {
	parsed, err := ParseCookies(strings.NewReader(cookies))
	require.NoError(t, err)
	return NewClient(parsed, WithBaseURL(srv.URL))
}

func TestCheckSession(t *testing.T) {
	srv := fakePatreon(t)

	require.NoError(t, testClient(t, srv, testCookies).CheckSession(context.Background()))

	err := testClient(t, srv, "").CheckSession(context.Background())
	assert.ErrorIs(t, err, ErrUnauthorized)
}

func TestCampaignByPageName(t *testing.T) {
	srv := fakePatreon(t)
	client := testClient(t, srv, testCookies)

	campaign, err := client.CampaignByPageName(context.Background(), "answermethis")
	require.NoError(t, err)
	assert.Equal(t, "13359703", campaign.ID)
	assert.Equal(t, "Answer Me This!", campaign.Name)
	assert.Equal(t, "https://img/avatar.png", campaign.AvatarPhotoURL)

	_, err = client.CampaignByPageName(context.Background(), "nobody")
	assert.ErrorIs(t, err, ErrNotFound)
}

func TestCollection(t *testing.T) {
	srv := fakePatreon(t)

	collection, err := testClient(t, srv, testCookies).Collection(context.Background(), "1349741")
	require.NoError(t, err)
	assert.Equal(t, "Bonus bits", collection.Title)
	assert.Equal(t, "13359703", collection.CampaignID)
	assert.Equal(t, "https://img/large.png", collection.ThumbnailURL)

	_, err = testClient(t, srv, testCookies).Collection(context.Background(), "999")
	assert.ErrorIs(t, err, ErrNotFound)
}

func TestListPosts_Pagination(t *testing.T) {
	srv := fakePatreon(t)
	client := testClient(t, srv, testCookies)

	posts, err := client.ListPosts(context.Background(), "13359703", "1349741", 10)
	require.NoError(t, err)
	require.Len(t, posts, 3)

	first := posts[0]
	assert.Equal(t, "174464677", first.ID)
	assert.Equal(t, "audio_file", first.PostType)
	assert.True(t, first.CanView)
	assert.Equal(t, 360*time.Second, first.Duration)
	assert.Equal(t, "<p>Hello</p>", first.ContentHTML)
	assert.Equal(t, "https://img/p1-large.png", first.ImageURL)
	assert.Equal(t, time.Date(2026, 11, 1, 10, 0, 0, 0, time.UTC), first.PublishedAt)

	embed := posts[1]
	assert.Equal(t, "video_embed", embed.PostType)
	assert.Equal(t, "Teaser", embed.TeaserText)
	assert.Equal(t, "", embed.ContentHTML)
	assert.Zero(t, embed.Duration, "patreon reports no duration for embedded videos")

	assert.Equal(t, "https://www.patreon.com/posts/171091754", posts[2].URL, "missing URL falls back to the canonical post URL")
}

func TestListPosts_Limit(t *testing.T) {
	srv := fakePatreon(t)

	posts, err := testClient(t, srv, testCookies).ListPosts(context.Background(), "13359703", "", 1)
	require.NoError(t, err)
	require.Len(t, posts, 1)
}

func TestListPosts_LockedWithoutCookies(t *testing.T) {
	srv := fakePatreon(t)

	posts, err := testClient(t, srv, "").ListPosts(context.Background(), "13359703", "", 1)
	require.NoError(t, err)
	require.Len(t, posts, 1)
	assert.False(t, posts[0].CanView)
}
