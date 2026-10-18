package fetch

import (
	"context"
	"net/http"
	"net/http/httptest"
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
	c := New(t.TempDir())
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
