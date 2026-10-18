// Package fetch is an HTTP client with a disk cache. Responses are cached with
// their ETag/Last-Modified validators so refreshes are cheap conditional
// requests, transient failures are retried with backoff, and a stale copy is
// served when the network fails. A body only replaces the cached copy once it
// passes the caller's validation, so a bot-check page never evicts a good feed.
package fetch

import (
	"context"
	"crypto/sha256"
	"crypto/tls"
	"crypto/x509"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math/rand/v2"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
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

// Validator vets a fresh 200 body before it is cached and returned. A
// rejected body is treated as a failed fetch: the stale copy, if any, is
// served instead and stays in the cache.
type Validator func(body []byte) error

// maxRetryAfter is the longest Retry-After the client will wait out; a server
// asking for more is treated as a hard failure for this fetch.
const maxRetryAfter = 10 * time.Second

// Client fetches URLs through the disk cache. It is safe for concurrent use;
// concurrent Gets of the same URL share one request.
type Client struct {
	HTTP *http.Client
	Dir  string // cache directory; empty disables the disk cache

	// MaxRetries is how many times a transient failure (timeout, reset
	// connection, 5xx, 408, 429) is retried. BaseDelay is the first backoff;
	// each further retry waits three times longer, with jitter.
	MaxRetries int
	BaseDelay  time.Duration

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
		HTTP:       &http.Client{Timeout: 25 * time.Second},
		Dir:        dir,
		MaxRetries: 2,
		BaseDelay:  500 * time.Millisecond,
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
	return c.GetValid(ctx, url, maxAge, nil)
}

// GetValid is Get with a validator for fresh bodies; validate may be nil.
// Concurrent calls for the same URL share one request and the first
// caller's validator.
func (c *Client) GetValid(ctx context.Context, url string, maxAge time.Duration, validate Validator) (Result, error) {
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

	cl.res, cl.err = c.get(ctx, url, maxAge, validate)

	c.mu.Lock()
	delete(c.inflight, url)
	c.mu.Unlock()
	close(cl.done)
	return cl.res, cl.err
}

func (c *Client) get(ctx context.Context, url string, maxAge time.Duration, validate Validator) (Result, error) {
	m, body, cached := c.load(url)
	if cached && maxAge > 0 && time.Since(m.FetchedAt) < maxAge {
		return Result{Body: body, Source: Cache, FetchedAt: m.FetchedAt}, nil
	}

	res, err := c.fetch(ctx, url, m, cached, validate)
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

// fetch performs the request, retrying transient failures with backoff for
// as long as ctx allows.
func (c *Client) fetch(ctx context.Context, url string, m meta, cached bool, validate Validator) (Result, error) {
	for attempt := 0; ; attempt++ {
		res, err := c.fetchOnce(ctx, url, m, cached, validate)
		if err == nil {
			return res, nil
		}
		wait, ok := c.retryDelay(ctx, err, attempt)
		if !ok {
			return Result{}, err
		}
		if dl, has := ctx.Deadline(); has && time.Until(dl) < wait {
			return Result{}, err // no time left for another attempt
		}
		t := time.NewTimer(wait)
		select {
		case <-ctx.Done():
			t.Stop()
			return Result{}, err
		case <-t.C:
		}
	}
}

// statusError is a non-200 response.
type statusError struct {
	url        string
	code       int
	retryAfter time.Duration
}

func (e *statusError) Error() string {
	// Not resp.Status: its reason phrase is server-controlled text that
	// would end up on the user's screen.
	return fmt.Sprintf("GET %s: %d %s", e.url, e.code, http.StatusText(e.code))
}

// permanentError is a failure that retrying cannot fix.
type permanentError struct{ err error }

func (e *permanentError) Error() string { return e.err.Error() }
func (e *permanentError) Unwrap() error { return e.err }

func (c *Client) retryDelay(ctx context.Context, err error, attempt int) (time.Duration, bool) {
	if attempt >= c.MaxRetries || !retryable(ctx, err) {
		return 0, false
	}
	var se *statusError
	if errors.As(err, &se) && se.retryAfter > 0 {
		if se.retryAfter > maxRetryAfter {
			return 0, false
		}
		return se.retryAfter, true
	}
	d := c.BaseDelay
	for range attempt {
		d *= 3
	}
	return time.Duration(float64(d) * (0.75 + rand.Float64()/2)), true
}

func retryable(ctx context.Context, err error) bool {
	if ctx.Err() != nil {
		return false // the caller gave up, not the server
	}
	var (
		perm     *permanentError
		se       *statusError
		certErr  *tls.CertificateVerificationError
		unknown  x509.UnknownAuthorityError
		invalid  x509.CertificateInvalidError
		hostname x509.HostnameError
		dnsErr   *net.DNSError
	)
	switch {
	case errors.As(err, &perm):
		return false
	case errors.As(err, &se):
		switch se.code {
		case http.StatusRequestTimeout, http.StatusTooManyRequests,
			http.StatusInternalServerError, http.StatusBadGateway,
			http.StatusServiceUnavailable, http.StatusGatewayTimeout:
			return true
		}
		return false
	case errors.As(err, &certErr), errors.As(err, &unknown),
		errors.As(err, &invalid), errors.As(err, &hostname):
		return false // a bad certificate won't fix itself in a second
	case errors.As(err, &dnsErr) && dnsErr.IsNotFound:
		return false
	}
	return true // timeouts, resets, refused connections, truncated bodies
}

func parseRetryAfter(v string) time.Duration {
	if v == "" {
		return 0
	}
	if s, err := strconv.Atoi(v); err == nil && s >= 0 {
		return time.Duration(s) * time.Second
	}
	if t, err := http.ParseTime(v); err == nil {
		return max(0, time.Until(t))
	}
	return 0
}

func (c *Client) fetchOnce(ctx context.Context, url string, m meta, cached bool, validate Validator) (Result, error) {
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
		return Result{}, &statusError{url: url, code: resp.StatusCode, retryAfter: parseRetryAfter(resp.Header.Get("Retry-After"))}
	}

	body, err := io.ReadAll(io.LimitReader(resp.Body, MaxBodySize+1))
	if err != nil {
		return Result{}, err
	}
	if len(body) > MaxBodySize {
		return Result{}, &permanentError{fmt.Errorf("GET %s: response larger than %d MB", url, MaxBodySize>>20)}
	}
	if validate != nil {
		if err := validate(body); err != nil {
			return Result{}, &permanentError{fmt.Errorf("GET %s: unusable response: %w", url, err)}
		}
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
