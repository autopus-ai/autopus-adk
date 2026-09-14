package omp

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/insajin/autopus-adk/pkg/config"
)

func TestOMPModelIntegration_S12S14_FailsClosedBeforeProjectWrite(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name   string
		setup  func(*modelIntegrationFakeRunner, *config.HarnessConfig)
		reason string
	}{
		{"unsupported setting", func(r *modelIntegrationFakeRunner, _ *config.HarnessConfig) {
			r.unsupported = map[string]bool{config.OMPNativeAgentModelOverridesKey: true}
		}, "model_setting_unsupported"},
		{"metadata insufficient", func(r *modelIntegrationFakeRunner, _ *config.HarnessConfig) {
			r.catalog = []byte(`{"models":[{"provider":"p","id":"m","thinking":["high"]}]}`)
		}, "model_catalog_unavailable"},
		{"unresolved required route", func(_ *modelIntegrationFakeRunner, cfg *config.HarnessConfig) {
			profile := cfg.RoleModelPolicy.Profiles["p1"]
			route := profile.Capabilities["deep_reasoning"]
			route.Candidates[0].Selector = "anthropic/missing"
			profile.Capabilities["deep_reasoning"] = route
			cfg.RoleModelPolicy.Profiles["p1"] = profile
		}, "required_route_unresolved"},
		{"agent override mismatch", func(_ *modelIntegrationFakeRunner, cfg *config.HarnessConfig) {
			profile := cfg.RoleModelPolicy.Profiles["p1"]
			profile.Agents = map[string]config.RoleAgentOverrideConf{
				"executor": {Role: config.OMPAgentRoleName("reviewer"), Capability: config.CapabilityIndependentDissent},
			}
			cfg.RoleModelPolicy.Profiles["p1"] = profile
		}, "role_capability_mismatch"},
		{"unsupported safety", func(r *modelIntegrationFakeRunner, cfg *config.HarnessConfig) {
			profile := cfg.RoleModelPolicy.Profiles["p1"]
			profile.Safety.ApprovalMode = "write"
			cfg.RoleModelPolicy.Profiles["p1"] = profile
			r.unsupported = map[string]bool{"tools.approvalMode": true}
		}, "model_setting_unsupported"},
		{"activation readback mismatch", func(r *modelIntegrationFakeRunner, _ *config.HarnessConfig) {
			r.corruptKey = config.OMPNativeAgentModelOverridesKey
		}, "activation readback mismatch"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			root := t.TempDir()
			runner := newModelIntegrationRunner()
			cfg := integrationHarnessConfig("overlay")
			tc.setup(runner, cfg)
			_, err := NewWithRoot(root).WithModelIntegrationRunner(runner).
				Generate(context.Background(), cfg)
			if err == nil || !strings.Contains(err.Error(), tc.reason) {
				t.Fatalf("Generate() error = %v, want %q", err, tc.reason)
			}
			entries, readErr := os.ReadDir(root)
			if readErr != nil || len(entries) != 0 {
				t.Fatalf("pre-write failure changed project: entries=%v err=%v", entries, readErr)
			}
		})
	}
}

func TestOMPModelIntegration_S13_SafetyIsExplicitAndCapabilityGated(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	cfg := integrationHarnessConfig("overlay")
	profile := cfg.RoleModelPolicy.Profiles["p1"]
	profile.Safety.ApprovalMode = "write"
	profile.Safety.IsolationMode = "auto"
	cfg.RoleModelPolicy.Profiles["p1"] = profile
	runner := newModelIntegrationRunner()
	files, err := NewWithRoot(root).WithModelIntegrationRunner(runner).
		prepareFiles(context.Background(), cfg)
	if err != nil {
		t.Fatalf("prepare safety overlay: %v", err)
	}
	overlay := string(integrationMappingsByPath(files)[DefaultOMPModelOverlayPath].Content)
	for _, value := range []string{"approvalMode: write", "mode: auto"} {
		if !strings.Contains(overlay, value) {
			t.Fatalf("explicit safety %q missing:\n%s", value, overlay)
		}
	}
	plain, err := NewWithRoot(t.TempDir()).WithModelIntegrationRunner(newModelIntegrationRunner()).
		prepareFiles(context.Background(), integrationHarnessConfig("overlay"))
	if err != nil {
		t.Fatal(err)
	}
	plainOverlay := string(integrationMappingsByPath(plain)[DefaultOMPModelOverlayPath].Content)
	if strings.Contains(plainOverlay, "approvalMode") || strings.Contains(plainOverlay, "isolation") {
		t.Fatalf("implicit safety keys were emitted:\n%s", plainOverlay)
	}
}

func TestOMPModelIntegrationExecRunner_RejectsUnsafeArgsBeforeExecution(t *testing.T) {
	t.Parallel()

	runner := ompModelIntegrationExecRunner{}
	if _, err := runner.Run(context.Background(), os.Args[0], "models"); err == nil ||
		!strings.Contains(err.Error(), "unsafe OMP model metadata command") {
		t.Fatalf("unsafe command error = %v", err)
	}
	if _, err := runner.Run(context.Background(), os.Args[0], "--version"); err == nil {
		t.Fatal("test executable unexpectedly accepted --version")
	}
}

func assertNoIntegrationArtifacts(t *testing.T, root string) {
	t.Helper()
	for _, path := range []string{DefaultOMPModelOverlayPath, OMPModelReceiptRelativePath,
		filepath.Join(".autopus", "omp-manifest.json")} {
		if _, err := os.Stat(filepath.Join(root, path)); !os.IsNotExist(err) {
			t.Fatalf("unexpected artifact %q: %v", path, err)
		}
	}
}
