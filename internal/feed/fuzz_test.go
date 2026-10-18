package feed

import (
	"strings"
	"testing"
)

func FuzzParse(f *testing.F) {
	f.Add([]byte(rss))
	f.Add([]byte(atom))
	f.Add([]byte(`{"version":"https://jsonfeed.org/version/1.1","title":"j","items":[{"id":"1","url":"/a","content_html":"<p>x</p>"}]}`))
	f.Add([]byte("<rss><channel><item><title>&#x1b;[2J</title><link>javascript:alert(1)</link></item></channel></rss>"))
	f.Add([]byte("<html><body>not a feed</body></html>"))
	f.Fuzz(func(t *testing.T, body []byte) {
		fd, err := Parse(body, "https://example.test/feed.xml")
		if err != nil {
			return
		}
		for _, a := range fd.Articles {
			if strings.ContainsAny(a.Title+a.Author+a.Summary, "\x1b\x07\x00") {
				t.Fatalf("control bytes in article text: %q", a.Title)
			}
			if a.Link != "" && !strings.HasPrefix(a.Link, "http://") && !strings.HasPrefix(a.Link, "https://") {
				t.Fatalf("non-http link survived: %q", a.Link)
			}
		}
	})
}
