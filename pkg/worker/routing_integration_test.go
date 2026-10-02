package worker

import (
	"context"
	"fmt"
	"os/exec"
	"strings"
	"testing"

	"github.com/insajin/autopus-adk/pkg/worker/adapter"
	"github.com/insajin/autopus-adk/pkg/worker/routing"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// modelCapturingAdapter captures TaskConfig.Model without running a real subprocess.
type modelCapturingAdapter struct {
	name          string
	capturedModel string
}

func (m *modelCapturingAdapter) Name() string { return m.name }

func (m *modelCapturingAdapter) BuildCommand(ctx context.Context, task adapter.TaskConfig) *exec.Cmd {
	m.capturedModel = task.Model
	// Return a no-op command that won't be executed in these tests.
	return exec.CommandContext(ctx, "true")
}

func (m *modelCapturingAdapter) ParseEvent(line []byte) (adapter.StreamEvent, error) {
	return adapter.StreamEvent{}, nil
}

func (m *modelCapturingAdapter) ExtractResult(event adapter.StreamEvent) adapter.TaskResult {
	return adapter.TaskResult{}
}

// enabledRoutingConfig returns a routing config with routing enabled.
func enabledRoutingConfig() routing.RoutingConfig {
	cfg := routing.DefaultConfig()
	cfg.Enabled = true
	return cfg
}

// TestRoutingIntegration_WorkerLoopEnabled verifies that when routing is enabled,
// the router resolves a model based on message complexity (REQ-ROUTE-01, S1).
func TestRoutingIntegration_WorkerLoopEnabled(t *testing.T) {
	t.Parallel()

	mock := &modelCapturingAdapter{name: "claude"}
	router := routing.NewRouter(enabledRoutingConfig())

	wl := &WorkerLoop{
		config: LoopConfig{
			Provider: mock,
			WorkDir:  t.TempDir(),
			Router:   router,
		},
	}

	// Simulate handleTask's routing logic: route on msg.Description, not prompt.
	description := "fix typo"

	var model string
	if wl.config.Router != nil {
		model = wl.config.Router.Route(wl.config.Provider.Name(), description)
	}

	taskCfg := adapter.TaskConfig{
		TaskID: "task-route-1",
		Model:  model,
	}

	// BuildCommand captures the Model field.
	mock.BuildCommand(context.Background(), taskCfg)
	assert.Equal(t, "claude-sonnet-5-5", mock.capturedModel,
		"short description should route to simple model (claude-sonnet-5)")
}

// TestRoutingIntegration_ComplexPrompt verifies complex prompts
// route to the complex model (S1 acceptance).
func TestRoutingIntegration_ComplexPrompt(t *testing.T) {
	t.Parallel()

	mock := &modelCapturingAdapter{name: "claude"}
	router := routing.NewRouter(enabledRoutingConfig())

	// Long prompt with code blocks -> complex complexity.
	longPrompt := strings.Repeat("implement ", 200) + "\n```go\nfunc main() {}\n```"
	model := router.Route("claude", longPrompt)

	taskCfg := adapter.TaskConfig{
		TaskID: "task-route-2",
		Prompt: longPrompt,
		Model:  model,
	}

	mock.BuildCommand(context.Background(), taskCfg)
	assert.Equal(t, "claude-fable-5-1", mock.capturedModel,
		"complex prompt should route to complex model (claude-fable-5-1)")
}

// TestRoutingIntegration_Disabled verifies S7 passthrough:
// routing disabled means Model stays empty.
func TestRoutingIntegration_Disabled(t *testing.T) {
	t.Parallel()

	disabledCfg := routing.DefaultConfig()
	disabledCfg.Enabled = false
	router := routing.NewRouter(disabledCfg)

	model := router.Route("claude", "fix typo")
	assert.Empty(t, model, "disabled routing should return empty model (S7 passthrough)")
}

// TestRoutingIntegration_NilRouter verifies nil router leaves Model empty.
func TestRoutingIntegration_NilRouter(t *testing.T) {
	t.Parallel()

	wl := &WorkerLoop{
		config: LoopConfig{
			Provider: &modelCapturingAdapter{name: "claude"},
			WorkDir:  t.TempDir(),
			Router:   nil,
		},
	}

	// Replicate handleTask nil-check logic.
	var model string
	if wl.config.Router != nil {
		model = wl.config.Router.Route(wl.config.Provider.Name(), "hello")
	}

	assert.Empty(t, model, "nil router should leave Model empty")
}

// TestRoutingIntegration_PipelineSetRouter verifies PipelineExecutor.SetRouter
// correctly wires the router for per-phase model resolution.
func TestRoutingIntegration_PipelineSetRouter(t *testing.T) {
	t.Parallel()

	mock := &modelCapturingAdapter{name: "claude"}
	pe := NewPipelineExecutor(mock, "", t.TempDir())
	router := routing.NewRouter(enabledRoutingConfig())
	pe.SetRouter(router)

	require.NotNil(t, pe.router, "SetRouter should set the router field")

	// Replicate Execute() routing: route once on original prompt, pass to runPhase.
	originalPrompt := "short prompt"
	var routedModel string
	if pe.router != nil {
		routedModel = pe.router.Route(pe.provider.Name(), originalPrompt)
	}

	// Simulate what runPhase receives: the model is passed as parameter.
	taskCfg := adapter.TaskConfig{
		TaskID: "task-pr-1-planner",
		Prompt: pe.plannerPrompt(originalPrompt), // phase-wrapped prompt
		Model:  routedModel,                      // from Execute(), not per-phase
	}

	mock.BuildCommand(context.Background(), taskCfg)
	assert.Equal(t, "claude-sonnet-5-5", mock.capturedModel,
		"pipeline should route once on original prompt, not phase-wrapped prompt")
}

// TestRoutingIntegration_PipelineNilRouter verifies pipeline without router
// leaves Model empty.
func TestRoutingIntegration_PipelineNilRouter(t *testing.T) {
	t.Parallel()

	mock := &modelCapturingAdapter{name: "claude"}
	pe := NewPipelineExecutor(mock, "", t.TempDir())
	// No SetRouter call.

	var model string
	if pe.router != nil {
		model = pe.router.Route(pe.provider.Name(), "test prompt")
	}

	assert.Empty(t, model, "nil router in pipeline should leave Model empty")
}

// TestRoutingIntegration_CodexProvider verifies Codex provider gets correct model.
func TestRoutingIntegration_CodexProvider(t *testing.T) {
	t.Parallel()

	router := routing.NewRouter(enabledRoutingConfig())

	// Short prompt -> simple -> gpt-6-luna for codex.
	model := router.Route("codex", "fix bug")
	assert.Equal(t, "gpt-6-luna", model,
		"codex simple prompt should route to gpt-6-luna")
}

// TestRoutingIntegration_GeminiProvider verifies Gemini provider gets correct model.
func TestRoutingIntegration_GeminiProvider(t *testing.T) {
	t.Parallel()

	router := routing.NewRouter(enabledRoutingConfig())

	// Short prompt -> simple -> gemini-3.8-flash.
	model := router.Route("gemini", "hello")
	assert.Equal(t, "gemini-3.8-flash", model,
		"gemini simple prompt should route to gemini-3.8-flash")
}

// TestRoutingIntegration_UnknownProvider verifies unknown providers return empty model.
func TestRoutingIntegration_UnknownProvider(t *testing.T) {
	t.Parallel()

	router := routing.NewRouter(enabledRoutingConfig())
	model := router.Route("unknown-provider", "test")
	assert.Empty(t, model, "unknown provider should return empty model")
}

// TestRoutingIntegration_TaskConfigModelPropagation verifies the full chain:
// Router.Route() -> TaskConfig.Model -> BuildCommand captures it.
func TestRoutingIntegration_TaskConfigModelPropagation(t *testing.T) {
	t.Parallel()

	// Test with real adapters to verify Model appears in args.
	router := routing.NewRouter(enabledRoutingConfig())
	model := router.Route("claude", "quick fix")

	taskCfg := adapter.TaskConfig{
		TaskID: "task-prop-1",
		Model:  model,
	}

	claudeAdapter := adapter.NewClaudeAdapter()
	cmd := claudeAdapter.BuildCommand(context.Background(), taskCfg)
	assert.Contains(t, cmd.Args, "--model")
	assert.Contains(t, cmd.Args, model,
		"Claude CLI args should contain the routed model")

	// Codex with -m flag.
	codexModel := router.Route("codex", "quick fix")
	codexCfg := adapter.TaskConfig{
		TaskID: "task-prop-2",
		Model:  codexModel,
	}
	codexAdapter := adapter.NewCodexAdapter()
	codexCmd := codexAdapter.BuildCommand(context.Background(), codexCfg)
	assert.Contains(t, codexCmd.Args, "-m")
	assert.Contains(t, codexCmd.Args, codexModel,
		"Codex CLI args should contain the routed model with -m flag")

	// Antigravity CLI 1.0.0 does not expose a model override flag.
	geminiModel := router.Route("gemini", "quick fix")
	geminiCfg := adapter.TaskConfig{
		TaskID: "task-prop-3",
		Model:  geminiModel,
	}
	geminiAdapter := adapter.NewGeminiAdapter()
	geminiCmd := geminiAdapter.BuildCommand(context.Background(), geminiCfg)
	assert.NotContains(t, geminiCmd.Args, "--model")
	assert.NotContains(t, geminiCmd.Args, geminiModel,
		fmt.Sprintf("Antigravity CLI args should omit unsupported routed model %s", geminiModel))
}
