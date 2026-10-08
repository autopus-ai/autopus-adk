package cli

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os/exec"
	"sort"
	"strings"
	"time"

	"github.com/insajin/autopus-adk/pkg/brainstorm"
	"github.com/insajin/autopus-adk/pkg/config"
	"github.com/insajin/autopus-adk/pkg/healthband"
	"github.com/insajin/autopus-adk/pkg/orchestra"
	"github.com/insajin/autopus-adk/pkg/promptlayer"
)

// Diagnose claims of auto react band (SPEC-SIGMABAND-001 REQ-11, REQ-12,
// REQ-18, Provider Read-Only Contract). One claim runs at most one provider,
// read-only, and always ends with an attempt to write one BS: an unavailable
// provider is recorded, never retried with another provider, and never
// fails the run.

// Diagnosis statuses and the reasons of an unavailable diagnosis (REQ-12).
const (
	bandDiagnosisOK                = "ok"
	bandDiagnosisSkippedNoAgent    = "skipped(no_agent)"
	bandProviderUnconfigured       = "provider_unconfigured"
	bandProviderUnsupported        = "provider_unsupported"
	bandProviderPolicyRejected     = "provider_policy_rejected"
	bandProviderPolicyIncomplete   = "provider_policy_incomplete"
	bandProviderBackendUnavailable = "provider_backend_unavailable"
	bandProviderMissing            = "provider_missing"
	bandProviderTimedOut           = "provider_timeout"
	bandProviderExitNonzero        = "provider_exit_nonzero"
	bandProviderEmptyOutput        = "provider_empty_output"
	// bandPromptInvalid marks evidence the prompt layers refused; band's own
	// sanitizer never produces it, so it signals a broken caller.
	bandPromptInvalid = "prompt_invalid"
	bandBSWritten     = "written"
)

func bandUnavailable(reason string) string { return "unavailable(" + reason + ")" }

// bandProviderUnsetEnv are the inherited credentials a read-only diagnosis
// never needs, so a provider steered by injected evidence cannot use them:
// GitHub tokens, the GitHub Actions OIDC and runtime tokens, and cloud
// credentials, the AWS web identity, container (AWS_CONTAINER_*), and
// Bedrock forms included. A provider that authenticates only through one of
// them (Bedrock, Vertex AI) is therefore unavailable to band.
var bandProviderUnsetEnv = []string{
	"GH_TOKEN", "GITHUB_TOKEN", "GH_ENTERPRISE_TOKEN", "GITHUB_ENTERPRISE_TOKEN",
	"ACTIONS_ID_TOKEN_REQUEST_TOKEN", "ACTIONS_ID_TOKEN_REQUEST_URL", "ACTIONS_RUNTIME_TOKEN",
	"AWS_ACCESS_KEY_ID", "AWS_SECRET_ACCESS_KEY", "AWS_SESSION_TOKEN", "AWS_SECURITY_TOKEN",
	"AWS_WEB_IDENTITY_TOKEN_FILE", "AWS_CONTAINER_*", "AWS_BEARER_TOKEN_BEDROCK",
	"GOOGLE_APPLICATION_CREDENTIALS", "GOOGLE_OAUTH_ACCESS_TOKEN", "CLOUDSDK_AUTH_ACCESS_TOKEN",
	"AZURE_CLIENT_SECRET", "AZURE_CLIENT_CERTIFICATE_PASSWORD", "AZURE_STORAGE_KEY", "AZURE_STORAGE_CONNECTION_STRING",
	"ARM_CLIENT_SECRET", "ARM_ACCESS_KEY",
}

// bandEvidenceSource gathers the sanitized evidence of one diagnose claim:
// failed-step logs and existing react reports of its current block.
type bandEvidenceSource interface {
	Evidence(ctx context.Context, claim healthband.DueClaim) ([]healthband.RunLog, []healthband.ReactReport)
}

// bandDiagnoser executes the diagnose claims of one band run (phase B). Its
// Run method is the healthband.ClaimRunner of the run.
type bandDiagnoser struct {
	// projectDir is absolute: the BS directory, and the provider cwd while
	// health_band.allow_local_patch is off; a flag-on diagnosis runs in its
	// band worktree instead (SPEC-SIGMABAND-002).
	projectDir string
	harness    *config.HarnessConfig // nil selects no provider
	noAgent    bool
	evidence   bandEvidenceSource // nil gathers no evidence
	timeout    time.Duration      // provider call (REQ-11)
	now        func() time.Time
	bsOptions  brainstorm.Options
	warn       io.Writer // BS ID scan warnings; nil discards them
	// Seams: the shared read-only projection, the routed backends of an
	// execution config, and the single-provider runner.
	project  func([]orchestra.ProviderConfig, readOnlyPolicyOptions) ([]orchestra.ProviderConfig, error)
	backends func(orchestra.OrchestraConfig) map[string]orchestra.ExecutionBackend
	run      func(context.Context, orchestra.OrchestraConfig, orchestra.ProviderConfig, string) (*orchestra.ProviderResponse, error)
	// localPatch is the flag-on flow that enableLocalPatch switched on; nil
	// keeps every claim exactly as SPEC-SIGMABAND-001 runs it.
	localPatch *bandDiagnoseLocalPatch
}

func newBandDiagnoser(projectDir string, harness *config.HarnessConfig, noAgent bool, evidence bandEvidenceSource) *bandDiagnoser {
	return &bandDiagnoser{
		projectDir: projectDir, harness: harness, noAgent: noAgent, evidence: evidence,
		timeout: healthband.ProviderTimeout, now: time.Now,
		project: applyReadOnlyProviderPolicy, backends: ompProviderBackends, run: orchestra.RunSingleProvider,
	}
}

// bandDiagnosis is what the provider step produced for one claim.
type bandDiagnosis struct {
	provider string // selected provider; empty when none was selected
	status   string
	output   healthband.Evidence // sanitized provider output; empty unless ok
	manifest []promptlayer.ManifestEntry
}

// Run executes one diagnose claim: evidence, at most one read-only provider
// call, and the BS. A provider that is unavailable for any reason leaves an
// evidence-only BS and a done claim; only a BS that cannot be written fails
// the claim, with the BS writer's reason (bs_lock_timeout, bs_id_exhausted).
// While enableLocalPatch has switched the flag-on flow on, steps 1–2 of the
// Local Patch Flow come first and the provider is the confined claude of
// the Local Patch Provider Contract (SPEC-SIGMABAND-002).
func (d *bandDiagnoser) Run(ctx context.Context, claim healthband.DueClaim) healthband.ClaimOutcome {
	var logs []healthband.RunLog
	var reports []healthband.ReactReport
	if d.evidence != nil {
		logs, reports = d.evidence.Evidence(ctx, claim)
	}
	// --no-agent starts no provider, so a flag-on claim needs no worktree.
	if d.localPatch != nil && !d.noAgent {
		return d.localPatch.run(ctx, claim, logs, reports)
	}
	diagnosis := d.diagnose(ctx, claim.Event, logs, reports)
	return d.writeBS(ctx, d.bsRequest(claim, diagnosis, logs, reports), diagnosis.manifest)
}

// bsRequest is the BS of one claim and its diagnosis.
func (d *bandDiagnoser) bsRequest(claim healthband.DueClaim, diagnosis bandDiagnosis, logs []healthband.RunLog, reports []healthband.ReactReport) brainstorm.Request {
	return brainstorm.Request{
		Evaluation: claim.Event.Evaluation, EpisodeID: claim.EpisodeID, Created: d.now(),
		Provider: diagnosis.provider, DiagnosisStatus: diagnosis.status, Diagnosis: diagnosis.output,
		Logs: logs, Reports: reports,
	}
}

// writeBS writes the claim's BS and returns the claim outcome: the BS ID,
// or the BS writer's reason as the claim's failure.
func (d *bandDiagnoser) writeBS(ctx context.Context, req brainstorm.Request, manifest []promptlayer.ManifestEntry) healthband.ClaimOutcome {
	outcome := healthband.ClaimOutcome{DiagnosisStatus: req.DiagnosisStatus, PromptManifest: manifest}
	bsCtx, cancel := context.WithTimeout(ctx, healthband.BSWriteTimeout)
	defer cancel()
	result, err := brainstorm.Write(bsCtx, d.projectDir, req, d.bsOptions)
	if d.warn != nil {
		for _, path := range result.Ignored {
			fmt.Fprintf(d.warn, "react band: BS-BAND ID scan ignored %q (a symlink, not a regular file, or not a valid BS)\n", path)
		}
		for _, path := range result.RangeEnd {
			fmt.Fprintf(d.warn, "react band: BS-BAND ID range end is held by %q; the BS took the lowest free ID\n", path)
		}
	}
	if err != nil {
		reason := brainstorm.Reason(err)
		outcome.Status, outcome.BSStatus = healthband.ClaimFailedPrefix+reason, reason
		return outcome
	}
	outcome.BSID, outcome.BSStatus = result.ID, bandBSWritten
	return outcome
}

// diagnose selects, projects, checks, and runs one provider.
func (d *bandDiagnoser) diagnose(ctx context.Context, event healthband.Event, logs []healthband.RunLog, reports []healthband.ReactReport) bandDiagnosis {
	if d.noAgent {
		return bandDiagnosis{status: bandDiagnosisSkippedNoAgent}
	}
	name := selectBandProvider(d.harness)
	if name == "" {
		return bandDiagnosis{status: bandUnavailable(bandProviderUnconfigured)}
	}
	diagnosis := bandDiagnosis{provider: name}
	provider, reason := d.resolveProvider(name)
	if reason != "" {
		diagnosis.status = bandUnavailable(reason)
		return diagnosis
	}
	rendered, err := healthband.DiagnosisPrompt(event, logs, reports)
	if err != nil {
		diagnosis.status = bandUnavailable(bandPromptInvalid)
		return diagnosis
	}
	diagnosis.manifest = rendered.Manifest.Entries
	diagnosis.status, diagnosis.output = d.execute(ctx, provider, rendered.Prompt)
	return diagnosis
}

// selectBandProvider is the selection order of the Provider Read-Only
// Contract: health_band.diagnosis_provider (REQ-18), else orchestra.judge,
// else the lexicographically first configured provider name.
func selectBandProvider(harness *config.HarnessConfig) string {
	if harness == nil {
		return ""
	}
	for _, name := range []string{harness.HealthBand.DiagnosisProvider, harness.Orchestra.Judge} {
		if name = strings.TrimSpace(name); name != "" {
			return name
		}
	}
	names := make([]string, 0, len(harness.Orchestra.Providers))
	for name := range harness.Orchestra.Providers {
		names = append(names, name)
	}
	sort.Strings(names)
	if len(names) == 0 {
		return ""
	}
	return names[0]
}

// resolveProvider projects the selected provider through the shared
// read-only policy and refuses a projection without every required control.
// An unconfigured name is projected as the bare config orchestra commands
// use, so the policy alone decides whether the name is supported at all.
func (d *bandDiagnoser) resolveProvider(name string) (orchestra.ProviderConfig, string) {
	entry, configured := d.harness.Orchestra.Providers[name]
	provider := orchestra.ProviderConfig{Name: name, Binary: name, Args: []string{}}
	if configured {
		provider = providerConfigFromEntry(name, entry)
	}
	projected, err := d.project([]orchestra.ProviderConfig{provider}, readOnlyPolicyOptions{})
	var violation *readOnlyPolicyViolation
	switch {
	case errors.As(err, &violation) && violation.Kind == readOnlyUnsupportedProvider:
		return orchestra.ProviderConfig{}, bandProviderUnsupported
	case err != nil:
		return orchestra.ProviderConfig{}, bandProviderPolicyRejected
	case !configured:
		return orchestra.ProviderConfig{}, bandProviderUnconfigured
	case len(projected) != 1 || !bandReadOnlyControls(projected[0]):
		return orchestra.ProviderConfig{}, bandProviderPolicyIncomplete
	}
	return projected[0], ""
}

// execute runs the provider once in the project dir under the provider
// timeout and keeps only the sanitized head of its output.
func (d *bandDiagnoser) execute(ctx context.Context, provider orchestra.ProviderConfig, prompt string) (string, healthband.Evidence) {
	provider.WorkDir, provider.ExecutionTimeout = d.projectDir, d.timeout
	provider.UnsetEnv = bandProviderUnsetEnv
	// Bound the capture while the provider runs; one byte past the 1 MiB
	// head tells the capture below that bytes were dropped (size_cap).
	provider.MaxOutputBytes = healthband.ProviderCaptureBytes + 1
	cfg := orchestra.OrchestraConfig{
		Providers: []orchestra.ProviderConfig{provider}, TimeoutSeconds: int((d.timeout + time.Second - 1) / time.Second),
		WorkingDir: d.projectDir, ProviderWorkDir: d.projectDir, ReadOnly: true,
	}
	cfg.ProviderBackends = d.backends(cfg)
	response, err := d.run(ctx, cfg, provider, prompt)
	if reason := bandRunFailure(response, err); reason != "" {
		return bandUnavailable(reason), healthband.Evidence{}
	}
	capture := healthband.NewHeadBuffer(healthband.ProviderCaptureBytes)
	_, _ = io.WriteString(capture, response.Output)
	text, dropped := capture.Captured()
	output := healthband.SanitizeProviderOutput(text, dropped, d.projectDir)
	if strings.TrimSpace(output.Text) == "" {
		return bandUnavailable(bandProviderEmptyOutput), healthband.Evidence{}
	}
	return bandDiagnosisOK, output
}

// bandRunFailure maps a failed provider run to its REQ-12 reason; a run
// that produced output returns "".
func bandRunFailure(response *orchestra.ProviderResponse, err error) string {
	switch {
	case errors.Is(err, orchestra.ErrBackendUnavailable):
		return bandProviderBackendUnavailable
	case errors.Is(err, exec.ErrNotFound):
		return bandProviderMissing
	case response != nil && response.TimedOut, errors.Is(err, context.DeadlineExceeded):
		return bandProviderTimedOut
	case err != nil, response == nil, response.ExitCode != 0:
		return bandProviderExitNonzero
	}
	return ""
}
