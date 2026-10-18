// Package content turns untrusted article HTML into clean, terminal-safe
// Markdown: clutter is removed, links are unwrapped to plain text, and every
// control or escape sequence that could drive the terminal is stripped.
package content

import (
	"bytes"
	"net/url"
	"regexp"
	"strings"
	"unicode"

	htmltomarkdown "github.com/JohannesKaufmann/html-to-markdown/v2"
	"github.com/JohannesKaufmann/html-to-markdown/v2/converter"
	"github.com/charmbracelet/x/ansi"
	readability "github.com/go-shiori/go-readability"
	"golang.org/x/net/html"
)

// dropTags are removed along with everything inside them.
var dropTags = map[string]bool{
	"script": true, "style": true, "noscript": true, "iframe": true,
	"object": true, "embed": true, "form": true, "input": true,
	"button": true, "select": true, "textarea": true, "nav": true,
	"aside": true, "footer": true, "svg": true, "canvas": true,
	"video": true, "audio": true, "picture": true, "img": true,
	"source": true, "template": true, "link": true, "meta": true,
}

// clutterTokens mark share bars, ads, newsletter boxes and similar when they
// appear as a whole word in an element's class or id.
var clutterTokens = map[string]bool{
	"share": true, "sharing": true, "social": true, "subscribe": true,
	"newsletter": true, "advert": true, "advertisement": true, "ad": true,
	"ads": true, "sponsor": true, "sponsored": true, "promo": true,
	"related": true, "comments": true, "cookie": true, "popup": true,
	"modal": true, "signup": true,
}

// ToMarkdown converts an HTML fragment or document to clean Markdown.
// base resolves relative URLs; it may be empty.
func ToMarkdown(htmlSrc, base string) (string, error) {
	doc, err := html.Parse(strings.NewReader(htmlSrc))
	if err != nil {
		return "", err
	}
	clean(doc)

	var opts []converter.ConvertOptionFunc
	if u, err := url.Parse(base); err == nil && u.Host != "" {
		opts = append(opts, converter.WithDomain(u.Scheme+"://"+u.Host))
	}
	md, err := htmltomarkdown.ConvertNode(doc, opts...)
	if err != nil {
		return "", err
	}
	return tidy(Sanitize(string(md))), nil
}

// Extract runs readability over a full web page and returns the main article
// body as HTML, with site chrome (headers, sidebars, comments) removed.
func Extract(page []byte, pageURL string) (string, error) {
	u, err := url.Parse(pageURL)
	if err != nil {
		return "", err
	}
	art, err := readability.FromReader(bytes.NewReader(page), u)
	if err != nil {
		return "", err
	}
	return art.Content, nil
}

// PlainText flattens HTML to a single line of sanitized text.
func PlainText(htmlSrc string) string {
	if !strings.ContainsAny(htmlSrc, "<&") {
		return strings.Join(strings.Fields(Sanitize(htmlSrc)), " ")
	}
	doc, err := html.Parse(strings.NewReader(htmlSrc))
	if err != nil {
		return ""
	}
	var b strings.Builder
	var walk func(*html.Node)
	walk = func(n *html.Node) {
		if n.Type == html.ElementNode && dropTags[n.Data] {
			return
		}
		if n.Type == html.TextNode {
			b.WriteString(n.Data)
			b.WriteByte(' ')
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			walk(c)
		}
	}
	walk(doc)
	return strings.Join(strings.Fields(Sanitize(b.String())), " ")
}

// Sanitize removes terminal escape sequences, control characters (except
// newline and tab) and bidi overrides from untrusted text, so feed content
// cannot recolour, retitle or otherwise drive the user's terminal.
func Sanitize(s string) string {
	s = ansi.Strip(s)
	return strings.Map(func(r rune) rune {
		switch {
		case r == '\n' || r == '\t':
			return r
		case r == '\r':
			return -1
		case unicode.IsControl(r):
			return -1
		case r >= 0x202A && r <= 0x202E, r >= 0x2066 && r <= 0x2069:
			return -1
		case r == unicode.ReplacementChar:
			return -1
		}
		return r
	}, s)
}

func clean(n *html.Node) {
	for c := n.FirstChild; c != nil; {
		next := c.NextSibling
		switch {
		case c.Type == html.CommentNode:
			n.RemoveChild(c)
		case c.Type == html.ElementNode && (dropTags[c.Data] || isClutter(c) || isBoilerplate(c)):
			n.RemoveChild(c)
		case headingLevel(c) > 0 && sponsorHeading.MatchString(strings.TrimSpace(textOf(c))):
			// Drop the sponsor heading and its section, up to the next
			// heading of the same or a higher level.
			level := headingLevel(c)
			for next != nil && (headingLevel(next) == 0 || headingLevel(next) > level) {
				nn := next.NextSibling
				n.RemoveChild(next)
				next = nn
			}
			n.RemoveChild(c)
		case c.Type == html.ElementNode && c.Data == "figcaption":
			// The image itself is gone; label the caption so it does not
			// read as a stray sentence of body text.
			clean(c)
			if t := strings.TrimSpace(textOf(c)); t != "" {
				p := &html.Node{Type: html.ElementNode, Data: "p"}
				em := &html.Node{Type: html.ElementNode, Data: "em"}
				em.AppendChild(&html.Node{Type: html.TextNode, Data: "Figure: " + strings.Join(strings.Fields(t), " ")})
				p.AppendChild(em)
				n.InsertBefore(p, c)
			}
			n.RemoveChild(c)
		case c.Type == html.ElementNode && c.Data == "a":
			// Unwrap links: keep the text, drop the URL noise.
			clean(c)
			for gc := c.FirstChild; gc != nil; {
				gnext := gc.NextSibling
				c.RemoveChild(gc)
				n.InsertBefore(gc, c)
				gc = gnext
			}
			n.RemoveChild(c)
		default:
			clean(c)
		}
		c = next
	}
}

var (
	nonWord        = regexp.MustCompile(`[^a-z0-9]+`)
	sponsorHeading = regexp.MustCompile(`(?i)^(brought to you by|(this (post|episode|issue) is )?sponsored by|our sponsors?|sponsors?)\b`)
	copyrightLine  = regexp.MustCompile(`(?i)^(copyright|©|\(c\))\s`)
)

func headingLevel(n *html.Node) int {
	if n.Type == html.ElementNode && len(n.Data) == 2 && n.Data[0] == 'h' && n.Data[1] >= '1' && n.Data[1] <= '6' {
		return int(n.Data[1] - '0')
	}
	return 0
}

// isBoilerplate matches short copyright footers that readability keeps.
func isBoilerplate(n *html.Node) bool {
	if n.Data != "p" && n.Data != "small" {
		return false
	}
	t := strings.TrimSpace(textOf(n))
	return len(t) < 200 && copyrightLine.MatchString(t)
}

func textOf(n *html.Node) string {
	if n.Type == html.TextNode {
		return n.Data
	}
	var b strings.Builder
	for c := n.FirstChild; c != nil; c = c.NextSibling {
		b.WriteString(textOf(c))
	}
	return b.String()
}

func isClutter(n *html.Node) bool {
	for _, a := range n.Attr {
		if a.Key != "class" && a.Key != "id" {
			continue
		}
		for _, tok := range nonWord.Split(strings.ToLower(a.Val), -1) {
			if clutterTokens[tok] {
				return true
			}
		}
	}
	return false
}

var blankRuns = regexp.MustCompile(`\n{3,}`)

func tidy(md string) string {
	lines := strings.Split(md, "\n")
	for i, l := range lines {
		lines[i] = strings.TrimRight(l, " \t")
	}
	md = strings.Join(lines, "\n")
	return strings.TrimSpace(blankRuns.ReplaceAllString(md, "\n\n"))
}
