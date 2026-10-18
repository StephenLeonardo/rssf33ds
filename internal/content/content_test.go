package content

import (
	"strings"
	"testing"
)

func TestSanitizeStripsTerminalControl(t *testing.T) {
	in := "safe \x1b]0;pwned\x07title \x1b[31mred\x1b[0m\r\n\x07bell‮evil\x00"
	got := Sanitize(in)
	if strings.ContainsAny(got, "\x1b\x07\r\x00‮") {
		t.Fatalf("control bytes survived: %q", got)
	}
	if want := "safe title red\nbellevil"; got != want {
		t.Fatalf("Sanitize = %q, want %q", got, want)
	}
}

func TestToMarkdownRemovesClutter(t *testing.T) {
	in := `<article>
<nav>Home | Blog</nav>
<h1>Consensus</h1>
<p>Raft uses <a href="/terms">leader election</a>.</p>
<script>alert(1)</script><style>p{}</style>
<img src="https://medium.com/_/stat?event=post" alt="">
<div class="share-bar">Share on X</div>
<div id="newsletter-signup">Subscribe!</div>
<iframe src="https://ads.example"></iframe>
<pre><code>func main() {}</code></pre>
<h2>Brought to you by</h2>
<p>Acme DB — the fastest database.</p>
<h2>Log replication</h2>
<p>Entries are appended.</p>
<p>Copyright © 2026 Example, LLC.</p>
</article>`
	md, err := ToMarkdown(in, "https://example.com/post")
	if err != nil {
		t.Fatal(err)
	}
	for _, bad := range []string{"Home | Blog", "alert", "p{}", "stat?event", "Share on X", "Subscribe", "ads.example", "](/terms)", "Acme DB", "Brought to you", "Copyright"} {
		if strings.Contains(md, bad) {
			t.Errorf("clutter %q survived:\n%s", bad, md)
		}
	}
	for _, good := range []string{"# Consensus", "leader election", "func main() {}", "## Log replication", "Entries are appended."} {
		if !strings.Contains(md, good) {
			t.Errorf("content %q lost:\n%s", good, md)
		}
	}
}

func TestFigureCaptionsAreLabelled(t *testing.T) {
	md, err := ToMarkdown(`<figure><img src="x.png"><figcaption>Request  flow <b>overview</b></figcaption></figure><p>Body</p>`, "")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(md, "*Figure: Request flow overview*") || strings.Contains(md, "x.png") {
		t.Fatalf("caption not labelled:\n%s", md)
	}
}

func TestClutterMatchesWholeTokensOnly(t *testing.T) {
	// "threads" and "shared-nothing" contain clutter substrings but are content.
	in := `<div class="threads">Thread pools</div><div class="shared-nothing">Shards</div>`
	md, err := ToMarkdown(in, "")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(md, "Thread pools") {
		t.Errorf("content removed by substring match: %q", md)
	}
}

func TestPlainText(t *testing.T) {
	got := PlainText(`<p>Hello <b>world</b></p><script>x()</script><p>again &amp; again</p>`)
	if want := "Hello world again & again"; got != want {
		t.Fatalf("PlainText = %q, want %q", got, want)
	}
	if got := PlainText("plain\x1b[2J text"); got != "plain text" {
		t.Fatalf("PlainText(plain) = %q", got)
	}
}
