package content

import (
	"strings"
	"testing"
	"unicode"
)

func FuzzToMarkdown(f *testing.F) {
	f.Add(`<article><h2>Brought to you by</h2><p>x</p><h2>Real</h2><figure><img src=a><figcaption>c</figcaption></figure><a href="/x">y</a></article>`)
	f.Add("<p>\x1b]0;title\x07 \x1b[31mred</p><script>x</script>")
	f.Add("<table><tr><td><pre><code>a\tb</code></pre></td></tr></table>")
	f.Add("<<<>>>&#;&amp;<p")
	f.Fuzz(func(t *testing.T, in string) {
		md, err := ToMarkdown(in, "https://example.test/post")
		if err != nil {
			return
		}
		for _, r := range md + PlainText(in) {
			if r != '\n' && r != '\t' && unicode.IsControl(r) {
				t.Fatalf("control rune %U in output %q", r, md)
			}
		}
		if strings.Contains(md, "‮") {
			t.Fatal("bidi override in output")
		}
	})
}
