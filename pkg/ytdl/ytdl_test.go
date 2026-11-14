package ytdl

import (
	"errors"
	"testing"

	log "github.com/sirupsen/logrus"
	logtest "github.com/sirupsen/logrus/hooks/test"

	"github.com/mxpv/podsync/pkg/feed"
	"github.com/mxpv/podsync/pkg/model"

	"github.com/stretchr/testify/assert"
)

func TestBuildArgs(t *testing.T) {
	tests := []struct {
		name         string
		format       model.Format
		customFormat feed.CustomFormat
		quality      model.Quality
		maxHeight    int
		output       string
		videoURL     string
		ytdlArgs     []string
		cookiesFile  string
		expect       []string
	}{
		{
			name:     "Audio unknown quality",
			format:   model.FormatAudio,
			output:   "/tmp/1",
			videoURL: "http://url",
			expect:   []string{"--extract-audio", "--audio-format", "mp3", "--format", "bestaudio/best", "--output", "/tmp/1", "http://url"},
		},
		{
			name:     "Audio low quality",
			format:   model.FormatAudio,
			quality:  model.QualityLow,
			output:   "/tmp/1",
			videoURL: "http://url",
			expect:   []string{"--extract-audio", "--audio-format", "mp3", "--format", "worstaudio/worst", "--output", "/tmp/1", "http://url"},
		},
		{
			name:     "Audio best quality",
			format:   model.FormatAudio,
			quality:  model.QualityHigh,
			output:   "/tmp/1",
			videoURL: "http://url",
			expect:   []string{"--extract-audio", "--audio-format", "mp3", "--format", "bestaudio/best", "--output", "/tmp/1", "http://url"},
		},
		{
			name:     "Video unknown quality",
			format:   model.FormatVideo,
			output:   "/tmp/1",
			videoURL: "http://url",
			expect:   []string{"--format", "bestvideo[ext=mp4][vcodec^=avc1]+bestaudio[ext=m4a]/best[ext=mp4][vcodec^=avc1]/best[ext=mp4]/best", "--output", "/tmp/1", "http://url"},
		},
		{
			name:      "Video unknown quality with maxheight",
			format:    model.FormatVideo,
			maxHeight: 720,
			output:    "/tmp/1",
			videoURL:  "http://url",
			expect:    []string{"--format", "bestvideo[ext=mp4][vcodec^=avc1]+bestaudio[ext=m4a]/best[ext=mp4][vcodec^=avc1]/best[ext=mp4]/best", "--output", "/tmp/1", "http://url"},
		},
		{
			name:     "Video low quality",
			format:   model.FormatVideo,
			quality:  model.QualityLow,
			output:   "/tmp/2",
			videoURL: "http://url",
			expect:   []string{"--format", "worstvideo[ext=mp4][vcodec^=avc1]+worstaudio[ext=m4a]/worst[ext=mp4][vcodec^=avc1]/worst[ext=mp4]/worst", "--output", "/tmp/2", "http://url"},
		},
		{
			name:      "Video low quality with maxheight",
			format:    model.FormatVideo,
			quality:   model.QualityLow,
			maxHeight: 720,
			output:    "/tmp/2",
			videoURL:  "http://url",
			expect:    []string{"--format", "worstvideo[ext=mp4][vcodec^=avc1]+worstaudio[ext=m4a]/worst[ext=mp4][vcodec^=avc1]/worst[ext=mp4]/worst", "--output", "/tmp/2", "http://url"},
		},
		{
			name:     "Video high quality",
			format:   model.FormatVideo,
			quality:  model.QualityHigh,
			output:   "/tmp/2",
			videoURL: "http://url1",
			expect:   []string{"--format", "bestvideo[ext=mp4][vcodec^=avc1]+bestaudio[ext=m4a]/best[ext=mp4][vcodec^=avc1]/best[ext=mp4]/best", "--output", "/tmp/2", "http://url1"},
		},
		{
			name:      "Video high quality with maxheight",
			format:    model.FormatVideo,
			quality:   model.QualityHigh,
			maxHeight: 1024,
			output:    "/tmp/2",
			videoURL:  "http://url1",
			expect:    []string{"--format", "bestvideo[height<=1024][ext=mp4][vcodec^=avc1]+bestaudio[ext=m4a]/best[height<=1024][ext=mp4][vcodec^=avc1]/best[ext=mp4]/best", "--output", "/tmp/2", "http://url1"},
		},
		{
			name:     "Video high quality with custom youtube-dl arguments",
			format:   model.FormatVideo,
			quality:  model.QualityHigh,
			output:   "/tmp/2",
			videoURL: "http://url1",
			ytdlArgs: []string{"--write-sub", "--embed-subs", "--sub-lang", "en,en-US,en-GB"},
			expect:   []string{"--format", "bestvideo[ext=mp4][vcodec^=avc1]+bestaudio[ext=m4a]/best[ext=mp4][vcodec^=avc1]/best[ext=mp4]/best", "--write-sub", "--embed-subs", "--sub-lang", "en,en-US,en-GB", "--output", "/tmp/2", "http://url1"},
		},
		{
			name:         "Custom format",
			format:       model.FormatCustom,
			customFormat: feed.CustomFormat{YouTubeDLFormat: "bestaudio[ext=m4a]", Extension: "m4a"},
			quality:      model.QualityHigh,
			output:       "/tmp/2",
			videoURL:     "http://url1",
			expect:       []string{"--audio-format", "m4a", "--format", "bestaudio[ext=m4a]", "--output", "/tmp/2", "http://url1"},
		},
		{
			name:        "Cookies file before extra args",
			format:      model.FormatAudio,
			quality:     model.QualityHigh,
			output:      "/tmp/3",
			videoURL:    "http://url2",
			cookiesFile: "/data/cookies.txt",
			ytdlArgs:    []string{"--no-mtime"},
			expect:      []string{"--extract-audio", "--audio-format", "mp3", "--format", "bestaudio/best", "--cookies", "/data/cookies.txt", "--no-mtime", "--output", "/tmp/3", "http://url2"},
		},
	}

	for _, tst := range tests {
		t.Run(tst.name, func(t *testing.T) {
			result := buildArgs(&feed.Config{
				Format:        tst.format,
				Quality:       tst.quality,
				CustomFormat:  tst.customFormat,
				MaxHeight:     tst.maxHeight,
				YouTubeDLArgs: tst.ytdlArgs,
				CookiesFile:   tst.cookiesFile,
			}, &model.Episode{
				VideoURL: tst.videoURL,
			}, tst.output)

			assert.EqualValues(t, tst.expect, result)
		})
	}
}

func TestClassifyOutput(t *testing.T) {
	tests := []struct {
		name   string
		output string
		expect error
	}{
		{"unknown", "ERROR: something else broke", nil},
		{"too many requests", "ERROR: unable to download video data: HTTP Error 429: Too Many Requests", ErrTooManyRequests},
		{"upcoming live", "ERROR: [youtube] oPPg6POS6D8: This live event will begin in 25 hours.", ErrNotYetAvailable},
		{"premiere", "ERROR: [youtube] abc: Premieres in 3 hours", ErrNotYetAvailable},
		{"patreon locked", "WARNING: [patreon] 174306894: You do not have access to this post\nERROR: [patreon] 174306894: No video formats found!", ErrAccessDenied},
	}

	for _, tst := range tests {
		t.Run(tst.name, func(t *testing.T) {
			assert.Equal(t, tst.expect, classifyOutput(tst.output))
		})
	}
}

func TestDownloadError(t *testing.T) {
	hook := logtest.NewGlobal()
	defer hook.Reset()

	t.Run("upcoming stream is not logged as an error", func(t *testing.T) {
		hook.Reset()
		err := downloadError("/tmp/x", "ERROR: [youtube] abc: This live event will begin in 25 hours.", errors.New("exit status 1"))
		assert.Equal(t, ErrNotYetAvailable, err)
		for _, entry := range hook.AllEntries() {
			assert.NotEqual(t, log.ErrorLevel, entry.Level, entry.Message)
		}
	})

	t.Run("access denied keeps youtube-dl output", func(t *testing.T) {
		hook.Reset()
		err := downloadError("/tmp/x", "WARNING: [patreon] 1: You do not have access to this post\n", errors.New("exit status 1"))
		assert.ErrorIs(t, err, ErrAccessDenied)
		assert.Contains(t, err.Error(), "You do not have access to this post")
		assert.Equal(t, log.ErrorLevel, hook.LastEntry().Level)
	})

	t.Run("unknown failure returns output", func(t *testing.T) {
		hook.Reset()
		err := downloadError("/tmp/x", "ERROR: boom", errors.New("exit status 1"))
		assert.EqualError(t, err, "ERROR: boom")
	})
}
