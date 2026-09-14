package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/insajin/autopus-adk/pkg/adapter/omp"
	"github.com/insajin/autopus-adk/pkg/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
)

func TestOMPModelDoctor_GeneratedOverlayAndProjectSourcesDetectMutation(t *testing.T) {
	t.Parallel()

	for _, mode := range []string{
		config.RoleModelConfigModeOverlay,
		config.RoleModelConfigModeProjectManaged,
	} {
		mode := mode
		t.Run(mode, func(t *testing.T) {
			root := t.TempDir()
			cfg := ompModelDoctorE2EConfig(mode)
			if mode == config.RoleModelConfigModeProjectManaged {
				path := filepath.Join(root, ".omp", "config.yml")
				require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o755))
				require.NoError(t, os.WriteFile(path, []byte("credential: ${SECRET_REF}\nuser: keep\n"), 0o600))
			}
			runner := &ompModelDoctorE2ERunner{}
			_, err := omp.NewWithRoot(root).WithModelIntegrationRunner(runner).
				Generate(context.Background(), cfg)
			require.NoError(t, err)

			input := buildOMPModelDoctorInput(context.Background(), root, cfg, runner)
			report := omp.CheckOMPModelRoutingDoctor(input)
			assert.Equal(t, "supported", report.Status)
			assert.Equal(t, "fresh", report.Reason)
			assert.Equal(t, mode, input.ConfigSource)

			configPath := filepath.Join(root, filepath.FromSlash(omp.DefaultOMPModelOverlayPath))
			if mode == config.RoleModelConfigModeProjectManaged {
				configPath = filepath.Join(root, ".omp", "config.yml")
			}
			data, err := os.ReadFile(configPath)
			require.NoError(t, err)
			mutated := strings.Replace(string(data), "modelFallback: false", "modelFallback: true", 1)
			require.NotEqual(t, string(data), mutated)
			require.NoError(t, os.WriteFile(configPath, []byte(mutated), 0o600))

			input = buildOMPModelDoctorInput(context.Background(), root, cfg, runner)
			report = omp.CheckOMPModelRoutingDoctor(input)
			assert.Equal(t, "blocked", report.Status)
			assert.Equal(t, "projection_mismatch", report.Reason)
		})
	}
}

// TestOMPModelDoctor_S7_EmitsOneRoleCheckPerAgent fixes SPEC-OMP-005 S7: a
// generated ultra workspace yields sixteen agent-keyed doctor role checks.
func TestOMPModelDoctor_S7_EmitsOneRoleCheckPerAgent(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	cfg := config.DefaultFullConfig("doctor-ultra")
	cfg.Platforms = []string{"omp"}
	cfg.Quality.Default = "ultra"
	cfg.RoleModelPolicy = config.RoleModelPolicyConf{
		Version: config.RoleModelPolicyVersionV1, Profile: "ultra", Family: "anthropic",
	}
	runner := &ompModelDoctorE2ERunner{catalog: ompModelDoctorPresetCatalogJSON()}
	_, err := omp.NewWithRoot(root).WithModelIntegrationRunner(runner).
		Generate(context.Background(), cfg)
	require.NoError(t, err)

	input := buildOMPModelDoctorInput(context.Background(), root, cfg, runner)
	report := omp.CheckOMPModelRoutingDoctor(input)
	require.Equal(t, "supported", report.Status)
	require.Equal(t, "fresh", report.Reason)
	require.Len(t, report.Roles, len(config.OMPNativeAgentNames()))
	for _, row := range report.Roles {
		policy, resolveErr := config.ResolveOMPPolicyAgent(row.Agent)
		require.NoError(t, resolveErr, row.Agent)
		assert.Equal(t, policy.Role, row.Role, row.Agent)
		assert.Equal(t, "operator_attested", row.EvidenceClass, row.Agent)
	}

	details := make([]string, 0, len(report.Roles))
	for _, check := range projectOMPModelRoutingDoctorChecks(report) {
		if strings.HasPrefix(check.ID, "doctor.platform.omp.model-routing.role.") {
			details = append(details, check.Detail)
		}
	}
	assert.Len(t, details, 5)
	joined := strings.Join(details, "\n")
	assert.Contains(t, joined, "agent=task role=autopus_planner capability=deep_reasoning status=supported")
	assert.Contains(t, joined, "agent=scout role=autopus_explorer capability=fast_validation status=supported")
	assert.Contains(t, joined, "agent=reviewer role=autopus_reviewer capability=independent_dissent status=supported reason=selected family_diversity=satisfied")
	// No row may name an agent OMP does not register.
	for _, retired := range []string{"agent=executor ", "agent=debugger ", "agent=planner ", "agent=validator "} {
		assert.NotContains(t, joined, retired)
	}
}

type ompModelDoctorE2ERunner struct {
	catalog []byte
}

func (runner *ompModelDoctorE2ERunner) Run(_ context.Context, _ string, args ...string) ([]byte, error) {
	joined := strings.Join(args, " ")
	if joined == "--version" {
		return []byte("omp/17.2.6\n"), nil
	}
	if joined == "models --json --no-extensions" {
		if runner.catalog != nil {
			return append([]byte(nil), runner.catalog...), nil
		}
		return []byte(`{"models":[{"provider":"acme","id":"model","family":"acme","capabilities":["coding_tool_use","deep_reasoning","deterministic_transform","fast_validation","independent_dissent","vision_design"],"thinking":["high"],"auth_enabled":true}]}`), nil
	}
	key := ompModelDoctorConfigGetKey(args)
	if key == "" {
		return nil, fmt.Errorf("unexpected OMP doctor args: %v", args)
	}
	value := any(map[string]any{})
	if len(args) > 1 && args[0] == "--config" {
		data, err := os.ReadFile(args[1])
		if err != nil {
			return nil, err
		}
		value, err = ompModelDoctorYAMLPath(data, key)
		if err != nil {
			return nil, err
		}
	}
	return json.Marshal(map[string]any{"key": key, "value": value})
}

func ompModelDoctorConfigGetKey(args []string) string {
	for index := 0; index+3 < len(args); index++ {
		if args[index] == "config" && args[index+1] == "get" && args[index+3] == "--json" {
			return args[index+2]
		}
	}
	return ""
}

func ompModelDoctorYAMLPath(data []byte, path string) (any, error) {
	var root map[string]any
	if err := yaml.Unmarshal(data, &root); err != nil {
		return nil, err
	}
	var current any = root
	for _, segment := range strings.Split(path, ".") {
		mapping, ok := current.(map[string]any)
		if !ok {
			return nil, fmt.Errorf("%s parent is not a mapping", path)
		}
		current, ok = mapping[segment]
		if !ok {
			return nil, fmt.Errorf("%s missing", path)
		}
	}
	return current, nil
}

func ompModelDoctorE2EConfig(mode string) *config.HarnessConfig {
	cfg := ompDoctorPolicyConfig("acme")
	cfg.Platforms = []string{"omp"}
	profile := cfg.RoleModelPolicy.Profiles["balanced"]
	profile.ConfigMode = mode
	profile.FamilyDiversity = config.FamilyDiversityPolicyConf{
		Enabled: true,
		Roles:   []string{config.OMPAgentRoleName("reviewer"), config.OMPAgentRoleName("security-auditor")},
	}
	if mode == config.RoleModelConfigModeProjectManaged {
		missing := omp.OMPMissingManagedValueFingerprint()
		profile.ManagedKeys = map[string]config.RoleManagedKeyClaimConf{
			config.OMPNativeAgentModelOverridesKey: {PriorFingerprint: missing, Complete: true},
			"retry.fallbackChains": {
				PriorFingerprint: missing, Complete: true, FullArrayOwnership: true,
			},
			"retry.modelFallback": {PriorFingerprint: missing, Complete: true},
		}
	}
	cfg.RoleModelPolicy.Profiles["balanced"] = profile
	return cfg
}

// ompModelDoctorPresetCatalogJSON mirrors the metadata-light OMP catalog the
// built-in profiles attest: four Claude rungs and four Codex rungs.
func ompModelDoctorPresetCatalogJSON() []byte {
	rungs := []struct{ provider, model, thinking string }{
		{"anthropic", config.ClaudeFableModel, "max"},
		{"anthropic", config.ClaudeOpusModel, "xhigh"},
		{"anthropic", config.ClaudeSonnetModel, "medium"},
		{"anthropic", config.ClaudeHaikuModel, "low"},
		{"openai-codex", config.CodexAstraModel, "max"},
		{"openai-codex", config.CodexSolModel, "xhigh"},
		{"openai-codex", config.CodexTerraModel, "medium"},
		{"openai-codex", config.CodexLunaModel, "low"},
	}
	entries := make([]string, 0, len(rungs))
	for _, rung := range rungs {
		entries = append(entries, fmt.Sprintf(
			`{"provider":%q,"id":%q,"selector":%q,"thinking":[%q]}`,
			rung.provider, rung.model, rung.provider+"/"+rung.model, rung.thinking,
		))
	}
	return []byte(`{"models":[` + strings.Join(entries, ",") + `]}`)
}
