package securityscan

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func fixture(t *testing.T, name string) []byte {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("testdata", name))
	require.NoError(t, err)
	return data
}

func TestParsers_RecordedOutput(t *testing.T) {
	for _, row := range []struct {
		fixture string
		parse   func([]byte) ([]Finding, error)
		want    []Finding
	}{
		{"gitleaks.json", parseGitleaks, []Finding{
			{Tool: "gitleaks", Kind: KindSecret, ID: "generic-api-key", Severity: SeverityHigh, File: "config/settings.py",
				Location: "config/settings.py:12 @9f2c4e1", Summary: "Detected a Generic API Key, potentially exposing access to various services.",
				Fingerprint: "9f2c4e1a7b3d5c6e8f0a1b2c3d4e5f6a7b8c9d0e:config/settings.py:generic-api-key:12"},
			{Tool: "gitleaks", Kind: KindSecret, ID: "private-key", Severity: SeverityHigh, File: "deploy/id_fixture",
				Location: "deploy/id_fixture:1", Summary: "Identified a Private Key, which may compromise cryptographic security and sensitive data encryption.",
				Fingerprint: "deploy/id_fixture:private-key:1"},
		}},
		{"govulncheck.json", parseGovulncheck, []Finding{
			{Tool: "govulncheck", Kind: KindVuln, ID: "GO-2099-0001", Severity: SeverityUnknown, Package: "example.test/web/http@v1.4.0",
				Location: "cmd/app/main.go:21", FixedVersion: "v1.4.2", Summary: "Request smuggling in example.test/web (CVE-2099-0001)"},
		}},
		{"npm-v7.json", parseNodeAudit, []Finding{
			{Tool: "node-audit", Kind: KindVuln, ID: "GHSA-aaaa-bbbb-cccc", Severity: SeverityHigh, Package: "fixture-parser@<2.1.0",
				FixedVersion: "fixture-parser@2.1.0", Summary: "Prototype pollution in fixture-parser"},
			{Tool: "node-audit", Kind: KindVuln, ID: "via:fixture-parser", Severity: SeverityMedium, Package: "fixture-wrapper@1.0.0 - 1.3.0",
				FixedVersion: "available (audit fix)", Summary: "depends on a vulnerable fixture-parser"},
		}},
		{"pnpm.json", parseNodeAudit, []Finding{
			{Tool: "node-audit", Kind: KindVuln, ID: "GHSA-dddd-eeee-ffff", Severity: SeverityCritical, Package: "fixture-yaml@3.0.0",
				FixedVersion: ">=3.0.1", Summary: "Code execution in fixture-yaml"},
		}},
		{"yarn-berry.ndjson", parseNodeAudit, []Finding{
			{Tool: "node-audit", Kind: KindVuln, ID: "GHSA-gggg-hhhh-iiii", Severity: SeverityLow, Package: "fixture-glob@5.0.1",
				FixedVersion: "", Summary: "ReDoS in fixture-glob (vulnerable <5.0.2)"},
		}},
		{"yarn-classic.ndjson", parseNodeAudit, []Finding{
			{Tool: "node-audit", Kind: KindVuln, ID: "GHSA-jjjj-kkkk-llll", Severity: SeverityMedium, Package: "fixture-ms",
				FixedVersion: ">=2.0.0", Summary: "Inefficient regex in fixture-ms"},
		}},
		{"cargo-audit.json", parseCargoAudit, []Finding{
			{Tool: "cargo-audit", Kind: KindVuln, ID: "RUSTSEC-2099-0001", Severity: SeverityUnknown, Package: "fixture-tls@0.5.1",
				FixedVersion: ">=0.5.3", Summary: "Certificate check bypass in fixture-tls"},
		}},
		{"pip-audit.json", parsePipAudit, []Finding{
			{Tool: "pip-audit", Kind: KindVuln, ID: "PYSEC-2099-1", Severity: SeverityUnknown, Package: "fixture-requests@2.0.0",
				FixedVersion: "2.0.5, 2.1.0", Summary: "Header injection in fixture-requests. (CVE-2099-0002)"},
		}},
	} {
		t.Run(row.fixture, func(t *testing.T) {
			got, err := row.parse(fixture(t, row.fixture))
			require.NoError(t, err)
			assert.Equal(t, row.want, got)
		})
	}
}

// The parsed shape has no field for the matched value, and the recorded
// value never survives a round trip through the JSON report.
func TestParseGitleaks_NeverKeepsTheValue(t *testing.T) {
	got, err := parseGitleaks(fixture(t, "gitleaks.json"))
	require.NoError(t, err)
	data, err := json.Marshal(got)
	require.NoError(t, err)
	assert.NotContains(t, string(data), "fixture-not-a-real-value-0001")
}

func TestParsers_RejectUnusableOutput(t *testing.T) {
	_, err := parseNodeAudit(fixture(t, "npm-error.json"))
	assert.ErrorContains(t, err, "ENOTFOUND")
	for name, parse := range map[string]func([]byte) ([]Finding, error){
		"gitleaks": parseGitleaks, "govulncheck": parseGovulncheck, "node": parseNodeAudit,
		"cargo": parseCargoAudit, "pip": parsePipAudit,
	} {
		_, err := parse([]byte("not json at all"))
		assert.Error(t, err, name)
	}
	empty, err := parseGitleaks([]byte("[]"))
	require.NoError(t, err)
	assert.Empty(t, empty)
}
