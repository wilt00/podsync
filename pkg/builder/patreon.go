package builder

import (
	"context"
	"html"
	"net/url"
	"regexp"
	"strings"
	"time"

	"github.com/pkg/errors"
	log "github.com/sirupsen/logrus"

	"github.com/mxpv/podsync/pkg/feed"
	"github.com/mxpv/podsync/pkg/model"
	"github.com/mxpv/podsync/pkg/patreon"
)

const (
	maxPatreonResults = 50

	patreonAudioBytesPerSecond = 16000
	patreonVideoBytesPerSecond = 125000
)

var patreonMediaPostTypes = map[string]bool{
	"podcast":             true,
	"audio_file":          true,
	"audio_embed":         true,
	"video_external_file": true,
	"video_embed":         true,
}

type PatreonBuilder struct {
	opts []patreon.Option
}

func NewPatreonBuilder(opts ...patreon.Option) (*PatreonBuilder, error) {
	return &PatreonBuilder{opts: opts}, nil
}

func (p *PatreonBuilder) Build(ctx context.Context, cfg *feed.Config) (*model.Feed, error) {
	info, err := ParseURL(cfg.URL)
	if err != nil {
		return nil, err
	}

	client, err := p.newClient(ctx, cfg)
	if err != nil {
		return nil, err
	}

	_feed := &model.Feed{
		ItemID:          info.ItemID,
		Provider:        info.Provider,
		LinkType:        info.LinkType,
		Format:          cfg.Format,
		Quality:         cfg.Quality,
		CoverArtQuality: cfg.Custom.CoverArtQuality,
		PageSize:        cfg.PageSize,
		PlaylistSort:    cfg.PlaylistSort,
		PrivateFeed:     cfg.PrivateFeed,
		UpdatedAt:       time.Now().UTC(),
	}

	if _feed.PageSize == 0 {
		_feed.PageSize = maxPatreonResults
	}

	var campaign *patreon.Campaign
	collectionID := ""

	switch info.LinkType {
	case model.TypeChannel:
		campaign, err = client.CampaignByPageName(ctx, info.ItemID)
		if err != nil {
			return nil, p.wrapErr(err, cfg)
		}

		_feed.Title = campaign.Name
		_feed.Description = stripHTML(campaign.SummaryHTML)
		_feed.ItemURL = campaign.URL
		_feed.CoverArt = firstNonEmpty(campaign.AvatarPhotoURL, campaign.ImageURL)

	case model.TypePlaylist:
		collection, err := client.Collection(ctx, info.ItemID)
		if err != nil {
			return nil, p.wrapErr(err, cfg)
		}

		campaign, err = client.Campaign(ctx, collection.CampaignID)
		if err != nil {
			return nil, p.wrapErr(err, cfg)
		}

		collectionID = collection.ID
		_feed.Title = collection.Title
		_feed.Description = stripHTML(collection.Description)
		_feed.ItemURL = "https://www.patreon.com/collection/" + collection.ID
		_feed.CoverArt = firstNonEmpty(collection.ThumbnailURL, campaign.AvatarPhotoURL, campaign.ImageURL)

	default:
		return nil, errors.Errorf("unsupported patreon link type %q", info.LinkType)
	}

	_feed.Author = campaign.Name
	if _feed.Description == "" {
		_feed.Description = _feed.Title
	}
	if _feed.ItemURL == "" {
		_feed.ItemURL = cfg.URL
	}

	posts, err := client.ListPosts(ctx, campaign.ID, collectionID, _feed.PageSize)
	if err != nil {
		return nil, p.wrapErr(err, cfg)
	}

	locked := 0
	for _, post := range posts {
		if !patreonMediaPostTypes[post.PostType] {
			continue
		}

		if !post.CanView {
			locked++
		}

		_feed.Episodes = append(_feed.Episodes, p.episode(post, _feed))

		if post.PublishedAt.After(_feed.PubDate) {
			_feed.PubDate = post.PublishedAt
		}
	}

	if locked > 0 {
		log.Infof("patreon: %d post(s) in %q are not accessible with the current membership", locked, _feed.Title)
	}

	return _feed, nil
}

func (p *PatreonBuilder) newClient(ctx context.Context, cfg *feed.Config) (*patreon.Client, error) {
	if cfg.CookiesFile == "" {
		log.Warnf("patreon: no cookies_file configured for %s, only public posts will be available", cfg.URL)
		return patreon.NewClient(nil, p.opts...), nil
	}

	cookies, err := patreon.LoadCookiesFile(cfg.CookiesFile)
	if err != nil {
		return nil, err
	}

	if _, ok := cookies.Session(); !ok {
		return nil, errors.Wrapf(model.ErrUnauthorized,
			"patreon: %s has no %s cookie, export cookies while logged in to patreon.com", cfg.CookiesFile, patreon.SessionCookieName)
	}

	client := patreon.NewClient(cookies, p.opts...)
	if err := client.CheckSession(ctx); err != nil {
		return nil, p.wrapErr(err, cfg)
	}

	return client, nil
}

func (p *PatreonBuilder) wrapErr(err error, cfg *feed.Config) error {
	if errors.Is(err, patreon.ErrUnauthorized) {
		return errors.Wrapf(model.ErrUnauthorized,
			"patreon: session expired or was revoked, re-export cookies to %s", cfg.CookiesFile)
	}

	return errors.Wrap(err, "patreon")
}

func (p *PatreonBuilder) episode(post *patreon.Post, _feed *model.Feed) *model.Episode {
	duration := int64(post.Duration.Seconds())

	bytesPerSecond := int64(patreonVideoBytesPerSecond)
	if _feed.Format == model.FormatAudio {
		bytesPerSecond = patreonAudioBytesPerSecond
	}

	description := stripHTML(post.ContentHTML)
	if description == "" {
		description = post.TeaserText
	}

	return &model.Episode{
		ID:          post.ID,
		Title:       post.Title,
		Description: description,
		Thumbnail:   firstNonEmpty(post.ImageURL, _feed.CoverArt),
		Duration:    duration,
		VideoURL:    post.URL,
		PubDate:     post.PublishedAt,
		Size:        duration * bytesPerSecond,
		Status:      model.EpisodeNew,
		Unavailable: !post.CanView,
	}
}

func parsePatreonURL(parsed *url.URL) (model.Type, string, error) {
	parts := strings.Split(strings.Trim(parsed.EscapedPath(), "/"), "/")
	if len(parts) == 0 || parts[0] == "" {
		return "", "", errors.New("invalid patreon link path")
	}

	if parts[0] == "collection" {
		if len(parts) != 2 || !isDigits(parts[1]) {
			return "", "", errors.New("invalid patreon collection link")
		}
		return model.TypePlaylist, parts[1], nil
	}

	if parts[0] == "c" || parts[0] == "cw" {
		parts = parts[1:]
		if len(parts) == 0 || parts[0] == "" {
			return "", "", errors.New("invalid patreon creator link")
		}
	}

	pageName := parts[0]
	if patreonReservedPaths[strings.ToLower(pageName)] {
		return "", "", errors.Errorf("unsupported patreon link: /%s", pageName)
	}

	if len(parts) > 2 || (len(parts) == 2 && parts[1] != "posts") {
		return "", "", errors.New("unsupported patreon link, use a creator page or a /collection/<id> link")
	}

	return model.TypeChannel, pageName, nil
}

var patreonReservedPaths = map[string]bool{
	"posts": true, "home": true, "login": true, "signup": true, "messages": true, "notifications": true,
	"settings": true, "search": true, "explore": true, "user": true, "membership": true, "memberships": true,
	"library": true, "collections": true, "checkout": true, "join": true, "api": true,
}

var (
	htmlBreaks = regexp.MustCompile(`(?i)<br\s*/?>|</p>|</li>|</h[1-6]>`)
	htmlTags   = regexp.MustCompile(`<[^>]*>`)
	blankLines = regexp.MustCompile(`\n{3,}`)
)

func stripHTML(s string) string {
	if s == "" {
		return ""
	}

	s = htmlBreaks.ReplaceAllString(s, "\n")
	s = htmlTags.ReplaceAllString(s, "")
	s = html.UnescapeString(s)
	s = blankLines.ReplaceAllString(s, "\n\n")
	return strings.TrimSpace(s)
}

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if v != "" {
			return v
		}
	}
	return ""
}

func isDigits(s string) bool {
	if s == "" {
		return false
	}
	for _, r := range s {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}
