package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/spf13/cobra"
)

type canaryCommandTarget struct {
	ID      string
	Dir     string
	Command string
	Args    []string
}

// canaryCheckFailure is the FAIL detail used when a check produced no output of
// its own. An exec failure before the process starts (missing directory,
// missing binary) leaves CombinedOutput empty, and a bare "H1 failed:" tells a
// reader nothing about what to fix.
func canaryCheckFailure(id, display string, output []byte, err error) canaryTargetResult {
	detail := strings.TrimSpace(string(output))
	if detail == "" && err != nil {
		detail = err.Error()
	}
	if detail == "" {
		detail = "command failed without output"
	}
	return canaryTargetResult{ID: id, Command: display, Status: "FAIL", Detail: detail}
}

func runCanaryExternal(ctx context.Context, id, display, dir string, argv ...string) canaryTargetResult {
	if len(argv) == 0 {
		return canaryTargetResult{ID: id, Command: display, Status: "FAIL", Detail: "empty command"}
	}
	timeoutCtx, cancel := context.WithTimeout(ctx, 5*time.Minute)
	defer cancel()
	c := exec.CommandContext(timeoutCtx, argv[0], argv[1:]...) //nolint:gosec
	c.Dir = dir
	output, err := c.CombinedOutput()
	if timeoutCtx.Err() == context.DeadlineExceeded {
		return canaryTargetResult{ID: id, Command: display, Status: "FAIL", Detail: "timed out"}
	}
	if err != nil {
		return canaryCheckFailure(id, display, output, err)
	}
	return canaryTargetResult{ID: id, Command: display, Status: "PASS"}
}

func runCanaryEndpointChecks(ctx context.Context, baseURL string, result *canaryResult) string {
	status := "PASS"
	endpoints := []struct {
		path               string
		acceptUnauthorized bool
	}{
		{path: "/health"},
		{path: "/metrics", acceptUnauthorized: true},
	}
	for _, endpoint := range endpoints {
		code, err := canaryHTTPStatus(ctx, strings.TrimRight(baseURL, "/")+endpoint.path)
		successResponse := code >= http.StatusOK && code < http.StatusBadRequest
		protectedResponse := endpoint.acceptUnauthorized && code == http.StatusUnauthorized
		passed := err == nil && (successResponse || protectedResponse)
		targetStatus := "PASS"
		detail := fmt.Sprintf("HTTP %d", code)
		if protectedResponse {
			detail += " (protected endpoint)"
		}
		if err != nil {
			detail = err.Error()
		}
		if !passed {
			status = "FAIL"
			targetStatus = "FAIL"
		}
		result.Targets = append(result.Targets, canaryTargetResult{
			ID:     "endpoint" + endpoint.path,
			Status: targetStatus,
			Detail: detail,
		})
	}
	return status
}

func canaryHTTPStatus(ctx context.Context, url string) (int, error) {
	reqCtx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(reqCtx, http.MethodGet, url, nil)
	if err != nil {
		return 0, err
	}
	// A transport of its own: anything that closes idle connections on the
	// shared http.DefaultTransport (httptest.Server.Close does) can break a
	// request in flight on it, which reads as "connection broken" instead of
	// the endpoint's real status.
	transport := http.DefaultTransport.(*http.Transport).Clone()
	defer transport.CloseIdleConnections()
	resp, err := (&http.Client{Transport: transport}).Do(req)
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()
	return resp.StatusCode, nil
}

func writeCanaryLatest(projectDir string, result canaryResult) error {
	dir := filepath.Join(projectDir, ".autopus", "canary")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(result, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(dir, "latest.json"), append(data, '\n'), 0o644)
}

func canarySummary(result canaryResult) map[string]string {
	return map[string]string{
		"build":    result.Build,
		"e2e":      result.E2E,
		"doctor":   result.Doctor,
		"endpoint": result.Endpoint,
		"browser":  result.Browser,
	}
}

// canarySummaryOrder pins the emission order of the summary rows. Canary stdout
// and its JSON envelope are captured verbatim as QAMESH canary-explicit
// evidence, so two runs of the same result must produce comparable artifacts;
// Go map iteration reordered them on every run.
var canarySummaryOrder = []string{"build", "e2e", "doctor", "endpoint", "browser"}

// canarySummaryRows flattens Summary into canarySummaryOrder, then appends any
// key outside that list in sorted order so a future summary field is still
// reported - just never in a random position.
func canarySummaryRows(summary map[string]string) [][2]string {
	rows := make([][2]string, 0, len(summary))
	seen := make(map[string]bool, len(canarySummaryOrder))
	for _, key := range canarySummaryOrder {
		seen[key] = true
		if value, ok := summary[key]; ok {
			rows = append(rows, [2]string{key, value})
		}
	}
	extra := make([]string, 0, len(summary))
	for key := range summary {
		if !seen[key] {
			extra = append(extra, key)
		}
	}
	sort.Strings(extra)
	for _, key := range extra {
		rows = append(rows, [2]string{key, summary[key]})
	}
	return rows
}

func canaryChecks(result canaryResult) []jsonCheck {
	rows := canarySummaryRows(result.Summary)
	checks := make([]jsonCheck, 0, len(rows))
	for _, row := range rows {
		checks = append(checks, jsonCheck{ID: "canary." + row[0], Status: strings.ToLower(row[1]), Detail: row[0] + "=" + row[1]})
	}
	return checks
}

func printCanaryText(cmd *cobra.Command, result canaryResult) {
	out := cmd.OutOrStdout()
	fmt.Fprintf(out, "canary %s\n", result.Verdict)
	for _, row := range canarySummaryRows(result.Summary) {
		fmt.Fprintf(out, "%s: %s\n", row[0], row[1])
	}
	for _, skipped := range result.Skipped {
		fmt.Fprintf(out, "skipped %s: %s\n", skipped.Area, skipped.Reason)
	}
	for _, target := range result.Targets {
		if target.Status != "PASS" && strings.TrimSpace(target.Detail) != "" {
			fmt.Fprintf(out, "%s %s: %s\n", target.ID, target.Status, firstCanaryLine(target.Detail))
		}
	}
}

// firstCanaryLine keeps the text verdict scannable: build failures dump whole
// compiler transcripts, which belong in latest.json, not in the gate summary.
func firstCanaryLine(detail string) string {
	detail = strings.TrimSpace(detail)
	if idx := strings.IndexByte(detail, '\n'); idx >= 0 {
		return strings.TrimSpace(detail[:idx])
	}
	return detail
}

func errOrDefault(err error, message string) error {
	if err != nil {
		return err
	}
	return fmt.Errorf("%s", message)
}

func defaultString(value, fallback string) string {
	if strings.TrimSpace(value) == "" {
		return fallback
	}
	return value
}
