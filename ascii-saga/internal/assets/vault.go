package assets

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
)

// Vault is the on-disk registry of locked assets. Once a portrait or
// location passes the curate gate, it lives here forever — guaranteeing
// visual continuity across sessions.
type Vault struct {
	root  string
	mu    sync.RWMutex
	cache map[string]string // key -> art
}

func OpenVault(root string) (*Vault, error) {
	if err := os.MkdirAll(root, 0o755); err != nil {
		return nil, err
	}
	for _, sub := range []string{"portraits", "locations", "items", "scenes"} {
		_ = os.MkdirAll(filepath.Join(root, sub), 0o755)
	}
	return &Vault{root: root, cache: map[string]string{}}, nil
}

// Get returns a locked asset or ("", false) if missing. Category is one
// of "portraits" | "locations" | "items" | "scenes". Name is the stable
// short identifier ("aldric", "santuario_caido", etc.).
func (v *Vault) Get(category, name string) (string, bool) {
	cacheKey := category + "/" + name
	v.mu.RLock()
	if a, ok := v.cache[cacheKey]; ok {
		v.mu.RUnlock()
		return a, true
	}
	v.mu.RUnlock()

	path := filepath.Join(v.root, category, sanitize(name)+".txt")
	b, err := os.ReadFile(path)
	if err != nil {
		return "", false
	}
	art := strings.TrimRight(string(b), "\n")
	v.mu.Lock()
	v.cache[cacheKey] = art
	v.mu.Unlock()
	return art, true
}

// Lock writes an asset to the vault permanently. Idempotent — refuses to
// overwrite an existing file (use Replace for that).
func (v *Vault) Lock(category, name, art string) error {
	path := filepath.Join(v.root, category, sanitize(name)+".txt")
	if _, err := os.Stat(path); err == nil {
		return fmt.Errorf("vault: %s/%s already locked", category, name)
	}
	if err := os.WriteFile(path, []byte(art), 0o644); err != nil {
		return err
	}
	v.mu.Lock()
	v.cache[category+"/"+name] = art
	v.mu.Unlock()
	return nil
}

// Replace forcibly overwrites an existing asset. Use sparingly — defeats
// the consistency guarantee.
func (v *Vault) Replace(category, name, art string) error {
	path := filepath.Join(v.root, category, sanitize(name)+".txt")
	if err := os.WriteFile(path, []byte(art), 0o644); err != nil {
		return err
	}
	v.mu.Lock()
	v.cache[category+"/"+name] = art
	v.mu.Unlock()
	return nil
}

// CacheScene stores a composed scene under a sha256 key for fast reuse.
func (v *Vault) CacheScene(key, art string) error {
	path := filepath.Join(v.root, "scenes", "cache_"+key+".txt")
	return os.WriteFile(path, []byte(art), 0o644)
}

func (v *Vault) GetCachedScene(key string) (string, bool) {
	path := filepath.Join(v.root, "scenes", "cache_"+key+".txt")
	b, err := os.ReadFile(path)
	if err != nil {
		return "", false
	}
	return strings.TrimRight(string(b), "\n"), true
}

func sanitize(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	out := make([]rune, 0, len(s))
	for _, r := range s {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9':
			out = append(out, r)
		case r == ' ', r == '-', r == '_':
			out = append(out, '_')
		}
	}
	if len(out) == 0 {
		return "unnamed"
	}
	return string(out)
}
