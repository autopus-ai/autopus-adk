package design

import (
	"context"
	"errors"
	"io"
	"net"
	"net/http"
	"strings"
	"testing"
)

// errRoundTripper fails every request, standing in for a dial/TLS failure.
type errRoundTripper struct{ err error }

func (e errRoundTripper) RoundTrip(*http.Request) (*http.Response, error) {
	return nil, e.err
}

// errBodyRoundTripper returns a 200 whose body fails mid-read.
type errBodyRoundTripper struct{}

type errReader struct{}

func (errReader) Read([]byte) (int, error) { return 0, errors.New("body broke") }

func (errBodyRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	return &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{},
		Body:       io.NopCloser(errReader{}),
		Request:    req,
	}, nil
}

// TestValidateExternalURL_MalformedAndHostlessURLs pins the two rejections that
// happen before any DNS work: an unparseable/scheme-less URL and an https URL
// with no host at all.
func TestValidateExternalURL_MalformedAndHostlessURLs(t *testing.T) {
	tests := []struct {
		raw  string
		want DiagnosticCategory
	}{
		{"", CategoryUnsafeScheme},
		{"example.com/design.md", CategoryUnsafeScheme},
		{"https://%zz/design.md", CategoryUnsafeScheme},
		{"https:///design.md", CategoryLocalHostname},
	}
	for _, test := range tests {
		diag := ValidateExternalURL(context.TODO(), test.raw, publicResolver)
		if diag == nil || diag.Category != test.want {
			t.Errorf("ValidateExternalURL(%q) = %+v, want category %q", test.raw, diag, test.want)
		}
	}
}

// TestValidateExternalURL_DNSFailureModes keeps a resolver that errors or
// returns nothing from being treated as a clean public host.
func TestValidateExternalURL_DNSFailureModes(t *testing.T) {
	failing := func(context.Context, string) ([]net.IP, error) {
		return nil, errors.New("nxdomain")
	}
	empty := func(context.Context, string) ([]net.IP, error) { return nil, nil }

	for name, resolver := range map[string]Resolver{"error": failing, "empty": empty} {
		diag := ValidateExternalURL(context.TODO(), "https://example.com/design.md", resolver)
		if diag == nil || diag.Category != CategoryPrivateAddress {
			t.Errorf("%s resolver: ValidateExternalURL = %+v, want a private-address rejection", name, diag)
		}
	}
}

// TestFetchPublicHTTPS_RejectedURLNeedsNoClient proves the SSRF check runs
// before any transport is used: with no client configured the unsafe target is
// reported without a dial attempt.
func TestFetchPublicHTTPS_RejectedURLNeedsNoClient(t *testing.T) {
	for _, opts := range []ImportOptions{
		{},                           // no client at all
		{HTTPClient: &http.Client{}}, // client without a transport
	} {
		body, gaps, err := fetchPublicHTTPS(context.TODO(), "http://127.0.0.1/design.md", opts)
		if err != nil || body != nil {
			t.Fatalf("fetchPublicHTTPS = (%q, %v), want no body and no error", body, err)
		}
		if len(gaps) != 1 || gaps[0] != string(CategoryUnsafeScheme) {
			t.Errorf("gaps = %v, want the scheme rejection", gaps)
		}
	}
}

// TestFetchPublicHTTPS_TransportAndBodyFailures pins the distinct gap codes the
// caller reports, so a transport error is never confused with an HTTP status or
// a truncated body.
func TestFetchPublicHTTPS_TransportAndBodyFailures(t *testing.T) {
	const target = "https://example.com/design.md"
	tests := []struct {
		name      string
		transport http.RoundTripper
		want      string
	}{
		{"dial failure", errRoundTripper{err: errors.New("no route")}, "fetch_failed"},
		{"body read failure", errBodyRoundTripper{}, "read_failed"},
		{"server error status", responseRoundTripper{status: http.StatusInternalServerError}, "http_status_500"},
		{"not found status", responseRoundTripper{status: http.StatusNotFound}, "http_status_404"},
		{"informational status", responseRoundTripper{status: http.StatusContinue}, "http_status_100"},
	}
	for _, test := range tests {
		opts := ImportOptions{HTTPClient: fakeHTTPClient(test.transport), Resolver: publicResolver}
		body, gaps, err := fetchPublicHTTPS(context.TODO(), target, opts)
		if err != nil {
			t.Fatalf("%s: unexpected error %v", test.name, err)
		}
		if body != nil {
			t.Errorf("%s: body = %q, want nil", test.name, body)
		}
		if len(gaps) != 1 || gaps[0] != test.want {
			t.Errorf("%s: gaps = %v, want [%s]", test.name, gaps, test.want)
		}
	}
}

// TestFetchPublicHTTPS_RedirectWithoutLocationIsUnsafe keeps a redirect that
// names no target from silently retrying the same URL forever.
func TestFetchPublicHTTPS_RedirectWithoutLocationIsUnsafe(t *testing.T) {
	transport := responseRoundTripper{status: http.StatusFound, header: http.Header{}}
	opts := ImportOptions{HTTPClient: fakeHTTPClient(transport), Resolver: publicResolver}

	_, gaps, err := fetchPublicHTTPS(context.TODO(), "https://example.com/design.md", opts)
	if err != nil {
		t.Fatalf("fetchPublicHTTPS: %v", err)
	}
	if len(gaps) != 1 || gaps[0] != "unsafe_redirect" {
		t.Errorf("gaps = %v, want [unsafe_redirect]", gaps)
	}
}

// TestSafePublicTransport_DialGuards pins the dial-time half of the SSRF
// defense, which a redirect or a DNS rebind would otherwise slip past.
func TestSafePublicTransport_DialGuards(t *testing.T) {
	transport, ok := safePublicTransport(nil).(*http.Transport)
	if !ok || transport.DialContext == nil {
		t.Fatal("safePublicTransport did not install a guarded DialContext")
	}

	if _, err := transport.DialContext(context.TODO(), "tcp", "example.com"); err == nil {
		t.Error("dial to an address without a port = nil error, want failure")
	}
	if _, err := transport.DialContext(context.TODO(), "tcp", "127.0.0.1:443"); err == nil {
		t.Error("dial to loopback = nil error, want blocked address")
	}

	failing := safePublicTransport(func(context.Context, string) ([]net.IP, error) {
		return nil, errors.New("nxdomain")
	}).(*http.Transport)
	if _, err := failing.DialContext(context.TODO(), "tcp", "example.com:443"); err == nil {
		t.Error("dial with a failing resolver = nil error, want propagated DNS failure")
	}
}

// TestResolveRedirect_UnparseableInputs keeps a malformed base or Location from
// producing a URL that skips revalidation.
func TestResolveRedirect_UnparseableInputs(t *testing.T) {
	if _, err := resolveRedirect("https://%zz/a", "/b"); err == nil {
		t.Error("resolveRedirect with an unparseable base = nil error, want failure")
	}
	if _, err := resolveRedirect("https://example.com/a", "https://%zz/b"); err == nil {
		t.Error("resolveRedirect with an unparseable location = nil error, want failure")
	}
}

// TestMustParseCIDRs_PanicsOnBadPolicy makes the deny-list a build-time
// invariant: a typo in the security policy must not degrade into an empty
// range set that allows everything.
func TestMustParseCIDRs_PanicsOnBadPolicy(t *testing.T) {
	defer func() {
		recovered := recover()
		if recovered == nil {
			t.Fatal("mustParseCIDRs accepted an invalid CIDR without panicking")
		}
		if msg, _ := recovered.(error); msg != nil && !strings.Contains(msg.Error(), "not-a-cidr") {
			t.Errorf("panic value = %v, want it to name the offending entry", recovered)
		}
	}()
	_ = mustParseCIDRs([]string{"10.0.0.0/8", "not-a-cidr"})
}
