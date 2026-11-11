package freshness

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"
)

// Endpoints are the base URLs of the public sources. Tests point them all at
// one httptest server.
type Endpoints struct {
	GoProxy, NPM, PyPI, Crates, DockerHub string
	GoDL, NodeDist, PythonEOL             string
}

// DefaultEndpoints are the documented public sources:
// proxy.golang.org, registry.npmjs.org, pypi.org, crates.io, Docker Hub,
// go.dev/dl, nodejs.org/dist and endoflife.date (Python release cycles).
var DefaultEndpoints = Endpoints{
	GoProxy: "https://proxy.golang.org", NPM: "https://registry.npmjs.org", PyPI: "https://pypi.org",
	Crates: "https://crates.io", DockerHub: "https://hub.docker.com",
	GoDL: "https://go.dev", NodeDist: "https://nodejs.org", PythonEOL: "https://endoflife.date",
}

// userAgent identifies the tool; crates.io refuses requests without one.
const userAgent = "autopus-adk-deps (+https://github.com/autopus-ai/autopus-adk)"

// maxBodyBytes bounds one registry response (PyPI documents can be large).
const maxBodyBytes = 32 << 20

// Client looks up latest versions with a bounded timeout, small
// concurrency, and an on-disk cache.
type Client struct {
	HTTP        *http.Client
	Endpoints   Endpoints
	CacheDir    string
	TTL         time.Duration
	Offline     bool
	Concurrency int
	// Env reads environment settings (GOPROXY, GOPRIVATE, PIP_INDEX_URL...).
	Env func(string) string
	// GoEnvFile is the `go env -w` file; "" uses the default location.
	GoEnvFile string
	Now       func() time.Time
}

// NewClient returns a client for the public sources, honouring
// AUTOPUS_DEPS_OFFLINE=1 (cache only) and AUTOPUS_DEPS_CACHE_DIR.
func NewClient() *Client {
	return &Client{
		HTTP:        &http.Client{Timeout: 5 * time.Second},
		Endpoints:   DefaultEndpoints,
		CacheDir:    DefaultCacheDir(),
		TTL:         24 * time.Hour,
		Offline:     os.Getenv("AUTOPUS_DEPS_OFFLINE") == "1",
		Concurrency: 4,
		Env:         os.Getenv,
		Now:         time.Now,
	}
}

// Lookup returns the latest stable release for the item's ecosystem.
func (c *Client) Lookup(ctx context.Context, it Item) Latest {
	kind, name := lookupTarget(it)
	key := kind + ":" + name
	if kind == string(EcoNPM) {
		key += "|" + c.npmBase(it)
	}
	now := c.now()
	if cached, ok := c.readCache(key, now); ok {
		return cached
	}
	if c.Offline {
		return Latest{Err: "offline and no cached answer", FetchedAt: now}
	}
	latest := c.fetch(ctx, kind, name, it)
	latest.FetchedAt = now
	if latest.Err == "" {
		c.writeCache(key, latest)
	}
	return latest
}

// LookupAll looks up every distinct item once, Concurrency at a time, and
// returns the answers in item order. ctx bounds the whole batch.
func (c *Client) LookupAll(ctx context.Context, items []Item) []Latest {
	type job struct{ idx []int }
	jobs := map[string]*job{}
	var order []string
	for i, it := range items {
		kind, name := lookupTarget(it)
		key := kind + ":" + name + "|" + it.Registry
		if jobs[key] == nil {
			jobs[key] = &job{}
			order = append(order, key)
		}
		jobs[key].idx = append(jobs[key].idx, i)
	}
	out := make([]Latest, len(items))
	sem := make(chan struct{}, max(c.Concurrency, 1))
	var wg sync.WaitGroup
	for _, key := range order {
		j := jobs[key]
		wg.Add(1)
		go func() {
			defer wg.Done()
			select {
			case sem <- struct{}{}:
				defer func() { <-sem }()
			case <-ctx.Done():
				for _, i := range j.idx {
					out[i] = Latest{Err: "lookup deadline passed", FetchedAt: c.now()}
				}
				return
			}
			got := c.Lookup(ctx, items[j.idx[0]])
			for _, i := range j.idx {
				out[i] = got
			}
		}()
	}
	wg.Wait()
	return out
}

// lookupTarget maps runtime images onto their toolchain release feeds, so
// `node:20-alpine` is judged against the Node LTS line, not Docker tags.
func lookupTarget(it Item) (kind, name string) {
	if it.Ecosystem == EcoImage {
		switch it.Name {
		case "node":
			return string(EcoToolchain), "node"
		case "golang":
			return string(EcoToolchain), "go"
		case "python":
			return string(EcoToolchain), "python"
		}
	}
	return string(it.Ecosystem), it.Name
}

func (c *Client) now() time.Time {
	if c.Now != nil {
		return c.Now()
	}
	return time.Now()
}

func (c *Client) env(key string) string {
	if c.Env == nil {
		return ""
	}
	return c.Env(key)
}

// getJSON fetches url into v. Any failure is returned as a short reason.
func (c *Client) getJSON(ctx context.Context, url string, v any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return err
	}
	req.Header.Set("User-Agent", userAgent)
	req.Header.Set("Accept", "application/json")
	client := c.HTTP
	if client == nil {
		client = http.DefaultClient
	}
	resp, err := client.Do(req)
	if err != nil {
		if ctx.Err() != nil {
			return fmt.Errorf("timed out")
		}
		return fmt.Errorf("unreachable (%s)", shortNetErr(err))
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("HTTP %d", resp.StatusCode)
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, maxBodyBytes)).Decode(v); err != nil {
		return fmt.Errorf("unreadable response")
	}
	return nil
}

func shortNetErr(err error) string {
	s := err.Error()
	if i := strings.LastIndex(s, ": "); i >= 0 {
		return s[i+2:]
	}
	return s
}
