package testscenario

import (
	"fmt"
	"strings"
)

// PlaceholderPort is the free local port a service case's argv may name; the
// service also gets it as the PORT environment variable.
const PlaceholderPort = "$PORT"

// HTTPRequest is one request sent to a service case's server. Requests run in
// order against the same process, so a POST can be read back by a GET.
type HTTPRequest struct {
	Method  string            `yaml:"method" json:"method"`
	Path    string            `yaml:"path" json:"path"`
	Headers map[string]string `yaml:"headers,omitempty" json:"headers,omitempty"`
	Body    string            `yaml:"body,omitempty" json:"body,omitempty"`
	Expect  HTTPExpect        `yaml:"expect" json:"expect"`
}

// HTTPExpect is what a client observes from one response. Body is an exact
// match, the Contains lists are substrings, and Headers values are substrings
// of the response header with that name.
type HTTPExpect struct {
	Status          *int              `yaml:"status,omitempty" json:"status,omitempty"`
	Body            *string           `yaml:"body,omitempty" json:"body,omitempty"`
	BodyContains    []string          `yaml:"body_contains,omitempty" json:"body_contains,omitempty"`
	BodyNotContains []string          `yaml:"body_not_contains,omitempty" json:"body_not_contains,omitempty"`
	Headers         map[string]string `yaml:"headers,omitempty" json:"headers,omitempty"`
}

func (e HTTPExpect) empty() bool {
	return e.Status == nil && e.Body == nil && len(e.BodyContains) == 0 && len(e.BodyNotContains) == 0 && len(e.Headers) == 0
}

var httpMethods = []string{"GET", "HEAD", "POST", "PUT", "PATCH", "DELETE", "OPTIONS"}

// IsService reports whether the case starts argv as a server and sends
// requests to it, instead of running argv once.
func (x Exec) IsService() bool { return len(x.Requests) > 0 }

// validateService checks the parts of a service case that a one-shot run
// does not have. Requests only ever go to the local server the case started:
// a path, never a URL, so a case cannot reach another host.
func validateService(x *Exec) error {
	if !x.Expect.empty() {
		return fmt.Errorf("exec.expect is for one-shot runs; a case with requests states what it expects on each request")
	}
	if x.Stdin != "" {
		return fmt.Errorf("exec.stdin is for one-shot runs; send input to a service as a request body")
	}
	if x.Ready != "" && !strings.HasPrefix(x.Ready, "/") {
		return fmt.Errorf("exec.ready %q must be a path such as /healthz", x.Ready)
	}
	for i, r := range x.Requests {
		where := fmt.Sprintf("exec.requests[%d]", i)
		method := strings.ToUpper(strings.TrimSpace(r.Method))
		if !contains(httpMethods, method) {
			return fmt.Errorf("%s.method %q must be one of: %s", where, r.Method, strings.Join(httpMethods, ", "))
		}
		if !strings.HasPrefix(r.Path, "/") || strings.Contains(r.Path, "://") {
			return fmt.Errorf("%s.path %q must be a path on the case's own server, starting with /", where, r.Path)
		}
		if r.Expect.empty() {
			return fmt.Errorf("%s.expect must state at least one observable outcome", where)
		}
		required := append([]string{}, r.Expect.BodyContains...)
		if r.Expect.Body != nil {
			required = append(required, *r.Expect.Body)
		}
		for _, banned := range r.Expect.BodyNotContains {
			if banned == "" {
				return fmt.Errorf("%s.expect.body_not_contains has an empty entry, which every body contains", where)
			}
			for _, need := range required {
				if strings.Contains(need, banned) {
					return fmt.Errorf("%s.expect.body_not_contains %q is part of required body %q; no response satisfies both", where, banned, need)
				}
			}
		}
	}
	return nil
}

func contains(list []string, s string) bool {
	for _, v := range list {
		if v == s {
			return true
		}
	}
	return false
}
