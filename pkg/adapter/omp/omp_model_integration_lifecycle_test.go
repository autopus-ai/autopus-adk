package omp

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/insajin/autopus-adk/pkg/adapter"
)

func TestOMPModelIntegration_S10S14_OverlayManifestModeAndCleanLifecycle(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	a := NewWithRoot(root).WithModelIntegrationRunner(newModelIntegrationRunner())
	if _, err := a.Generate(context.Background(), integrationHarnessConfig("overlay")); err != nil {
		t.Fatalf("Generate() error: %v", err)
	}
	manifest, err := adapter.LoadManifest(root, adapterName)
	if err != nil || manifest == nil {
		t.Fatalf("load manifest: %v", err)
	}
	for _, path := range []string{DefaultOMPModelOverlayPath, OMPModelReceiptRelativePath} {
		if _, ok := manifest.Files[path]; !ok {
			t.Fatalf("manifest missing %q", path)
		}
		info, statErr := os.Stat(filepath.Join(root, path))
		if statErr != nil || info.Mode().Perm() != 0o600 {
			t.Fatalf("%s mode = %v, err %v", path, info.Mode().Perm(), statErr)
		}
	}
	if err := a.Clean(context.Background()); err != nil {
		t.Fatalf("Clean() error: %v", err)
	}
	for _, path := range []string{DefaultOMPModelOverlayPath, OMPModelReceiptRelativePath} {
		if _, err := os.Stat(filepath.Join(root, path)); !os.IsNotExist(err) {
			t.Fatalf("Clean retained %q: %v", path, err)
		}
	}
}

func TestOMPModelIntegration_S10_ProjectManagedPreservesUnknownBytesAndRequiresClaims(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	path := filepath.Join(root, configFile)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	original := []byte("# user comment\nunknown: ${MODEL_ID}\n")
	if err := os.WriteFile(path, original, 0o640); err != nil {
		t.Fatal(err)
	}
	a := NewWithRoot(root).WithModelIntegrationRunner(newModelIntegrationRunner())
	if _, err := a.Generate(context.Background(), integrationHarnessConfig("project-managed")); err != nil {
		t.Fatalf("project-managed Generate: %v", err)
	}
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(got), string(original)) || !strings.Contains(string(got), "modelFallback: true") {
		t.Fatalf("project config did not preserve/merge bytes:\n%s", got)
	}
	if !strings.Contains(string(got), "agentModelOverrides:") || strings.Contains(string(got), "modelRoles") {
		t.Fatalf("project config did not claim the native override key only:\n%s", got)
	}
	info, err := os.Stat(path)
	if err != nil || info.Mode().Perm() != 0o640 {
		t.Fatalf("project config mode = %v, err %v", info.Mode().Perm(), err)
	}
	manifest, err := adapter.LoadManifest(root, adapterName)
	if err != nil || manifest == nil {
		t.Fatal(err)
	}
	if _, hasOverlay := manifest.Files[DefaultOMPModelOverlayPath]; hasOverlay {
		t.Fatal("project-managed mode emitted overlay")
	}
}

// A user who already binds a model to a bundled agent owns that key: the
// managed claim must refuse it rather than replace their choice.
func TestOMPModelIntegration_ProjectManagedConflictIsByteIdentical(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	path := filepath.Join(root, configFile)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	original := []byte("task:\n  agentModelOverrides:\n    task: user/model:high\nunknown: keep\n")
	if err := os.WriteFile(path, original, 0o640); err != nil {
		t.Fatal(err)
	}
	_, err := NewWithRoot(root).WithModelIntegrationRunner(newModelIntegrationRunner()).
		Generate(context.Background(), integrationHarnessConfig("project-managed"))
	if err == nil || !strings.Contains(err.Error(), "prior fingerprint mismatch") {
		t.Fatalf("Generate() error = %v", err)
	}
	got, readErr := os.ReadFile(path)
	if readErr != nil || string(got) != string(original) {
		t.Fatalf("conflict changed config: %v\n%s", readErr, got)
	}
	assertNoIntegrationArtifacts(t, root)
}

// Leaving project-managed mode must hand .omp/config.yml back to its ledger
// preimage; otherwise the managed keys outlive the pruned ledger and the next
// project-managed apply is rejected as a prior-fingerprint mismatch.
func TestOMPModelIntegration_ProjectManagedToOverlayRestoresPreimageAndRoundTrips(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	path := filepath.Join(root, configFile)
	runner := newModelIntegrationRunner()
	a := NewWithRoot(root).WithModelIntegrationRunner(runner)
	if _, err := a.Generate(context.Background(), integrationHarnessConfig("project-managed")); err != nil {
		t.Fatalf("project-managed Generate: %v", err)
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("project-managed apply did not write %s: %v", configFile, err)
	}

	if _, err := a.Update(context.Background(), integrationHarnessConfig("overlay")); err != nil {
		t.Fatalf("switch to overlay: %v", err)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("overlay switch left %s behind (err=%v)", configFile, err)
	}
	if _, err := os.Stat(filepath.Join(root, OMPModelProjectOwnershipRelativePath)); !os.IsNotExist(err) {
		t.Fatalf("overlay switch kept the ownership ledger (err=%v)", err)
	}

	if _, err := a.Update(context.Background(), integrationHarnessConfig("project-managed")); err != nil {
		t.Fatalf("switch back to project-managed: %v", err)
	}
	got, err := os.ReadFile(path)
	if err != nil || !strings.Contains(string(got), "agentModelOverrides:") {
		t.Fatalf("project-managed re-apply did not restore managed keys: %v\n%s", err, got)
	}
}

func TestOMPModelIntegration_S10_ManagedInvocationReturnsExactConfigWithoutCallingModel(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	runner := newModelIntegrationRunner()
	if _, err := NewWithRoot(root).WithModelIntegrationRunner(runner).
		Generate(context.Background(), integrationHarnessConfig("overlay")); err != nil {
		t.Fatal(err)
	}
	before := runner.modelRequests
	args, err := OMPManagedModelInvocationArgv(root, DefaultOMPModelOverlayPath, []string{"prompt"})
	if err != nil {
		t.Fatal(err)
	}
	if len(args) != 3 || args[0] != "--config" || args[1] != filepath.Join(root, DefaultOMPModelOverlayPath) || args[2] != "prompt" {
		t.Fatalf("managed argv = %v", args)
	}
	if runner.modelRequests != before {
		t.Fatal("argv helper invoked a model")
	}
	if _, err := OMPManagedModelInvocationArgv(root, DefaultOMPModelOverlayPath, []string{"--config", "other"}); err == nil {
		t.Fatal("duplicate --config accepted")
	}
	data, err := os.ReadFile(filepath.Join(root, OMPModelReceiptRelativePath))
	if err != nil {
		t.Fatal(err)
	}
	var receipt OMPModelResolutionReceipt
	if err := json.Unmarshal(data, &receipt); err != nil || receipt.Activation.Argv[1] != DefaultOMPModelOverlayPath {
		t.Fatalf("receipt activation = %#v, err %v", receipt.Activation, err)
	}
}

func TestOMPModelIntegration_PreviewPreparationWritesOnlyTemporaryReadbackRoot(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	files, err := NewWithRoot(root).WithModelIntegrationRunner(newModelIntegrationRunner()).
		prepareFiles(context.Background(), integrationHarnessConfig("overlay"))
	if err != nil || len(files) == 0 {
		t.Fatalf("prepare preview mappings: files=%d err=%v", len(files), err)
	}
	entries, err := os.ReadDir(root)
	if err != nil || len(entries) != 0 {
		t.Fatalf("prepareFiles wrote project root: entries=%v err=%v", entries, err)
	}
}
