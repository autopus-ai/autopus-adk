package agentprobe

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"
)

type openCodeClient struct {
	options     OpenCodeOptions
	client      *http.Client
	metadata    *OpenCodeTransport
	cleanupMode bool
}

func newOpenCodeClient(options OpenCodeOptions, metadata *OpenCodeTransport) (*openCodeClient, error) {
	u, err := url.Parse(options.Endpoint)
	if err != nil || u.Scheme != "http" || u.User != nil || u.RawQuery != "" || u.Fragment != "" || (u.Path != "" && u.Path != "/") {
		return nil, fmt.Errorf("opencode endpoint must be plain loopback HTTP")
	}
	ip := net.ParseIP(u.Hostname())
	if ip == nil || !ip.IsLoopback() || u.Port() == "" {
		return nil, fmt.Errorf("opencode endpoint requires literal loopback IP and explicit port")
	}
	if options.RuntimeVersion != "" && !token(options.RuntimeVersion) || !token(options.ProviderID) || !token(options.ModelID) {
		return nil, fmt.Errorf("valid runtime and explicit model identities required")
	}
	options.Endpoint = strings.TrimRight(options.Endpoint, "/")
	transport := &http.Transport{Proxy: nil}
	client := &http.Client{Transport: transport, Timeout: 20 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return fmt.Errorf("redirect refused") }}
	return &openCodeClient{options: options, client: client, metadata: metadata}, nil
}

func (c *openCodeClient) request(ctx context.Context, method, path string, input, output any) (int, error) {
	if !c.cleanupMode && (c.metadata.Requests >= 1000 || c.metadata.BytesReceived > 16<<20) {
		return 0, fmt.Errorf("opencode transport limit exceeded")
	}
	var body io.Reader
	if input != nil {
		data, err := json.Marshal(input)
		if err != nil {
			return 0, err
		}
		body = bytes.NewReader(data)
	}
	u := c.options.Endpoint + path
	if c.options.Directory != "" {
		u += "?directory=" + url.QueryEscape(c.options.Directory)
	}
	request, err := http.NewRequestWithContext(ctx, method, u, body)
	if err != nil {
		return 0, fmt.Errorf("invalid opencode request")
	}
	request.Header.Set("Content-Type", "application/json")
	if c.options.Password != "" {
		username := c.options.Username
		if username == "" {
			username = "opencode"
		}
		request.SetBasicAuth(username, c.options.Password)
	}
	c.metadata.Requests++
	response, err := c.client.Do(request)
	if err != nil {
		return 0, fmt.Errorf("opencode transport unavailable or deadline exceeded")
	}
	defer response.Body.Close()
	limit := int64(1 << 20)
	if path == "/doc" {
		limit = 4 << 20
	}
	data, err := io.ReadAll(io.LimitReader(response.Body, limit+1))
	c.metadata.BytesReceived += int64(len(data))
	if err != nil || int64(len(data)) > limit {
		return response.StatusCode, fmt.Errorf("opencode response unreadable or exceeds limit")
	}
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return response.StatusCode, fmt.Errorf("opencode protocol HTTP %d", response.StatusCode)
	}
	if output != nil {
		if err := json.Unmarshal(data, output); err != nil {
			return response.StatusCode, fmt.Errorf("opencode response schema invalid")
		}
	}
	return response.StatusCode, nil
}

func (c *openCodeClient) preflight(ctx context.Context) error {
	var health struct {
		Healthy bool   `json:"healthy"`
		Version string `json:"version"`
	}
	if _, err := c.request(ctx, "GET", "/global/health", nil, &health); err != nil {
		return err
	}
	if !health.Healthy || !token(health.Version) || c.options.RuntimeVersion != "" && health.Version != c.options.RuntimeVersion {
		return fmt.Errorf("opencode runtime identity unverified")
	}
	c.metadata.ObservedRuntimeVersion = health.Version
	var doc map[string]any
	if _, err := c.request(ctx, "GET", "/doc", nil, &doc); err != nil {
		return err
	}
	paths, _ := doc["paths"].(map[string]any)
	for path, methods := range map[string][]string{"/session": {"post"}, "/session/{sessionID}/message": {"get", "post"}, "/session/{sessionID}/prompt_async": {"post"}, "/session/{sessionID}/abort": {"post"}, "/session/{sessionID}/children": {"get"}, "/session/{sessionID}": {"get", "delete"}, "/session/status": {"get"}} {
		route, _ := paths[path].(map[string]any)
		for _, method := range methods {
			if _, ok := route[method]; !ok {
				return fmt.Errorf("opencode installed lifecycle schema unverified")
			}
		}
	}
	route, _ := paths["/session"].(map[string]any)
	node := route["post"]
	for _, key := range []string{"requestBody", "content", "application/json", "schema", "properties"} {
		node = resolveOpenCodeSchema(doc, node)
		mapping, _ := node.(map[string]any)
		node = mapping[key]
	}
	properties, _ := node.(map[string]any)
	if _, ok := properties["permission"]; !ok {
		return fmt.Errorf("opencode deny-all permission schema unverified")
	}
	if _, ok := properties["parentID"]; !ok {
		return fmt.Errorf("opencode native child schema unverified")
	}
	return nil
}

func resolveOpenCodeSchema(doc map[string]any, node any) any {
	for count := 0; count < 8; count++ {
		mapping, _ := node.(map[string]any)
		ref, _ := mapping["$ref"].(string)
		if ref == "" {
			return node
		}
		if !strings.HasPrefix(ref, "#/") {
			return nil
		}
		node = doc
		for _, key := range strings.Split(strings.TrimPrefix(ref, "#/"), "/") {
			object, _ := node.(map[string]any)
			node = object[key]
		}
	}
	return nil
}
