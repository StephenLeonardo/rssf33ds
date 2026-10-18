package search

import "testing"

func FuzzSearch(f *testing.F) {
	f.Add("Raft consensus", "leader election and log replication", `raft "log rep`)
	f.Add("Ünïcödé İstanbul", "ſtraße KELVIN K sign", "straße k")
	f.Add("x", "\xff\xfe invalid utf8 \xc3", "\xc3")
	f.Fuzz(func(t *testing.T, title, body, query string) {
		d := doc("Feed", title, body, 1)
		for _, h := range Search([]*Doc{d}, query, 10) {
			_ = h.Snippet
		}
	})
}
