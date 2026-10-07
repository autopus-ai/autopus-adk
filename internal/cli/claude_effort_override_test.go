package cli

import (
	"reflect"
	"strings"
	"testing"

	"github.com/insajin/autopus-adk/pkg/config"
)

func TestUpsertClaudeEffortArg_CopiesAndPreservesArgvShape(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		args   []string
		effort string
		want   []string
	}{
		{
			name:   "split value is replaced in place",
			args:   []string{"--print", "--model", "fable", "--effort", "high", "--verbose"},
			effort: "ultracode",
			want:   []string{"--print", "--model", "fable", "--effort", "ultracode", "--verbose"},
		},
		{
			name:   "equals value is replaced in place",
			args:   []string{"--model", "claude-opus-4-8", "--effort=medium", "--permission-mode", "plan"},
			effort: "max",
			want:   []string{"--model", "claude-opus-4-8", "--effort=max", "--permission-mode", "plan"},
		},
		{
			name:   "missing flag is appended",
			args:   []string{"--print", "--model", "opus"},
			effort: "xhigh",
			want:   []string{"--print", "--model", "opus", "--effort", "xhigh"},
		},
		{
			name:   "nil argv accepts explicit effort",
			args:   nil,
			effort: "low",
			want:   []string{"--effort", "low"},
		},
		{
			name:   "empty effort returns unchanged nil copy",
			args:   nil,
			effort: "",
			want:   nil,
		},
		{
			name:   "empty effort returns unchanged nonnil copy",
			args:   []string{"--model", "opus"},
			effort: "",
			want:   []string{"--model", "opus"},
		},
		{
			name:   "trailing split flag receives a value",
			args:   []string{"--print", "--effort"},
			effort: "max",
			want:   []string{"--print", "--effort", "max"},
		},
		{
			name:   "split flag preserves an option looking follower",
			args:   []string{"--effort", "--permission-mode", "plan"},
			effort: "max",
			want:   []string{"--effort", "max", "--permission-mode", "plan"},
		},
		{
			name:   "first split flag wins over mixed duplicate forms",
			args:   []string{"--print", "--effort", "high", "--model", "fable", "--effort=medium", "--verbose", "--effort", "low", "prompt"},
			effort: "ultracode",
			want:   []string{"--print", "--effort", "ultracode", "--model", "fable", "--verbose", "prompt"},
		},
		{
			name:   "first equals flag wins over a later split duplicate",
			args:   []string{"--effort=high", "--model", "fable", "--effort", "medium", "--permission-mode", "plan"},
			effort: "xhigh",
			want:   []string{"--effort=xhigh", "--model", "fable", "--permission-mode", "plan"},
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			before := append([]string(nil), tt.args...)
			got := upsertClaudeEffortArg(tt.args, tt.effort)
			if !reflect.DeepEqual(got, tt.want) {
				t.Fatalf("upsertClaudeEffortArg(%v, %q) = %v, want %v", tt.args, tt.effort, got, tt.want)
			}
			if !reflect.DeepEqual(tt.args, before) {
				t.Fatalf("input argv mutated: got %v, want %v", tt.args, before)
			}
			if len(got) > 0 && len(tt.args) > 0 {
				got[0] = "mutated-result"
				if !reflect.DeepEqual(tt.args, before) {
					t.Fatalf("result aliases input argv: got %v, want %v", tt.args, before)
				}
			}
		})
	}
}

func TestApplyRuntimeHarnessOverrides_ClaudeEffortDoesNotRequireCodex(t *testing.T) {
	t.Parallel()

	cfg := config.DefaultFullConfig(".")
	cfg.Orchestra.Providers = map[string]config.ProviderEntry{
		"claude": {
			Binary: "claude-wrapper",
			Args:   []string{"--print", "--model", "fable", "--effort", "high", "--verbose"},
		},
	}

	got := applyRuntimeHarnessOverrides(effectiveHarnessConfig{Config: cfg}, globalFlags{Effort: "ultracode"})
	entry := got.Config.Orchestra.Providers["claude"]
	wantArgs := []string{"--print", "--model", "fable", "--effort", "ultracode", "--verbose"}
	if !reflect.DeepEqual(entry.Args, wantArgs) {
		t.Fatalf("Claude override = args %v, want %v", entry.Args, wantArgs)
	}
}

func TestApplyRuntimeHarnessOverrides_PreservesPinnedCodex(t *testing.T) {
	t.Parallel()

	cfg := config.DefaultFullConfig(".")
	wantCodex := config.ProviderEntry{
		Binary:      "codex-wrapper",
		Args:        []string{"exec", "-m", "user/model", "-c", `model_reasoning_effort="low"`},
		ModelPolicy: config.ProviderModelPolicyPinned,
	}
	cfg.Orchestra.Providers = map[string]config.ProviderEntry{
		"claude": {Binary: "claude", Args: []string{"--model", "fable"}},
		"codex":  wantCodex,
	}

	got := applyRuntimeHarnessOverrides(effectiveHarnessConfig{Config: cfg}, globalFlags{Effort: "max"})
	claude := got.Config.Orchestra.Providers["claude"]
	if want := []string{"--model", "fable", "--effort", "max"}; !reflect.DeepEqual(claude.Args, want) {
		t.Fatalf("Claude args = %v, want %v", claude.Args, want)
	}
	if codex := got.Config.Orchestra.Providers["codex"]; !reflect.DeepEqual(codex, wantCodex) {
		t.Fatalf("pinned Codex provider changed: got %+v want %+v", codex, wantCodex)
	}
}

func TestApplyRuntimeHarnessOverrides_AbsentClaudeLeavesProvidersUnchanged(t *testing.T) {
	t.Parallel()

	cfg := config.DefaultFullConfig(".")
	wantCodex := config.ProviderEntry{
		Binary:      "codex",
		Args:        []string{"exec", "-m", "user/model"},
		ModelPolicy: config.ProviderModelPolicyPinned,
	}
	cfg.Orchestra.Providers = map[string]config.ProviderEntry{"codex": wantCodex}

	got := applyRuntimeHarnessOverrides(effectiveHarnessConfig{Config: cfg}, globalFlags{Effort: "xhigh"})
	if !reflect.DeepEqual(got.Config.Orchestra.Providers, map[string]config.ProviderEntry{"codex": wantCodex}) {
		t.Fatalf("providers changed without Claude: got %+v", got.Config.Orchestra.Providers)
	}
}

func TestBuildProviderConfigsForRuntime_AppliesClaudeExplicitEffort(t *testing.T) {
	t.Parallel()

	got := buildProviderConfigsForRuntime([]string{"claude"}, "balanced", "xhigh")
	if len(got) != 1 {
		t.Fatalf("providers = %d, want 1", len(got))
	}
	want := []string{"--print", "--model", config.ClaudeFableModel, "--effort", "xhigh"}
	if !reflect.DeepEqual(got[0].Args, want) {
		t.Fatalf("fallback Claude = args %v, want %v", got[0].Args, want)
	}

	empty := buildProviderConfigsForRuntime([]string{"claude"}, "balanced", "")
	defaultArgs := config.DefaultClaudeProviderEntry().Args
	if len(empty) != 1 || !reflect.DeepEqual(empty[0].Args, defaultArgs) {
		t.Fatalf("fallback Claude = %+v, want the shipped default argv %v", empty, defaultArgs)
	}
}

func TestApplyRuntimeHarnessOverrides_UltracodeStaysAtClaudeBoundary(t *testing.T) {
	t.Parallel()

	cfg := config.DefaultFullConfig(".")
	cfg.Orchestra.Providers = map[string]config.ProviderEntry{
		"claude": {
			Binary: "claude",
			Args:   []string{"--print", "--model", "fable"},
		},
		"codex": config.CodexProviderEntryForQuality(config.QualityConf{Default: "balanced"}),
	}

	got := applyRuntimeHarnessOverrides(effectiveHarnessConfig{Config: cfg}, globalFlags{Effort: "ultracode"})
	claude := got.Config.Orchestra.Providers["claude"]
	if !strings.Contains(strings.Join(claude.Args, "\x00"), "ultracode") {
		t.Fatalf("Claude argv lost ultracode: %v", claude.Args)
	}
	codex := got.Config.Orchestra.Providers["codex"]
	joined := strings.Join(codex.Args, "\x00")
	if strings.Contains(joined, "ultracode") {
		t.Fatalf("Codex argv contains Claude-only ultracode: %v", codex.Args)
	}
	if !strings.Contains(joined, `model_reasoning_effort="xhigh"`) {
		t.Fatalf("Codex argv effort = %v, want xhigh", codex.Args)
	}
}

func TestBuildProviderConfigsForRuntime_UltracodeStaysAtClaudeBoundary(t *testing.T) {
	t.Parallel()

	got := buildProviderConfigsForRuntime([]string{"claude", "codex"}, "balanced", "ultracode")
	if len(got) != 2 {
		t.Fatalf("providers = %d, want 2", len(got))
	}
	if !strings.Contains(strings.Join(got[0].Args, "\x00"), "ultracode") {
		t.Fatalf("fallback Claude argv lost ultracode: %v", got[0].Args)
	}
	joined := strings.Join(got[1].Args, "\x00")
	if strings.Contains(joined, "ultracode") {
		t.Fatalf("fallback Codex argv contains Claude-only ultracode: %v", got[1].Args)
	}
	if !strings.Contains(joined, `model_reasoning_effort="xhigh"`) {
		t.Fatalf("fallback Codex argv effort = %v, want xhigh", got[1].Args)
	}
}
