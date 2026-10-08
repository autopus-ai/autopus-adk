package cli

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/insajin/autopus-adk/pkg/harneval"
)

// reconstructStart is the protocol started_at of the reconstruction fixture.
var reconstructStart = time.Date(2026, 10, 9, 1, 2, 3, 0, time.UTC)

// reconstructWorld is a main checkout whose golden set declares the given
// signed-lane floor, the trusted binding computed from it, and a live result
// of its one black-box task GT-AG-001 whose trials all passed.
type reconstructWorld struct {
	root, input, meta string
	deps              evalHarnessDeps
	digest            string
	sources           harnessTrustSources
}

func sha256String(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

func writeReconstructFile(t *testing.T, path string, data []byte) {
	t.Helper()
	require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o755))
	require.NoError(t, os.WriteFile(path, data, 0o644))
}

func newReconstructWorld(t *testing.T, floor int) *reconstructWorld {
	return newReconstructWorldWith(t, floor, harnessDeps(harnessRouter))
}

// newReconstructWorldWith builds the world with deps, whose adapters generate
// the candidate surface the binding digests.
func newReconstructWorldWith(t *testing.T, floor int, deps evalHarnessDeps) *reconstructWorld {
	t.Helper()
	tree := exportGitTree(t)
	manifest := harnessManifest()
	manifest["floors"].(map[string]any)["signed_agent_tasks"] = floor
	tree.writeJSON(harneval.ManifestPath, manifest)
	world := &reconstructWorld{root: tree.root, input: filepath.Join(t.TempDir(), "unsigned"),
		meta: filepath.Join(t.TempDir(), "run-meta.json"), deps: deps}
	binding, err := harneval.ComputeBinding(context.Background(), tree.root, harneval.BindingOptions{Adapters: world.deps.run.Adapters})
	require.NoError(t, err)
	world.digest = binding.Digest()
	assertions := map[string][]string{"GT-AG-001": {"exit", "stdout", "result"}}
	baseline := strings.Repeat("ba", 32)
	set, err := harneval.LoadSet(tree.root)
	require.NoError(t, err)
	trusted, err := harneval.RebuildTrustedProtocol(set, harneval.TrustedInputs{
		BaselineCommit: binding.BaselineCommit, RunnerTreeDigest: binding.RunnerTreeDigest, BaselineSurfaceDigest: baseline,
		CandidateSurfaceDigest: binding.CandidateSurfaceDigest, BindingDigest: world.digest, OracleAssertions: assertions,
	})
	require.NoError(t, err)
	world.writeLiveResult(t, trusted)
	world.sources = harnessTrustSources{
		attestations: func(context.Context, harnessExportRequest, harneval.RunMeta) (harneval.AttestedSession, error) {
			return world.attested(t), nil
		},
		baselineSurface:  func(context.Context, harnessExportRequest) (string, error) { return baseline, nil },
		oracleAssertions: func(*harneval.Set) (map[string][]string, error) { return assertions, nil },
	}
	return world
}

// writeLiveResult writes what the live-eval job uploads for the trusted
// protocol: every trial accepted by one shared oracle result.
func (w *reconstructWorld) writeLiveResult(t *testing.T, trusted harneval.TrustedProtocol) {
	t.Helper()
	const session = "6f1d2c3b4a5960718293a4b5c6d7e8f9"
	result := []byte(`{"schema_version":"harness_oracle_result.v1","task_id":"GT-AG-001","output_check":"ok",` +
		`"assertions":[{"id":"exit","passed":true},{"id":"stdout","passed":true},{"id":"result","passed":true}],"artifact_exit":0,"timed_out":false}`)
	digest := sha256String(result)
	writeReconstructFile(t, filepath.Join(w.input, harneval.OracleResultsDir, digest+".json"), result)
	phase := harneval.CalibrationPhase{Status: harneval.CalibrationPassed, Tasks: []harneval.CalibrationTask{{TaskID: "GT-AG-001", CleanAccepted: true}}}
	corpus := []json.RawMessage{}
	for _, row := range trusted.CorpusDigests {
		data, err := json.Marshal(row)
		require.NoError(t, err)
		corpus = append(corpus, data)
	}
	protocol := harneval.Protocol{SchemaVersion: harneval.ProtocolSchemaV1, SessionID: session, StartedAt: reconstructStart.Format(time.RFC3339),
		WorkspaceRevision: trusted.WorkspaceRevision, BaselineRef: trusted.BaselineRef, BaselineSurfaceDigest: trusted.BaselineSurfaceDigest,
		CandidateSurfaceDigest: trusted.CandidateSurfaceDigest, AgentSetDigest: trusted.AgentSetDigest, CorpusDigests: corpus,
		RunnerSHA256: strings.Repeat("5c", 32), GraderProfileSHA256: strings.Repeat("9a", 32), Calibration: phase, Policy: trusted.Policy,
		Pins: trusted.Pins, CLIVersion: trusted.CLIVersion, Model: trusted.Model, Order: trusted.Order,
		PromptLayers: []json.RawMessage{json.RawMessage(`{"layer":"stable","identifiers":[]}`)},
		RunID:        18300000002, RunAttempt: 1, BindingDigest: trusted.BindingDigest, BaselineCommit: trusted.BaselineCommit,
		RunnerTreeDigest: trusted.RunnerTreeDigest}
	var records bytes.Buffer
	exit := 0
	for _, attempt := range trusted.Order {
		line, err := json.Marshal(harneval.Record{SchemaVersion: harneval.RecordSchemaV1, SessionID: session, TaskID: attempt.TaskID,
			Arm: attempt.Arm, Trial: attempt.Trial, Outcome: harneval.OutcomePass, Signal: "accepted",
			Oracle: &harneval.OracleObservation{Ran: true, ExpectedPassed: 3}, DurationS: 12.5, OracleResultSHA256: &digest,
			StageReached: harneval.StageOracle, AgentTermination: &harneval.AgentTermination{Launched: true, ExitCode: &exit}})
		require.NoError(t, err)
		records.Write(append(line, '\n'))
	}
	for name, value := range map[string]any{harneval.ProtocolFile: protocol,
		harneval.CalibrationFile: harneval.Calibration{SessionID: session, Before: phase, After: &phase}} {
		data, err := json.Marshal(value)
		require.NoError(t, err)
		writeReconstructFile(t, filepath.Join(w.input, name), data)
	}
	writeReconstructFile(t, filepath.Join(w.input, harneval.RecordsFile), records.Bytes())
	writeReconstructFile(t, w.meta, []byte(`{"run_id":18300000002,"run_attempt":1,"run_created_at":"2026-10-09T00:40:00Z","attempt_started_at":"2026-10-09T00:40:05Z"}`))
}

// attested is the verified attestation pair of the bytes now in the input.
func (w *reconstructWorld) attested(t *testing.T) harneval.AttestedSession {
	t.Helper()
	in, err := harneval.LoadSignerInput(w.input)
	require.NoError(t, err)
	bound := harneval.BoundPredicate{RunID: 18300000002, RunAttempt: 1, BindingDigest: w.digest, Event: harneval.EventBound}
	result := harneval.SessionResultPredicate{BoundPredicate: bound, ProtocolSHA256: sha256String(in.Protocol),
		RecordsSHA256: sha256String(in.Records), CalibrationSHA256: sha256String(in.Calibration)}
	result.Event = harneval.EventSessionResult
	for _, data := range in.OracleResults {
		result.OracleResultSHA256 = append(result.OracleResultSHA256, sha256String(data))
	}
	return harneval.AttestedSession{Bound: bound, BoundLogTime: reconstructStart.Add(-10 * time.Minute), Result: result,
		ResultLogTime: reconstructStart.Add(time.Hour)}
}

// export runs the production export over the world: the real binding and the
// trusted reconstruction with the given sources.
func (w *reconstructWorld) export(t *testing.T, sources harnessTrustSources) (harnessOutcome, string, ed25519.PublicKey) {
	t.Helper()
	pub, _, key := exportKey(t)
	seams := exportSeams(t, nil, pub)
	seams.reconstruct, seams.binding, seams.sources = nil, nil, sources
	seams.now = func() time.Time { return reconstructStart.Add(30 * time.Hour) }
	out := filepath.Join(t.TempDir(), "evidence")
	cmd := newEvalHarnessExportCmdWith(w.deps, &w.root, seams)
	cmd.SilenceUsage, cmd.SilenceErrors = true, true
	var stdout, stderr bytes.Buffer
	cmd.SetIn(strings.NewReader(key))
	cmd.SetOut(&stdout)
	cmd.SetErr(&stderr)
	cmd.SetArgs([]string{"--input", w.input, "--run-meta", w.meta, "--output", out})
	code := 0
	if err := cmd.Execute(); err != nil {
		code = exitCodeForError(err)
	}
	return harnessOutcome{stdout: stdout.String(), stderr: stderr.String(), code: code}, out, pub
}

// TestEvalHarnessExport_TrustedReconstruction_SignsTheVerifiedSession: the
// default reconstruction loads the run meta and the received bytes, rebuilds
// the trusted protocol with the binding digest of main, verifies the session,
// and signs its verdict under main's signed-lane floor.
func TestEvalHarnessExport_TrustedReconstruction_SignsTheVerifiedSession(t *testing.T) {
	for _, tt := range []struct {
		name         string
		floor        int
		line, reason string
	}{
		{"floor met", 1, "ok", harneval.ReasonWithinThreshold},
		{"fewer black-box tasks than the floor", 2, "regression_blocked", harneval.ReportReasonVacuous},
	} {
		world := newReconstructWorld(t, tt.floor)

		got, out, _ := world.export(t, world.sources)

		require.Equal(t, 0, got.code, tt.name+": "+got.stderr)
		assert.Equal(t, "eval-regression: "+tt.line+" (version="+world.digest+")\n", got.stdout, tt.name)
		report, err := os.ReadFile(filepath.Join(out, harnessEvidenceReport))
		require.NoError(t, err)
		assert.Contains(t, string(report), `"reason": "`+tt.reason+`"`, tt.name)
		assert.Contains(t, string(report), `"produced_at": "2026-10-09T01:02:03Z"`, tt.name)
	}
}

// TestEvalHarnessExport_TrustedReconstruction_RefusesWithoutWriting: bytes
// changed after the attestation and a missing trust source are refused, and
// nothing is written.
func TestEvalHarnessExport_TrustedReconstruction_RefusesWithoutWriting(t *testing.T) {
	world := newReconstructWorld(t, 1)
	attested := world.attested(t)
	tampered := world.sources
	tampered.attestations = func(context.Context, harnessExportRequest, harneval.RunMeta) (harneval.AttestedSession, error) {
		return attested, nil
	}
	records := filepath.Join(world.input, harneval.RecordsFile)
	data, err := os.ReadFile(records)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(records, bytes.Replace(data, []byte(`"duration_s":12.5`), []byte(`"duration_s":12.6`), 1), 0o644))
	for name, tc := range map[string]struct {
		sources harnessTrustSources
		want    string
	}{
		"records changed after attestation": {tampered, "attestation_digest_mismatch: records.jsonl differs from records_sha256"},
		"no trust sources in this build":    {harnessTrustSources{}, "reconstruction_unavailable: this build has no black_box_oracle"},
	} {
		got, out, _ := world.export(t, tc.sources)

		assert.Equal(t, 1, got.code, name)
		assert.Contains(t, got.stderr, "harness-eval: export refused: "+tc.want, name)
		assert.Empty(t, got.stdout, name)
		assert.NoDirExists(t, out, name)
	}
}
