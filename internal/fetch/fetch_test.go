package fetch

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestCacheRevalidateAndStale(t *testing.T) {
	var hits, notModified atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		if r.Header.Get("If-None-Match") == `"v1"` {
			notModified.Add(1)
			w.WriteHeader(http.StatusNotModified)
			return
		}
		w.Header().Set("ETag", `"v1"`)
		w.Write([]byte("<rss/>"))
	}))
	c := fastClient(t.TempDir())
	ctx := context.Background()

	r, err := c.Get(ctx, srv.URL, time.Hour)
	if err != nil || r.Source != Network || string(r.Body) != "<rss/>" {
		t.Fatalf("first get: %+v %v", r, err)
	}
	r, err = c.Get(ctx, srv.URL, time.Hour)
	if err != nil || r.Source != Cache || hits.Load() != 1 {
		t.Fatalf("fresh cache should not hit network: %+v hits=%d", r, hits.Load())
	}
	r, err = c.Get(ctx, srv.URL, 0)
	if err != nil || r.Source != Cache || notModified.Load() != 1 || string(r.Body) != "<rss/>" {
		t.Fatalf("revalidate: %+v %v 304s=%d", r, err, notModified.Load())
	}

	srv.Close()
	r, err = c.Get(ctx, srv.URL, 0)
	if err != nil || r.Source != Stale || r.Err == nil || string(r.Body) != "<rss/>" {
		t.Fatalf("offline should serve stale: %+v %v", r, err)
	}
}

func TestErrorsWithoutCache(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "nope", http.StatusForbidden)
	}))
	defer srv.Close()
	if _, err := New(t.TempDir()).Get(context.Background(), srv.URL, time.Hour); err == nil || !strings.Contains(err.Error(), "403") {
		t.Fatalf("want 403 error, got %v", err)
	}
}

func TestSizeLimit(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		chunk := make([]byte, 1<<20)
		for i := 0; i <= MaxBodySize>>20; i++ {
			w.Write(chunk)
		}
	}))
	defer srv.Close()
	if _, err := New("").Get(context.Background(), srv.URL, 0); err == nil {
		t.Fatal("oversized body accepted")
	}
}

func TestConcurrentGetsShareOneRequest(t *testing.T) {
	var hits atomic.Int32
	release := make(chan struct{})
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		<-release
		w.Write([]byte("ok"))
	}))
	defer srv.Close()
	c := New("")
	var wg sync.WaitGroup
	for range 5 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if r, err := c.Get(context.Background(), srv.URL, 0); err != nil || string(r.Body) != "ok" {
				t.Errorf("get: %v", err)
			}
		}()
	}
	time.Sleep(100 * time.Millisecond)
	close(release)
	wg.Wait()
	if hits.Load() != 1 {
		t.Fatalf("hits = %d, want 1", hits.Load())
	}
}

func fastClient(dir string) *Client {
	c := New(dir)
	c.BaseDelay = time.Millisecond
	return c
}

func TestValidatorProtectsGoodCache(t *testing.T) {
	var challenge atomic.Bool
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if challenge.Load() {
			w.Write([]byte("<html>Just a moment...</html>"))
			return
		}
		w.Write([]byte("<rss>good</rss>"))
	}))
	defer srv.Close()
	isFeed := func(b []byte) error {
		if !strings.HasPrefix(string(b), "<rss") {
			return errors.New("not a feed")
		}
		return nil
	}
	c := fastClient(t.TempDir())
	ctx := context.Background()
	if _, err := c.GetValid(ctx, srv.URL, 0, isFeed); err != nil {
		t.Fatal(err)
	}

	challenge.Store(true)
	for range 2 { // twice: the bad body must not have been cached by the first
		r, err := c.GetValid(ctx, srv.URL, 0, isFeed)
		if err != nil || r.Source != Stale || string(r.Body) != "<rss>good</rss>" || r.Err == nil {
			t.Fatalf("challenge page replaced the good copy: %+v %v", r, err)
		}
	}
	if _, err := fastClient(t.TempDir()).GetValid(ctx, srv.URL, 0, isFeed); err == nil || !strings.Contains(err.Error(), "unusable response") {
		t.Fatalf("with nothing cached, want a validation error, got %v", err)
	}
}

func TestRetriesTransientFailures(t *testing.T) {
	var hits atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if hits.Add(1) <= 2 {
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		w.Write([]byte("ok"))
	}))
	defer srv.Close()
	r, err := fastClient("").Get(context.Background(), srv.URL, 0)
	if err != nil || string(r.Body) != "ok" || hits.Load() != 3 {
		t.Fatalf("want success on third try: %v hits=%d", err, hits.Load())
	}
}

func TestDoesNotRetryPermanentFailures(t *testing.T) {
	for _, tc := range []struct {
		name     string
		status   int
		header   string
		validate Validator
	}{
		{name: "404", status: http.StatusNotFound},
		{name: "403", status: http.StatusForbidden},
		{name: "429 asking too long", status: http.StatusTooManyRequests, header: "120"},
		{name: "invalid body", status: http.StatusOK, validate: func([]byte) error { return errors.New("bad") }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var hits atomic.Int32
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				hits.Add(1)
				if tc.header != "" {
					w.Header().Set("Retry-After", tc.header)
				}
				w.WriteHeader(tc.status)
			}))
			defer srv.Close()
			if _, err := fastClient("").GetValid(context.Background(), srv.URL, 0, tc.validate); err == nil {
				t.Fatal("want error")
			}
			if hits.Load() != 1 {
				t.Fatalf("hits = %d, want 1", hits.Load())
			}
		})
	}
}

func TestRetryAfterIsHonoured(t *testing.T) {
	var hits atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if hits.Add(1) == 1 {
			w.Header().Set("Retry-After", "1")
			w.WriteHeader(http.StatusTooManyRequests)
			return
		}
		w.Write([]byte("ok"))
	}))
	defer srv.Close()
	start := time.Now()
	if _, err := fastClient("").Get(context.Background(), srv.URL, 0); err != nil {
		t.Fatal(err)
	}
	if d := time.Since(start); d < time.Second {
		t.Fatalf("retried after %v, before Retry-After elapsed", d)
	}
}

func TestRetryStopsAtDeadline(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadGateway)
	}))
	defer srv.Close()
	c := New("")
	c.BaseDelay = 5 * time.Second
	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()
	start := time.Now()
	if _, err := c.Get(ctx, srv.URL, 0); err == nil || !strings.Contains(err.Error(), "502") {
		t.Fatalf("want the 502, got %v", err)
	}
	if d := time.Since(start); d > time.Second {
		t.Fatalf("waited %v past the deadline", d)
	}
}

func TestRetryable(t *testing.T) {
	ctx := context.Background()
	if retryable(ctx, &tls.CertificateVerificationError{Err: x509.UnknownAuthorityError{}}) {
		t.Error("certificate errors must not be retried")
	}
	if retryable(ctx, &net.DNSError{IsNotFound: true}) {
		t.Error("unknown hosts must not be retried")
	}
	if !retryable(ctx, &net.OpError{Op: "read", Err: errors.New("connection reset by peer")}) {
		t.Error("resets should be retried")
	}
	done, cancel := context.WithCancel(ctx)
	cancel()
	if retryable(done, errors.New("anything")) {
		t.Error("nothing is retried once the caller has given up")
	}
}

func TestParseRetryAfter(t *testing.T) {
	if d := parseRetryAfter("3"); d != 3*time.Second {
		t.Errorf("seconds: %v", d)
	}
	if d := parseRetryAfter(time.Now().Add(5 * time.Second).UTC().Format(http.TimeFormat)); d < 3*time.Second || d > 6*time.Second {
		t.Errorf("date: %v", d)
	}
	if d := parseRetryAfter("soon"); d != 0 {
		t.Errorf("garbage: %v", d)
	}
}

func TestPrune(t *testing.T) {
	dir := t.TempDir()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(strings.Repeat("x", 1000)))
	}))
	defer srv.Close()
	c := New(dir)
	urls := []string{srv.URL + "/old", srv.URL + "/mid", srv.URL + "/new", srv.URL + "/newest"}
	for _, u := range urls {
		if _, err := c.Get(context.Background(), u, 0); err != nil {
			t.Fatal(err)
		}
	}
	now := time.Now()
	age := func(u string, d time.Duration) {
		mp, bp := c.paths(u)
		for _, p := range []string{mp, bp} {
			if err := os.Chtimes(p, now.Add(-d), now.Add(-d)); err != nil {
				t.Fatal(err)
			}
		}
	}
	age(urls[0], 40*24*time.Hour) // past maxAge
	age(urls[1], 3*time.Hour)
	age(urls[2], 2*time.Hour)
	age(urls[3], time.Minute)

	old := now.Add(-2 * tmpGrace)
	write := func(name string, mod time.Time) {
		p := filepath.Join(dir, name)
		os.WriteFile(p, []byte("x"), 0o600)
		os.Chtimes(p, mod, mod)
	}
	write(".tmp-123", old)                      // debris
	write(".tmp-456", now)                      // write in progress
	write(strings.Repeat("a", 32)+".body", old) // orphan body
	write(strings.Repeat("b", 32)+".body", now) // body whose meta is being written
	write("notes.txt", old)                     // not ours
	write(strings.Repeat("z", 32)+".json", old) // not a hex key

	// Room for two entries: "old" goes for age, "mid" for size.
	if _, err := c.Prune(30*24*time.Hour, 2*1100); err != nil {
		t.Fatal(err)
	}
	for i, u := range urls {
		_, _, ok := c.load(u)
		if want := i >= 2; ok != want {
			t.Errorf("%s: cached=%v, want %v", u, ok, want)
		}
	}
	for name, want := range map[string]bool{
		".tmp-123": false, ".tmp-456": true,
		strings.Repeat("a", 32) + ".body": false, strings.Repeat("b", 32) + ".body": true,
		"notes.txt": true, strings.Repeat("z", 32) + ".json": true,
	} {
		if _, err := os.Stat(filepath.Join(dir, name)); (err == nil) != want {
			t.Errorf("%s: exists=%v, want %v", name, err == nil, want)
		}
	}
}
