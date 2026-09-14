package design

import (
	"context"
	"errors"
	"net/http"
	"path/filepath"
	"testing"
)

// TestResolveFigmaToken_ExplicitBeatsEnvironment pins the credential
// precedence, so an explicit flag is never shadowed by a stale shell export.
func TestResolveFigmaToken_ExplicitBeatsEnvironment(t *testing.T) {
	t.Setenv("FIGMA_ACCESS_TOKEN", "")
	t.Setenv("FIGMA_TOKEN", "")

	if got := resolveFigmaToken("  flag-token  "); got != "flag-token" {
		t.Errorf("resolveFigmaToken(explicit) = %q, want trimmed flag-token", got)
	}
	if got := resolveFigmaToken("   "); got != "" {
		t.Errorf("resolveFigmaToken with blank explicit and no env = %q, want empty", got)
	}

	t.Setenv("FIGMA_TOKEN", " fallback ")
	if got := resolveFigmaToken(""); got != "fallback" {
		t.Errorf("resolveFigmaToken fell back to %q, want trimmed FIGMA_TOKEN", got)
	}

	t.Setenv("FIGMA_ACCESS_TOKEN", "primary")
	if got := resolveFigmaToken(""); got != "primary" {
		t.Errorf("resolveFigmaToken = %q, want FIGMA_ACCESS_TOKEN to win over FIGMA_TOKEN", got)
	}
	if got := resolveFigmaToken("explicit"); got != "explicit" {
		t.Errorf("resolveFigmaToken = %q, want the explicit token to win over both env vars", got)
	}
}

// TestFetchFigmaNodes_MissingTokenReportsGapWithoutFetching is the fail-closed
// path: with no credential the audit still reports refs, adds one gap, and
// never issues a request.
func TestFetchFigmaNodes_MissingTokenReportsGapWithoutFetching(t *testing.T) {
	t.Setenv("FIGMA_ACCESS_TOKEN", "")
	t.Setenv("FIGMA_TOKEN", "")
	root := t.TempDir()
	writeFile(t, root, "DESIGN.md", "https://figma.com/design/abc123/Product?node-id=1-2")

	report, err := FetchFigmaNodes(context.TODO(), root, FigmaFetchOptions{
		HTTPClient: fakeHTTPClient(errRoundTripper{err: errors.New("must not be called")}),
	})
	if err != nil {
		t.Fatalf("FetchFigmaNodes: %v", err)
	}
	if len(report.Nodes) != 0 {
		t.Errorf("nodes = %+v, want none without a token", report.Nodes)
	}
	if len(report.FigmaRefs) != 1 {
		t.Errorf("refs = %+v, want the audited ref to survive", report.FigmaRefs)
	}
	var gapCount int
	for _, gap := range report.SetupGaps {
		if gap == "figma_token_missing" {
			gapCount++
		}
	}
	if gapCount != 1 {
		t.Errorf("setup gaps = %v, want exactly one figma_token_missing", report.SetupGaps)
	}
	if report.Version != 1 || report.GeneratedAt == "" {
		t.Errorf("report envelope = %+v, want version 1 and a timestamp", report)
	}
}

// TestFetchFigmaNodes_MissingRootIsAnError keeps a bad workspace path from
// yielding an empty-but-successful fetch report.
func TestFetchFigmaNodes_MissingRootIsAnError(t *testing.T) {
	_, err := FetchFigmaNodes(context.TODO(), filepath.Join(t.TempDir(), "absent"), FigmaFetchOptions{Token: "t"})
	if err == nil {
		t.Error("FetchFigmaNodes on a missing root = nil error, want failure")
	}
}

// TestFetchFigmaNodes_NoRefsNeedsNoClientOrBaseURL proves the defaults path is
// inert: with nothing to fetch the call must not touch the network even though
// it fills in the default client and API base URL.
func TestFetchFigmaNodes_NoRefsNeedsNoClientOrBaseURL(t *testing.T) {
	root := t.TempDir()
	writeFile(t, root, "notes.md", "no figma links here")

	report, err := FetchFigmaNodes(context.TODO(), root, FigmaFetchOptions{Token: "t"})
	if err != nil {
		t.Fatalf("FetchFigmaNodes: %v", err)
	}
	if len(report.Nodes) != 0 || len(report.FigmaRefs) != 0 {
		t.Errorf("report = %+v, want nothing fetched", report)
	}
	if len(report.SetupGaps) == 0 {
		t.Error("setup gaps = none, want the audit gaps forwarded")
	}
}

// TestFetchFigmaNode_SkipsIncompleteRefs pins the two skip reasons: without a
// file key or node id there is no endpoint to call, and the skip must carry the
// specific cause rather than a generic error.
func TestFetchFigmaNode_SkipsIncompleteRefs(t *testing.T) {
	client := fakeHTTPClient(errRoundTripper{err: errors.New("must not be called")})

	noKey := fetchFigmaNode(context.TODO(), client, "https://api.figma.com", "t",
		FigmaRef{SourcePath: "a.md", NodeID: "1:2"}, 1)
	if noKey.Status != "skipped" || noKey.Error != "file_key_missing" || noKey.Endpoint != "" {
		t.Errorf("missing file key = %+v, want a skipped file_key_missing with no endpoint", noKey)
	}

	noNode := fetchFigmaNode(context.TODO(), client, "https://api.figma.com", "t",
		FigmaRef{SourcePath: "a.md", FileKey: "abc"}, 1)
	if noNode.Status != "skipped" || noNode.Error != "node_id_missing" || noNode.Endpoint != "" {
		t.Errorf("missing node id = %+v, want a skipped node_id_missing with no endpoint", noNode)
	}
}

// TestFetchFigmaNode_TransportAndURLFailures keeps a malformed base URL and a
// dial failure reported as per-node errors instead of aborting the whole run.
func TestFetchFigmaNode_TransportAndURLFailures(t *testing.T) {
	ref := FigmaRef{SourcePath: "a.md", FileKey: "abc/def", NodeID: "1:2"}

	badBase := fetchFigmaNode(context.TODO(), http.DefaultClient, "http://exa mple.com", "t", ref, 1)
	if badBase.Status != "error" || badBase.Error == "" {
		t.Errorf("malformed base URL = %+v, want a per-node error", badBase)
	}
	if badBase.Endpoint == "" || badBase.FileKey != "abc/def" {
		t.Errorf("node = %+v, want the endpoint and ref identity retained", badBase)
	}

	client := fakeHTTPClient(errRoundTripper{err: errors.New("no route to host")})
	dialFail := fetchFigmaNode(context.TODO(), client, "https://api.figma.com", "t", ref, 2)
	if dialFail.Status != "error" || dialFail.StatusCode != 0 {
		t.Errorf("dial failure = %+v, want an error node with no status code", dialFail)
	}
	if dialFail.Endpoint != "/v1/files/abc%2Fdef/nodes?ids=1%3A2&depth=2" {
		t.Errorf("endpoint = %q, want the file key and node id escaped with depth 2", dialFail.Endpoint)
	}
}
