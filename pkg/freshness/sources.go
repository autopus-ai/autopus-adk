package freshness

import (
	"context"
	"fmt"
	"net/url"
	"regexp"
	"strings"
)

// numericTag is a bare release tag such as `17`, `8.0` or `7.4.2`.
var numericTag = regexp.MustCompile(`^\d+(\.\d+){0,2}$`)

func (c *Client) fetch(ctx context.Context, kind, name string, it Item) Latest {
	switch Ecosystem(kind) {
	case EcoGo:
		return c.goModule(ctx, name)
	case EcoNPM:
		return c.npm(ctx, name, c.npmBase(it))
	case EcoPyPI:
		return c.pypi(ctx, name)
	case EcoCrates:
		return c.crate(ctx, name)
	case EcoImage:
		return c.image(ctx, name)
	case EcoToolchain:
		switch name {
		case "go":
			return c.goToolchain(ctx)
		case "node":
			return c.node(ctx)
		case "python":
			return c.python(ctx)
		}
	}
	return Latest{Err: fmt.Sprintf("no release source for %s:%s", kind, name)}
}

func (c *Client) npmBase(it Item) string {
	if it.Registry != "" {
		return it.Registry
	}
	return c.Endpoints.NPM
}

func (c *Client) npm(ctx context.Context, name, base string) Latest {
	u := base + "/-/package/" + strings.Replace(name, "/", "%2f", 1) + "/dist-tags"
	var tags map[string]string
	if err := c.getJSON(ctx, u, &tags); err != nil {
		return Latest{Err: "npm: " + err.Error(), Source: u}
	}
	if tags["latest"] == "" {
		return Latest{Err: "npm: no latest dist-tag", Source: u}
	}
	return Latest{Version: tags["latest"], Source: u}
}

func (c *Client) pypi(ctx context.Context, name string) Latest {
	for _, key := range []string{"PIP_INDEX_URL", "UV_INDEX_URL", "UV_DEFAULT_INDEX"} {
		if v := c.env(key); v != "" && !strings.Contains(v, "pypi.org") {
			return Latest{Err: "pypi: " + key + " names a private index; not sent to pypi.org"}
		}
	}
	u := c.Endpoints.PyPI + "/pypi/" + url.PathEscape(name) + "/json"
	var doc struct {
		Info struct {
			Version string `json:"version"`
		} `json:"info"`
	}
	if err := c.getJSON(ctx, u, &doc); err != nil {
		return Latest{Err: "pypi: " + err.Error(), Source: u}
	}
	return Latest{Version: doc.Info.Version, Source: u}
}

func (c *Client) crate(ctx context.Context, name string) Latest {
	u := c.Endpoints.Crates + "/api/v1/crates/" + url.PathEscape(name)
	var doc struct {
		Crate struct {
			MaxStable string `json:"max_stable_version"`
		} `json:"crate"`
	}
	if err := c.getJSON(ctx, u, &doc); err != nil || doc.Crate.MaxStable == "" {
		return Latest{Err: "crates.io: " + errText(err, "no stable version"), Source: u}
	}
	return Latest{Version: doc.Crate.MaxStable, Source: u}
}

func (c *Client) goModule(ctx context.Context, path string) Latest {
	if reason := c.goPrivate(path); reason != "" {
		return Latest{Err: "go: " + reason}
	}
	proxy, reason := c.goProxy()
	if proxy == "" {
		return Latest{Err: "go: " + reason}
	}
	u := proxy + "/" + escapeModulePath(path) + "/@latest"
	var doc struct{ Version string }
	if err := c.getJSON(ctx, u, &doc); err != nil || doc.Version == "" {
		return Latest{Err: "go proxy: " + errText(err, "no version"), Source: u}
	}
	return Latest{Version: doc.Version, Source: u}
}

func (c *Client) image(ctx context.Context, name string) Latest {
	repo := name
	if !strings.Contains(repo, "/") {
		repo = "library/" + repo
	}
	u := c.Endpoints.DockerHub + "/v2/repositories/" + repo + "/tags?page_size=100&ordering=last_updated"
	var doc struct {
		Results []struct {
			Name string `json:"name"`
		} `json:"results"`
	}
	if err := c.getJSON(ctx, u, &doc); err != nil {
		return Latest{Err: "docker hub: " + err.Error(), Source: u}
	}
	var tags []string
	for _, r := range doc.Results {
		if numericTag.MatchString(r.Name) {
			tags = append(tags, r.Name)
		}
	}
	best, ok := MaxStable(tags)
	if !ok {
		return Latest{Err: "docker hub: no numeric release tag", Source: u}
	}
	return Latest{Version: best, Source: u}
}

func (c *Client) goToolchain(ctx context.Context) Latest {
	u := c.Endpoints.GoDL + "/dl/?mode=json"
	var releases []struct {
		Version string `json:"version"`
		Stable  bool   `json:"stable"`
	}
	if err := c.getJSON(ctx, u, &releases); err != nil {
		return Latest{Err: "go.dev: " + err.Error(), Source: u}
	}
	var stable []string
	for _, r := range releases {
		if r.Stable {
			stable = append(stable, strings.TrimPrefix(r.Version, "go"))
		}
	}
	best, ok := MaxStable(stable)
	if !ok {
		return Latest{Err: "go.dev: no stable release", Source: u}
	}
	return Latest{Version: best, Source: u}
}

func (c *Client) node(ctx context.Context) Latest {
	u := c.Endpoints.NodeDist + "/dist/index.json"
	var releases []struct {
		Version string `json:"version"`
		LTS     any    `json:"lts"`
	}
	if err := c.getJSON(ctx, u, &releases); err != nil {
		return Latest{Err: "nodejs.org: " + err.Error(), Source: u}
	}
	var all, lts []string
	for _, r := range releases {
		v := strings.TrimPrefix(r.Version, "v")
		all = append(all, v)
		if name, ok := r.LTS.(string); ok && name != "" {
			lts = append(lts, v)
		}
	}
	current, ok := MaxStable(all)
	if !ok {
		return Latest{Err: "nodejs.org: no release", Source: u}
	}
	active, _ := MaxStable(lts)
	return Latest{Version: current, LTS: active, Source: u}
}

func (c *Client) python(ctx context.Context) Latest {
	u := c.Endpoints.PythonEOL + "/api/python.json"
	var cycles []struct {
		Latest string `json:"latest"`
	}
	if err := c.getJSON(ctx, u, &cycles); err != nil {
		return Latest{Err: "endoflife.date: " + err.Error(), Source: u}
	}
	var versions []string
	for _, cy := range cycles {
		versions = append(versions, cy.Latest)
	}
	best, ok := MaxStable(versions)
	if !ok {
		return Latest{Err: "endoflife.date: no released cycle", Source: u}
	}
	return Latest{Version: best, Source: u}
}

func errText(err error, fallback string) string {
	if err != nil {
		return err.Error()
	}
	return fallback
}
