package cli

import (
	"bytes"
	"crypto/ed25519"
	"encoding/json"
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/insajin/autopus-adk/pkg/evalregression"
	"github.com/insajin/autopus-adk/pkg/harneval"
)

// autopusLane holds the Autopus gate's lane literals
// (../Autopus/.github/workflows/eval-regression-gate.yml L200-203: trust lane,
// source and target environment; the key id is evalRegressionPromotionKeyID).
// The revision and workspace scope are per-run values there.
var autopusLane = evalregression.EvalRegressionAttestationPolicyV2{
	ExpectedKeyID: "autopus-eval-staging-to-main-2026-07", TrustLane: "staging-to-main",
	SourceEnvironment: "staging", TargetEnvironment: "production",
	SourceRevision: "0123456789abcdef0123456789abcdef01234567", WorkspaceScope: "autopus-staging",
}

// laneEvidence writes report and attestation bytes as the strict check reads
// them and returns the report path.
func laneEvidence(t *testing.T, report, attestation []byte) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, harnessEvidenceReport)
	require.NoError(t, os.WriteFile(path, report, 0o600))
	require.NoError(t, os.WriteFile(filepath.Join(dir, harnessEvidenceAttestation), attestation, 0o600))
	return path
}

func strictLine(t *testing.T, path string, at time.Time, trusted map[string]ed25519.PublicKey, policy evalregression.EvalRegressionAttestationPolicyV2) string {
	t.Helper()
	var line bytes.Buffer
	checkEvalRegressionStrict("", path, deriveEvalRegressionAttestationPath(path), harnessEvidenceMaxAge, at, trusted, policy, &line, false, false)
	return line.String()
}

// TestEvalHarnessLane_S1_LanesRejectEachOtherWithRealValues: harness lane
// evidence from the export's signer verifies only under the harness policy,
// and evidence of the Autopus lane, signed under its real lane values, does
// not verify under the harness policy, though both keys are trusted. Against
// the committed allowlist, which holds the real key of each lane, the lanes
// still reject each other's evidence, and a test key verifies in neither.
func TestEvalHarnessLane_S1_LanesRejectEachOtherWithRealValues(t *testing.T) {
	t.Parallel()
	harnessPub, harnessPriv, _ := exportKey(t)
	autopusPub, autopusPriv, _ := exportKey(t)
	trusted := map[string]ed25519.PublicKey{evalregression.ADKHarnessEvalKeyID: harnessPub, autopusLane.ExpectedKeyID: autopusPub}
	binding := exportBinding().Digest()
	report, attestation, err := signHarnessEvidence(harnessSignable{Session: exportSession("pass", "fail"), SignedTaskFloor: 1}, binding, harnessPriv)
	require.NoError(t, err)
	harness := laneEvidence(t, report, attestation)
	at := exportStarted(t).Add(time.Hour)

	autopusReport := bytes.Replace(bytes.Replace(report, []byte(`"`+binding+`"`), []byte(`"`+autopusLane.SourceRevision+`"`), 1),
		[]byte(`"workspace_scope": "autopus-adk"`), []byte(`"workspace_scope": "autopus-staging"`), 1)
	signed, err := evalregression.SignEvalRegressionAttestationV2(autopusReport, autopusLane, autopusPriv)
	require.NoError(t, err)
	autopusAttestation, err := json.Marshal(signed)
	require.NoError(t, err)
	autopus := laneEvidence(t, autopusReport, autopusAttestation)

	assert.Equal(t, "eval-regression: regression_blocked (version="+binding+")\n", strictLine(t, harness, at, trusted, harneval.LanePolicy(binding)))
	assert.Equal(t, "eval-regression: attestation_policy_mismatch\n", strictLine(t, harness, at, trusted, autopusLane))
	assert.Equal(t, "eval-regression: attestation_policy_mismatch\n", strictLine(t, autopus, at, trusted, harneval.LanePolicy(binding)))
	assert.Equal(t, "eval-regression: regression_blocked (version="+autopusLane.SourceRevision+")\n", strictLine(t, autopus, at, trusted, autopusLane),
		"the Autopus control evidence itself verifies in its own lane")

	committed := evalregression.CommittedEvalRegressionPublicKeys()
	assert.Equal(t, "eval-regression: attestation_policy_mismatch\n", strictLine(t, harness, at, committed, autopusLane))
	assert.Equal(t, "eval-regression: attestation_policy_mismatch\n", strictLine(t, autopus, at, committed, harneval.LanePolicy(binding)))
	assert.Equal(t, "eval-regression: signature_invalid\n", strictLine(t, harness, at, committed, harneval.LanePolicy(binding)),
		"the committed harness lane key does not verify a test key's signature")
	assert.Equal(t, "eval-regression: signature_invalid\n", strictLine(t, autopus, at, committed, autopusLane),
		"the committed promotion key does not verify a test key's signature")
}

// TestEvalHarnessLane_S12_SelfVerificationUsesTheCommittedAllowlist: the
// production export trusts exactly the committed allowlist, whose
// ADKHarnessEvalKeyID entry is the operator's harness lane key, so evidence
// signed with any other key stops as self_verify_failed: signature_invalid and
// nothing is left to upload; the same run with its key allowlisted
// self-verifies and succeeds.
func TestEvalHarnessLane_S12_SelfVerificationUsesTheCommittedAllowlist(t *testing.T) {
	t.Parallel()
	pub, _, key := exportKey(t)
	production := exportSeams(t, exportSession("pass", "pass"), pub)
	production.trusted = nil
	out := filepath.Join(t.TempDir(), "evidence")

	got := runExport(t, production, key, exportArgs(out)...)

	assert.Equal(t, 1, got.code)
	assert.Contains(t, got.stderr, "harness-eval: export refused: self_verify_failed: signature_invalid")
	assert.Empty(t, got.stdout)
	assert.NoDirExists(t, out)

	normal := runExport(t, exportSeams(t, exportSession("pass", "pass"), pub), key, exportArgs(out)...)
	require.Equal(t, 0, normal.code, normal.stderr)
	assert.Equal(t, "eval-regression: ok (version="+exportBinding().Digest()+")\n", normal.stdout)
}

// TestEvalHarnessLane_RunbookHasTheLaneAndRotationOrder: the runbook names
// the six harness lane values and the rotation steps in order, including
// cancelling runs that are in progress or waiting for approval.
func TestEvalHarnessLane_RunbookHasTheLaneAndRotationOrder(t *testing.T) {
	t.Parallel()
	doc := readRunbook(t)
	requireContainsAll(t, doc, "## Harness eval lane (SPEC-HARNEVAL-003)", evalregression.ADKHarnessEvalKeyID,
		evalregression.ADKHarnessEvalTrustLane, harneval.LaneSourceEnvironment, harneval.LaneTargetEnvironment,
		harneval.ReportWorkspaceScope, "auto eval harness policy --binding B", "adk-harness-eval-agent",
		"adk-harness-eval-signing", "self-review", "attestation_policy_mismatch", "self_verify_failed")
	section := doc[strings.Index(doc, "### Harness lane key rotation"):]
	last := -1
	for _, step := range []string{
		"1. Stop dispatching", "2. Cancel every run that is in progress or waiting for Environment approval",
		"--status waiting", "gh run cancel", "3. Apply together", "`evalRegressionPublicKeys`", "`ADKHarnessEvalKeyID`",
		"`HARNESS_EVAL_SIGNING_KEY` Environment secret", "4. Resume dispatching", "5. Produce a new session",
	} {
		index := strings.Index(section, step)
		require.Greater(t, index, last, "rotation step %q is missing or out of order", step)
		last = index
	}
}

// TestEvalHarnessLane_V1VerifierHasNoProductionCaller: the v1 verifier trusts
// the whole allowlist without a lane policy, so no non-test Go file of the
// module references it beside its own declaration, and the two v1 CLI helpers
// exist only in test files (REQ-HR-05).
func TestEvalHarnessLane_V1VerifierHasNoProductionCaller(t *testing.T) {
	t.Parallel()
	root := filepath.Join("..", "..")
	var callers, helpers []string
	files := 0
	for _, top := range []string{"cmd", "internal", "pkg"} {
		require.NoError(t, filepath.WalkDir(filepath.Join(root, top), func(path string, entry fs.DirEntry, err error) error {
			if err != nil || entry.IsDir() || !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") ||
				strings.Contains(filepath.ToSlash(path), "/testdata/") {
				return err
			}
			file, err := parser.ParseFile(token.NewFileSet(), path, nil, parser.SkipObjectResolution)
			if err != nil {
				return err
			}
			files++
			declared := map[*ast.Ident]bool{}
			for _, decl := range file.Decls {
				if fn, ok := decl.(*ast.FuncDecl); ok {
					declared[fn.Name] = true
					if fn.Recv == nil && (fn.Name.Name == "checkEvalRegression" || fn.Name.Name == "evaluateEvalRegression") {
						helpers = append(helpers, path)
					}
				}
			}
			ast.Inspect(file, func(node ast.Node) bool {
				if ident, ok := node.(*ast.Ident); ok && ident.Name == "VerifyEvalRegressionArtifact" && !declared[ident] {
					callers = append(callers, path)
				}
				return true
			})
			return nil
		}))
	}
	require.Greater(t, files, 500, "the scan reads the module's sources")
	assert.Empty(t, callers, "non-test references to the v1 verifier")
	assert.Empty(t, helpers, "the v1 CLI helpers belong to eval_regression_v1_test.go")
}
