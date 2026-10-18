// Package catalog holds the curated list of backend and systems engineering
// feeds. Every URL here was verified live (HTTP 200 + parseable RSS/Atom);
// run `go run ./cmd/verifyfeeds` to re-check them and append the results to
// the findings graph in .staged-agents/findings.json.
package catalog

// Feed is one curated source.
type Feed struct {
	Name     string
	URL      string
	Category string
}

const (
	Company = "Company engineering"
	People  = "Engineers & researchers"
	Pubs    = "Publications"
)

// Feeds is the default catalog, grouped by category.
var Feeds = []Feed{
	{"Netflix TechBlog", "https://netflixtechblog.com/feed", Company},
	{"Cloudflare Blog", "https://blog.cloudflare.com/rss/", Company},
	{"AWS Architecture Blog", "https://aws.amazon.com/blogs/architecture/feed/", Company},
	{"Stripe Blog", "https://stripe.com/blog/feed.rss", Company},
	{"Stripe Dev Blog", "https://stripe.dev/feed", Company},
	{"Datadog Engineering", "https://www.datadoghq.com/blog/engineering/index.xml", Company},
	{"Meta Engineering", "https://engineering.fb.com/feed/", Company},
	{"Discord Blog", "https://discord.com/blog/rss.xml", Company},
	{"Slack Engineering", "https://slack.engineering/feed/", Company},
	{"Dropbox Tech", "https://dropbox.tech/feed", Company},
	{"Spotify Engineering", "https://engineering.atspotify.com/feed/", Company},
	{"GitHub Engineering", "https://github.blog/engineering/feed/", Company},
	{"Airbnb Engineering", "https://medium.com/feed/airbnb-engineering", Company},
	{"Pinterest Engineering", "https://medium.com/feed/pinterest-engineering", Company},
	{"Shopify Engineering", "https://shopify.engineering/blog.atom", Company},
	{"Lyft Engineering", "https://eng.lyft.com/feed", Company},
	{"Grab Engineering", "https://engineering.grab.com/feed.xml", Company},
	{"Zalando Engineering", "https://engineering.zalando.com/atom.xml", Company},
	{"Salesforce Engineering", "https://engineering.salesforce.com/feed/", Company},
	{"Fly.io Blog", "https://fly.io/blog/feed.xml", Company},
	{"PlanetScale Blog", "https://planetscale.com/blog/rss.xml", Company},
	{"Tailscale Blog", "https://tailscale.com/blog/index.xml", Company},
	{"Honeycomb Blog", "https://www.honeycomb.io/feed", Company},
	{"Confluent Blog", "https://www.confluent.io/rss.xml", Company},
	{"Jane Street Tech Blog", "https://blog.janestreet.com/feed.xml", Company},

	{"Martin Fowler", "https://martinfowler.com/feed.atom", People},
	{"All Things Distributed (Werner Vogels)", "https://www.allthingsdistributed.com/atom.xml", People},
	{"Marc Brooker", "https://brooker.co.za/blog/rss.xml", People},
	{"Brendan Gregg", "https://www.brendangregg.com/blog/rss.xml", People},
	{"Metadata (Murat Demirbas)", "https://muratbuffalo.blogspot.com/feeds/posts/default", People},
	{"Jepsen", "https://jepsen.io/blog.rss", People},
	{"Aphyr", "https://aphyr.com/posts.atom", People},
	{"Dan Luu", "https://danluu.com/atom.xml", People},

	{"High Scalability", "https://highscalability.com/rss/", Pubs},
	{"The Pragmatic Engineer", "https://newsletter.pragmaticengineer.com/feed", Pubs},
	{"ByteByteGo", "https://blog.bytebytego.com/feed", Pubs},
	{"InfoQ Architecture & Design", "https://www.infoq.com/feed/architecture-design/", Pubs},
}
