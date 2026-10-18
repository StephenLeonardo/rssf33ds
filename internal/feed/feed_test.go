package feed

import (
	"strings"
	"testing"
)

const rss = `<?xml version="1.0"?>
<rss version="2.0" xmlns:content="http://purl.org/rss/1.0/modules/content/">
<channel><title>Example Eng</title><link>https://eng.example.com/</link>
<item><title>Old &lt;b&gt;post&lt;/b&gt;</title><link>/old</link>
  <pubDate>Mon, 02 Jan 2023 15:04:05 GMT</pubDate><description>&lt;p&gt;Old summary&lt;/p&gt;</description></item>
<item><title>New post &#x1b;]0;pwned&#x7;</title><link>https://eng.example.com/new</link>
  <pubDate>Tue, 10 Oct 2026 10:00:00 GMT</pubDate>
  <description>Short</description><content:encoded><![CDATA[<p>Full body</p>]]></content:encoded></item>
<item><title></title><guid isPermaLink="false">urn:uuid:1234</guid><description>Untitled entry text</description></item>
</channel></rss>`

const atom = `<?xml version="1.0" encoding="utf-8"?>
<feed xmlns="http://www.w3.org/2005/Atom"><title>Atom Blog</title>
<link href="https://atom.example.org/"/>
<entry><title>Only updated</title><link rel="alternate" href="posts/1"/>
  <updated>2026-05-01T12:00:00Z</updated><summary>S1</summary></entry>
<entry><title>Published</title><link href="https://atom.example.org/posts/2"/>
  <published>2026-06-01T12:00:00Z</published><content type="html">&lt;p&gt;Body&lt;/p&gt;</content></entry>
</feed>`

func TestParseRSS(t *testing.T) {
	f, err := Parse([]byte(rss), "https://eng.example.com/feed.xml")
	if err != nil {
		t.Fatal(err)
	}
	if len(f.Articles) != 3 {
		t.Fatalf("got %d articles", len(f.Articles))
	}
	a := f.Articles[0]
	if !strings.HasPrefix(a.Title, "New post") || a.Link != "https://eng.example.com/new" || a.HTML != "<p>Full body</p>" {
		t.Errorf("newest = %+v", a)
	}
	if f.Articles[1].Link != "https://eng.example.com/old" || f.Articles[1].Title != "Old post" || f.Articles[1].Summary != "Old summary" {
		t.Errorf("relative link/title not resolved: %+v", f.Articles[1])
	}
	last := f.Articles[2]
	if last.Link != "" || !last.Published.IsZero() || last.Title != "Untitled entry text" {
		t.Errorf("undated/untitled entry = %+v", last)
	}
	for _, a := range f.Articles {
		if strings.ContainsAny(a.Title, "\x1b\x07") {
			t.Errorf("escape in title %q", a.Title)
		}
	}
}

func TestParseAtom(t *testing.T) {
	f, err := Parse([]byte(atom), "https://atom.example.org/feed.atom")
	if err != nil {
		t.Fatal(err)
	}
	if f.Title != "Atom Blog" || len(f.Articles) != 2 {
		t.Fatalf("feed = %+v", f)
	}
	if f.Articles[0].Title != "Published" || f.Articles[1].Link != "https://atom.example.org/posts/1" {
		t.Errorf("articles = %+v", f.Articles)
	}
	if f.Articles[1].Published.IsZero() {
		t.Error("updated date not used as fallback")
	}
}

func TestScrubXML(t *testing.T) {
	in := "a&#x1b;b&#27;c&#9;d&#x1F;e&#32;f\x01g&#xB;h&#12;i&#10;"
	if got, want := string(scrubXML([]byte(in))), "abc&#9;de&#32;fghi&#10;"; got != want {
		t.Fatalf("scrubXML = %q, want %q", got, want)
	}
}

func TestParseGarbage(t *testing.T) {
	if _, err := Parse([]byte("<html><body>not a feed</body></html>"), "https://x"); err == nil {
		t.Fatal("expected error for HTML page")
	}
}
