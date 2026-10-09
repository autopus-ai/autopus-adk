package evidence

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/insajin/autopus-adk/pkg/qa/desktopobserve"
)

func TestDesktopObservationEvidence_V2TypedPayloadRoundTripsWithExactAllowlist(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	observation := successfulObservationEvidence(t)
	manifest := desktopObservationManifest(t, dir, observation, "passed")

	manifestPath, err := WriteFinalManifest(manifest, filepath.Join(dir, "published"))
	require.NoError(t, err)
	loaded, err := LoadManifest(manifestPath)
	require.NoError(t, err)
	require.NotNil(t, loaded.OracleResults.DesktopObservation)
	assert.Equal(t, observation.SemanticProjection.Digest, loaded.OracleResults.DesktopObservation.SemanticProjection.Digest)

	body, err := os.ReadFile(manifestPath)
	require.NoError(t, err)
	var object map[string]any
	require.NoError(t, json.Unmarshal(body, &object))
	oracles := object["oracle_results"].(map[string]any)
	desktop := oracles["desktop_observation"].(map[string]any)
	assert.ElementsMatch(t, []string{"semantic_projection", "deterministic_checks", "runtime_receipt"}, keysOf(desktop))
	assertElevenSemanticConcepts(t, desktop["semantic_projection"].(map[string]any))
	assertForbiddenDesktopInventoryZero(t, body)
}

func TestDesktopObservationEvidence_UnknownRawPayloadFieldsAreRejected(t *testing.T) {
	t.Parallel()

	observation := successfulObservationEvidence(t)
	body, err := json.Marshal(observation)
	require.NoError(t, err)
	body = bytes.Replace(body, []byte("{"), []byte(`{"raw_tree":{"pid":42},"screenshot":"bytes",`), 1)

	_, err = desktopobserve.DecodeObservationEvidence(body)
	require.Error(t, err)
	assert.ErrorIs(t, err, desktopobserve.ErrUnknownField)
}

func TestRedactionFailClosed_PublishTimeRedactionMustBeNoOp(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	observation := successfulObservationEvidence(t)
	projection := observation.SemanticProjection
	projection.Root.Name = "sk-" + "proj-qamesh-secret-1234567890" // split so no scanner matches the source
	normalized, err := desktopobserve.NormalizeProjection(*projection, func(value string) (string, error) { return value, nil })
	require.NoError(t, err)
	observation.SemanticProjection = &normalized
	manifest := desktopObservationManifest(t, dir, observation, "passed")
	output := filepath.Join(dir, "published")

	_, err = WriteFinalManifest(manifest, output)
	require.Error(t, err)
	assert.NoDirExists(t, output)
}

func TestQuarantineFailClosed_ReceiptOnlyFailurePublishesNoSemanticPayload(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		reason     desktopobserve.ReasonCode
		redaction  desktopobserve.RedactionStatus
		quarantine desktopobserve.QuarantineStatus
	}{
		{name: "redaction", reason: desktopobserve.ReasonRedactionFailed, redaction: desktopobserve.RedactionFailed, quarantine: desktopobserve.QuarantineBlocked},
		{name: "quarantine", reason: desktopobserve.ReasonEvidenceQuarantined, redaction: desktopobserve.RedactionApplied, quarantine: desktopobserve.QuarantineLocalOnly},
	}

	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			dir := t.TempDir()
			reason := test.reason
			receipt := observationReceipt(desktopobserve.RuntimeProviderLocal)
			receipt.ReasonCode = &reason
			nextStep := desktopobserve.NextStep(reason)
			receipt.NextStep = &nextStep
			receipt.Redaction.Status = test.redaction
			receipt.Quarantine.Status = test.quarantine
			observation := desktopobserve.ObservationEvidence{
				SemanticProjection: nil,
				DeterministicChecks: []desktopobserve.DeterministicCheck{{
					ID: "desktop-semantic-landmarks", Status: desktopobserve.CheckBlocked,
					ReasonCode: &reason,
				}},
				RuntimeReceipt: receipt,
			}
			manifest := desktopObservationManifest(t, dir, observation, "blocked")
			path, err := WriteFinalManifest(manifest, filepath.Join(dir, "published"))
			require.NoError(t, err)
			body, err := os.ReadFile(path)
			require.NoError(t, err)
			assert.NotContains(t, string(body), `"semantic_projection"`)
			assert.NotContains(t, string(body), "provider error")
			assertForbiddenDesktopInventoryZero(t, body)
		})
	}
}

func TestDesktopObservationEvidence_V1DesktopContractUnchanged(t *testing.T) {
	t.Parallel()

	manifest := Manifest{
		SchemaVersion: SchemaVersionV1, QAResultID: "legacy-desktop", Surface: "desktop",
		Lane: "desktop-native", ScenarioRef: "desktop:legacy", Runner: Runner{Name: "appium"},
		Status: "passed", StartedAt: "2026-07-18T00:00:00Z", EndedAt: "2026-07-18T00:00:01Z",
		RetentionClass: "local-redacted",
		Artifacts: []ArtifactRef{
			{Kind: "screenshot", Path: "shot.json"}, {Kind: "app_log", Path: "app.log"},
			{Kind: "driver_log", Path: "driver.log"}, {Kind: "command_output", Path: "command.log"},
		},
		OracleResults:   OracleResults{Desktop: &DesktopOracle{TimeoutClassification: "none"}},
		RedactionStatus: RedactionStatus{Status: "passed"},
		SourceRefs: SourceRefs{
			SourceSpec: "SPEC-DESKTOP-017", AcceptanceRefs: []string{"AC-DESKTOP17-001"},
		},
	}
	require.NoError(t, manifest.Validate())
}

func successfulObservationEvidence(t *testing.T) desktopobserve.ObservationEvidence {
	t.Helper()
	enabled, focused := true, true
	projection := desktopobserve.SemanticProjection{
		SchemaVersion: desktopobserve.SemanticProjectionSchemaVersion,
		ProviderRef:   "provider-local", AppRef: "autopus-desktop", WindowRef: "main-window", StateRef: "state-1",
		Root: desktopobserve.SemanticNode{
			Role: desktopobserve.RoleApplication, Name: "Autopus",
			SemanticState:     desktopobserve.SemanticState{Enabled: &enabled},
			Frame:             &desktopobserve.Frame{X: 0, Y: 0, Width: 1440, Height: 900},
			AdvertisedActions: []desktopobserve.Action{desktopobserve.ActionPress},
			Children: []desktopobserve.SemanticNode{{
				Role: desktopobserve.RoleWindow, Name: "Autopus",
				SemanticState: desktopobserve.SemanticState{Focused: &focused},
			}},
		},
	}
	normalized, err := desktopobserve.NormalizeProjection(projection, func(value string) (string, error) { return value, nil })
	require.NoError(t, err)
	return desktopobserve.ObservationEvidence{
		SemanticProjection:  &normalized,
		DeterministicChecks: []desktopobserve.DeterministicCheck{{ID: "desktop-semantic-landmarks", Status: desktopobserve.CheckPassed}},
		RuntimeReceipt:      observationReceipt(desktopobserve.RuntimeProviderLocal),
	}
}

func observationReceipt(provider desktopobserve.RuntimeProvider) desktopobserve.RuntimeReceipt {
	name := "autopus-desktop-local"
	if provider == desktopobserve.RuntimeProviderOrca {
		name = "orca-computer-use-macos"
	}
	capabilities := make([]desktopobserve.CapabilityStatus, 0, 5)
	for _, operation := range desktopobserve.ReadOnlyOperations() {
		capabilities = append(capabilities, desktopobserve.CapabilityStatus{Name: operation, Status: desktopobserve.CapabilitySupported})
	}
	return desktopobserve.RuntimeReceipt{
		SchemaVersion:     desktopobserve.RuntimeReceiptSchemaVersion,
		Provider:          desktopobserve.ProviderIdentity{Name: name, Version: "1.0.0", ProtocolVersion: desktopobserve.ProtocolVersion},
		Scope:             desktopobserve.ReceiptScope{Kind: desktopobserve.ScopeWindow, PublicRef: "main-window"},
		CapabilitySummary: capabilities,
		Redaction:         desktopobserve.RedactionReceipt{Status: desktopobserve.RedactionApplied},
		Quarantine:        desktopobserve.QuarantineReceipt{Status: desktopobserve.QuarantineCleared},
	}
}

func desktopObservationManifest(t *testing.T, dir string, observation desktopobserve.ObservationEvidence, status string) Manifest {
	t.Helper()
	body, err := json.Marshal(observation)
	require.NoError(t, err)
	artifact := filepath.Join(dir, "desktop-observation.json")
	require.NoError(t, os.WriteFile(artifact, body, 0o600))
	checkStatus := status
	if checkStatus != "passed" {
		checkStatus = "blocked"
	}
	return Manifest{
		SchemaVersion: SchemaVersionV2, QAResultID: "qa-desktop-observe", Surface: "desktop",
		Lane: "desktop-native", ScenarioRef: "desktop-accessibility-observe", Runner: Runner{Name: "desktop-accessibility-observe"},
		Status: status, StartedAt: "2026-07-18T00:00:00Z", EndedAt: "2026-07-18T00:00:01Z",
		RetentionClass: "local-redacted",
		Artifacts:      []ArtifactRef{{Kind: "desktop_observation", Path: artifact, Publishable: true, Redaction: "pre_redacted_and_scanned"}},
		OracleResults: OracleResults{
			DesktopObservation: &observation,
			Checks:             []CheckResult{{ID: "desktop-semantic-landmarks", Type: "desktop_accessibility_semantic", Status: checkStatus, FailureSummary: failureSummary(checkStatus)}},
		},
		RedactionStatus: RedactionStatus{Status: "passed"},
		SourceRefs:      SourceRefs{SourceSpec: "SPEC-QAMESH-012", AcceptanceRefs: []string{"AC-QAMESH12-008", "AC-QAMESH12-009"}, JourneyID: "desktop-accessibility-observe", StepID: "step-1", Adapter: "desktop-accessibility-observe"},
	}
}

func failureSummary(status string) string {
	if status == "blocked" {
		return "desktop observation blocked"
	}
	return ""
}

func keysOf(values map[string]any) []string {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	return keys
}

func assertElevenSemanticConcepts(t *testing.T, projection map[string]any) {
	t.Helper()
	for _, key := range []string{"provider_ref", "app_ref", "window_ref", "state_ref", "digest"} {
		assert.Contains(t, projection, key)
	}
	root := projection["root"].(map[string]any)
	for _, key := range []string{"node_ref", "role", "name", "semantic_state", "frame", "advertised_actions"} {
		assert.Contains(t, root, key)
	}
}

func assertForbiddenDesktopInventoryZero(t *testing.T, body []byte) {
	t.Helper()
	for _, pattern := range []*regexp.Regexp{
		regexp.MustCompile(`(?i)raw[_ -]?ax|raw_tree|screenshot|\.png`),
		regexp.MustCompile(`(?i)"(pid|handle|socket|helper_path|index)"\s*:`),
		regexp.MustCompile(`/Users/|/tmp/|/private/var/|/var/folders/|/Volumes/|/Applications/|sk-proj-`),
	} {
		assert.Zero(t, len(pattern.FindAll(body, -1)), pattern.String())
	}
}
