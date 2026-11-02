package cli

import (
	"bufio"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/insajin/autopus-adk/pkg/pipeline"
)

const pipelineOMPRPCFixtureEnv = "AUTOPUS_TEST_OMP_PIPELINE_RPC"
const pipelineOMPActiveRPCFixtureEnv = "AUTOPUS_TEST_OMP_ACTIVE_RPC"

type pipelineOMPRPCRecord struct {
	Kind     string   `json:"kind"`
	PID      int      `json:"pid,omitempty"`
	Args     []string `json:"args,omitempty"`
	ID       string   `json:"id,omitempty"`
	Type     string   `json:"type,omitempty"`
	Enabled  *bool    `json:"enabled,omitempty"`
	Provider string   `json:"provider,omitempty"`
	ModelID  string   `json:"modelId,omitempty"`
	Message  string   `json:"message,omitempty"`
	Protocol int      `json:"protocolVersion,omitempty"`
}

func TestMain(m *testing.M) {
	if len(os.Args) == 2 && os.Args[1] == "--version" {
		_, _ = os.Stdout.WriteString("omp/17.2.7\n")
		os.Exit(0)
	}
	if logPath, unsafe, ok := pipelineOMPActiveNativeFixture(os.Args[1:]); ok {
		_ = os.Setenv("AUTOPUS_TEST_OMP_ACTIVE_LOG", logPath)
		_ = os.Setenv("AUTOPUS_TEST_OMP_ACTIVE_UNSAFE", unsafe)
		os.Exit(runPipelineOMPActiveRPCFixture())
	}
	if os.Getenv(pipelineOMPActiveRPCFixtureEnv) == "1" {
		os.Exit(runPipelineOMPActiveRPCFixture())
	}
	if os.Getenv(ompReviewRPCFixtureEnv) == "1" {
		os.Exit(runOMPReviewRPCFixture())
	}
	if os.Getenv(pipelineOMPRPCFixtureEnv) == "1" {
		os.Exit(runPipelineOMPRPCFixture())
	}
	reaNodeExec = testReaNodeV24 // SPEC-REA-001: no CLI test runs a real node
	code := m.Run()
	_ = os.RemoveAll(diskTestStateRoot) // SPEC-DISK-002: see disk_state_guard_test.go
	os.Exit(code)
}

func TestPipelineOMPBackend_ReusesRPCSessionAndReturnsExactPhaseOutput(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("fixture executable uses a POSIX launcher")
	}
	config, logPath := pipelineOMPBackendTestConfig(t)
	backend, err := newPipelineOMPBackend(config)
	require.NoError(t, err)

	first := sealedPipelineOMPRequest(t, config, pipeline.PhasePlan, "PLAN-PHASE-PROMPT", nil)
	firstResponse, err := backend.Execute(context.Background(), first)
	require.NoError(t, err)
	require.NotNil(t, firstResponse)
	assert.Equal(t, "exact plan output", firstResponse.Output)

	second := sealedPipelineOMPRequest(
		t, config, pipeline.PhaseImplement, "IMPLEMENT-PHASE-PROMPT", []string{firstResponse.Output},
	)
	secondResponse, err := backend.Execute(context.Background(), second)
	require.NoError(t, err)
	require.NotNil(t, secondResponse)
	assert.Equal(t, "exact implementation output", secondResponse.Output)

	records := readPipelineOMPRPCRecords(t, logPath)
	starts, commands := pipelineOMPRPCRecordsByKind(records)
	require.Len(t, starts, 1, "all phases must share one OMP process")
	assert.Contains(t, starts[0].Args, "--no-session")
	assert.Contains(t, starts[0].Args, "--no-extensions")
	negotiations := filterPipelineOMPRPCCommands(commands, "negotiate_protocol")
	require.Len(t, negotiations, 1)
	assert.Equal(t, 2, negotiations[0].Protocol)
	assertPipelineOMPRPCBooleanCommand(t, commands, "set_auto_retry", false)
	assertPipelineOMPRPCBooleanCommand(t, commands, "set_auto_compaction", false)

	models := filterPipelineOMPRPCCommands(commands, "set_model")
	require.Len(t, models, 2)
	assert.Equal(t, "provider-a", models[0].Provider)
	assert.Equal(t, "model-plan", models[0].ModelID)
	assert.Equal(t, "provider-b", models[1].Provider)
	assert.Equal(t, "model-implement", models[1].ModelID)
	prompts := filterPipelineOMPRPCCommands(commands, "prompt")
	require.Len(t, prompts, 2)
	assert.Equal(t, "PLAN-PHASE-PROMPT", prompts[0].Message)
	assert.Equal(t, "IMPLEMENT-PHASE-PROMPT", prompts[1].Message)
	for _, prompt := range prompts {
		assert.False(t, strings.HasPrefix(strings.TrimSpace(prompt.Message), "/auto"))
	}
	assert.Equal(t, 2, countPipelineOMPRPCCommand(commands, "get_last_assistant_text"))

	entries, err := os.ReadDir(config.RuntimeBase)
	require.NoError(t, err)
	require.NotEmpty(t, entries, "backend must own an isolated task runtime while active")
	require.NoError(t, backend.Close())
	assert.False(t, pipelineOMPProcessExists(starts[0].PID), "Close must terminate the owned process")
	assertPipelineOMPRuntimeEmpty(t, config.RuntimeBase)
}

func TestPipelineOMPBackend_UnsealedRequestFailsBeforeSpawn(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("fixture executable uses a POSIX launcher")
	}
	config, logPath := pipelineOMPBackendTestConfig(t)
	backend, err := newPipelineOMPBackend(config)
	require.NoError(t, err)

	_, err = backend.Execute(context.Background(), pipeline.PhaseRequest{
		PhaseID: pipeline.PhasePlan, Attempt: 1, Prompt: "caller text is not authority",
	})
	require.ErrorContains(t, err, "sealed OMP execution view")
	_, statErr := os.Stat(logPath)
	assert.ErrorIs(t, statErr, os.ErrNotExist, "unsealed input must fail before process spawn")
	require.NoError(t, backend.Close())
}

func pipelineOMPBackendTestConfig(t *testing.T) (pipelineOMPBackendConfig, string) {
	t.Helper()
	fixtureDir := t.TempDir()
	logPath := filepath.Join(fixtureDir, "rpc.jsonl")
	executable := filepath.Join(fixtureDir, "omp-fixture")
	script := "#!/bin/sh\nexec " + shellQuotePipelineOMP(os.Args[0]) + " -test.run=^$ -- \"$@\"\n"
	require.NoError(t, os.WriteFile(executable, []byte(script), 0o700))
	projectDir := t.TempDir()
	specDir := filepath.Join(projectDir, ".autopus", "specs", "SPEC-OMP-004")
	runtimeBase := filepath.Join(t.TempDir(), "runtime")
	require.NoError(t, os.MkdirAll(runtimeBase, 0o700))
	return pipelineOMPBackendConfig{
		Executable: executable, ProjectDir: projectDir, SpecID: "SPEC-OMP-004", SpecDir: specDir,
		SnapshotHash: "sha256:" + strings.Repeat("a", 64), GitCommitHash: strings.Repeat("b", 40),
		RuntimeBase: runtimeBase, Environment: append(os.Environ(),
			pipelineOMPRPCFixtureEnv+"=1", "AUTOPUS_TEST_OMP_PIPELINE_LOG="+logPath),
		PhaseModels: map[pipeline.PhaseID]string{
			pipeline.PhasePlan: "provider-a/model-plan", pipeline.PhaseTestScaffold: "provider-a/model-test",
			pipeline.PhaseImplement: "provider-b/model-implement", pipeline.PhaseValidate: "provider-a/model-validate",
			pipeline.PhaseReview: "provider-b/model-review",
		},
		MaxTime: 5 * time.Second,
	}, logPath
}

func sealedPipelineOMPRequest(
	t *testing.T, config pipelineOMPBackendConfig, phase pipeline.PhaseID, prompt string, history []string,
) pipeline.PhaseRequest {
	t.Helper()
	activePrompt := prompt
	if strings.HasPrefix(strings.TrimSpace(activePrompt), "/auto") {
		activePrompt = "active phase prompt"
	}
	view, err := pipeline.NewOMPExecutionView(pipeline.OMPExecutionViewInput{
		ProjectDir: config.ProjectDir, SpecID: config.SpecID, SpecDir: config.SpecDir,
		SnapshotHash: config.SnapshotHash, GitCommitHash: config.GitCommitHash,
		PhaseID: phase, Attempt: 1, Prompt: prompt, ActivePrompt: activePrompt, CompletedHistory: history,
	})
	require.NoError(t, err)
	return pipeline.PhaseRequest{PhaseID: phase, Attempt: 1, Prompt: "untrusted duplicate", OMPExecutionView: view}
}

func runPipelineOMPRPCFixture() int {
	logFile, err := os.OpenFile(os.Getenv("AUTOPUS_TEST_OMP_PIPELINE_LOG"), os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		return 91
	}
	defer logFile.Close()
	logEncoder, output := json.NewEncoder(logFile), json.NewEncoder(os.Stdout)
	_ = logEncoder.Encode(pipelineOMPRPCRecord{Kind: "start", PID: os.Getpid(), Args: os.Args})
	_ = output.Encode(map[string]any{"type": "ready"})
	lastOutput := ""
	scanner := bufio.NewScanner(os.Stdin)
	for scanner.Scan() {
		var command pipelineOMPRPCRecord
		if json.Unmarshal(scanner.Bytes(), &command) != nil {
			return 92
		}
		command.Kind = "command"
		_ = logEncoder.Encode(command)
		switch command.Type {
		case "prompt":
			lastOutput = "exact plan output"
			if command.Message == "IMPLEMENT-PHASE-PROMPT" {
				lastOutput = "exact implementation output"
			}
			writePipelineOMPRPCResponse(output, command, map[string]any{"agentInvoked": true})
			_ = output.Encode(map[string]any{"type": "agent_start"})
			_ = output.Encode(map[string]any{"type": "turn_end"})
			_ = output.Encode(map[string]any{"type": "agent_end"})
		case "get_state":
			messageCount := 0
			if lastOutput != "" {
				messageCount = 2
				if lastOutput == "exact implementation output" {
					messageCount = 4
				}
			}
			writePipelineOMPRPCResponse(output, command, map[string]any{
				"sessionId": "pipeline-session", "isStreaming": false, "isCompacting": false,
				"messageCount": messageCount, "queuedMessageCount": 0,
			})
		case "get_last_assistant_text":
			writePipelineOMPRPCResponse(output, command, map[string]any{"text": lastOutput})
		case "negotiate_protocol":
			writePipelineOMPRPCResponse(output, command, map[string]any{"protocolVersion": 2})
		default:
			writePipelineOMPRPCResponse(output, command, nil)
		}
	}
	if scanner.Err() != nil {
		return 93
	}
	return 0
}

func writePipelineOMPRPCResponse(output *json.Encoder, command pipelineOMPRPCRecord, data any) {
	_ = output.Encode(map[string]any{
		"id": command.ID, "type": "response", "command": command.Type, "success": true, "data": data,
	})
}

func readPipelineOMPRPCRecords(t *testing.T, path string) []pipelineOMPRPCRecord {
	t.Helper()
	body, err := os.ReadFile(path)
	require.NoError(t, err)
	var records []pipelineOMPRPCRecord
	for _, line := range strings.Split(strings.TrimSpace(string(body)), "\n") {
		var record pipelineOMPRPCRecord
		require.NoError(t, json.Unmarshal([]byte(line), &record))
		records = append(records, record)
	}
	return records
}

func pipelineOMPRPCRecordsByKind(records []pipelineOMPRPCRecord) ([]pipelineOMPRPCRecord, []pipelineOMPRPCRecord) {
	var starts, commands []pipelineOMPRPCRecord
	for _, record := range records {
		if record.Kind == "start" {
			starts = append(starts, record)
		} else if record.Kind == "command" {
			commands = append(commands, record)
		}
	}
	return starts, commands
}

func filterPipelineOMPRPCCommands(records []pipelineOMPRPCRecord, kind string) []pipelineOMPRPCRecord {
	var filtered []pipelineOMPRPCRecord
	for _, record := range records {
		if record.Type == kind {
			filtered = append(filtered, record)
		}
	}
	return filtered
}

func countPipelineOMPRPCCommand(records []pipelineOMPRPCRecord, kind string) int {
	return len(filterPipelineOMPRPCCommands(records, kind))
}

func assertPipelineOMPRPCBooleanCommand(t *testing.T, records []pipelineOMPRPCRecord, kind string, want bool) {
	t.Helper()
	filtered := filterPipelineOMPRPCCommands(records, kind)
	require.Len(t, filtered, 1)
	require.NotNil(t, filtered[0].Enabled)
	assert.Equal(t, want, *filtered[0].Enabled)
}

func assertPipelineOMPRuntimeEmpty(t *testing.T, root string) {
	t.Helper()
	entries, err := os.ReadDir(root)
	if os.IsNotExist(err) {
		return
	}
	require.NoError(t, err)
	assert.Empty(t, entries)
}

func pipelineOMPProcessExists(pid int) bool {
	return pid > 0 && processAliveForTest(pid)
}

func shellQuotePipelineOMP(value string) string {
	return "'" + strings.ReplaceAll(value, "'", "'\"'\"'") + "'"
}
