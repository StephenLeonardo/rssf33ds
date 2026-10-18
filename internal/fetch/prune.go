package fetch

import (
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// Default limits for Prune.
const (
	PruneAge   = 30 * 24 * time.Hour
	PruneBytes = 200 << 20
)

// tmpGrace is how old a temp file or a half-written entry must be before it
// counts as debris: younger ones may belong to a write still in progress.
const tmpGrace = time.Hour

// Prune deletes cache entries not written or revalidated within maxAge, then
// the oldest remaining entries until the cache holds at most maxBytes. It
// also removes temp files and half-written entries left by interrupted
// writes. Only files named like cache entries are touched, so pointing the
// cache at a directory with other files in it is safe.
func (c *Client) Prune(maxAge time.Duration, maxBytes int64) (removed int, err error) {
	if c.Dir == "" {
		return 0, nil
	}
	dirents, err := os.ReadDir(c.Dir)
	if err != nil {
		if os.IsNotExist(err) {
			return 0, nil
		}
		return 0, err
	}

	type entry struct {
		key        string
		meta, body bool
		mod        time.Time // of the meta file: rewritten on every revalidation
		newest     time.Time // of either file
		size       int64
	}
	entries := map[string]*entry{}
	now := time.Now()
	remove := func(name string) {
		if os.Remove(filepath.Join(c.Dir, name)) == nil {
			removed++
		}
	}

	for _, d := range dirents {
		name := d.Name()
		info, err := d.Info()
		if err != nil || !info.Mode().IsRegular() {
			continue
		}
		if strings.HasPrefix(name, ".tmp-") {
			if now.Sub(info.ModTime()) > tmpGrace {
				remove(name)
			}
			continue
		}
		ext := filepath.Ext(name)
		key := strings.TrimSuffix(name, ext)
		if (ext != ".json" && ext != ".body") || !isKey(key) {
			continue
		}
		e := entries[key]
		if e == nil {
			e = &entry{key: key}
			entries[key] = e
		}
		e.size += info.Size()
		if info.ModTime().After(e.newest) {
			e.newest = info.ModTime()
		}
		if ext == ".json" {
			e.meta, e.mod = true, info.ModTime()
		} else {
			e.body = true
		}
	}

	drop := func(e *entry) {
		remove(e.key + ".json")
		remove(e.key + ".body")
	}
	var live []*entry
	var total int64
	for _, e := range entries {
		switch {
		case !e.meta || !e.body:
			if now.Sub(e.newest) > tmpGrace {
				drop(e)
			}
		case now.Sub(e.mod) > maxAge:
			drop(e)
		default:
			live = append(live, e)
			total += e.size
		}
	}

	sort.Slice(live, func(i, j int) bool { return live[i].mod.Before(live[j].mod) })
	for _, e := range live {
		if total <= maxBytes {
			break
		}
		drop(e)
		total -= e.size
	}
	return removed, nil
}

// isKey reports whether s looks like a cache key: 32 lower-case hex digits.
func isKey(s string) bool {
	if len(s) != 32 {
		return false
	}
	for _, r := range s {
		if (r < '0' || r > '9') && (r < 'a' || r > 'f') {
			return false
		}
	}
	return true
}
