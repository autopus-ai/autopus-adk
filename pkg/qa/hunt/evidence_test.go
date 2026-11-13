package hunt

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func baseFinding(oracle string) Finding {
	return Finding{Schema: FindingSchema, ID: "x", Title: "t", Severity: "major", Oracle: oracle, Expected: "e", Actual: "a"}
}

func TestValidate_PolicyMismatchNeedsTheCitedRule(t *testing.T) {
	t.Parallel()
	f := baseFinding(OraclePolicyMismatch)
	assert.ErrorContains(t, f.Validate(), "policy.source")

	f.Policy = &PolicyRef{Source: "docs/billing.md:42", Quote: "  "}
	assert.Error(t, f.Validate(), "a blank quote cites nothing")

	f.Policy.Quote = "Refunds are allowed within 14 days."
	assert.NoError(t, f.Validate())
}

func TestValidate_InefficiencyNeedsAMeasureWithABasis(t *testing.T) {
	t.Parallel()
	f := baseFinding(OracleInefficiency)
	assert.ErrorContains(t, f.Validate(), "measure.metric")

	f.Measure = &Measure{Metric: "GET /api/items per load", Threshold: "1", Observed: "3"}
	assert.Error(t, f.Validate(), "a threshold without a basis can be picked to fail")

	f.Measure.Basis = "the page renders one list; the same response is fetched three times"
	assert.NoError(t, f.Validate())
}

func TestValidate_OtherOraclesNeedNoPremise(t *testing.T) {
	t.Parallel()
	assert.NoError(t, baseFinding("crash").Validate())
}

func TestWriteReport_ShowsPremisesAndLinksObservations(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(root, Dir), 0o755))
	notes := "# Observations\n\n- config is parsed on every request\n- two helpers do the same walk\n"
	require.NoError(t, os.WriteFile(filepath.Join(root, Dir, ObservationsFile), []byte(notes), 0o644))

	policy := baseFinding(OraclePolicyMismatch)
	policy.ID, policy.Status = "refund-window", Confirmed
	policy.Policy = &PolicyRef{Source: "docs/billing.md:42", Quote: "Refunds are allowed within 14 days."}
	slow := baseFinding(OracleInefficiency)
	slow.ID, slow.Status = "triple-fetch", Confirmed
	slow.Measure = &Measure{Metric: "GET /api/items per load", Threshold: "1", Observed: "3", Basis: "same response"}

	path, err := WriteReport(root, []Finding{policy, slow}, nil)
	require.NoError(t, err)
	data, err := os.ReadFile(path)
	require.NoError(t, err)
	text := string(data)
	assert.Contains(t, text, `docs/billing.md:42 — "Refunds are allowed within 14 days."`)
	assert.Contains(t, text, "observed 3, threshold 1 (same response)")
	assert.Contains(t, text, "## Observations, not verified (2)")
	assert.NotContains(t, text, "config is parsed", "observations are linked, not presented as findings")
}

func TestWriteReport_NoObservationsSectionWhenFileIsAbsent(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(root, Dir), 0o755))
	path, err := WriteReport(root, nil, nil)
	require.NoError(t, err)
	data, _ := os.ReadFile(path)
	assert.NotContains(t, string(data), "Observations")
}

func savePolicyFinding(t *testing.T, root, id, source, quote string) {
	t.Helper()
	f := baseFinding(OraclePolicyMismatch)
	f.ID = id
	f.Policy = &PolicyRef{Source: source, Quote: quote}
	require.NoError(t, os.MkdirAll(FindingDir(root, id), 0o755))
	require.NoError(t, Save(root, f))
}

func TestLoad_PolicyQuoteMustBeWrittenInItsSource(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(root, "docs"), 0o755))
	doc := "# Billing\n\nRefunds are allowed within\n7 days of purchase.\n"
	require.NoError(t, os.WriteFile(filepath.Join(root, "docs", "billing.md"), []byte(doc), 0o644))

	savePolicyFinding(t, root, "wrapped", "docs/billing.md:3-4", "Refunds are allowed within 7 days of purchase.")
	savePolicyFinding(t, root, "invented", "docs/billing.md:3", "Refunds are allowed within 14 days of purchase.")
	savePolicyFinding(t, root, "missing-file", "docs/nope.md", "anything")
	savePolicyFinding(t, root, "outside", "../secrets.md", "anything")

	findings, bad, err := Load(root)
	require.NoError(t, err)
	require.Len(t, findings, 1)
	assert.Equal(t, "wrapped", findings[0].ID, "a quote across a wrapped line still matches")
	assert.ErrorContains(t, bad["invented"], "not written in")
	assert.ErrorContains(t, bad["missing-file"], "cannot be read")
	assert.ErrorContains(t, bad["outside"], "inside the project")
}

func TestValidate_MeasureNeedsNumbers(t *testing.T) {
	t.Parallel()
	f := baseFinding(OracleInefficiency)
	f.Measure = &Measure{Metric: "latency", Threshold: "snappy", Observed: "sluggish", Basis: "feel"}
	assert.ErrorContains(t, f.Validate(), "must be numbers")
	f.Measure.Threshold, f.Measure.Observed = "200ms", "350ms"
	assert.NoError(t, f.Validate())
}
