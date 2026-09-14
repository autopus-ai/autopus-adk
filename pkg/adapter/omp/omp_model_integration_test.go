package omp

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/insajin/autopus-adk/pkg/adapter"
	"github.com/insajin/autopus-adk/pkg/config"
	"gopkg.in/yaml.v3"
)

type modelIntegrationFakeRunner struct {
	catalog       []byte
	calls         [][]string
	unsupported   map[string]bool
	corruptKey    string
	modelRequests int
}

func (r *modelIntegrationFakeRunner) Run(_ context.Context, _ string, args ...string) ([]byte, error) {
	r.calls = append(r.calls, append([]string(nil), args...))
	joined := strings.Join(args, " ")
	if joined == "--version" {
		return []byte("omp/17.2.6\n"), nil
	}
	if joined == "models --json --no-extensions" {
		return append([]byte(nil), r.catalog...), nil
	}
	if strings.Contains(joined, " prompt") || strings.Contains(joined, " agent") {
		r.modelRequests++
		return nil, fmt.Errorf("model requests are forbidden")
	}
	key := integrationConfigGetKey(args)
	if key == "" {
		return nil, fmt.Errorf("unexpected args: %v", args)
	}
	if r.unsupported[key] {
		return nil, fmt.Errorf("unsupported setting")
	}
	value := any(map[string]any{})
	if len(args) > 0 && args[0] == "--config" {
		data, err := os.ReadFile(args[1])
		if err != nil {
			return nil, err
		}
		value, err = integrationYAMLPath(data, key)
		if err != nil {
			return nil, err
		}
		if key == r.corruptKey {
			value = map[string]any{"corrupt": true}
		}
	}
	return json.Marshal(map[string]any{"key": key, "value": value})
}

func integrationConfigGetKey(args []string) string {
	for index := 0; index+3 < len(args); index++ {
		if args[index] == "config" && args[index+1] == "get" && args[index+3] == "--json" {
			return args[index+2]
		}
	}
	return ""
}

func integrationYAMLPath(data []byte, path string) (any, error) {
	var root map[string]any
	if err := yaml.Unmarshal(data, &root); err != nil {
		return nil, err
	}
	var current any = root
	for _, segment := range strings.Split(path, ".") {
		mapping, ok := current.(map[string]any)
		if !ok {
			return nil, fmt.Errorf("%s parent is not a map", path)
		}
		current, ok = mapping[segment]
		if !ok {
			return nil, fmt.Errorf("%s missing", path)
		}
	}
	return current, nil
}

func TestOMPModelIntegration_S1_NoOptInPreservesExistingSurface(t *testing.T) {
	t.Parallel()

	cfg := config.DefaultFullConfig("baseline")
	runner := &modelIntegrationFakeRunner{}
	files, err := NewWithRoot(t.TempDir()).WithModelIntegrationRunner(runner).prepareFiles(context.Background(), cfg)
	if err != nil {
		t.Fatalf("prepare baseline files: %v", err)
	}
	if len(runner.calls) != 0 {
		t.Fatalf("no-opt-in invoked model runner: %v", runner.calls)
	}
	for _, file := range files {
		if file.TargetPath == DefaultOMPModelOverlayPath || file.TargetPath == OMPModelReceiptRelativePath {
			t.Fatalf("no-opt-in emitted routing artifact %q", file.TargetPath)
		}
	}
}

func TestOMPModelIntegration_ConnectsPolicyCatalogProjectionAndReceipt(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	runner := newModelIntegrationRunner()
	clock := func() time.Time { return time.Date(2026, 8, 2, 12, 0, 0, 0, time.UTC) }
	a := NewWithRoot(root).WithModelIntegrationRunner(runner).WithModelIntegrationClock(clock)
	files, err := a.prepareFiles(context.Background(), integrationHarnessConfig("overlay"))
	if err != nil {
		t.Fatalf("prepare opt-in files: %v", err)
	}
	if runner.modelRequests != 0 {
		t.Fatalf("model request count = %d", runner.modelRequests)
	}
	activationCalls := 0
	for _, call := range runner.calls {
		if len(call) == 0 || call[0] != "--config" {
			continue
		}
		activationCalls++
		if len(call) != 6 || call[2] != "config" || call[3] != "get" || call[5] != "--json" {
			t.Fatalf("activation readback argv = %v", call)
		}
	}
	if activationCalls != 3 {
		t.Fatalf("activation readback calls = %d, want 3", activationCalls)
	}
	byPath := integrationMappingsByPath(files)
	for _, path := range []string{DefaultOMPModelOverlayPath, OMPModelReceiptRelativePath} {
		if _, ok := byPath[path]; !ok {
			t.Fatalf("missing integrated mapping %q", path)
		}
	}
	for path := range byPath {
		if strings.HasPrefix(path, ompRetiredAgentDir+"/") {
			t.Fatalf("generation emitted a retired agent definition %q", path)
		}
	}
	overlay := string(byPath[DefaultOMPModelOverlayPath].Content)
	if !strings.Contains(overlay, "task:\n  agentModelOverrides:\n") ||
		!strings.Contains(overlay, "task: anthropic/alpha-reasoner:xhigh") {
		t.Fatalf("native override projection missing:\n%s", overlay)
	}
	if strings.Contains(overlay, "autopus_") || strings.Contains(overlay, "modelRoles") {
		t.Fatalf("overlay still carries retired role aliases:\n%s", overlay)
	}
	if !strings.Contains(overlay, "modelFallback: true") {
		t.Fatalf("fallback activation missing:\n%s", overlay)
	}
	var receipt OMPModelResolutionReceipt
	if err := json.Unmarshal(byPath[OMPModelReceiptRelativePath].Content, &receipt); err != nil {
		t.Fatalf("parse receipt: %v", err)
	}
	if receipt.Profile != "p1" || len(receipt.Roles) != len(config.OMPNativeAgentNames()) ||
		receipt.GeneratedAt != clock() {
		t.Fatalf("unexpected receipt: profile=%q roles=%d generated=%s", receipt.Profile, len(receipt.Roles), receipt.GeneratedAt)
	}
	if !validOMPModelHash(receipt.ResolutionDigest) {
		t.Fatalf("invalid resolution digest %q", receipt.ResolutionDigest)
	}
}

func TestOMPModelIntegration_OptionalAndRuntimeDefaultRoutesInherit(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name           string
		required       bool
		degradedAction string
	}{
		{name: "optional", required: false},
		{name: "optional explicit runtime default", required: false, degradedAction: "runtime_default"},
		{name: "required explicit runtime default", required: true, degradedAction: "runtime_default"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			cfg := integrationHarnessConfig("overlay")
			profile := cfg.RoleModelPolicy.Profiles["p1"]
			route := profile.Capabilities[config.CapabilityIndependentDissent]
			route.Required = test.required
			route.DegradedAction = test.degradedAction
			for index := range route.Candidates {
				route.Candidates[index].Selector = fmt.Sprintf("missing/reviewer-%d", index)
			}
			profile.Capabilities[config.CapabilityIndependentDissent] = route
			cfg.RoleModelPolicy.Profiles["p1"] = profile

			runner := newModelIntegrationRunner()
			files, err := NewWithRoot(t.TempDir()).WithModelIntegrationRunner(runner).
				prepareFiles(context.Background(), cfg)
			if err != nil {
				t.Fatalf("prepare optional route: %v", err)
			}
			byPath := integrationMappingsByPath(files)
			overlay := string(byPath[DefaultOMPModelOverlayPath].Content)
			if strings.Contains(overlay, "missing/reviewer") {
				t.Fatalf("optional route leaked into overlay:\n%s", overlay)
			}
			// An unresolved dissent route leaves both dissent agents absent so
			// each inherits the parent session model at spawn time.
			for _, agent := range []string{"reviewer", "security-reviewer"} {
				if strings.Contains(overlay, "\n    "+agent+":") {
					t.Fatalf("%s did not inherit runtime defaults:\n%s", agent, overlay)
				}
			}
			if runner.modelRequests != 0 {
				t.Fatalf("model request count = %d", runner.modelRequests)
			}
		})
	}
}

func TestBridgeOMPIntegrationRoutes_KeysRoutesByBundledAgent(t *testing.T) {
	t.Parallel()

	profile := integrationHarnessConfig("overlay").RoleModelPolicy.Profiles["p1"]
	profile.FamilyDiversity.Roles = []string{config.OMPAgentRoleName("reviewer")}
	routes, err := bridgeOMPIntegrationRoutes(profile)
	if err != nil {
		t.Fatalf("bridge routes: %v", err)
	}
	if len(routes) != len(config.OMPNativeAgentNames()) {
		t.Fatalf("route count = %d, want one per bundled agent", len(routes))
	}
	for _, agent := range config.OMPNativeAgentNames() {
		route, ok := routes[agent]
		if !ok {
			t.Fatalf("route missing for agent %q", agent)
		}
		resolved, resolveErr := config.ResolveOMPPolicyAgent(agent)
		if resolveErr != nil {
			t.Fatal(resolveErr)
		}
		if route.Agent != agent || route.Role != resolved.Role || route.Capability != resolved.Capability {
			t.Fatalf("route %q carries wrong identity: %+v", agent, route)
		}
	}
	if !routes["reviewer"].PreferDistinctExecutorFamily {
		t.Fatal("configured reviewer role did not request family diversity")
	}
	if routes["security-reviewer"].PreferDistinctExecutorFamily {
		t.Fatal("unconfigured security-auditor role requested family diversity")
	}
}

func TestOMPModelIntegration_S17_ResolutionDigestBindsGeneratedAt(t *testing.T) {
	t.Parallel()

	compile := func(at time.Time) string {
		a := NewWithRoot(t.TempDir()).WithModelIntegrationRunner(newModelIntegrationRunner()).
			WithModelIntegrationClock(func() time.Time { return at })
		files, err := a.prepareFiles(context.Background(), integrationHarnessConfig("overlay"))
		if err != nil {
			t.Fatalf("prepare integration: %v", err)
		}
		var receipt OMPModelResolutionReceipt
		if err := json.Unmarshal(integrationMappingsByPath(files)[OMPModelReceiptRelativePath].Content, &receipt); err != nil {
			t.Fatal(err)
		}
		return receipt.ResolutionDigest
	}
	if first, second := compile(time.Unix(1, 0)), compile(time.Unix(999, 0)); first == second {
		t.Fatalf("generated_at did not change digest: %q", first)
	}
}

func newModelIntegrationRunner() *modelIntegrationFakeRunner {
	return &modelIntegrationFakeRunner{catalog: []byte(`{"models":[
{"provider":"anthropic","id":"alpha-reasoner","family":"anthropic","capabilities":["deep_reasoning","independent_dissent"],"thinking":["high","xhigh"],"auth_enabled":true},
{"provider":"openai","id":"beta-coder","family":"openai","capabilities":["coding_tool_use","fast_validation","deterministic_transform","independent_dissent"],"thinking":["medium","high"],"auth_enabled":true},
{"provider":"google","id":"gamma-vision","family":"google","capabilities":["vision_design"],"thinking":["high"],"auth_enabled":true}]}`)}
}

func integrationMappingsByPath(files []adapter.FileMapping) map[string]adapter.FileMapping {
	result := make(map[string]adapter.FileMapping, len(files))
	for _, file := range files {
		result[filepath.ToSlash(file.TargetPath)] = file
	}
	return result
}
