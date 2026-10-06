package cli

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/insajin/autopus-adk/pkg/config"
	"github.com/insajin/autopus-adk/pkg/orchestra"
	"github.com/insajin/autopus-adk/pkg/spec"
)

// stubSpecReviewAssembly replaces the read-only reviewer assembly with a fixed
// reviewer set for the selected names, as the retired specReviewConfigProviders
// stubs did: the set skips the gate, installed filter, and projection but gets
// the capability, configure, and timeout steps. Readiness stays off the host.
func stubSpecReviewAssembly(t *testing.T, build func(cfg *config.HarnessConfig, names []string) []orchestra.ProviderConfig) {
	t.Helper()
	original := specReviewProviderAssembly
	specReviewProviderAssembly = func(ctx context.Context, req specReviewProviderRequest) (specReviewProviderSet, error) {
		names, _ := selectSpecReviewProviderNames(req.Config, req.FlagProviders, req.Multi)
		providers := configureSpecReviewProviders(resolveCodexProviderCapabilities(ctx, build(req.Config, names)))
		return specReviewProviderSet{Names: names, Providers: applySpecReviewExecutionTimeout(providers, req.RequestedTimeout)}, nil
	}
	t.Cleanup(func() { specReviewProviderAssembly = original })
}

// Unit tests never run host provider CLIs: in this test binary the spec review
// preflight and the doctor readiness checks report every provider skipped
// unless a test opts into classification with useReadinessRunner.
func init() {
	reviewReadinessProbe = func(ctx context.Context, providers []orchestra.ProviderConfig, _ providerReadinessOptions) []providerReadinessResult {
		return probeProviderReadiness(ctx, providers, providerReadinessOptions{Skip: true})
	}
}

// useReadinessRunner routes readiness through the real classification with a
// scripted runner seam; such tests stay serial.
func useReadinessRunner(t *testing.T, reply readinessReply) *readinessRunnerSpy {
	t.Helper()
	original := reviewReadinessProbe
	reviewReadinessProbe = probeProviderReadiness
	t.Cleanup(func() { reviewReadinessProbe = original })
	return installReadinessRunner(t, reply)
}

// useHermeticReadiness classifies with a runner that cannot start any probe:
// unknown(probe_failed), which never blocks a review.
func useHermeticReadiness(t *testing.T) *readinessRunnerSpy {
	t.Helper()
	return useReadinessRunner(t, func(context.Context, providerReadinessCommand) (providerReadinessProcess, error) {
		return providerReadinessProcess{}, errors.New("readiness probes are disabled in this test")
	})
}

// replyByExecutable answers each status probe by the executable it runs.
func replyByExecutable(replies map[string]readinessReply) readinessReply {
	return func(ctx context.Context, command providerReadinessCommand) (providerReadinessProcess, error) {
		if reply, ok := replies[filepath.Base(command.Argv[0])]; ok {
			return reply(ctx, command)
		}
		return providerReadinessProcess{}, errors.New("no scripted readiness reply")
	}
}

// clearProviderCredentialEnv hides host credentials so status classification
// depends only on the scripted probe output.
func clearProviderCredentialEnv(t *testing.T) {
	t.Helper()
	for _, key := range append(append([]string{"PI_CODING_AGENT_DIR"}, claudeCredentialEnv...), codexCredentialEnv...) {
		t.Setenv(key, "")
	}
}

// recordingReviewBackend is the fake model backend: reviewers return PASS,
// the judge returns a PASS decision, and every request is recorded. Like the
// subprocess backend it records the launch of every CLI provider; the OMP
// review backend records none.
type recordingReviewBackend struct {
	mu        sync.Mutex
	requests  []orchestra.ProviderRequest
	execution map[string]*orchestra.ProviderExecution
	// replies scripts the answer to "<provider>/<role>".
	replies map[string]recordedReviewReply
}

// recordedReviewReply is a scripted answer; a nil response with an error is a
// provider that failed before any process started.
type recordedReviewReply struct {
	resp *orchestra.ProviderResponse
	err  error
}

func (b *recordingReviewBackend) Execute(_ context.Context, req orchestra.ProviderRequest) (*orchestra.ProviderResponse, error) {
	b.mu.Lock()
	b.requests = append(b.requests, req)
	b.mu.Unlock()
	if reply, ok := b.replies[req.Provider+"/"+req.Role]; ok {
		return reply.resp, reply.err
	}
	output := `{"verdict":"PASS","summary":"ok","findings":[]}`
	if req.Role == "judge" {
		output = `{"verdict":"PASS","findings":[],"rationale":"ok"}`
	}
	execution := b.execution[req.Provider]
	if execution == nil && req.Config.Backend != config.ProviderBackendOMP {
		execution = &orchestra.ProviderExecution{Command: append([]string{req.Config.Binary}, req.Config.Args...)}
	}
	return &orchestra.ProviderResponse{
		Provider: req.Provider, Output: output, Duration: time.Millisecond, Execution: execution,
	}, nil
}

func (b *recordingReviewBackend) Name() string { return "fake" }

// calls counts requests by provider and role.
func (b *recordingReviewBackend) calls(provider, role string) int {
	b.mu.Lock()
	defer b.mu.Unlock()
	count := 0
	for _, req := range b.requests {
		if req.Provider == provider && req.Role == role {
			count++
		}
	}
	return count
}

func (b *recordingReviewBackend) total() int {
	b.mu.Lock()
	defer b.mu.Unlock()
	return len(b.requests)
}

// request returns the first recorded request of provider in role.
func (b *recordingReviewBackend) request(t *testing.T, provider, role string) orchestra.ProviderRequest {
	t.Helper()
	b.mu.Lock()
	defer b.mu.Unlock()
	for _, req := range b.requests {
		if req.Provider == provider && req.Role == role {
			return req
		}
	}
	t.Fatalf("no %s request for %s", role, provider)
	return orchestra.ProviderRequest{}
}

// readOnlyReviewFixture is a SPEC review project on fixture F-default with
// judge claude, one review round, and recorder binaries for the native CLIs.
type readOnlyReviewFixture struct {
	root, specID, specDir string
	evidence              string
	catalogProbes         *atomic.Int32
}

func newReadOnlyReviewFixture(t *testing.T, edit func(*config.HarnessConfig)) *readOnlyReviewFixture {
	t.Helper()
	root := t.TempDir()
	specID := "SPEC-REVIEWRO-WIRING-001"
	specDir := scaffoldReviewSpec(t, root, specID)
	cfg := config.DefaultFullConfig("reviewro-wiring")
	fDefault := fDefaultSpecReviewConfig()
	cfg.Spec.ReviewGate.Providers = fDefault.Spec.ReviewGate.Providers
	cfg.Spec.ReviewGate.Judge = "claude"
	cfg.Spec.ReviewGate.MaxRevisions = new(0)
	cfg.Spec.ReviewGate.AutoCollectContext = false
	cfg.Orchestra.Providers = fDefault.Orchestra.Providers
	if edit != nil {
		edit(cfg)
	}
	require.NoError(t, config.Save(root, cfg))
	evidence := installReviewJSONRecorders(t, "claude", "codex", "agy")
	probes := countCodexCatalogProbes(t)
	clearProviderCredentialEnv(t)
	chdirForTest(t, root)
	return &readOnlyReviewFixture{root: root, specID: specID, specDir: specDir, evidence: evidence, catalogProbes: probes}
}

// useFakeBackend routes every provider request to the recording fake.
func (fixture *readOnlyReviewFixture) useFakeBackend(t *testing.T) *recordingReviewBackend {
	t.Helper()
	backend := &recordingReviewBackend{}
	withSpecReviewBackend(t, backend)
	return backend
}

func readSpecReviewReceipt(t *testing.T, specDir string) specReviewPromotionReceipt {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(specDir, "review-receipt.json"))
	require.NoError(t, err)
	var receipt specReviewPromotionReceipt
	require.NoError(t, json.Unmarshal(data, &receipt))
	return receipt
}

// specReviewVerdictLine returns the review.md verdict line.
func specReviewVerdictLine(t *testing.T, specDir string) string {
	t.Helper()
	for _, line := range strings.Split(readReviewMd(t, specDir), "\n") {
		if strings.HasPrefix(line, "**Verdict**:") {
			return line
		}
	}
	t.Fatalf("review.md has no verdict line")
	return ""
}

func providerHealthRows(statuses []spec.ProviderStatus) []string {
	rows := make([]string, 0, len(statuses))
	for _, status := range statuses {
		rows = append(rows, status.Provider+" "+status.Status+" "+status.Note)
	}
	return rows
}
