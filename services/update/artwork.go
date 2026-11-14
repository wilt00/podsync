package update

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"io"
	"mime"
	"net/http"
	"net/url"
	"os"
	"path"
	"strings"

	"github.com/pkg/errors"
	log "github.com/sirupsen/logrus"
)

const (
	artworkDir     = "artwork"
	maxArtworkSize = 20 << 20
)

var artworkExtensions = map[string]string{
	"image/jpeg": ".jpg",
	"image/png":  ".png",
	"image/gif":  ".gif",
	"image/webp": ".webp",
}

func (u *Manager) mirrorArtwork(ctx context.Context, feedID, imageURL string) (string, error) {
	parsed, err := url.Parse(imageURL)
	if err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") {
		return "", errors.Errorf("unsupported artwork url %q", imageURL)
	}

	base := artworkDir + "/" + artworkKey(parsed)

	ext := artworkExtension(path.Ext(parsed.Path))
	if ext != "" {
		if _, err := u.fs.Size(ctx, feedID+"/"+base+ext); err == nil {
			return base + ext, nil
		}
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, imageURL, nil)
	if err != nil {
		return "", err
	}

	resp, err := u.http.Do(req)
	if err != nil {
		return "", errors.Wrap(err, "artwork request failed")
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", errors.Errorf("artwork request returned %d", resp.StatusCode)
	}

	if ext == "" {
		mediaType, _, _ := mime.ParseMediaType(resp.Header.Get("Content-Type"))
		ext = artworkExtensions[mediaType]
		if ext == "" {
			return "", errors.Errorf("unsupported artwork content type %q", resp.Header.Get("Content-Type"))
		}
	}

	name := base + ext
	size, err := u.fs.Create(ctx, feedID+"/"+name, io.LimitReader(resp.Body, maxArtworkSize+1))
	if err == nil && size > maxArtworkSize {
		err = errors.Errorf("artwork is larger than %d bytes", maxArtworkSize)
	}
	if err != nil {
		_ = u.fs.Delete(ctx, feedID+"/"+name)
		return "", err
	}

	return name, nil
}

func (u *Manager) mirrorArtworkOrWarn(ctx context.Context, feedID, imageURL string) string {
	if imageURL == "" {
		return ""
	}

	name, err := u.mirrorArtwork(ctx, feedID, imageURL)
	if err != nil {
		log.WithError(err).Warnf("failed to mirror artwork %s", imageURL)
		return ""
	}

	return name
}

func (u *Manager) pruneArtwork(ctx context.Context, feedID string, candidates []string) error {
	if len(candidates) == 0 {
		return nil
	}

	stored, err := u.db.GetFeed(ctx, feedID)
	if err != nil {
		return err
	}

	referenced := map[string]bool{stored.CoverArtFile: true}
	for _, episode := range stored.Episodes {
		referenced[episode.ThumbnailFile] = true
	}

	for _, name := range candidates {
		if name == "" || referenced[name] {
			continue
		}

		if err := u.fs.Delete(ctx, feedID+"/"+name); err != nil && !errors.Is(err, os.ErrNotExist) {
			return errors.Wrapf(err, "failed to delete artwork %s", name)
		}
		referenced[name] = true
	}

	return nil
}

// artworkKey ignores the token-* query parameters Patreon signs image URLs with, which change on every API call.
func artworkKey(u *url.URL) string {
	stable := *u
	query := stable.Query()
	for key := range query {
		if strings.HasPrefix(key, "token-") {
			query.Del(key)
		}
	}
	stable.RawQuery = query.Encode()
	stable.Fragment = ""

	sum := sha256.Sum256([]byte(stable.String()))
	return hex.EncodeToString(sum[:])[:32]
}

func artworkExtension(ext string) string {
	switch ext = strings.ToLower(ext); ext {
	case ".jpg", ".jpeg":
		return ".jpg"
	case ".png", ".gif", ".webp":
		return ext
	default:
		return ""
	}
}
