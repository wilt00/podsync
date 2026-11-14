// Package patreon talks to the undocumented JSON API behind patreon.com (the one the site itself and
// yt-dlp's Patreon extractor use). It has no API keys: requests are authenticated with a patron's
// browser session cookies.
package patreon

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"time"

	"github.com/pkg/errors"
)

const (
	DefaultBaseURL = "https://www.patreon.com/api"
	defaultTimeout = 30 * time.Second
)

var (
	ErrUnauthorized = errors.New("patreon session is not authorized (missing or expired cookies)")
	ErrNotFound     = errors.New("patreon object not found")
)

type Client struct {
	baseURL string
	http    *http.Client
	cookies Cookies
}

type Option func(*Client)

func WithBaseURL(baseURL string) Option {
	return func(c *Client) { c.baseURL = baseURL }
}

func NewClient(cookies Cookies, opts ...Option) *Client {
	c := &Client{
		baseURL: DefaultBaseURL,
		http:    &http.Client{Timeout: defaultTimeout},
		cookies: cookies,
	}

	for _, opt := range opts {
		opt(c)
	}

	return c
}

type Campaign struct {
	ID             string
	Name           string
	SummaryHTML    string
	URL            string
	AvatarPhotoURL string
	ImageURL       string
}

type Collection struct {
	ID           string
	CampaignID   string
	Title        string
	Description  string
	ThumbnailURL string
}

type Post struct {
	ID          string
	Title       string
	ContentHTML string
	TeaserText  string
	URL         string
	PostType    string
	PublishedAt time.Time
	CanView     bool
	Duration    time.Duration
	ImageURL    string
}

func (c *Client) CheckSession(ctx context.Context) error {
	var doc document[resource[userAttributes]]

	params := url.Values{"fields[user]": {"full_name"}}
	if err := c.get(ctx, "/current_user", params, &doc); err != nil {
		return err
	}

	if doc.Data.ID == "" {
		return ErrUnauthorized
	}

	return nil
}

func (c *Client) CampaignByPageName(ctx context.Context, pageName string) (*Campaign, error) {
	var doc document[[]resource[campaignAttributes]]

	params := url.Values{
		"filter[vanity]":   {pageName},
		"fields[campaign]": {"name,summary,url,avatar_photo_url,image_url"},
	}
	if err := c.get(ctx, "/campaigns", params, &doc); err != nil {
		return nil, err
	}

	if len(doc.Data) == 0 {
		return nil, errors.Wrapf(ErrNotFound, "campaign %q", pageName)
	}

	return toCampaign(doc.Data[0]), nil
}

func (c *Client) Campaign(ctx context.Context, id string) (*Campaign, error) {
	var doc document[resource[campaignAttributes]]

	params := url.Values{"fields[campaign]": {"name,summary,url,avatar_photo_url,image_url"}}
	if err := c.get(ctx, "/campaigns/"+url.PathEscape(id), params, &doc); err != nil {
		return nil, err
	}

	if doc.Data.ID == "" {
		return nil, errors.Wrapf(ErrNotFound, "campaign %q", id)
	}

	return toCampaign(doc.Data), nil
}

func (c *Client) Collection(ctx context.Context, id string) (*Collection, error) {
	var doc document[resource[collectionAttributes]]

	params := url.Values{"include": {"campaign"}}
	if err := c.get(ctx, "/collection/"+url.PathEscape(id), params, &doc); err != nil {
		return nil, err
	}

	if doc.Data.ID == "" {
		return nil, errors.Wrapf(ErrNotFound, "collection %q", id)
	}

	attrs := doc.Data.Attributes
	collection := &Collection{
		ID:           doc.Data.ID,
		Title:        attrs.Title,
		Description:  attrs.Description,
		ThumbnailURL: attrs.Thumbnail.best(),
	}

	if rel, ok := doc.Data.Relationships["campaign"]; ok {
		var ref resourceRef
		if err := json.Unmarshal(rel.Data, &ref); err == nil {
			collection.CampaignID = ref.ID
		}
	}

	if collection.CampaignID == "" {
		return nil, errors.Errorf("collection %q has no campaign", id)
	}

	return collection, nil
}

func (c *Client) ListPosts(ctx context.Context, campaignID, collectionID string, limit int) ([]*Post, error) {
	if limit <= 0 {
		return nil, nil
	}

	params := url.Values{
		"filter[campaign_id]":              {campaignID},
		"filter[contains_exclusive_posts]": {"true"},
		"filter[is_draft]":                 {"false"},
		"sort":                             {"-published_at"},
		"page[count]":                      {strconv.Itoa(limit)},
		"fields[post]":                     {"title,content,teaser_text,url,post_type,published_at,current_user_can_view,post_file,image"},
	}

	if collectionID != "" {
		params.Set("filter[collection_id]", collectionID)
	}

	var (
		posts []*Post
		next  = c.baseURL + "/posts?" + params.Encode()
	)

	for next != "" && len(posts) < limit {
		var doc document[[]resource[postAttributes]]
		if err := c.getURL(ctx, next, &doc); err != nil {
			return nil, err
		}

		for _, item := range doc.Data {
			posts = append(posts, toPost(item))
			if len(posts) >= limit {
				break
			}
		}

		next = doc.Links.Next
		if len(doc.Data) == 0 {
			break
		}
	}

	return posts, nil
}

func (c *Client) get(ctx context.Context, path string, params url.Values, out interface{}) error {
	u := c.baseURL + path
	if len(params) > 0 {
		u += "?" + params.Encode()
	}

	return c.getURL(ctx, u, out)
}

func (c *Client) getURL(ctx context.Context, rawURL string, out interface{}) error {
	u, err := url.Parse(rawURL)
	if err != nil {
		return errors.Wrapf(err, "invalid patreon api url")
	}

	query := u.Query()
	if query.Get("json-api-version") == "" {
		query.Set("json-api-version", "1.0")
		u.RawQuery = query.Encode()
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return err
	}

	req.Header.Set("Accept", "application/vnd.api+json, application/json")
	if len(c.cookies) > 0 {
		req.Header.Set("Cookie", c.cookies.Header())
	}

	resp, err := c.http.Do(req)
	if err != nil {
		return errors.Wrap(err, "patreon api request failed")
	}
	defer resp.Body.Close()

	switch {
	case resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden:
		return ErrUnauthorized
	case resp.StatusCode == http.StatusNotFound:
		return ErrNotFound
	case resp.StatusCode != http.StatusOK:
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		return errors.Errorf("patreon api %s returned %d: %s", u.Path, resp.StatusCode, body)
	}

	if err := json.NewDecoder(resp.Body).Decode(out); err != nil {
		return errors.Wrapf(err, "failed to decode patreon api response from %s", u.Path)
	}

	return nil
}

type document[T any] struct {
	Data  T `json:"data"`
	Links struct {
		Next string `json:"next"`
	} `json:"links"`
}

type resource[T any] struct {
	ID            string                  `json:"id"`
	Type          string                  `json:"type"`
	Attributes    T                       `json:"attributes"`
	Relationships map[string]relationship `json:"relationships"`
}

type relationship struct {
	Data json.RawMessage `json:"data"`
}

type resourceRef struct {
	ID   string `json:"id"`
	Type string `json:"type"`
}

type userAttributes struct {
	FullName string `json:"full_name"`
}

type campaignAttributes struct {
	Name           string `json:"name"`
	Summary        string `json:"summary"`
	URL            string `json:"url"`
	AvatarPhotoURL string `json:"avatar_photo_url"`
	ImageURL       string `json:"image_url"`
}

func toCampaign(r resource[campaignAttributes]) *Campaign {
	attrs := r.Attributes
	return &Campaign{
		ID:             r.ID,
		Name:           attrs.Name,
		SummaryHTML:    attrs.Summary,
		URL:            attrs.URL,
		AvatarPhotoURL: attrs.AvatarPhotoURL,
		ImageURL:       attrs.ImageURL,
	}
}

type imageSizes struct {
	Original     string `json:"original"`
	Default      string `json:"default"`
	DefaultLarge string `json:"default_large"`
	URL          string `json:"url"`
}

func (i *imageSizes) best() string {
	if i == nil {
		return ""
	}

	for _, candidate := range []string{i.DefaultLarge, i.Original, i.Default, i.URL} {
		if candidate != "" {
			return candidate
		}
	}

	return ""
}

type collectionAttributes struct {
	Title       string      `json:"title"`
	Description string      `json:"description"`
	Thumbnail   *imageSizes `json:"thumbnail"`
}

type postAttributes struct {
	Title       string  `json:"title"`
	Content     *string `json:"content"`
	TeaserText  *string `json:"teaser_text"`
	URL         string  `json:"url"`
	PostType    string  `json:"post_type"`
	PublishedAt string  `json:"published_at"`
	CanView     bool    `json:"current_user_can_view"`
	PostFile    *struct {
		DurationSeconds float64 `json:"duration"`
	} `json:"post_file"`
	Image *struct {
		URL      string `json:"url"`
		LargeURL string `json:"large_url"`
	} `json:"image"`
}

func toPost(r resource[postAttributes]) *Post {
	attrs := r.Attributes

	post := &Post{
		ID:       r.ID,
		Title:    attrs.Title,
		URL:      attrs.URL,
		PostType: attrs.PostType,
		CanView:  attrs.CanView,
	}

	if attrs.Content != nil {
		post.ContentHTML = *attrs.Content
	}
	if attrs.TeaserText != nil {
		post.TeaserText = *attrs.TeaserText
	}
	if attrs.PostFile != nil {
		post.Duration = time.Duration(attrs.PostFile.DurationSeconds * float64(time.Second))
	}
	if attrs.Image != nil {
		post.ImageURL = attrs.Image.LargeURL
		if post.ImageURL == "" {
			post.ImageURL = attrs.Image.URL
		}
	}
	if published, err := time.Parse(time.RFC3339, attrs.PublishedAt); err == nil {
		post.PublishedAt = published.UTC()
	}
	if post.URL == "" {
		post.URL = fmt.Sprintf("https://www.patreon.com/posts/%s", r.ID)
	}

	return post
}
