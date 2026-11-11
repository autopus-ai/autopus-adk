package freshness

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"time"
)

// DefaultCacheDir is AUTOPUS_DEPS_CACHE_DIR, else $XDG_STATE_HOME/autopus/deps,
// else <home>/.local/state/autopus/deps, the same root the disk harness uses.
// "" (no home) turns the cache off.
func DefaultCacheDir() string {
	if dir := os.Getenv("AUTOPUS_DEPS_CACHE_DIR"); dir != "" {
		return dir
	}
	if xdg := os.Getenv("XDG_STATE_HOME"); filepath.IsAbs(xdg) {
		return filepath.Join(xdg, "autopus", "deps")
	}
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		return ""
	}
	return filepath.Join(home, ".local", "state", "autopus", "deps")
}

type cacheEntry struct {
	Key    string `json:"key"`
	Latest Latest `json:"latest"`
}

func (c *Client) cachePath(key string) string {
	if c.CacheDir == "" {
		return ""
	}
	sum := sha256.Sum256([]byte(key))
	return filepath.Join(c.CacheDir, hex.EncodeToString(sum[:12])+".json")
}

// readCache returns a stored answer younger than the TTL. Offline mode
// accepts any age: a stale answer beats none when nothing can be fetched.
func (c *Client) readCache(key string, now time.Time) (Latest, bool) {
	p := c.cachePath(key)
	if p == "" {
		return Latest{}, false
	}
	data, err := os.ReadFile(p)
	if err != nil {
		return Latest{}, false
	}
	var e cacheEntry
	if json.Unmarshal(data, &e) != nil || e.Key != key || e.Latest.Version == "" {
		return Latest{}, false
	}
	if !c.Offline && now.Sub(e.Latest.FetchedAt) > c.TTL {
		return Latest{}, false
	}
	return e.Latest, true
}

// writeCache stores an answer; a failure only costs the next lookup.
func (c *Client) writeCache(key string, latest Latest) {
	p := c.cachePath(key)
	if p == "" || os.MkdirAll(filepath.Dir(p), 0o700) != nil {
		return
	}
	data, err := json.Marshal(cacheEntry{Key: key, Latest: latest})
	if err != nil {
		return
	}
	f, err := os.CreateTemp(filepath.Dir(p), ".entry-*")
	if err != nil {
		return
	}
	_, werr := f.Write(data)
	if cerr := f.Close(); werr != nil || cerr != nil || os.Rename(f.Name(), p) != nil {
		_ = os.Remove(f.Name())
	}
}
