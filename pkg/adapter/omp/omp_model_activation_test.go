package omp

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type modelActivationFakeRunner struct {
	output     []byte
	args       [][]string
	err        error
	beforeRun  func()
	readConfig bool
	configRead []byte
}

func (r *modelActivationFakeRunner) Run(_ context.Context, args []string) ([]byte, error) {
	r.args = append(r.args, append([]string(nil), args...))
	if r.beforeRun != nil {
		r.beforeRun()
	}
	if r.readConfig {
		for i := 0; i+1 < len(args); i++ {
			if args[i] != "--config" {
				continue
			}
			data, err := os.ReadFile(args[i+1])
			if err != nil {
				return nil, err
			}
			r.configRead = data
			break
		}
	}
	if r.err != nil {
		return nil, r.err
	}
	return append([]byte(nil), r.output...), nil
}

func TestVerifyOMPModelActivation_RejectsAncestorSwapWithoutReadingOutside(t *testing.T) {
	container := t.TempDir()
	root := filepath.Join(container, "workspace")
	require.NoError(t, os.Mkdir(root, 0o700))
	evidence, err := WriteOMPModelOverlay(OMPModelOverlayWriteInput{
		WorkspaceRoot: root,
		Projection:    OMPModelOverlayProjection{AgentModelOverrides: map[string]string{"task": "p/m"}},
	})
	require.NoError(t, err)
	moved := filepath.Join(container, "moved")
	outside := t.TempDir()
	external := []byte("external sentinel\n")
	mustWriteOMPTestFile(t, filepath.Join(outside, DefaultOMPModelOverlayPath), external, 0o600)
	readback := []byte(`{"task.agentModelOverrides":{"task":"p/m"}}`)
	runner := &modelActivationFakeRunner{output: readback, readConfig: true, beforeRun: func() {
		require.NoError(t, os.Rename(root, moved))
		require.NoError(t, os.Symlink(outside, root))
	}}
	overlayPath := filepath.Join(root, filepath.FromSlash(DefaultOMPModelOverlayPath))
	request := OMPModelActivationRequest{
		WorkspaceRoot: root, OverlayRelativePath: DefaultOMPModelOverlayPath,
		InvocationArgv:     []string{"--config", overlayPath, "prompt"},
		ReadbackArgv:       []string{"--config", overlayPath, "config", "get", "task.agentModelOverrides"},
		ExpectedConfigHash: evidence.ConfigHash, ExpectedReadbackHash: OMPModelSHA256(readback),
	}

	_, err = VerifyOMPModelActivation(context.Background(), runner, request)
	require.ErrorContains(t, err, "workspace changed")
	assert.Equal(t, evidence.Bytes, runner.configRead, "child must read the pinned owner-only snapshot")
	assert.NotEqual(t, external, runner.configRead)
	assert.Equal(t, external, mustReadOMPReviewFile(t, filepath.Join(outside, DefaultOMPModelOverlayPath)))
	assert.Equal(t, evidence.Bytes, mustReadOMPReviewFile(t, filepath.Join(moved, DefaultOMPModelOverlayPath)))
}

func TestCompileOMPModelOverlay_MapOrderIsDeterministic(t *testing.T) {
	t.Parallel()
	projection := OMPModelOverlayProjection{
		AgentModelOverrides: map[string]string{"task": "p/z", "sonic": "p/a"},
		FallbackChains: map[string][]string{
			"p/z": {"q/b", "r/c"},
			"p/a": {"q/a"},
		},
	}

	first, err := CompileOMPModelOverlay(projection)
	if err != nil {
		t.Fatalf("compile first overlay: %v", err)
	}
	for i := 0; i < 100; i++ {
		got, compileErr := CompileOMPModelOverlay(projection)
		if compileErr != nil {
			t.Fatalf("compile overlay %d: %v", i, compileErr)
		}
		if string(got) != string(first) {
			t.Fatalf("overlay changed at iteration %d\nfirst:\n%s\ngot:\n%s", i, first, got)
		}
	}
	want := "task:\n  agentModelOverrides:\n    sonic: p/a\n    task: p/z\n" +
		"retry:\n  fallbackChains:\n    p/a:\n      - q/a\n    p/z:\n      - q/b\n      - r/c\n"
	if string(first) != want {
		t.Fatalf("canonical overlay mismatch\nwant:\n%s\ngot:\n%s", want, first)
	}
}

func TestWriteOMPModelOverlay_PreservesProjectConfigAndUses0600(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	projectConfig := filepath.Join(root, ".omp", "config.yml")
	if err := os.MkdirAll(filepath.Dir(projectConfig), 0o755); err != nil {
		t.Fatal(err)
	}
	original := []byte("# user\nmodel: ${MODEL_ID}\nunknown: keep\n")
	if err := os.WriteFile(projectConfig, original, 0o640); err != nil {
		t.Fatal(err)
	}

	evidence, err := WriteOMPModelOverlay(OMPModelOverlayWriteInput{
		WorkspaceRoot: root,
		RelativePath:  DefaultOMPModelOverlayPath,
		Projection: OMPModelOverlayProjection{
			AgentModelOverrides: map[string]string{"task": "p/m"},
			FallbackChains:      map[string][]string{"p/m": {"q/f"}},
		},
	})
	if err != nil {
		t.Fatalf("write overlay: %v", err)
	}
	if evidence.ConfigHash != OMPModelSHA256(evidence.Bytes) {
		t.Fatalf("config hash mismatch: %q", evidence.ConfigHash)
	}
	gotProject, err := os.ReadFile(projectConfig)
	if err != nil {
		t.Fatal(err)
	}
	if string(gotProject) != string(original) {
		t.Fatalf("project config changed\nwant: %q\n got: %q", original, gotProject)
	}
	info, err := os.Stat(projectConfig)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o640 {
		t.Fatalf("project config mode changed: %o", info.Mode().Perm())
	}
	overlayInfo, err := os.Stat(filepath.Join(root, DefaultOMPModelOverlayPath))
	if err != nil {
		t.Fatal(err)
	}
	if overlayInfo.Mode().Perm() != 0o600 {
		t.Fatalf("overlay mode = %o, want 0600", overlayInfo.Mode().Perm())
	}
}

func TestWriteOMPModelOverlay_RejectsSymlinkedParent(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	outside := t.TempDir()
	if err := os.Symlink(outside, filepath.Join(root, ".autopus")); err != nil {
		t.Skipf("symlink unavailable: %v", err)
	}
	_, err := WriteOMPModelOverlay(OMPModelOverlayWriteInput{
		WorkspaceRoot: root,
		RelativePath:  DefaultOMPModelOverlayPath,
		Projection:    OMPModelOverlayProjection{AgentModelOverrides: map[string]string{"task": "p/m"}},
	})
	if err == nil {
		t.Fatal("expected symlink containment failure")
	}
	if _, statErr := os.Stat(filepath.Join(outside, "runtime", "omp-model-routing-v1.yml")); !os.IsNotExist(statErr) {
		t.Fatalf("outside path was written: %v", statErr)
	}
}

func TestVerifyOMPModelActivation_RequiresExactOverlayAndReadback(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	evidence, err := WriteOMPModelOverlay(OMPModelOverlayWriteInput{
		WorkspaceRoot: root,
		RelativePath:  DefaultOMPModelOverlayPath,
		Projection:    OMPModelOverlayProjection{AgentModelOverrides: map[string]string{"task": "p/m"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	overlay := filepath.Join(root, DefaultOMPModelOverlayPath)
	readback := []byte(`{"task.agentModelOverrides":{"task":"p/m"},"retry":{"fallbackChains":{}}}`)
	runner := &modelActivationFakeRunner{output: readback}
	request := OMPModelActivationRequest{
		WorkspaceRoot:        root,
		OverlayRelativePath:  DefaultOMPModelOverlayPath,
		InvocationArgv:       []string{"--config", overlay, "prompt"},
		ReadbackArgv:         []string{"--config", overlay, "config", "get", "task.agentModelOverrides"},
		ExpectedConfigHash:   evidence.ConfigHash,
		ExpectedReadbackHash: OMPModelSHA256(readback),
	}

	activation, err := VerifyOMPModelActivation(context.Background(), runner, request)
	if err != nil {
		t.Fatalf("verify activation: %v", err)
	}
	if activation.ConfigHash != evidence.ConfigHash || activation.ReadbackHash != request.ExpectedReadbackHash {
		t.Fatalf("unexpected activation evidence: %+v", activation)
	}
	if len(runner.args) != 1 {
		t.Fatalf("runner calls = %d, want 1", len(runner.args))
	}

	cases := []struct {
		name   string
		mutate func(*OMPModelActivationRequest, *modelActivationFakeRunner)
	}{
		{"overlay omitted", func(r *OMPModelActivationRequest, _ *modelActivationFakeRunner) {
			r.InvocationArgv = []string{"prompt"}
		}},
		{"wrong overlay", func(r *OMPModelActivationRequest, _ *modelActivationFakeRunner) {
			r.ReadbackArgv[1] = filepath.Join(root, "other.yml")
		}},
		{"config hash", func(r *OMPModelActivationRequest, _ *modelActivationFakeRunner) {
			r.ExpectedConfigHash = "sha256:" + string(make([]byte, 64))
		}},
		{"readback hash", func(_ *OMPModelActivationRequest, f *modelActivationFakeRunner) {
			f.output = []byte(`{"different":true}`)
		}},
		{"runner error", func(_ *OMPModelActivationRequest, f *modelActivationFakeRunner) {
			f.err = errors.New("readback failed")
		}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			bad := request
			bad.InvocationArgv = append([]string(nil), request.InvocationArgv...)
			bad.ReadbackArgv = append([]string(nil), request.ReadbackArgv...)
			fake := &modelActivationFakeRunner{output: readback}
			tc.mutate(&bad, fake)
			if _, verifyErr := VerifyOMPModelActivation(context.Background(), fake, bad); verifyErr == nil {
				t.Fatal("expected fail-closed activation error")
			}
		})
	}
}
