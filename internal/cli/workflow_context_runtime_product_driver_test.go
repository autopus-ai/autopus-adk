package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	ompadapter "github.com/insajin/autopus-adk/pkg/adapter/omp"
	"github.com/insajin/autopus-adk/pkg/config"
)

func TestWorkflowContextManagedRPCProduct_ArgsSeparateProjectFromTaskRuntime(t *testing.T) {
	options := managedProductTestOptions(t)
	args := workflowContextManagedRPCArgs(options)

	for flag, want := range map[string]string{
		"--mode":          "rpc",
		"--cwd":           options.ProjectDir,
		"--tools":         "read,bash,edit,write,grep,glob,task,todo",
		"--skills":        "auto,auto-go",
		"--approval-mode": "write",
	} {
		if got, ok := managedProductArgValue(args, flag); !ok || got != want {
			t.Fatalf("%s = %q, %v; want %q, true (args=%q)", flag, got, ok, want, args)
		}
	}
	if !managedProductHasArg(args, "--no-session") {
		t.Fatalf("production args must disable session persistence: %q", args)
	}
	if !managedProductHasArg(args, "--no-rules") {
		t.Fatalf("production args must disable ambient project rules: %q", args)
	}
	for _, forbidden := range []string{"--no-tools", "--no-skills"} {
		if managedProductHasArg(args, forbidden) {
			t.Fatalf("production args contain %s: %q", forbidden, args)
		}
	}

	canary := options
	canary.Prompts = nil
	canaryArgs := workflowContextManagedRPCArgs(canary)
	for _, required := range []string{"--no-tools", "--no-skills", "--no-rules"} {
		if !managedProductHasArg(canaryArgs, required) {
			t.Fatalf("canary args must retain %s: %q", required, canaryArgs)
		}
	}
}

func TestWorkflowContextManagedRPCProduct_Validation(t *testing.T) {
	t.Run("valid", func(t *testing.T) {
		t.Parallel()
		if _, err := validateWorkflowContextManagedRPCOptions(managedProductTestOptions(t)); err != nil {
			t.Fatalf("valid product options rejected: %v", err)
		}
	})

	tests := []struct {
		name  string
		setup func(*testing.T, *WorkflowContextManagedRPCOptions)
	}{
		{name: "missing prompt", setup: func(_ *testing.T, options *WorkflowContextManagedRPCOptions) {
			options.Prompts = nil
		}},
		{name: "first prompt is not auto", setup: func(_ *testing.T, options *WorkflowContextManagedRPCOptions) {
			options.Prompts = []string{"review SPEC-OMP-004"}
		}},
		{name: "project under runtime root", setup: func(t *testing.T, options *WorkflowContextManagedRPCOptions) {
			options.ProjectDir = filepath.Join(options.RuntimeRoot, "ambient-project")
			managedProductMkdir(t, options.ProjectDir)
		}},
		{name: "ambient project extension", setup: func(t *testing.T, options *WorkflowContextManagedRPCOptions) {
			directory := filepath.Join(options.ProjectDir, ".omp", "extensions")
			managedProductMkdir(t, directory)
			managedProductWriteFile(t, filepath.Join(directory, "ambient.ts"), []byte("export {}\n"), 0o600)
		}},
		{name: "command source drift", setup: func(t *testing.T, options *WorkflowContextManagedRPCOptions) {
			path := filepath.Join(options.ProjectDir, ".omp", "commands", "auto-go.md")
			managedProductWriteFile(t, path, []byte("shadow command\n"), 0o600)
		}},
		{name: "skill source drift", setup: func(t *testing.T, options *WorkflowContextManagedRPCOptions) {
			path := filepath.Join(options.ProjectDir, ".omp", "skills", "auto", "SKILL.md")
			managedProductWriteFile(t, path, []byte("shadow skill\n"), 0o600)
		}},
		{name: "extra native OMP command", setup: func(t *testing.T, options *WorkflowContextManagedRPCOptions) {
			path := filepath.Join(options.ProjectDir, ".omp", "commands", "shadow.md")
			managedProductWriteFile(t, path, []byte("shadow command\n"), 0o600)
		}},
		{name: "native OMP system shadow", setup: func(t *testing.T, options *WorkflowContextManagedRPCOptions) {
			path := filepath.Join(options.ProjectDir, ".omp", "SYSTEM.md")
			managedProductWriteFile(t, path, []byte("shadow system\n"), 0o600)
		}},
		{name: "native OMP hook shadow", setup: func(t *testing.T, options *WorkflowContextManagedRPCOptions) {
			managedProductMkdir(t, filepath.Join(options.ProjectDir, ".omp", "hooks", "pre"))
		}},
		{name: "native OMP tool shadow", setup: func(t *testing.T, options *WorkflowContextManagedRPCOptions) {
			managedProductMkdir(t, filepath.Join(options.ProjectDir, ".omp", "tools"))
		}},
		{name: "native OMP plugin shadow", setup: func(t *testing.T, options *WorkflowContextManagedRPCOptions) {
			managedProductMkdir(t, filepath.Join(options.ProjectDir, ".omp", "plugins"))
		}},
		{name: "native OMP settings extension shadow", setup: func(t *testing.T, options *WorkflowContextManagedRPCOptions) {
			path := filepath.Join(options.ProjectDir, ".omp", "settings.json")
			managedProductWriteFile(t, path, []byte(`{"extensions":["../evil.ts"]}`), 0o600)
		}},
		{name: "native OMP config shadow", setup: func(t *testing.T, options *WorkflowContextManagedRPCOptions) {
			path := filepath.Join(options.ProjectDir, ".omp", "config.yml")
			managedProductWriteFile(t, path, []byte("extensions: [../evil.ts]\n"), 0o600)
		}},
		{name: "Agents prompt shadow", setup: func(t *testing.T, options *WorkflowContextManagedRPCOptions) {
			managedProductMkdir(t, filepath.Join(options.ProjectDir, ".agents", "prompts"))
		}},
		{name: "legacy pi provider shadow", setup: func(t *testing.T, options *WorkflowContextManagedRPCOptions) {
			managedProductMkdir(t, filepath.Join(options.ProjectDir, ".pi", "extensions"))
		}},
		{name: "Claude extension shadow", setup: func(t *testing.T, options *WorkflowContextManagedRPCOptions) {
			managedProductMkdir(t, filepath.Join(options.ProjectDir, ".claude", "extensions"))
		}},
		{name: "Claude tool shadow", setup: func(t *testing.T, options *WorkflowContextManagedRPCOptions) {
			managedProductMkdir(t, filepath.Join(options.ProjectDir, ".claude", "tools"))
		}},
		{name: "Codex prompt shadow", setup: func(t *testing.T, options *WorkflowContextManagedRPCOptions) {
			managedProductMkdir(t, filepath.Join(options.ProjectDir, ".codex", "prompts"))
		}},
		{name: "Gemini system shadow", setup: func(t *testing.T, options *WorkflowContextManagedRPCOptions) {
			path := filepath.Join(options.ProjectDir, ".gemini", "system.md")
			managedProductMkdir(t, filepath.Dir(path))
			managedProductWriteFile(t, path, []byte("shadow system\n"), 0o600)
		}},
		{name: "OpenCode plugin shadow", setup: func(t *testing.T, options *WorkflowContextManagedRPCOptions) {
			managedProductMkdir(t, filepath.Join(options.ProjectDir, ".opencode", "plugins"))
		}},
		{name: "GitHub prompt shadow", setup: func(t *testing.T, options *WorkflowContextManagedRPCOptions) {
			managedProductMkdir(t, filepath.Join(options.ProjectDir, ".github", "prompts"))
		}},
		{name: "ambient parent symlink", setup: func(t *testing.T, options *WorkflowContextManagedRPCOptions) {
			target := filepath.Join(t.TempDir(), "claude")
			managedProductMkdir(t, target)
			if err := os.Symlink(target, filepath.Join(options.ProjectDir, ".claude")); err != nil {
				t.Fatalf("symlink ambient parent: %v", err)
			}
		}},
	}
	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			options := managedProductTestOptions(t)
			test.setup(t, &options)
			if _, err := validateWorkflowContextManagedRPCOptions(options); err == nil {
				t.Fatal("invalid product options were accepted")
			}
		})
	}
}

func managedProductTestOptions(t *testing.T) WorkflowContextManagedRPCOptions {
	t.Helper()
	base := filepath.Join(t.TempDir(), "task")
	workspace := filepath.Join(base, "workspace")
	runtimeRoot := filepath.Join(base, "omp-runtime")
	sessionDir := filepath.Join(runtimeRoot, "sessions", "product")
	projectDir := filepath.Join(t.TempDir(), "repo")
	for _, directory := range []string{base, workspace, runtimeRoot, sessionDir, projectDir} {
		managedProductMkdir(t, directory)
	}
	configPath := filepath.Join(runtimeRoot, "config.json")
	managedProductWriteFile(t, configPath, []byte("{}\n"), 0o600)
	const credentialKey = "AUTOPUS_OMP_CONTEXT_PROVIDER_TOKEN"
	models := fmt.Sprintf(`providers:
  fake:
    baseUrl: http://127.0.0.1:1/v1
    apiKey: %s
    authHeader: true
    api: openai-completions
    models:
      - id: product
`, credentialKey)
	managedProductWriteFile(t, filepath.Join(runtimeRoot, "models.yml"), []byte(models), 0o600)
	executable := filepath.Join(base, "omp")
	managedProductWriteFile(t, executable, []byte("#!/bin/sh\nexit 0\n"), 0o700)
	installWorkflowContextManagedLiveBridge(t, workspace)
	projectConfig := config.DefaultFullConfig("managed-context-product")
	projectConfig.Platforms = []string{"omp"}
	projectConfig.OMPContextPolicy = config.OMPContextPolicyConf{
		Profile: "managed", Profiles: map[string]config.OMPContextProfileConf{"managed": {}},
	}
	_, err := ompadapter.NewWithRoot(workspace).Generate(context.Background(), projectConfig)
	if err != nil {
		t.Fatalf("generate product task surface: %v", err)
	}
	_, err = ompadapter.NewWithRoot(projectDir).Generate(context.Background(), projectConfig)
	if err != nil {
		t.Fatalf("generate product project surface: %v", err)
	}
	return WorkflowContextManagedRPCOptions{
		Executable: executable, ProjectDir: projectDir, Workspace: workspace,
		RuntimeBase: base, RuntimeRoot: runtimeRoot, SessionDir: sessionDir,
		ConfigPath: configPath, Model: "fake/product", AllowedEndpoint: "http://127.0.0.1:1",
		Environment: []string{
			"PI_CODING_AGENT_DIR=" + runtimeRoot, "PI_CONFIG_FILES=" + configPath,
			credentialKey + "=task-owned-test-token",
		},
		MaxTime: time.Second, Prompts: []string{
			"/auto go SPEC-OMP-004 --auto", "continue with the authoritative task",
		},
	}
}

func managedProductMkdir(t *testing.T, path string) {
	t.Helper()
	if err := os.MkdirAll(path, 0o700); err != nil {
		t.Fatalf("mkdir %s: %v", path, err)
	}
	if err := os.Chmod(path, 0o700); err != nil {
		t.Fatalf("chmod %s: %v", path, err)
	}
}

func managedProductWriteFile(t *testing.T, path string, body []byte, mode os.FileMode) {
	t.Helper()
	if err := os.WriteFile(path, body, mode); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
	if err := os.Chmod(path, mode); err != nil {
		t.Fatalf("chmod %s: %v", path, err)
	}
}

func managedProductArgValue(args []string, flag string) (string, bool) {
	for index := 0; index+1 < len(args); index++ {
		if args[index] == flag {
			return args[index+1], true
		}
	}
	return "", false
}

func managedProductHasArg(args []string, target string) bool {
	for _, arg := range args {
		if arg == target {
			return true
		}
	}
	return false
}

func managedProductPushFrame(t *testing.T, frames chan<- []byte, frame workflowContextManagedRPCFrame) {
	t.Helper()
	encoded, err := json.Marshal(frame)
	if err != nil {
		t.Fatalf("encode frame: %v", err)
	}
	frames <- encoded
}

func managedProductBridgeFrame(
	t *testing.T, id, event string, binding WorkflowContextBridgeBinding,
) workflowContextManagedRPCFrame {
	t.Helper()
	envelope, err := json.Marshal(workflowContextManagedBridgeEnvelope{
		SchemaVersion: binding.SchemaVersion, Event: event,
		BindingHash: binding.BindingHash, OptionsHash: binding.OptionsHash,
		SessionHash: binding.SessionHash, NonceHash: binding.NonceHash,
	})
	if err != nil {
		t.Fatalf("encode bridge envelope: %v", err)
	}
	message, err := json.Marshal(string(envelope))
	if err != nil {
		t.Fatalf("encode bridge message: %v", err)
	}
	return workflowContextManagedRPCFrame{
		ID: id, Type: "extension_ui_request", Method: "confirm",
		Title: "Autopus context " + event, Message: message,
	}
}
