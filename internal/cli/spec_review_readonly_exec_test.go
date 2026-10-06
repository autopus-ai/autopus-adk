package cli

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/insajin/autopus-adk/pkg/config"
	"github.com/insajin/autopus-adk/pkg/orchestra"
)

// executeSpecReviewProvider runs one assembled provider through the real
// subprocess backend and returns the argv it executed.
func executeSpecReviewProvider(t *testing.T, provider orchestra.ProviderConfig, schemaPath string) []string {
	t.Helper()
	resp, err := orchestra.NewSubprocessBackendImpl().Execute(context.Background(), orchestra.ProviderRequest{
		Provider: provider.Name, Prompt: "Review SPEC-X", Config: provider, SchemaPath: schemaPath, Timeout: 10 * time.Second,
	})
	require.NoError(t, err)
	require.NotNil(t, resp.Execution)
	return resp.Execution.Command
}

func assertNoWideningArgv(t *testing.T, command []string) {
	t.Helper()
	for index, item := range command {
		for _, forbidden := range []string{"workspace-write", "--dangerously", "bypass", "--allowedTools", "--mcp-config"} {
			assert.NotContains(t, item, forbidden, "argv %v", command)
		}
		if item == "--tools" || (strings.HasPrefix(item, "--tools=") && item != "--tools=Read,Grep,Glob") {
			t.Errorf("argv %v carries a --tools item other than --tools=Read,Grep,Glob at %d", command, index)
		}
	}
}

// S1 (REQ-01, REQ-19): each reviewer executes exactly its projected Args plus
// only the runtime items of the Execution Boundary Contract.
func TestAssembleSpecReviewProviders_ExecutesProjectedArgvPlusRuntimeItems(t *testing.T) {
	evidence := installReadOnlyArgvRecorders(t, "claude", "codex", "agy")
	countCodexCatalogProbes(t)

	set, err := assembleSpecReviewProviders(context.Background(), specReviewProviderRequest{Config: fDefaultSpecReviewConfig(), RequestedTimeout: 30})

	require.NoError(t, err)
	require.Equal(t, []string{"claude", "codex", "gemini"}, providerConfigNames(set.Providers))
	assert.Equal(t, []string{"claude", "codex", "gemini"}, set.Names)
	assert.Empty(t, set.Excluded)
	assert.Equal(t, map[string]string{"claude": "read-only", "codex": "read-only", "gemini": "unverified"},
		sandboxModesByName(set.Providers))
	for _, provider := range set.Providers {
		assert.Equal(t, 30*time.Second, provider.ExecutionTimeout, provider.Name)
		assert.Contains(t, provider.ResultReadyPatterns, "VERDICT:", provider.Name)
	}

	schemaPath := filepath.Join(t.TempDir(), "schema.json")
	claude := executeSpecReviewProvider(t, set.Providers[0], schemaPath)
	assert.Equal(t, append([]string{"claude"}, withClaudeReadOnlySuffix("--print", "--model", "claude-fable-5-1", "--effort", "max")...), claude)
	stdin, err := os.ReadFile(filepath.Join(evidence, "claude.stdin"))
	require.NoError(t, err)
	assert.Equal(t, "Review SPEC-X", string(stdin), "claude receives the prompt on stdin")

	codex := executeSpecReviewProvider(t, set.Providers[1], schemaPath)
	codexHead := []string{
		"codex", "exec", "--json", "--sandbox", "read-only", "-m", "gpt-5.6-sol", "-c", `model_reasoning_effort="max"`,
		"--ephemeral", "--ignore-user-config", "--ignore-rules",
	}
	require.Len(t, codex, len(codexHead)+4, "codex argv %v", codex)
	assert.Equal(t, codexHead, codex[:len(codexHead)])
	assert.Equal(t, []string{"--output-schema", schemaPath, "--output-last-message"}, codex[len(codexHead):len(codexHead)+3])

	gemini := executeSpecReviewProvider(t, set.Providers[2], schemaPath)
	assert.Equal(t, []string{"agy", "--print", "Review SPEC-X", "--mode", "plan", "--sandbox", "--disable-slash-commands"}, gemini)

	for name, command := range map[string][]string{"claude": claude, "codex": codex, "agy": gemini} {
		assert.Equal(t, command[1:], readRecordedArgv(t, evidence, name), "%s recorded argv", name)
		assertNoWideningArgv(t, command)
	}
}

// S1 discriminator: a quality codex with empty args gets "exec --sandbox
// workspace-write" seeded by the capability step; the projection runs after
// it, so exactly one sandbox item, "--sandbox read-only", executes.
func TestAssembleSpecReviewProviders_SeededCodexSandboxIsProjected(t *testing.T) {
	installReadOnlyArgvRecorders(t, "codex")
	installRuntimeCodexCatalogFixture(t)
	cfg := fDefaultSpecReviewConfig()
	cfg.Orchestra.Providers["codex"] = config.ProviderEntry{
		Binary: "codex", ModelPolicy: config.ProviderModelPolicyQuality, Subprocess: config.SubprocessProvConf{SchemaFlag: "--output-schema"},
	}

	set, err := assembleSpecReviewProviders(context.Background(), specReviewProviderRequest{Config: cfg, FlagProviders: []string{"codex"}})
	require.NoError(t, err)
	require.Len(t, set.Providers, 1)
	command := executeSpecReviewProvider(t, set.Providers[0], "")

	var sandboxItems []string
	for index, item := range command {
		if item == "--sandbox" || item == "-s" || strings.HasPrefix(item, "--sandbox=") || strings.HasPrefix(item, "-s=") {
			sandboxItems = append(sandboxItems, strings.Join(command[index:min(index+2, len(command))], " "))
		}
	}
	assert.Equal(t, []string{"--sandbox read-only"}, sandboxItems, "argv %v", command)
	assert.Equal(t, "exec", command[1])
	assertNoWideningArgv(t, command)
}

// REQ-01: OMP-backed providers keep the accept-as-read-only path; their
// config argv is not rewritten and the OMP review backend enforces tools.
func TestAssembleSpecReviewProviders_OMPProviderKeepsReviewBackendPath(t *testing.T) {
	installReadOnlyArgvRecorders(t, "omp")
	cfg := fDefaultSpecReviewConfig()
	cfg.Orchestra.Providers["claude"] = config.ProviderEntry{Backend: config.ProviderBackendOMP, Model: "anthropic/claude-opus-5-5:max"}

	set, err := assembleSpecReviewProviders(context.Background(), specReviewProviderRequest{Config: cfg, FlagProviders: []string{"claude"}})

	require.NoError(t, err)
	require.Len(t, set.Providers, 1)
	assert.Equal(t, config.ProviderBackendOMP, set.Providers[0].Backend)
	assert.Empty(t, set.Providers[0].Args)
	assert.Equal(t, orchestra.SandboxModeReadOnly, set.Providers[0].SandboxMode)
}

// Without a harness config the built-in provider registry is gated and
// projected the same way, as resolveSpecReviewProviderConfigs resolves it.
func TestAssembleSpecReviewProviders_WithoutConfigProjectsBuiltInProviders(t *testing.T) {
	installReadOnlyArgvRecorders(t, "claude")

	set, err := assembleSpecReviewProviders(context.Background(), specReviewProviderRequest{FlagProviders: []string{"claude"}})

	require.NoError(t, err)
	require.Len(t, set.Providers, 1)
	args := set.Providers[0].Args
	assert.Equal(t, "--print", args[0])
	assert.Equal(t, claudeReadOnlySuffix, args[len(args)-len(claudeReadOnlySuffix):])
	assert.Equal(t, orchestra.SandboxModeReadOnly, set.Providers[0].SandboxMode)
}

// S3: the judge gets the same gate and projection on both resolution paths.
func TestAssembleSpecReviewJudge_ProjectsBothResolutionPaths(t *testing.T) {
	installReadOnlyArgvRecorders(t, "claude", "codex", "agy")
	probes := countCodexCatalogProbes(t)
	pClaude := withClaudeReadOnlySuffix("--print", "--model", "claude-fable-5-1", "--effort", "max")
	cfg := fDefaultSpecReviewConfig()

	// Path A: the judge reuses the projected claude reviewer.
	reviewers, err := assembleSpecReviewProviders(context.Background(), specReviewProviderRequest{Config: cfg})
	require.NoError(t, err)
	judge, err := assembleSpecReviewJudge(context.Background(), cfg, reviewers.Providers, "claude", 0)
	require.NoError(t, err)
	require.NotNil(t, judge)
	assert.Equal(t, pClaude, judge.Args)
	assert.Equal(t, orchestra.SandboxModeReadOnly, judge.SandboxMode)

	// Path B: --providers codex, so the claude judge resolves separately.
	reviewers, err = assembleSpecReviewProviders(context.Background(), specReviewProviderRequest{Config: cfg, FlagProviders: []string{"codex"}})
	require.NoError(t, err)
	judge, err = assembleSpecReviewJudge(context.Background(), cfg, reviewers.Providers, "claude", 45)
	require.NoError(t, err)
	require.NotNil(t, judge)
	assert.Equal(t, pClaude, judge.Args)
	assert.Equal(t, orchestra.SandboxModeReadOnly, judge.SandboxMode)
	assert.Equal(t, 45*time.Second, judge.ExecutionTimeout)
	assert.Contains(t, judge.ResultReadyPatterns, "VERDICT:")

	// Path B with a widening judge entry fails with the S6 claude message.
	editProvider(cfg, "claude", func(e *config.ProviderEntry) { e.Args = []string{"--print", "--verbose"} })
	judge, err = assembleSpecReviewJudge(context.Background(), cfg, reviewers.Providers, "claude", 0)
	require.Error(t, err)
	assert.Nil(t, judge)
	assert.Equal(t, `spec review: provider "claude" rejected by the read-only policy: contains unsupported argv "--verbose" (config key: orchestra.providers.claude.args; remedy: remove "--verbose" from orchestra.providers.claude.args)`, err.Error())
	assert.Zero(t, probes.Load())
}

// S8 / REQ-06: an unsupported judge is an explicit selection and fails closed
// with spec.review_gate.judge as the config key; no judge means none.
func TestAssembleSpecReviewJudge_UnsupportedJudgeNamesGateKey(t *testing.T) {
	t.Parallel()

	judge, err := assembleSpecReviewJudge(context.Background(), fDefaultSpecReviewConfig(), nil, "opencode", 0)
	require.Error(t, err)
	assert.Nil(t, judge)
	assert.Equal(t, `spec review: provider "opencode" rejected by the read-only policy: unsupported provider "opencode" (config key: spec.review_gate.judge; remedy: remove "opencode" from spec.review_gate.judge)`, err.Error())

	judge, err = assembleSpecReviewJudge(context.Background(), fDefaultSpecReviewConfig(), nil, "", 0)
	require.NoError(t, err)
	assert.Nil(t, judge)

	judge, err = assembleSpecReviewJudge(context.Background(), nil, nil, "claude", 0)
	require.NoError(t, err)
	assert.Nil(t, judge, "without config a judge that is not a reviewer cannot be resolved")
}

// INV-13: a separately resolved judge is gated before the codex capability
// step, so a rejected wrapper judge never reaches the "debug models" probe.
func TestAssembleSpecReviewJudge_RejectedCodexJudgeNeverProbesCatalog(t *testing.T) {
	probes := countCodexCatalogProbes(t)
	cfg := fDefaultSpecReviewConfig()
	editProvider(cfg, "codex", func(e *config.ProviderEntry) {
		e.Binary, e.ModelPolicy = filepath.Join(t.TempDir(), "codex-wrapper"), config.ProviderModelPolicyQuality
	})

	judge, err := assembleSpecReviewJudge(context.Background(), cfg, nil, "codex", 0)

	require.Error(t, err)
	assert.Nil(t, judge)
	assert.Equal(t, `spec review: provider "codex" rejected by the read-only policy: requires native binary "codex" (config key: orchestra.providers.codex.binary; remedy: set orchestra.providers.codex.binary to "codex")`, err.Error())
	assert.Zero(t, probes.Load())
}
