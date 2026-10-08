package harneval

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestVerifySignedSession_ProtocolOffTheTrustedValues_NamesTheFirstField:
// each row moves one trusted field of an otherwise attested protocol, and the
// refusal names that field's path. The decoder makes the policy's revision,
// baseline ref, and model equal to the protocol's own, so those moves are
// named at their policy path.
func TestVerifySignedSession_ProtocolOffTheTrustedValues_NamesTheFirstField(t *testing.T) {
	t.Parallel()
	other := strings.Repeat("0d", 32)
	tests := []struct {
		path string
		edit func(p *Protocol)
	}{
		{"policy.k", func(p *Protocol) { p.Policy.K, p.Order = 3, BalancedOrder([]string{"GT-AG-001"}, 3) }},
		{"policy.threshold_bp", func(p *Protocol) { p.Policy.ThresholdBP = -2000 }},
		{"policy.completeness_floor", func(p *Protocol) { p.Policy.CompletenessFloor = 0.8 }},
		{"policy.max_agent_runs", func(p *Protocol) { p.Policy.MaxAgentRuns = 95 }},
		{"policy.trial_timeout_seconds", func(p *Protocol) { p.Policy.TrialTimeoutSeconds = 179 }},
		{"policy.workspace_revision", func(p *Protocol) { p.WorkspaceRevision, p.Policy.WorkspaceRevision = strings.Repeat("b", 40), "" }},
		{"policy.baseline_ref", func(p *Protocol) { p.BaselineRef, p.Policy.BaselineRef = "v0.50.121", "" }},
		{"policy.model", func(p *Protocol) { p.Model, p.Policy.Model = "gpt-other", "gpt-other" }},
		{"pins.generator_version", func(p *Protocol) { p.Pins.GeneratorVersion = "v0.50.124" }},
		{"pins.project_name", func(p *Protocol) { p.Pins.ProjectName = "harneval-other" }},
		{"pins.codex_model_catalog", func(p *Protocol) { p.Pins.CodexModelCatalog = "evals/harness/fixtures/codex-models.json" }},
		{"pins.codex_cli_version", func(p *Protocol) { p.Pins.CodexCLIVersion = "codex-cli 0.161.0" }},
		{"pins.opencode_cli_version", func(p *Protocol) { p.Pins.OpencodeCLIVersion = "1.18.8" }},
		{"cli_version", func(p *Protocol) { p.CLIVersion = "codex-cli 0.161.0" }},
		{"baseline_commit", func(p *Protocol) { p.BaselineCommit = strings.Repeat("d1", 20) }},
		{"baseline_commit", func(p *Protocol) {
			p.RunID, p.RunAttempt, p.BindingDigest, p.BaselineCommit, p.RunnerTreeDigest = 0, 0, "", "", ""
		}},
		{"agent_set_digest", func(p *Protocol) { p.AgentSetDigest = other }},
		{"corpus_digests", func(p *Protocol) { p.CorpusDigests = append(p.CorpusDigests, p.CorpusDigests[0]) }},
		{"corpus_digests[0]", func(p *Protocol) {
			p.CorpusDigests[0] = json.RawMessage(`{"file":"bench/corpus_a.json","file_sha256":"` + other + `","size":1}`)
		}},
		{"corpus_digests[0].file", func(p *Protocol) {
			p.CorpusDigests[0] = json.RawMessage(`{"file":"bench/corpus_z.json","file_sha256":"` + sha256Of(fixtureCorpus) + `"}`)
		}},
		{"corpus_digests[0].file_sha256", func(p *Protocol) {
			p.CorpusDigests[0] = json.RawMessage(`{"file":"bench/corpus_a.json","file_sha256":"` + other + `"}`)
		}},
		{"runner_tree_digest", func(p *Protocol) { p.RunnerTreeDigest = other }},
		{"order[2]", func(p *Protocol) { p.Order = []Attempt{signedB0, signedC0, signedB1, signedC1} }},
		{"order", func(p *Protocol) {
			p.Order = BalancedOrder([]string{"GT-AG-001", "GT-AG-002"}, 2)
			p.Calibration.Tasks = append(p.Calibration.Tasks, CalibrationTask{TaskID: "GT-AG-002", CleanAccepted: true})
		}},
		{"baseline_surface_digest", func(p *Protocol) { p.BaselineSurfaceDigest = other }},
		{"candidate_surface_digest", func(p *Protocol) { p.CandidateSurfaceDigest = other }},
		{"binding_digest", func(p *Protocol) { p.BindingDigest = other }},
		{"run_id", func(p *Protocol) { p.RunID = 7 }},
	}
	for _, tt := range tests {
		t.Run(tt.path, func(t *testing.T) {
			t.Parallel()
			run := newSignedRun(t)
			tt.edit(&run.protocol)

			session, err := run.verify()

			assert.Nil(t, session)
			requireTrustError(t, err, ReasonProtocolMismatch, tt.path)
		})
	}
}

// TestVerifySignedSession_RecordsAndCalibrationOffTheSession_AreRefused: the
// records must be the trusted order's attempts of the protocol's session,
// each once, and calibration.json must be that session's, over the scheduled
// tasks, with the before calibration the protocol froze.
func TestVerifySignedSession_RecordsAndCalibrationOffTheSession_AreRefused(t *testing.T) {
	t.Parallel()
	otherSession := strings.Repeat("ab", 16)
	tests := []struct {
		name, reason, detail string
		edit                 func(r *signedRun)
	}{
		{"record of another session", ReasonRecordsProtocolMismatch, "session_id GT-AG-001/baseline/0",
			func(r *signedRun) { r.trials[0].record.SessionID = otherSession }},
		{"record of an unscheduled trial", ReasonRecordsProtocolMismatch, "unscheduled GT-AG-001/baseline/2",
			func(r *signedRun) { r.trials[3].record.Trial = 2 }},
		{"record missing", ReasonRecordsProtocolMismatch, "missing GT-AG-001/baseline/1",
			func(r *signedRun) { r.trials = r.trials[:3] }},
		{"calibration of another session", ReasonRecordsProtocolMismatch, "calibration.json session_id",
			func(r *signedRun) { r.calibration.SessionID = otherSession }},
		{"before without the task", ReasonRecordsProtocolMismatch, "calibration.json before.tasks",
			func(r *signedRun) {
				r.calibration.Before = CalibrationPhase{Status: CalibrationFailed, Tasks: []CalibrationTask{}}
			}},
		{"after over another task", ReasonRecordsProtocolMismatch, "calibration.json after.tasks",
			func(r *signedRun) { r.calibration.After.Tasks[0].TaskID = "GT-AG-002" }},
		{"failed before labelled passed", ReasonOutcomeDerivationMismatch, "calibration.json before.status passed, its tasks give failed",
			func(r *signedRun) { r.calibration.Before.Tasks[0].CleanAccepted = false }},
		{"proven after labelled failed", ReasonOutcomeDerivationMismatch, "calibration.json after.status failed, its tasks give passed",
			func(r *signedRun) { r.calibration.After.Status = CalibrationFailed }},
		{"trials after a failed before calibration", ReasonRecordsProtocolMismatch,
			"calibration.json before failed, yet the trials ran", func(r *signedRun) {
				refused := func() CalibrationPhase {
					return CalibrationPhase{Status: CalibrationFailed, Tasks: []CalibrationTask{{TaskID: "GT-AG-001"}}}
				}
				r.protocol.Calibration, r.calibration.Before, r.calibration.After = refused(), refused(), nil
			}},
		{"before not the frozen calibration", ReasonRecordsProtocolMismatch,
			"calibration.json before differs from the calibration in protocol.json", func(r *signedRun) {
				r.protocol.Calibration = CalibrationPhase{Status: CalibrationFailed, Tasks: []CalibrationTask{{TaskID: "GT-AG-001"}}}
			}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			run := newSignedRun(t)
			tt.edit(run)

			session, err := run.verify()

			assert.Nil(t, session)
			requireTrustError(t, err, tt.reason, tt.detail)
		})
	}
}

// TestVerifySignedSession_AttestedDocumentBreaksItsContract_IsInvalid: an
// attested document that does not decode is refused as invalid with the
// file it was found in, as are trusted values the signer was handed wrong.
func TestVerifySignedSession_AttestedDocumentBreaksItsContract_IsInvalid(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name, detail, path string
		edit               func(t *testing.T, in *SignerInput, trusted *TrustedProtocol)
	}{
		{"protocol not JSON", DetailMalformedJSON, ProtocolFile, func(_ *testing.T, in *SignerInput, _ *TrustedProtocol) {
			in.Protocol = []byte("{")
		}},
		{"record line not JSON", DetailMalformedJSON, RecordsFile + ":5", func(_ *testing.T, in *SignerInput, _ *TrustedProtocol) {
			in.Records = append(in.Records, []byte("{\n")...)
		}},
		{"calibration unknown field", DetailUnknownField, CalibrationFile, func(t *testing.T, in *SignerInput, _ *TrustedProtocol) {
			in.Calibration = replaceOnce(t, in.Calibration, `"session_id"`, `"note":"x","session_id"`)
		}},
		{"calibration status unknown", DetailFieldInvalid, CalibrationFile, func(_ *testing.T, in *SignerInput, _ *TrustedProtocol) {
			in.Calibration = []byte(strings.Replace(string(in.Calibration), `"status":"passed"`, `"status":"skipped"`, 2))
		}},
		{"trusted binding malformed", DetailFieldInvalid, "", func(_ *testing.T, _ *SignerInput, trusted *TrustedProtocol) {
			trusted.BindingDigest = "b1"
		}},
		{"trusted policy without trials", DetailPolicyOutOfRange, "", func(_ *testing.T, _ *SignerInput, trusted *TrustedProtocol) {
			trusted.Policy.K = 0
		}},
		{"trusted order empty", DetailFieldInvalid, "", func(_ *testing.T, _ *SignerInput, trusted *TrustedProtocol) {
			trusted.Order = nil
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			run := newSignedRun(t)
			in, attested := run.attest()
			trusted := run.trusted
			tt.edit(t, &in, &trusted)
			attested.Result.ProtocolSHA256, attested.Result.RecordsSHA256 = sha256Hex(in.Protocol), sha256Hex(in.Records)
			attested.Result.CalibrationSHA256 = sha256Hex(in.Calibration)

			session, err := VerifySignedSession(in, trusted, run.meta, attested)

			assert.Nil(t, session)
			var invalid *InvalidError
			require.ErrorAs(t, err, &invalid)
			assert.Equal(t, tt.detail, invalid.Detail, err.Error())
			assert.Equal(t, tt.path, invalid.Path, err.Error())
		})
	}
	run := newSignedRun(t)
	run.trusted.Policy.TrialTimeoutSeconds = 19801
	_, err := run.verify()
	requireTrustError(t, err, ReasonPolicyOutOfRange, "max_agent_runs 96 x trial_timeout_seconds 19801 exceeds 19800 seconds")
}

// TestVerifySignedSession_UnprovenCalibration_CannotBeOK: a truthfully failed
// or absent end-of-session calibration is attested and verifies, and the 001
// verdict it feeds is vacuous, never ok.
func TestVerifySignedSession_UnprovenCalibration_CannotBeOK(t *testing.T) {
	t.Parallel()
	for name, edit := range map[string]func(r *signedRun){
		"after failed": func(r *signedRun) {
			r.calibration.After = &CalibrationPhase{Status: CalibrationFailed, Tasks: []CalibrationTask{
				{TaskID: "GT-AG-001", CleanAccepted: true, MutatedAccepted: true}}}
		},
		"after absent": func(r *signedRun) { r.calibration.After = nil },
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			run := newSignedRun(t)
			edit(run)

			session, err := run.verify()

			require.NoError(t, err)
			verdict, err := ComputeVerdict(session)
			require.NoError(t, err)
			assert.Equal(t, [2]string{VerdictVacuous, ReasonOracleCalibrationFailed}, [2]string{verdict.Verdict, verdict.Reason})
		})
	}
}
