package omp

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestCompileOMPModelOverlay_RejectsUnsafeProjection(t *testing.T) {
	t.Parallel()
	cases := []OMPModelOverlayProjection{
		{AgentModelOverrides: map[string]string{" task": "p/m"}},
		{AgentModelOverrides: map[string]string{"task": "p/m\nother"}},
		{AgentModelOverrides: map[string]string{"task": "p/m"},
			FallbackChains: map[string][]string{"p/m\n": {"q/f"}}},
		{AgentModelOverrides: map[string]string{"task": "p/m"},
			FallbackChains: map[string][]string{"p/m": {"q/f\n"}}},
	}
	for index, projection := range cases {
		if _, err := CompileOMPModelOverlay(projection); err == nil {
			t.Fatalf("case %d: expected invalid projection error", index)
		}
	}
}

func TestWriteOMPModelOverlay_DefaultPathAndCompileFailure(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	evidence, err := WriteOMPModelOverlay(OMPModelOverlayWriteInput{
		WorkspaceRoot: root,
		Projection:    OMPModelOverlayProjection{AgentModelOverrides: map[string]string{"task": "p/m"}},
	})
	if err != nil {
		t.Fatalf("write default overlay: %v", err)
	}
	if evidence.RelativePath != DefaultOMPModelOverlayPath {
		t.Fatalf("relative path = %q", evidence.RelativePath)
	}
	if _, err := WriteOMPModelOverlay(OMPModelOverlayWriteInput{WorkspaceRoot: root}); err == nil {
		t.Fatal("expected overlay compile failure")
	}
}

func TestOMPModelOwnedPath_RejectsEscapesAndSymlinkTargets(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	outside := t.TempDir()
	for _, relative := range []string{"../escape.yml", filepath.Join(outside, "absolute.yml")} {
		if _, err := resolveOMPModelOwnedPath(root, relative, true); err == nil {
			t.Fatalf("expected path rejection for %q", relative)
		}
	}
	target := filepath.Join(root, "target.yml")
	if err := os.Symlink(filepath.Join(outside, "outside.yml"), target); err != nil {
		t.Skipf("symlink unavailable: %v", err)
	}
	if _, err := resolveOMPModelOwnedPath(root, "target.yml", true); err == nil {
		t.Fatal("expected symlink target rejection")
	}
}

func TestVerifyOMPModelActivation_RejectsMissingInputsAndOversizedOverlay(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	if _, err := VerifyOMPModelActivation(context.Background(), nil, OMPModelActivationRequest{}); err == nil {
		t.Fatal("expected nil runner rejection")
	}
	missing := OMPModelActivationRequest{
		WorkspaceRoot: root, OverlayRelativePath: DefaultOMPModelOverlayPath,
		InvocationArgv: []string{"--config", filepath.Join(root, DefaultOMPModelOverlayPath)},
		ReadbackArgv:   []string{"--config", filepath.Join(root, DefaultOMPModelOverlayPath)},
	}
	if _, err := VerifyOMPModelActivation(context.Background(), &modelActivationFakeRunner{}, missing); err == nil {
		t.Fatal("expected missing overlay rejection")
	}
	path, err := resolveOMPModelOwnedPath(root, DefaultOMPModelOverlayPath, true)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, make([]byte, (4<<20)+1), 0o600); err != nil {
		t.Fatal(err)
	}
	missing.ExpectedConfigHash = OMPModelSHA256([]byte("unused"))
	missing.ExpectedReadbackHash = OMPModelSHA256(nil)
	if _, err := VerifyOMPModelActivation(context.Background(), &modelActivationFakeRunner{}, missing); err == nil || !strings.Contains(err.Error(), "size limit") {
		t.Fatalf("expected oversized overlay rejection, got %v", err)
	}
}

func TestMergeOMPProjectManagedConfig_ValidatesClaimGraphAndYAMLShape(t *testing.T) {
	t.Parallel()
	missing := OMPMissingManagedValueFingerprint()
	cases := []OMPProjectManagedInput{
		{Existing: []byte("a: b\n"), Mode: 0o600},
		{Existing: []byte("a: b\n"), Mode: 0o600, Claims: []OMPManagedKeyClaim{{Path: ".bad", Complete: true, PriorFingerprint: missing, Value: "x"}}},
		{Existing: []byte("a: b\n"), Mode: 0o600, Claims: []OMPManagedKeyClaim{{Path: "a", Complete: true, PriorFingerprint: missing, Value: "x"}, {Path: "a.b", Complete: true, PriorFingerprint: missing, Value: "y"}}},
		{Existing: []byte("a: b\n---\nc: d\n"), Mode: 0o600, Claims: []OMPManagedKeyClaim{{Path: "c", Complete: true, PriorFingerprint: missing, Value: "x"}}},
		{Existing: []byte("a: &base {x: y}\nb: *base\n"), Mode: 0o600, Claims: []OMPManagedKeyClaim{{Path: "a", Complete: true, PriorFingerprint: missing, Value: "x"}}},
		{Existing: []byte("retry: {}\n"), Mode: 0o600, Claims: []OMPManagedKeyClaim{{Path: "retry.fallbackChains", Complete: true, PriorFingerprint: missing, Value: map[string]any{}}}},
	}
	for index, input := range cases {
		result, err := MergeOMPProjectManagedConfig(input)
		if err == nil {
			t.Fatalf("case %d: expected fail-closed merge", index)
		}
		if string(result.Bytes) != string(input.Existing) || result.Changed {
			t.Fatalf("case %d: failed merge mutated input", index)
		}
	}
}

func TestFingerprintOMPManagedValue_RejectsUnsupportedType(t *testing.T) {
	t.Parallel()
	if _, err := FingerprintOMPManagedValue(make(chan int)); err == nil {
		t.Fatal("expected unsupported managed value rejection")
	}
}

func TestMergeOMPProjectManagedConfig_NestedInsertionIsDeterministic(t *testing.T) {
	t.Parallel()
	existing := []byte("model: keep\n")
	result, err := MergeOMPProjectManagedConfig(OMPProjectManagedInput{
		Existing: existing,
		Mode:     0o600,
		Claims: []OMPManagedKeyClaim{{
			Path:               "retry.fallbackChains",
			Complete:           true,
			PriorFingerprint:   OMPMissingManagedValueFingerprint(),
			Value:              map[string]any{"p/m": []any{"q/f"}},
			FullArrayOwnership: true,
		}},
	})
	if err != nil {
		t.Fatalf("nested insertion: %v", err)
	}
	wantTail := "retry:\n  fallbackChains:\n    p/m:\n      - q/f\n"
	if !strings.HasPrefix(string(result.Bytes), string(existing)) || !strings.HasSuffix(string(result.Bytes), wantTail) {
		t.Fatalf("nested insertion mismatch:\n%s", result.Bytes)
	}
}

func TestCanonicalOMPModelResolutionReceipt_RejectsUnsafeEvidence(t *testing.T) {
	t.Parallel()
	base := modelReceiptFixture(time.Now().UTC())
	cases := []func(*OMPModelResolutionReceipt){
		func(r *OMPModelResolutionReceipt) { r.OMPVersion = "other/1" },
		func(r *OMPModelResolutionReceipt) { r.CatalogFingerprint = "sha256:bad" },
		func(r *OMPModelResolutionReceipt) { r.Activation.ConfigHash = "bad" },
		func(r *OMPModelResolutionReceipt) { r.Activation.Argv = []string{"omp", "/tmp/private/config.yml"} },
		func(r *OMPModelResolutionReceipt) { r.Roles[0].Selector = "mismatch/model" },
		func(r *OMPModelResolutionReceipt) {
			r.Roles[0].FallbackAttempts = []OMPModelFallbackAttemptReceipt{{Selector: "q/f"}}
		},
		func(r *OMPModelResolutionReceipt) { r.Roles[0].DegradedReason = "bearer credential" },
	}
	for index, mutate := range cases {
		receipt := base
		receipt.Roles = append([]OMPModelRoleReceipt(nil), base.Roles...)
		mutate(&receipt)
		if _, _, err := CanonicalOMPModelResolutionReceipt(receipt); err == nil {
			t.Fatalf("case %d: expected unsafe receipt rejection", index)
		}
	}
}

func TestWriteOMPModelResolutionReceipt_FillsGeneratedAt(t *testing.T) {
	t.Parallel()
	receipt := modelReceiptFixture(time.Time{})
	evidence, err := WriteOMPModelResolutionReceipt(OMPModelReceiptWriteInput{WorkspaceRoot: t.TempDir(), Receipt: receipt})
	if err != nil {
		t.Fatalf("write receipt with generated time: %v", err)
	}
	if evidence.Receipt.GeneratedAt.IsZero() {
		t.Fatal("writer did not fill generated_at")
	}
}
