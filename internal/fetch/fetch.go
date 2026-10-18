// Package fetch is an HTTP client with a disk cache. Responses are cached with
// their ETag/Last-Modified validators so refreshes are cheap conditional
// requests, and a stale copy is served when the network fails.
package fetch

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// MaxBodySize caps every response; feeds and articles larger than this are
// rejected rather than buffered without bound.
const MaxBodySize = 10 << 20

const userAgent = "Mozilla/5.0 (compatible; rssf33ds/0.1; terminal RSS reader)"

// Source says where a Result's body came from.
type Source int

const (
	Network Source = iota // fresh 200 from the server
	Cache                 // cache entry within maxAge, or a 304 revalidation
	Stale                 // network failed; an expired cache entry was used
)

func (s Source) String() string {
	switch s {
	case Network:
		return "network"
	case Cache:
		return "cache"
	default:
		return "stale cache"
	}
}

// Result is the outcome of a Get. Err is set only alongside Source == Stale,
// to explain why the stale copy was served.
type Result struct {
	Body      []byte
	Source    Source
	FetchedAt time.Time
	Err       error
}

// Client fetches URLs through the disk cache. It is safe for concurrent use;
// concurrent Gets of the same URL share one request.
type Client struct {
	HTTP *http.Client
	Dir  string // cache directory; empty disables the disk cache

	mu       sync.Mutex
	inflight map[string]*call
}

type call struct {
	done chan struct{}
	res  Result
	err  error
}

type meta struct {
	URL          string    `json:"url"`
	ETag         string    `json:"etag,omitempty"`
	LastModified string    `json:"last_modified,omitempty"`
	FetchedAt    time.Time `json:"fetched_at"`
}

// New returns a Client caching under dir.
func New(dir string) *Client {
	return &Client{
		HTTP: &http.Client{Timeout: 25 * time.Second},
		Dir:  dir,
	}
}

// DefaultDir is the per-user cache directory for the app.
func DefaultDir() string {
	d, err := os.UserCacheDir()
	if err != nil {
		return ""
	}
	return filepath.Join(d, "rssf33ds")
}

// Get returns the body for url. A cache entry younger than maxAge is returned
// without touching the network; maxAge <= 0 always revalidates.
func (c *Client) Get(ctx context.Context, url string, maxAge time.Duration) (Result, error) {
	c.mu.Lock()
	if c.inflight == nil {
		c.inflight = make(map[string]*call)
	}
	if cl, ok := c.inflight[url]; ok {
		c.mu.Unlock()
		select {
		case <-cl.done:
			return cl.res, cl.err
		case <-ctx.Done():
			return Result{}, ctx.Err()
		}
	}
	cl := &call{done: make(chan struct{})}
	c.inflight[url] = cl
	c.mu.Unlock()

	cl.res, cl.err = c.get(ctx, url, maxAge)

	c.mu.Lock()
	delete(c.inflight, url)
	c.mu.Unlock()
	close(cl.done)
	return cl.res, cl.err
}

func (c *Client) get(ctx context.Context, url string, maxAge time.Duration) (Result, error) {
	m, body, cached := c.load(url)
	if cached && maxAge > 0 && time.Since(m.FetchedAt) < maxAge {
		return Result{Body: body, Source: Cache, FetchedAt: m.FetchedAt}, nil
	}

	res, err := c.fetch(ctx, url, m, cached)
	if err != nil {
		if cached {
			return Result{Body: body, Source: Stale, FetchedAt: m.FetchedAt, Err: err}, nil
		}
		return Result{}, err
	}
	if res.Body == nil { // 304 Not Modified
		m.FetchedAt = time.Now()
		c.store(url, m, body)
		return Result{Body: body, Source: Cache, FetchedAt: m.FetchedAt}, nil
	}
	return res, nil
}

func (c *Client) fetch(ctx context.Context, url string, m meta, cached bool) (Result, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return Result{}, err
	}
	req.Header.Set("User-Agent", userAgent)
	req.Header.Set("Accept", "application/rss+xml, application/atom+xml, application/xml;q=0.9, text/xml;q=0.9, text/html;q=0.8, */*;q=0.5")
	if cached {
		if m.ETag != "" {
			req.Header.Set("If-None-Match", m.ETag)
		}
		if m.LastModified != "" {
			req.Header.Set("If-Modified-Since", m.LastModified)
		}
	}

	resp, err := c.HTTP.Do(req)
	if err != nil {
		return Result{}, err
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotModified && cached {
		return Result{}, nil
	}
	if resp.StatusCode != http.StatusOK {
		// Not resp.Status: its reason phrase is server-controlled text that
		// would end up on the user's screen.
		return Result{}, fmt.Errorf("GET %s: %d %s", url, resp.StatusCode, http.StatusText(resp.StatusCode))
	}

	body, err := io.ReadAll(io.LimitReader(resp.Body, MaxBodySize+1))
	if err != nil {
		return Result{}, err
	}
	if len(body) > MaxBodySize {
		return Result{}, fmt.Errorf("GET %s: response larger than %d MB", url, MaxBodySize>>20)
	}

	now := time.Now()
	c.store(url, meta{
		URL:          url,
		ETag:         resp.Header.Get("ETag"),
		LastModified: resp.Header.Get("Last-Modified"),
		FetchedAt:    now,
	}, body)
	return Result{Body: body, Source: Network, FetchedAt: now}, nil
}

func (c *Client) paths(url string) (metaPath, bodyPath string) {
	sum := sha256.Sum256([]byte(url))
	key := hex.EncodeToString(sum[:16])
	return filepath.Join(c.Dir, key+".json"), filepath.Join(c.Dir, key+".body")
}

func (c *Client) load(url string) (meta, []byte, bool) {
	if c.Dir == "" {
		return meta{}, nil, false
	}
	mp, bp := c.paths(url)
	raw, err := os.ReadFile(mp)
	if err != nil {
		return meta{}, nil, false
	}
	var m meta
	if json.Unmarshal(raw, &m) != nil || m.URL != url {
		return meta{}, nil, false
	}
	body, err := os.ReadFile(bp)
	if err != nil {
		return meta{}, nil, false
	}
	return m, body, true
}

// store writes the cache entry. Failures are ignored: the cache is an
// optimisation and must never make a successful fetch fail.
func (c *Client) store(url string, m meta, body []byte) {
	if c.Dir == "" {
		return
	}
	if err := os.MkdirAll(c.Dir, 0o700); err != nil {
		return
	}
	mp, bp := c.paths(url)
	raw, err := json.Marshal(m)
	if err != nil {
		return
	}
	// Body first, then meta: a reader only trusts an entry whose meta exists.
	if writeAtomic(bp, body) == nil {
		_ = writeAtomic(mp, raw)
	}
}

func writeAtomic(path string, data []byte) error {
	f, err := os.CreateTemp(filepath.Dir(path), ".tmp-*")
	if err != nil {
		return err
	}
	_, werr := f.Write(data)
	cerr := f.Close()
	if err := errors.Join(werr, cerr); err != nil {
		os.Remove(f.Name())
		return err
	}
	return os.Rename(f.Name(), path)
}
