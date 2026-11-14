package patreon

import (
	"bufio"
	"io"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/pkg/errors"
)

const SessionCookieName = "session_id"

const httpOnlyPrefix = "#HttpOnly_"

type Cookies []*http.Cookie

func LoadCookiesFile(path string) (Cookies, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, errors.Wrap(err, "failed to open cookies file")
	}
	defer f.Close()

	cookies, err := ParseCookies(f)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to parse cookies file %s", path)
	}

	return cookies, nil
}

// ParseCookies reads the Netscape cookies.txt format used by yt-dlp --cookies and browser export extensions.
func ParseCookies(r io.Reader) (Cookies, error) {
	var (
		cookies Cookies
		scanner = bufio.NewScanner(r)
	)

	for scanner.Scan() {
		line := strings.TrimRight(scanner.Text(), "\r")

		httpOnly := false
		if strings.HasPrefix(line, httpOnlyPrefix) {
			httpOnly = true
			line = strings.TrimPrefix(line, httpOnlyPrefix)
		}

		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		fields := strings.Split(line, "\t")
		if len(fields) < 7 {
			continue
		}

		domain, path, secure, expires, name, value := fields[0], fields[2], fields[3], fields[4], fields[5], fields[6]
		if !isPatreonDomain(domain) {
			continue
		}

		cookie := &http.Cookie{
			Domain:   domain,
			Path:     path,
			Secure:   strings.EqualFold(secure, "TRUE"),
			Name:     name,
			Value:    value,
			HttpOnly: httpOnly,
		}

		if expiry, err := strconv.ParseInt(expires, 10, 64); err == nil && expiry > 0 {
			cookie.Expires = time.Unix(expiry, 0).UTC()
		}

		cookies = append(cookies, cookie)
	}

	if err := scanner.Err(); err != nil {
		return nil, err
	}

	return cookies, nil
}

func (c Cookies) Session() (*http.Cookie, bool) {
	for _, cookie := range c {
		if cookie.Name == SessionCookieName && cookie.Value != "" {
			return cookie, true
		}
	}

	return nil, false
}

func (c Cookies) SessionExpiry() (time.Time, bool) {
	session, ok := c.Session()
	if !ok || session.Expires.IsZero() {
		return time.Time{}, false
	}

	return session.Expires, true
}

func (c Cookies) Header() string {
	parts := make([]string, 0, len(c))
	for _, cookie := range c {
		parts = append(parts, cookie.Name+"="+cookie.Value)
	}

	return strings.Join(parts, "; ")
}

func isPatreonDomain(domain string) bool {
	domain = strings.TrimPrefix(strings.ToLower(domain), ".")
	return domain == "patreon.com" || strings.HasSuffix(domain, ".patreon.com")
}
