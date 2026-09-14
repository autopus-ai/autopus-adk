package cli

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/insajin/autopus-adk/pkg/adapter/omp"
	"github.com/insajin/autopus-adk/pkg/config"
)

const (
	ompDoctorSecretSentinel = "sk-omp-doctor-secret"
	ompDoctorRawPayload     = "RAW-OMP-PROVIDER-PAYLOAD"
)

func TestOMP002_S10_DoctorCLIProjectsStableOMPChecksInTextAndJSON(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("hermetic fake omp uses a POSIX shell script")
	}

	root := t.TempDir()
	cfg := config.DefaultFullConfig("omp-doctor-cli")
	cfg.Platforms = []string{"omp"}
	require.NoError(t, config.Save(root, cfg))
	_, err := omp.NewWithRoot(root).Generate(context.Background(), cfg)
	require.NoError(t, err)
	assert.NoDirExists(t, filepath.Join(root, ".omp", "agents"),
		"OMP registers its own agents; a generated file there would shadow one")

	logPath := installHermeticOMPDoctorCLI(t)
	healthyText, healthyEnvelope := runOMPDoctorEntrypoints(t, root)
	healthyChecks := ompPlatformDoctorChecks(healthyEnvelope.Checks)
	require.NotEmpty(t, healthyChecks)
	assert.Empty(t, failingOMPDoctorChecks(healthyChecks),
		"healthy behavioral fixture must produce a complete provider-free OMP readiness receipt; fixture invocations:\n"+
			ompDoctorFixtureLog(t, logPath))
	assert.Equal(t, jsonStatusWarn, healthyEnvelope.Status,
		"missing non-OMP dependencies may warn without changing OMP check outcomes")
	assert.NotEmpty(t, failingNonOMPDoctorChecks(healthyEnvelope.Checks),
		"the fixture deliberately keeps unrelated dependency warnings separate")
	assertOMPDoctorProjectionIsRedacted(t, root, healthyText, healthyChecks)

	skillPath := filepath.Join(root, ".omp", "skills", "auto", "SKILL.md")
	generated, err := os.ReadFile(skillPath)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(skillPath, append(generated, []byte("\ncorrupted\n")...), 0o600))

	failedText, failedEnvelope := runOMPDoctorEntrypoints(t, root)
	validationChecks := ompValidationDoctorChecks(failedEnvelope.Checks)
	require.NotEmpty(t, validationChecks)
	var checksumCheck jsonCheck
	for _, candidate := range validationChecks {
		if strings.Contains(candidate.Detail, "managed content checksum mismatch") {
			checksumCheck = candidate
			break
		}
	}
	require.NotEmpty(t, checksumCheck.ID)
	assert.Equal(t, "error", checksumCheck.Severity)
	assert.Equal(t, "fail", checksumCheck.Status)

	textLine := lineContainingOMPDoctorCheck(t, failedText, checksumCheck.ID)
	assert.Contains(t, textLine, "ERROR", "text status must mirror JSON status=fail")
	assert.Contains(t, textLine, checksumCheck.ID)
	assert.Contains(t, textLine, checksumCheck.Detail)
	assertOMPDoctorProjectionIsRedacted(t, root, failedText, validationChecks)

	invocations, err := os.ReadFile(logPath)
	require.NoError(t, err)
	log := string(invocations)
	for _, expected := range []string{
		"--version", "--help", "config get tools.intentTracing --json",
		"--mode rpc", "--model openai-codex/gpt-5.6-sol",
		"--tools task,hub,todo", "provider-requests=0",
	} {
		assert.Contains(t, log, expected)
	}
	for _, forbidden := range []string{
		ompDoctorSecretSentinel, ompDoctorRawPayload, "Authorization",
		"models --json", `"type":"prompt"`, "--tools write",
	} {
		assert.NotContains(t, log, forbidden)
	}
}

func runOMPDoctorEntrypoints(t *testing.T, root string) (string, jsonEnvelope) {
	t.Helper()
	var text bytes.Buffer
	textCmd := NewRootCmd()
	textCmd.SetOut(&text)
	textCmd.SetErr(&text)
	textCmd.SetArgs([]string{"doctor", "--dir", root})
	require.NoError(t, textCmd.Execute())

	var encoded bytes.Buffer
	jsonCmd := NewRootCmd()
	jsonCmd.SetOut(&encoded)
	jsonCmd.SetErr(&encoded)
	jsonCmd.SetArgs([]string{"doctor", "--dir", root, "--json"})
	require.NoError(t, jsonCmd.Execute())
	var envelope jsonEnvelope
	require.NoError(t, json.Unmarshal(encoded.Bytes(), &envelope), encoded.String())
	return text.String(), envelope
}

func installHermeticOMPDoctorCLI(t *testing.T) string {
	t.Helper()
	binDir := t.TempDir()
	logPath := filepath.Join(t.TempDir(), "omp-invocations.log")
	executable, err := os.Executable()
	require.NoError(t, err)
	script := fmt.Sprintf("#!/bin/sh\nexec %q -test.run='^TestOMPDoctorBehaviorFixtureProcess$' -- --fixture-log=%q \"$@\"\n",
		executable, logPath)
	require.NoError(t, os.WriteFile(filepath.Join(binDir, "omp"), []byte(script), 0o755))
	t.Setenv("PATH", binDir)
	return logPath
}

func ompDoctorFixtureLog(t *testing.T, logPath string) string {
	t.Helper()
	content, err := os.ReadFile(logPath)
	if err != nil {
		return "fixture log unreadable: " + err.Error()
	}
	return string(content)
}

func TestOMPDoctorBehaviorFixtureProcess(t *testing.T) {
	separator := slices.Index(os.Args, "--")
	if separator < 0 {
		return
	}
	os.Exit(runOMPDoctorBehaviorFixture(os.Args[separator+1:]))
}

func runOMPDoctorBehaviorFixture(args []string) int {
	logPath := strings.TrimPrefix(args[0], "--fixture-log=")
	args = args[1:]
	log, err := os.OpenFile(logPath, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		return 61
	}
	_, _ = fmt.Fprintln(log, strings.Join(args, " "))
	_ = log.Close()
	joined := strings.Join(args, " ")
	switch {
	case joined == "--version":
		fmt.Println("omp/18.0.5")
	case joined == "--help":
		fmt.Println("--mode rpc --no-session --cwd DIR --model MODEL --tools TOOLS --no-extensions --no-rules --no-lsp --no-pty")
	case joined == "config get tools.intentTracing --json":
		fmt.Println("true")
	case strings.Contains(joined, "--mode rpc"):
		return runOMPDoctorBehaviorRPC(logPath)
	default:
		return 62
	}
	return 0
}

func runOMPDoctorBehaviorRPC(logPath string) int {
	scanner := bufio.NewScanner(io.LimitReader(os.Stdin, 2049))
	var input bytes.Buffer
	for scanner.Scan() {
		input.Write(scanner.Bytes())
		input.WriteByte('\n')
	}
	wire := input.String()
	if scanner.Err() != nil || input.Len() > 2048 {
		return 63
	}
	for _, required := range []string{
		`"type":"negotiate_protocol"`, `"type":"get_state"`,
		`"type":"get_available_commands"`, `"type":"set_subagent_subscription"`,
	} {
		if !strings.Contains(wire, required) {
			return 63
		}
	}
	frames := []map[string]any{
		{"type": "ready", "protocolVersion": 1, "supportedProtocolVersions": []int{1, 2}},
		{"id": "readiness-negotiate", "type": "response", "command": "negotiate_protocol", "success": true, "data": map[string]any{"protocolVersion": 2}},
		{"id": "readiness-state", "type": "response", "command": "get_state", "success": true, "data": map[string]any{
			"sessionId": "provider-free-readiness", "isStreaming": false, "isCompacting": false,
			"messageCount": 0, "queuedMessageCount": 0,
			"dumpTools": []any{
				map[string]any{"name": "task", "parameters": map[string]any{"type": "object", "properties": map[string]any{
					"context": map[string]any{"type": "string"},
					"tasks":   map[string]any{"type": "array"},
				}}},
				map[string]any{"name": "hub", "parameters": map[string]any{"type": "object"}},
				map[string]any{"name": "todo", "parameters": map[string]any{"type": "object"}},
			},
		}},
		{"id": "readiness-commands", "type": "response", "command": "get_available_commands", "success": true,
			"data": map[string]any{"commands": []any{map[string]any{"name": "auto", "source": "project"}}}},
		{"id": "readiness-subscribe", "type": "response", "command": "set_subagent_subscription", "success": true},
		{"id": "readiness-unsubscribe", "type": "response", "command": "set_subagent_subscription", "success": true},
	}
	for _, frame := range frames {
		encoded, err := json.Marshal(frame)
		if err != nil {
			return 64
		}
		fmt.Println(string(encoded))
	}
	log, _ := os.OpenFile(logPath, os.O_APPEND|os.O_WRONLY, 0o600)
	_, _ = fmt.Fprintf(log, "provider-requests=0 emitted-frames=%d\n", len(frames))
	_ = log.Close()
	return 0
}
