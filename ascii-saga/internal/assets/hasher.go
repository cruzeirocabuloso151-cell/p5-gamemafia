// Package assets implements the Artist pipeline:
// vault (locked portraits/locations/items), hasher (cache keys),
// compositor (layered ASCII merge), validator (size + palette enforcement),
// and library (curated fallbacks).
package assets

import (
	"crypto/sha256"
	"encoding/hex"
	"sort"
	"strings"
)

// Key derives a stable cache key for an Artist request. Brief + tags +
// canvas dimensions + style + palette together. Order-independent for tags.
func Key(brief string, tags []string, w, h int, style, palette string) string {
	t := append([]string(nil), tags...)
	sort.Strings(t)
	src := strings.Join([]string{
		strings.TrimSpace(brief),
		strings.Join(t, ","),
		istring(w), istring(h),
		style, palette,
	}, "|")
	sum := sha256.Sum256([]byte(src))
	return hex.EncodeToString(sum[:16]) // 32 hex chars; collisions effectively zero
}

func istring(n int) string {
	if n == 0 {
		return "0"
	}
	buf := [12]byte{}
	i := len(buf)
	neg := n < 0
	if neg {
		n = -n
	}
	for n > 0 {
		i--
		buf[i] = byte('0' + n%10)
		n /= 10
	}
	if neg {
		i--
		buf[i] = '-'
	}
	return string(buf[i:])
}
