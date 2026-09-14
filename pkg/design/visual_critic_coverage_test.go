package design

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestLoadVisualCriticReport_EmptyPathIsNoReport keeps an unset critic path
// from being treated as a failure: callers rely on the zero report plus nil
// error to mean "no critic ran".
func TestLoadVisualCriticReport_EmptyPathIsNoReport(t *testing.T) {
	for _, raw := range []string{"", "   "} {
		report, err := LoadVisualCriticReport(t.TempDir(), raw)
		if err != nil {
			t.Fatalf("LoadVisualCriticReport(%q) error = %v, want nil", raw, err)
		}
		if report.Status != "" || report.Source != "" || len(report.Findings) != 0 {
			t.Errorf("LoadVisualCriticReport(%q) = %+v, want zero report", raw, report)
		}
	}
}

// TestLoadVisualCriticReport_RootMustBeDirectory rejects a root that does not
// exist or is a plain file, so a bad workspace argument fails loudly instead of
// silently reading a sibling path.
func TestLoadVisualCriticReport_RootMustBeDirectory(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "not-a-dir")
	if err := os.WriteFile(file, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}

	if _, err := LoadVisualCriticReport(filepath.Join(dir, "missing"), "critic.json"); err == nil {
		t.Error("LoadVisualCriticReport with a missing root = nil error, want failure")
	}
	_, err := LoadVisualCriticReport(file, "critic.json")
	if err == nil || !strings.Contains(err.Error(), "must resolve to a directory") {
		t.Errorf("LoadVisualCriticReport with a file root error = %v, want directory rejection", err)
	}
}

// TestResolveVisualCriticPath_RejectsEscapesAndNonJSON pins the path policy:
// traversal, absolute paths outside the root, and non-JSON extensions are all
// refused before any file is opened.
func TestResolveVisualCriticPath_RejectsEscapesAndNonJSON(t *testing.T) {
	root := t.TempDir()
	tests := []struct {
		raw  string
		want string
	}{
		{"../critic.json", "parent traversal"},
		{"reports/../../critic.json", "parent traversal"},
		{filepath.Join(filepath.Dir(root), "outside.json"), "escapes project root"},
		{"critic.txt", "must be json"},
		{"reports/critic", "must be json"},
	}
	for _, test := range tests {
		_, err := resolveVisualCriticPath(root, test.raw)
		if err == nil || !strings.Contains(err.Error(), test.want) {
			t.Errorf("resolveVisualCriticPath(%q) error = %v, want %q", test.raw, err, test.want)
		}
	}

	// A relative JSON path and its absolute form inside the root must both
	// normalize to the same root-relative key.
	rel, err := resolveVisualCriticPath(root, "reports/critic.json")
	if err != nil {
		t.Fatalf("resolveVisualCriticPath: %v", err)
	}
	abs, err := resolveVisualCriticPath(root, filepath.Join(root, "reports", "critic.json"))
	if err != nil {
		t.Fatalf("resolveVisualCriticPath(abs): %v", err)
	}
	if rel != filepath.FromSlash("reports/critic.json") || abs != rel {
		t.Errorf("relative %q and absolute %q forms disagree", rel, abs)
	}

	// Extension matching is case-insensitive; .JSON is still a report.
	if _, err := resolveVisualCriticPath(root, "critic.JSON"); err != nil {
		t.Errorf("uppercase .JSON was rejected: %v", err)
	}
}

// TestLoadVisualCriticReport_MalformedJSONIsAnError keeps a truncated or
// hand-edited report from being silently read as an empty PASS.
func TestLoadVisualCriticReport_MalformedJSONIsAnError(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "critic.json"), []byte("{\"findings\": ["), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadVisualCriticReport(root, "critic.json"); err == nil {
		t.Error("malformed critic JSON = nil error, want a decode failure")
	}
}

// TestLoadVisualCriticReport_MissingFileIsAnError separates "no critic path"
// from "critic path points at nothing"; the latter must not pass as a zero
// report.
func TestLoadVisualCriticReport_MissingFileIsAnError(t *testing.T) {
	if _, err := LoadVisualCriticReport(t.TempDir(), "critic.json"); err == nil {
		t.Error("missing critic report = nil error, want failure")
	}
}

// TestLoadVisualCriticReport_PreservesExplicitStatus proves the derived status
// never overrides a status the critic itself wrote, and that Source is reported
// as a slash path relative to the root.
func TestLoadVisualCriticReport_PreservesExplicitStatus(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "reports"), 0o755); err != nil {
		t.Fatal(err)
	}
	body := `{"version":2,"status":"PASS","findings":[{"severity":"FAIL","message":"m"}]}`
	if err := os.WriteFile(filepath.Join(root, "reports", "critic.json"), []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}

	report, err := LoadVisualCriticReport(root, "reports/critic.json")
	if err != nil {
		t.Fatalf("LoadVisualCriticReport: %v", err)
	}
	if report.Status != "PASS" {
		t.Errorf("Status = %q, want the explicit PASS to win over the FAIL finding", report.Status)
	}
	if report.Source != "reports/critic.json" || report.Version != 2 {
		t.Errorf("report = %+v, want slash source and version 2", report)
	}
}

// TestLstatVisualCriticReport_RejectsBadComponents pins the per-component walk:
// empty/dot components, a non-directory in the middle, and a directory as the
// final target are each refused with their own error.
func TestLstatVisualCriticReport_RejectsBadComponents(t *testing.T) {
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "sub"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "plain.json"), []byte("{}"), 0o600); err != nil {
		t.Fatal(err)
	}
	root, err := os.OpenRoot(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = root.Close() }()

	tests := []struct {
		rel  string
		want string
	}{
		{"..", "invalid visual critic path component"},
		{".", "invalid visual critic path component"},
		{"", "invalid visual critic path component"},
		{"plain.json/inner.json", "not a directory"},
		{"sub", "not a regular file"},
	}
	for _, test := range tests {
		_, err := lstatVisualCriticReport(root, test.rel)
		if err == nil || !strings.Contains(err.Error(), test.want) {
			t.Errorf("lstatVisualCriticReport(%q) error = %v, want %q", test.rel, err, test.want)
		}
	}

	if _, err := lstatVisualCriticReport(root, "plain.json"); err != nil {
		t.Errorf("lstatVisualCriticReport on a regular file failed: %v", err)
	}
}

// TestDeriveCriticStatus_SeverityPrecedence pins the aggregation order: any
// failing severity wins over every warning, warnings survive later passes, and
// unknown severities never downgrade a verdict.
func TestDeriveCriticStatus_SeverityPrecedence(t *testing.T) {
	finding := func(severity string) VisualCriticFinding {
		return VisualCriticFinding{Severity: severity}
	}
	tests := []struct {
		name     string
		findings []VisualCriticFinding
		want     string
	}{
		{"no findings", nil, "PASS"},
		{"info only", []VisualCriticFinding{finding("info"), finding("")}, "PASS"},
		{"warn is sticky", []VisualCriticFinding{finding("warn"), finding("info")}, "WARN"},
		{"warning alias", []VisualCriticFinding{finding("Warning")}, "WARN"},
		{"medium alias", []VisualCriticFinding{finding("medium")}, "WARN"},
		{"fail beats warn", []VisualCriticFinding{finding("warn"), finding("fail")}, "FAIL"},
		{"error alias", []VisualCriticFinding{finding("error")}, "FAIL"},
		{"high alias", []VisualCriticFinding{finding("HiGh")}, "FAIL"},
	}
	for _, test := range tests {
		if got := deriveCriticStatus(test.findings); got != test.want {
			t.Errorf("%s: deriveCriticStatus = %q, want %q", test.name, got, test.want)
		}
	}
}
