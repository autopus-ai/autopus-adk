package freshness

import (
	"os"
	"path"
	"path/filepath"
	"strings"
)

// goSetting reads a Go setting the way the go command does: the process
// environment first, then the `go env -w` file (GOENV, or the user config
// dir's go/env). A setting the user made with `go env -w GOPRIVATE=...`
// must keep private module paths away from a public proxy here too.
func (c *Client) goSetting(key string) string {
	if v := c.env(key); v != "" {
		return v
	}
	file := c.GoEnvFile
	if file == "" {
		file = c.env("GOENV")
	}
	if file == "" {
		if dir, err := os.UserConfigDir(); err == nil {
			file = filepath.Join(dir, "go", "env")
		}
	}
	if file == "" || file == "off" {
		return ""
	}
	data, err := os.ReadFile(file)
	if err != nil {
		return ""
	}
	for _, line := range strings.Split(string(data), "\n") {
		if k, v, ok := strings.Cut(strings.TrimSpace(line), "="); ok && k == key {
			return strings.TrimSpace(v)
		}
	}
	return ""
}

// goPrivate returns a reason when the module matches GOPRIVATE, GONOPROXY or
// GONOSUMDB: such a path is never sent to a proxy by this package.
func (c *Client) goPrivate(module string) string {
	for _, key := range []string{"GOPRIVATE", "GONOPROXY", "GONOSUMDB"} {
		if matchGlobPrefix(c.goSetting(key), module) {
			return "private module (" + key + "); not sent to a proxy"
		}
	}
	return ""
}

// goProxy picks the first HTTP proxy of GOPROXY. "off" or a list with only
// "direct" means there is no proxy to ask; the module is then unknown.
func (c *Client) goProxy() (proxy, reason string) {
	setting := c.goSetting("GOPROXY")
	if setting == "" {
		return strings.TrimRight(c.Endpoints.GoProxy, "/"), ""
	}
	for _, entry := range strings.FieldsFunc(setting, func(r rune) bool { return r == ',' || r == '|' }) {
		entry = strings.TrimSpace(entry)
		switch {
		case entry == "off":
			return "", "GOPROXY=off"
		case entry == "direct":
			return "", "GOPROXY reaches direct before any proxy; nothing to ask"
		case strings.HasPrefix(entry, "https://") || strings.HasPrefix(entry, "http://"):
			if entry == "https://proxy.golang.org" {
				return strings.TrimRight(c.Endpoints.GoProxy, "/"), ""
			}
			return strings.TrimRight(entry, "/"), ""
		}
	}
	return "", "GOPROXY names no HTTP proxy"
}

// matchGlobPrefix reports whether any comma-separated glob in patterns
// matches a leading path prefix of target, as GOPRIVATE matching does.
func matchGlobPrefix(patterns, target string) bool {
	for _, pattern := range strings.Split(patterns, ",") {
		pattern = strings.TrimSpace(pattern)
		if pattern == "" {
			continue
		}
		n := strings.Count(pattern, "/") + 1
		parts := strings.SplitN(target, "/", n+1)
		if len(parts) < n {
			continue
		}
		prefix := strings.Join(parts[:n], "/")
		if ok, _ := path.Match(pattern, prefix); ok {
			return true
		}
	}
	return false
}

// escapeModulePath applies the module proxy case encoding: an upper-case
// letter becomes '!' and its lower-case form.
func escapeModulePath(p string) string {
	var b strings.Builder
	for _, r := range p {
		if r >= 'A' && r <= 'Z' {
			b.WriteByte('!')
			b.WriteRune(r + ('a' - 'A'))
			continue
		}
		b.WriteRune(r)
	}
	return b.String()
}
