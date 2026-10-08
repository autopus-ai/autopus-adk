package cli

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"encoding/base64"
	"encoding/json"
	"errors"
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

func TestEvalHarnessExport_S2_SignsAFreshPrivateDirectoryThatVerifies(t *testing.T) {
	t.Parallel()
	pub, priv, key := exportKey(t)
	binding := exportBinding().Digest()
	for _, tc := range []struct {
		name      string
		candidate []string
		want      string
		pass      bool
	}{
		{"S4 normal session", []string{"pass", "fail"}, "eval-regression: regression_blocked (version=" + binding + ")\n", false},
		{"ok session", []string{"pass", "pass"}, "eval-regression: ok (version=" + binding + ")\n", true},
	} {
		out := filepath.Join(t.TempDir(), "evidence")
		got := runExport(t, exportSeams(t, exportSession(tc.candidate...), pub), key, exportArgs(out)...)
		require.Equal(t, 0, got.code, tc.name+": "+got.stderr)
		assert.Equal(t, tc.want, got.stdout, tc.name)

		info, err := os.Stat(out)
		require.NoError(t, err)
		assert.Equal(t, os.FileMode(0o700), info.Mode().Perm(), tc.name)
		entries, err := os.ReadDir(out)
		require.NoError(t, err)
		require.Len(t, entries, 2, tc.name)
		reportPath := filepath.Join(out, harnessEvidenceReport)
		for _, name := range []string{harnessEvidenceReport, harnessEvidenceAttestation} {
			fileInfo, err := os.Stat(filepath.Join(out, name))
			require.NoError(t, err, name)
			assert.Equal(t, os.FileMode(0o600), fileInfo.Mode().Perm(), name)
			data, err := os.ReadFile(filepath.Join(out, name))
			require.NoError(t, err)
			assertNoKey(t, priv, name, data)
		}
		assertNoKey(t, priv, "stdout", []byte(got.stdout))
		assertNoKey(t, priv, "stderr", []byte(got.stderr))

		reportBytes, err := os.ReadFile(reportPath)
		require.NoError(t, err)
		_, reason, err := decodeEvalRegressionReport(reportBytes)
		require.NoError(t, err, reason)
		assert.Contains(t, string(reportBytes), `"produced_at": "`+exportStartedAt+`"`, "produced_at is the started_at string")
		attBytes, err := os.ReadFile(filepath.Join(out, harnessEvidenceAttestation))
		require.NoError(t, err)
		decoder := json.NewDecoder(bytes.NewReader(attBytes))
		decoder.DisallowUnknownFields()
		var att evalregression.EvalRegressionAttestationV2
		require.NoError(t, decoder.Decode(&att))
		assert.Equal(t, harneval.LanePolicy(binding).SourceRevision, att.SourceRevision)

		var line bytes.Buffer
		trusted := map[string]ed25519.PublicKey{evalregression.ADKHarnessEvalKeyID: pub}
		passed := checkEvalRegressionStrict("", reportPath, deriveEvalRegressionAttestationPath(reportPath), 72*time.Hour,
			exportStarted(t).Add(31*time.Hour), trusted, harneval.LanePolicy(binding), &line, false, false)
		assert.Equal(t, tc.pass, passed, tc.name)
		assert.Equal(t, tc.want, line.String(), "the evidence verifies on the strict check path")
	}
}

func TestEvalHarnessExport_S2_RefusesBeforeWritingAnything(t *testing.T) {
	t.Parallel()
	pub, priv, key := exportKey(t)
	_, other, _ := exportKey(t)
	mixed := append(append([]byte{}, priv.Seed()...), other[32:]...)
	failing := func(field string, err error) func(*harnessExportSeams) {
		return func(seams *harnessExportSeams) {
			if field == "binding" {
				seams.binding = func(context.Context, string, harneval.BindingOptions) (harneval.Binding, error) {
					return harneval.Binding{}, err
				}
				return
			}
			seams.reconstruct = func(context.Context, harnessExportRequest) (harnessSignable, error) { return harnessSignable{}, err }
		}
	}
	cases := map[string]struct {
		stdin  string
		mutate func(*harnessExportSeams)
		want   string
	}{
		"empty stdin":               {"", nil, "private_key_missing"},
		"blank stdin":               {"\n \t\n", nil, "private_key_missing"},
		"not base64":                {"not base64!", nil, "private_key_invalid"},
		"seed only":                 {base64.StdEncoding.EncodeToString(priv.Seed()), nil, "private_key_invalid"},
		"foreign public half":       {base64.StdEncoding.EncodeToString(mixed), nil, "private_key_invalid"},
		"oversized stdin":           {strings.Repeat("A", 5000), nil, "private_key_invalid"},
		"binding fails":             {key, failing("binding", errors.New("templates unreadable")), "binding_failed: templates unreadable"},
		"reconstruction refuses":    {key, failing("reconstruct", errors.New("attestation_digest_mismatch: records.jsonl")), "attestation_digest_mismatch: records.jsonl"},
		"production reconstruction": {key, func(seams *harnessExportSeams) { seams.reconstruct = nil }, "run_meta_invalid"},
	}
	for name, tc := range cases {
		reconstructed := false
		seams := exportSeams(t, exportSession("pass", "fail"), pub)
		if tc.mutate != nil {
			tc.mutate(&seams)
		}
		if inner := seams.reconstruct; inner != nil && tc.mutate == nil {
			seams.reconstruct = func(ctx context.Context, req harnessExportRequest) (harnessSignable, error) {
				reconstructed = true
				return inner(ctx, req)
			}
		}
		out := filepath.Join(t.TempDir(), "evidence")
		got := runExport(t, seams, tc.stdin, exportArgs(out)...)
		assert.Equal(t, 1, got.code, name)
		assert.Contains(t, got.stderr, "harness-eval: export refused: "+tc.want, name)
		assert.Empty(t, got.stdout, name)
		assert.NoFileExists(t, out, name)
		assert.NoDirExists(t, out, name)
		assert.False(t, reconstructed, "%s: a key refusal precedes the reconstruction", name)
		assertNoKey(t, priv, name+" stderr", []byte(got.stderr))
	}
}

func TestEvalHarnessExport_S2_ExistingOutputIsLeftUntouched(t *testing.T) {
	t.Parallel()
	pub, _, key := exportKey(t)
	existing := filepath.Join(t.TempDir(), "evidence")
	require.NoError(t, os.Mkdir(existing, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(existing, "marker"), []byte("keep\n"), 0o644))
	dangling := filepath.Join(t.TempDir(), "evidence")
	require.NoError(t, os.Symlink(filepath.Join(t.TempDir(), "absent"), dangling))
	for _, out := range []string{existing, dangling} {
		got := runExport(t, exportSeams(t, exportSession("pass", "fail"), pub), key, exportArgs(out)...)
		assert.Equal(t, 1, got.code)
		assert.Contains(t, got.stderr, "harness-eval: export refused: output_exists")
	}
	entries, err := os.ReadDir(existing)
	require.NoError(t, err)
	require.Len(t, entries, 1, "nothing is added to an existing directory")
	data, err := os.ReadFile(filepath.Join(existing, "marker"))
	require.NoError(t, err)
	assert.Equal(t, "keep\n", string(data))
	_, err = os.Lstat(filepath.Join(t.TempDir(), "absent"))
	assert.True(t, errors.Is(err, os.ErrNotExist), "the dangling link is not followed")

	got := runExport(t, exportSeams(t, exportSession("pass", "fail"), pub), "", exportArgs(existing)...)
	assert.Contains(t, got.stderr, "export refused: private_key_missing", "the key is checked first")
}

func TestEvalHarnessExport_S2_KeyFlagsAreUnknown(t *testing.T) {
	t.Parallel()
	pub, _, key := exportKey(t)
	for _, flag := range []string{"--key", "--private-key", "--signing-key"} {
		out := filepath.Join(t.TempDir(), "evidence")
		got := runExport(t, exportSeams(t, exportSession("pass", "fail"), pub), "", append(exportArgs(out), flag, key)...)
		assert.Equal(t, 1, got.code, flag)
		assert.Contains(t, got.stderr, "unknown flag: "+flag, flag)
		assert.NoDirExists(t, out, flag)
	}
}

func TestEvalHarnessExport_S2_FailureAfterTheReportRemovesTheDirectory(t *testing.T) {
	t.Parallel()
	pub, _, key := exportKey(t)
	out := filepath.Join(t.TempDir(), "evidence")
	seams := exportSeams(t, exportSession("pass", "fail"), pub)
	var reportWritten bool
	seams.afterReport = func() error {
		_, err := os.Stat(filepath.Join(out, harnessEvidenceReport))
		reportWritten = err == nil
		return errors.New("disk full")
	}
	got := runExport(t, seams, key, exportArgs(out)...)
	assert.True(t, reportWritten, "the failure is injected after the report is on disk")
	assert.Equal(t, 1, got.code)
	assert.Contains(t, got.stderr, "harness-eval: export refused: output_write_failed")
	assert.NoDirExists(t, out)
}

// TestEvalHarnessExport_S12_SelfVerifyKeepsUnverifiableEvidenceFromUpload:
// evidence that does not verify against the trusted allowlist is removed.
func TestEvalHarnessExport_S12_SelfVerifyKeepsUnverifiableEvidenceFromUpload(t *testing.T) {
	t.Parallel()
	otherPub, _, _ := exportKey(t)
	cases := map[string]struct {
		trusted map[string]ed25519.PublicKey
		after   time.Duration
		want    string
	}{
		"another key under the lane id": {map[string]ed25519.PublicKey{evalregression.ADKHarnessEvalKeyID: otherPub}, time.Hour, "self_verify_failed: signature_invalid"},
		"lane key not committed":        {map[string]ed25519.PublicKey{}, time.Hour, "self_verify_failed: signature_key_unknown"},
		"older than the window":         {nil, 73 * time.Hour, "self_verify_failed: artifact_stale"},
	}
	for name, tc := range cases {
		pub, _, key := exportKey(t)
		trusted, now := tc.trusted, exportStarted(t).Add(tc.after)
		if trusted == nil {
			trusted = map[string]ed25519.PublicKey{evalregression.ADKHarnessEvalKeyID: pub}
		}
		seams := exportSeams(t, exportSession("pass", "fail"), pub)
		seams.trusted = func() map[string]ed25519.PublicKey { return trusted }
		seams.now = func() time.Time { return now }
		out := filepath.Join(t.TempDir(), "evidence")
		got := runExport(t, seams, key, exportArgs(out)...)
		assert.Equal(t, 1, got.code, name)
		assert.Contains(t, got.stderr, "harness-eval: export refused: "+tc.want, name)
		assert.Empty(t, got.stdout, name)
		assert.NoDirExists(t, out, name)
	}
}
