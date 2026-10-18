# rssf33ds

**A distraction-free terminal reader for backend and systems engineering blogs.**

Netflix, Cloudflare, AWS Architecture, Stripe, Datadog, Martin Fowler, High
Scalability, Marc Brooker, Jepsen and 27 more, in one keyboard-driven TUI.
Articles are fetched in full, stripped of ads and clutter, and wrapped to your
terminal. You can full-text search across every blog as you type.

Built in Go with [Bubble Tea](https://github.com/charmbracelet/bubbletea),
[Bubbles](https://github.com/charmbracelet/bubbles) and
[Glamour](https://github.com/charmbracelet/glamour).

```
  Search · all blogs

  / kafka
  34 results · 36/36 blogs
  │ Kafka 101
  │ High Scalability · 2024-05-09 · This is a guest article by Stanislav Kozlovski, an Apache Kafka Committ…

    TimeSeries Tiered Storage Journey: Kafka/Flink Streams to Native Cassandra Cold Reads
    Netflix TechBlog · 2026-10-14 · …data into cheaper object storage: Solution 1 : Uses Apache Kafka and F…

    Transforming How We Run Kafka at Honeycomb
    Honeycomb Blog · 2026-07-15 · We just completed a large-scale, multi-month Kafka migration project. We …
```

## Features

- **36 curated feeds.** Every URL was verified with a live request. The list
  is grouped into company engineering blogs, individual engineers and
  researchers, and publications.
- **Three views.** Feed list → article list (title, date, summary) → reader.
- **Clean reader.** If a feed only carries an excerpt, the article page is
  fetched and its main content extracted. Scripts, navigation, share bars,
  sponsor sections, images and link noise are removed. The text is rendered
  as Markdown, wrapped to at most 100 columns, and re-wrapped when you resize.
- **Full-text search.** Search titles, blog names, authors and article bodies,
  across all blogs or within one, with results as you type.
- **Never blocks.** Fetching, parsing, indexing, searching and rendering all
  run in the background. Feeds load 8 at a time at startup.
- **Cached and offline-friendly.** Responses are cached on disk and
  revalidated with ETag/Last-Modified. When the network is down you get the
  cached copy, flagged as offline.
- **Resilient to flaky sites.** See [Reliability](#reliability): retries with
  backoff, validation before caching, crash isolation and a bounded cache.
- **Terminal-safe.** Escape sequences and control characters in feed content
  are stripped before display, so a hostile feed can't drive your terminal.
  Responses are size-capped and time-limited, and "open in browser" only
  accepts http(s) links.

## Install

**Prebuilt binaries** for macOS, Linux and Windows (amd64 and arm64) are on
the [releases page](https://github.com/aelhadii/rssf33ds/releases/latest).
Download the archive for your platform, extract it, and put `rssf33ds` on
your `PATH`.

```sh
# macOS (Apple silicon). Swap the archive name for your platform.
curl -LO https://github.com/aelhadii/rssf33ds/releases/latest/download/rssf33ds_darwin_arm64.tar.gz
tar xzf rssf33ds_darwin_arm64.tar.gz && ./rssf33ds
```

On macOS, a binary downloaded with a browser is quarantined by Gatekeeper
(the binaries aren't notarized). Clear the flag with
`xattr -d com.apple.quarantine rssf33ds`. Downloading with `curl`, as above,
avoids this.

**With Go** (1.26 or newer):

```sh
go install github.com/aelhadii/rssf33ds@latest
```

Or from a clone:

```sh
git clone https://github.com/aelhadii/rssf33ds && cd rssf33ds
go run .
```

### Flags

| Flag | Default | |
|------|---------|--|
| `-style` | from terminal background | Reader theme: `dark`, `light`, `dracula`, `tokyo-night`, `pink`, `ascii`, `notty`. `GLAMOUR_STYLE` also works. |
| `-no-prefetch` | off | Don't load every feed at startup; load each one when you open it. |
| `-version` | | Print the version and exit. |
| `-cache` | OS user cache dir | Where to cache feeds and pages (`~/Library/Caches/rssf33ds` on macOS). Empty disables the disk cache. |

## Keys

| View | Keys |
|------|------|
| **Feeds** | `j`/`k` or `↑`/`↓` move · `enter` open · `/` filter blogs by name · `s` search all blogs · `r` refresh all · `q`/`esc` quit |
| **Articles** | `j`/`k` move · `enter` read · `/` filter titles · `s` search this blog · `r` refresh · `o` open in browser · `q`/`esc` back |
| **Search** | type to search · `enter`/`tab`/`↓` go to results · `j`/`k` move · `enter` read · `tab` or `/` edit query · `o` browser · `q`/`esc` back |
| **Reader** | `j`/`k` scroll · `space`/`b` or `pgdn`/`pgup` page · `d`/`u` half page · `g`/`G` top/end · `o` browser · `q`/`esc` back |

`ctrl+c` quits from anywhere. While you're typing a filter or a query, `q` is
just a letter and `esc` cancels.

## Search

Press `s` on the feed list to search every blog, or inside a blog to search
just that one.

- **All words must match**, at the start of a word: `go` finds "goroutine"
  but not "ago".
- **Phrases:** `"exactly once"`.
- **Narrow by blog:** words can match the blog name, so `netflix kafka` or
  `cloudflare dns` narrows to one blog.
- **Ranking:** title matches rank highest, then blog name, then
  author/summary, then body. Newer posts win ties. Each result shows a
  snippet around the match.

The index covers what the feeds publish, usually each blog's newest 10–100
posts (about 1,700 articles in total), not complete archives. Searches over
that take a few milliseconds.

## Feeds

**Company engineering** (24): [Netflix TechBlog](https://netflixtechblog.com/feed) · [Cloudflare Blog](https://blog.cloudflare.com/rss/) · [AWS Architecture Blog](https://aws.amazon.com/blogs/architecture/feed/) · [Stripe Dev Blog](https://stripe.dev/blog/feed) · [Datadog Engineering](https://www.datadoghq.com/blog/engineering/index.xml) · [Meta Engineering](https://engineering.fb.com/feed/) · [Discord Blog](https://discord.com/blog/rss.xml) · [Slack Engineering](https://slack.engineering/feed/) · [Dropbox Tech](https://dropbox.tech/feed) · [Spotify Engineering](https://engineering.atspotify.com/feed/) · [GitHub Engineering](https://github.blog/engineering/feed/) · [Airbnb Engineering](https://medium.com/feed/airbnb-engineering) · [Pinterest Engineering](https://medium.com/feed/pinterest-engineering) · [Shopify Engineering](https://shopify.engineering/blog.atom) · [Lyft Engineering](https://eng.lyft.com/feed) · [Grab Engineering](https://engineering.grab.com/feed.xml) · [Zalando Engineering](https://engineering.zalando.com/atom.xml) · [Salesforce Engineering](https://engineering.salesforce.com/feed/) · [Fly.io Blog](https://fly.io/blog/feed.xml) · [PlanetScale Blog](https://planetscale.com/blog/rss.xml) · [Tailscale Blog](https://tailscale.com/blog/index.xml) · [Honeycomb Blog](https://www.honeycomb.io/feed) · [Confluent Blog](https://www.confluent.io/rss.xml) · [Jane Street Tech Blog](https://blog.janestreet.com/feed.xml)

**Engineers & researchers** (8): [Martin Fowler](https://martinfowler.com/feed.atom) · [All Things Distributed (Werner Vogels)](https://www.allthingsdistributed.com/atom.xml) · [Marc Brooker](https://brooker.co.za/blog/rss.xml) · [Brendan Gregg](https://www.brendangregg.com/blog/rss.xml) · [Metadata (Murat Demirbas)](https://muratbuffalo.blogspot.com/feeds/posts/default) · [Jepsen](https://jepsen.io/blog.rss) · [Aphyr](https://aphyr.com/posts.atom) · [Dan Luu](https://danluu.com/atom.xml)

**Publications** (4): [High Scalability](https://highscalability.com/rss/) · [The Pragmatic Engineer](https://newsletter.pragmaticengineer.com/feed) · [ByteByteGo](https://blog.bytebytego.com/feed) · [InfoQ Architecture & Design](https://www.infoq.com/feed/architecture-design/)

Some blogs were searched for but left out because they no longer publish a
working feed: Uber Engineering, LinkedIn Engineering, DoorDash, Twitter/X
Engineering and Cockroach Labs. Stripe's main blog feed (`stripe.com/blog/feed.rss`)
is also out: Go's TLS verification rejects its certificate chain on some macOS
trust stores. The [Stripe Dev Blog](https://stripe.dev/blog) covers Stripe
engineering instead. The verifier re-checks all of these on every run (see
below).

If a feed fails to load while you're using the app and nothing is cached,
it's skipped: it disappears from the feed list (with a brief "Skipped …"
notice) rather than showing an error. Press `r` to retry; it comes back once
it loads.

To add a feed, append it to [`internal/catalog/catalog.go`](internal/catalog/catalog.go)
and run the verifier.

## Reliability

- **Retries with backoff.** Timeouts, reset connections, `5xx`, `408` and
  `429` are retried twice, waiting 0.5 s then about 1.5 s (with jitter).
  `Retry-After` is honoured up to 10 s. Retries stop at the request's
  deadline. Certificate errors, unknown hosts and other `4xx` responses fail
  immediately, since retrying won't fix them.
- **Validation before caching.** A new response replaces the cached copy only
  if it parses as a feed. A bot-check or error page served with `200 OK`
  never evicts a good feed; you get the last good copy instead, flagged as
  offline.
- **Crash isolation.** Every background job (fetching, extraction,
  rendering, search) recovers from panics. A malformed feed or page fails on
  its own: the feed is skipped, or the article shows a note. The rest of the
  app keeps running. Parsing, cleaning and search are fuzz-tested.
- **Bounded cache.** At startup, entries not refreshed in 30 days are deleted,
  then the oldest until the cache is under 200 MB, along with debris from
  interrupted writes. Only files named like cache entries are touched, so a
  custom `-cache` directory is safe.
- **Failing feeds are skipped.** A feed that can't load and has nothing cached
  drops out of the list until `r` loads it again.

## How it works

```
 feed URL ──► fetch (disk cache, ETag, 10 MB cap) ──► parse (RSS/Atom/JSON, gofeed)
                                                        │
                                   ┌────────────────────┴───────────────────┐
                                   ▼                                        ▼
                         search index (per feed)                   article list
                                                                            │ enter
                                                                            ▼
             feed body ≥ 1,500 chars? ── yes ──► use it
                       │ no
                       ▼
             fetch page ──► readability ──► longer than feed? ── no ──► feed excerpt + note
                                                       │ yes
                                                       ▼
             strip clutter ──► HTML→Markdown ──► sanitize ──► glamour (wrap) ──► viewport
```

| Package | Responsibility |
|---------|----------------|
| [`internal/catalog`](internal/catalog) | The curated feed list |
| [`internal/fetch`](internal/fetch) | HTTP client: disk cache, conditional requests, stale fallback, request dedup, size limit |
| [`internal/feed`](internal/feed) | RSS/Atom/JSON parsing, illegal-XML scrubbing, link resolution, newest-first ordering |
| [`internal/content`](internal/content) | HTML cleaning, readability extraction, HTML→Markdown, terminal sanitizing |
| [`internal/article`](internal/article) | Chooses the feed body or the extracted page for the reader |
| [`internal/search`](internal/search) | In-memory full-text index and ranking |
| [`internal/ui`](internal/ui) | The Bubble Tea state machine and views |
| [`cmd/verifyfeeds`](cmd/verifyfeeds) | Live re-verification of the catalog |

## Verifying the feeds

Feeds move and die, so the catalog has its own checker:

```sh
go run ./cmd/verifyfeeds -content
```

It fetches every feed live, parses it, and (with `-content`) resolves the
newest article through the same pipeline the reader uses. It compares each
result with the previous run in
[`.staged-agents/findings.json`](.staged-agents/findings.json) and prints any
status change (`valid` / `stale` / `invalid`). Results are appended, never
overwritten: each new finding links to the one it supersedes, so every feed
keeps its history. If a rejected blog starts publishing a feed again, the
checker tells you. The exit status is 1 if any catalog feed is invalid.

## Development

```sh
go test ./...                                   # unit tests
go test -race ./...
go test -tags live ./internal/ui -run Live -v   # end-to-end against the real feeds

# fuzzing (seeds and past crashers live in testdata/fuzz)
go test -run '^$' -fuzz '^FuzzParse$'      -fuzztime 60s ./internal/feed
go test -run '^$' -fuzz '^FuzzToMarkdown$' -fuzztime 60s ./internal/content
go test -run '^$' -fuzz '^FuzzSearch$'     -fuzztime 60s ./internal/search
```

The unit tests cover parsing, clutter removal, sanitizing, caching, retries,
cache pruning, panic recovery, ranking and the UI state machine (navigation, filtering vs. back, dropping stale async
results, wrapping and resize). The `live` tests drive the real model against
the network and print the rendered screens.

## License

[MIT](LICENSE)
