# How to set up Patreon feeds

Podsync can turn a Patreon creator page, or a single collection on it, into a podcast feed. Members-only
posts are fetched with your own Patreon login, using browser session cookies.

## Supported links

| Link | Feed contents |
|---|---|
| `https://www.patreon.com/<creator>`, `/c/<creator>`, `/cw/<creator>`, `/<creator>/posts` | All audio and video posts of the creator |
| `https://www.patreon.com/collection/<id>` | Only the posts in that collection (like a YouTube playlist) |

`<creator>` is the creator's page name: the part of their page address after `patreon.com/`, for
example `answermethis`. The collection ID is in the address bar when you open a collection on the
creator's page.

Only posts that contain media are included: Patreon-hosted audio and video, podcast episodes, and
embedded YouTube/Vimeo videos. Text, image, poll and link posts are skipped. With `format = "audio"`,
video posts are converted to MP3.

## 1. Export your cookies

Podsync needs a Netscape-format `cookies.txt` for `patreon.com`.

1. Open a **private/incognito window** and log in to patreon.com.
2. Export the cookies for patreon.com with a "cookies.txt" browser extension.
3. Close the private window **without logging out**.

Logging in from a private window gives podsync a session of its own. Logging out ends a session on
Patreon's servers, but just closing the window doesn't, so using and logging out of Patreon in your
normal browser won't affect podsync.

> On Windows, `--cookies-from-browser chrome` / `edge` usually fails because of Chromium's
> app-bound cookie encryption. Export a `cookies.txt` file instead.

Keep the file private: it gives full access to your Patreon account until the session expires.

## 2. Configure feeds

```toml
[tokens]
# No API key is needed for Patreon

[feeds.amt_bonus]
url = "https://www.patreon.com/collection/1349741"
format = "audio"
page_size = 20
update_period = "12h"
private_feed = true
cookies_file = '/path/to/patreon-cookies.txt'

[feeds.amt_bonus.custom]
title = "Answer Me This! - Bonus bits"
```

- `cookies_file` must be **writable**: yt-dlp updates it after each download. Podsync checks this at
  startup.
- On Windows, use TOML literal strings (single quotes) for paths, e.g. `'C:\podsync\cookies.txt'`.
- Several feeds can share one cookies file.
- Without `cookies_file`, only public posts are available.

Paid content belongs to the creator. Keep these feeds private: set `private_feed = true`, don't expose
the server publicly, and consider `server.no_index = true`.

## 3. Keeping it running unattended

A Patreon session typically lasts about a year after login. Podsync re-reads `cookies_file` on every
update, so replacing the file takes effect without a restart.

Podsync detects a lost login in several ways. All of them make `/health` return `503` with the reason
under `feeds`, and the first two also run your `on_episode_download_error` hooks (with an empty
`EPISODE_TITLE`):

1. **Session check:** before each update, podsync checks that Patreon still accepts the session. If it
   doesn't, the update fails with `session expired or was revoked, re-export cookies to <path>`.
2. **Known posts:** posts don't get unpublished, so if a post podsync downloaded before is now reported
   as inaccessible, the session expired or the membership lapsed. The update fails until you fix it.
3. **Expiry warning:** when the `session_id` cookie expires in less than 30 days, podsync logs a
   warning. With less than 7 days left, `/health` reports the feed as needing attention.
4. **Denied downloads:** if yt-dlp reports `You do not have access to this post`, the episode is marked
   as failed and `/health` shows a warning.

To fix any of these, export the cookies again (step 1) and overwrite the file.

Posts on tiers you don't pay for are skipped silently: they aren't downloaded and don't count as
errors.

## Upcoming livestreams

Some collections contain posts for livestreams that haven't happened yet. Podsync doesn't count those
as failures: the post stays queued and downloads automatically once the recording is available.

## Alternative for audio-only creators

For creators who only publish audio, Patreon gives each member a private RSS link ("Get audio RSS
link" on the membership page). It uses a long-lived token instead of browser cookies and can be added
to a podcast app directly, with no podsync involved.
