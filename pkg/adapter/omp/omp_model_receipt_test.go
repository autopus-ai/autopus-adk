package omp

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func modelReceiptFixture(generatedAt time.Time) OMPModelResolutionReceipt {
	return OMPModelResolutionReceipt{
		OMPVersion:         "omp/17.1.8",
		CatalogFingerprint: "sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
		Profile:            "balanced",
		ConfigSource:       "overlay",
		Activation: OMPModelActivationReceipt{
			Argv:         []string{"omp", "--config", ".autopus/runtime/omp-model-routing-v1.yml"},
			ConfigHash:   "sha256:bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb",
			ReadbackHash: "sha256:cccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccc",
		},
		Roles: []OMPModelRoleReceipt{
			{Agent: "reviewer", Profile: "balanced", ConfigSource: "overlay", RequestedRole: "autopus_reviewer", EffectiveRole: "autopus_reviewer", Capability: "independent_dissent", Provider: "q", Model: "review", Selector: "q/review", Thinking: "high", FamilyDiversity: OMPModelFamilyDiversityReceipt{Status: "satisfied", ExecutorFamily: "p", EffectiveFamily: "q"}, SafetySource: "user_effective"},
			{Agent: "task", Profile: "balanced", ConfigSource: "overlay", RequestedRole: "autopus_planner", EffectiveRole: "autopus_planner", Capability: "deep_reasoning", Provider: "p", Model: "code", Selector: "p/code", Thinking: "medium", FallbackAttempts: []OMPModelFallbackAttemptReceipt{{Selector: "p/old", Reason: "disabled"}}, SafetySource: "user_effective"},
		},
		Safety:      OMPModelSafetyReceipt{ApprovalMode: "write", IsolationMode: "auto", Source: "autopus_profile"},
		GeneratedAt: generatedAt,
	}
}

func TestCanonicalOMPModelResolutionReceipt_DigestBindsGeneratedAt(t *testing.T) {
	t.Parallel()
	first, firstBytes, err := CanonicalOMPModelResolutionReceipt(modelReceiptFixture(time.Date(2026, 8, 2, 1, 2, 3, 0, time.UTC)))
	if err != nil {
		t.Fatalf("canonical first receipt: %v", err)
	}
	second, secondBytes, err := CanonicalOMPModelResolutionReceipt(modelReceiptFixture(time.Date(2026, 8, 3, 4, 5, 6, 0, time.UTC)))
	if err != nil {
		t.Fatalf("canonical second receipt: %v", err)
	}
	if first.ResolutionDigest == second.ResolutionDigest {
		t.Fatalf("generated_at did not change digest: %s", first.ResolutionDigest)
	}
	if string(firstBytes) == string(secondBytes) {
		t.Fatal("canonical receipt bytes must retain generated_at metadata")
	}
	if first.Roles[0].Agent != "reviewer" || first.SchemaVersion != OMPModelReceiptSchemaVersion {
		t.Fatalf("receipt canonicalization mismatch: %+v", first)
	}
	var raw map[string]any
	if err := json.Unmarshal(firstBytes, &raw); err != nil {
		t.Fatal(err)
	}
	wantKeys := []string{"schema_version", "omp_version", "catalog_fingerprint", "catalog_trust", "profile", "config_source", "activation", "roles", "safety", "generated_at", "resolution_digest"}
	if len(raw) != len(wantKeys) {
		t.Fatalf("unexpected top-level fields: %+v", raw)
	}
	for _, key := range wantKeys {
		if _, ok := raw[key]; !ok {
			t.Fatalf("missing allowlisted field %q", key)
		}
	}
}

func TestCanonicalOMPModelResolutionReceipt_RejectsAuthorityDrift(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name   string
		mutate func(*OMPModelResolutionReceipt)
	}{
		{"generated at", func(r *OMPModelResolutionReceipt) { r.GeneratedAt = time.Time{} }},
		{"OMP version", func(r *OMPModelResolutionReceipt) { r.OMPVersion = "other/17" }},
		{"roles", func(r *OMPModelResolutionReceipt) { r.Roles = nil }},
		{"project ownership", func(r *OMPModelResolutionReceipt) {
			r.ConfigSource = "project-managed"
			for index := range r.Roles {
				r.Roles[index].ConfigSource = "project-managed"
			}
		}},
		{"config source", func(r *OMPModelResolutionReceipt) { r.ConfigSource = "ambient" }},
		{"activation hash", func(r *OMPModelResolutionReceipt) { r.Activation.ConfigHash = "bad" }},
		{"activation argv", func(r *OMPModelResolutionReceipt) { r.Activation.Argv = []string{"../omp"} }},
		{"role required value", func(r *OMPModelResolutionReceipt) { r.Roles[0].Agent = " bad " }},
		{"role selector", func(r *OMPModelResolutionReceipt) { r.Roles[0].Selector = "other/model" }},
		{"fallback attempt", func(r *OMPModelResolutionReceipt) {
			r.Roles[0].FallbackAttempts = []OMPModelFallbackAttemptReceipt{{Selector: "p/old"}}
		}},
		{"secret-like material", func(r *OMPModelResolutionReceipt) { r.Roles[0].DegradedReason = "password=value" }},
	}
	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			receipt := modelReceiptFixture(time.Date(2026, 8, 3, 4, 5, 6, 0, time.UTC))
			test.mutate(&receipt)
			_, _, err := CanonicalOMPModelResolutionReceipt(receipt)
			if err == nil {
				t.Fatalf("authority drift %q was accepted", test.name)
			}
		})
	}
}

func TestWriteOMPModelResolutionReceipt_Atomic0600AndSecretFree(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	receipt := modelReceiptFixture(time.Date(2026, 8, 2, 1, 2, 3, 0, time.UTC))
	written, err := WriteOMPModelResolutionReceipt(OMPModelReceiptWriteInput{
		WorkspaceRoot:   root,
		Receipt:         receipt,
		ForbiddenValues: []string{"SECRET-SENTINEL"},
	})
	if err != nil {
		t.Fatalf("write receipt: %v", err)
	}
	path := filepath.Join(root, OMPModelReceiptRelativePath)
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("receipt mode = %o, want 0600", info.Mode().Perm())
	}
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != string(written.Bytes) || !strings.HasSuffix(string(got), "\n") {
		t.Fatalf("written receipt mismatch: %q", got)
	}
	if strings.Contains(strings.ToLower(string(got)), "auth") || strings.Contains(string(got), root) {
		t.Fatalf("receipt leaked auth/path material: %s", got)
	}

	receipt.Roles[0].DegradedReason = "SECRET-SENTINEL"
	if _, secretErr := WriteOMPModelResolutionReceipt(OMPModelReceiptWriteInput{WorkspaceRoot: root, Receipt: receipt, ForbiddenValues: []string{"SECRET-SENTINEL"}}); secretErr == nil {
		t.Fatal("expected forbidden secret sentinel rejection")
	}
}

func TestWriteOMPModelResolutionReceipt_RejectsSymlinkedAutopus(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	outside := t.TempDir()
	if err := os.Symlink(outside, filepath.Join(root, ".autopus")); err != nil {
		t.Skipf("symlink unavailable: %v", err)
	}
	_, err := WriteOMPModelResolutionReceipt(OMPModelReceiptWriteInput{WorkspaceRoot: root, Receipt: modelReceiptFixture(time.Now().UTC())})
	if err == nil {
		t.Fatal("expected symlink containment error")
	}
	if _, statErr := os.Stat(filepath.Join(outside, filepath.Base(OMPModelReceiptRelativePath))); !os.IsNotExist(statErr) {
		t.Fatalf("outside receipt was written: %v", statErr)
	}
}

func TestOMPModelResolutionReceipt_ExactGitignoreRule(t *testing.T) {
	t.Parallel()
	data, err := os.ReadFile(filepath.Join("..", "..", "..", ".gitignore"))
	if err != nil {
		t.Fatal(err)
	}
	rule := ".autopus/omp-model-resolution-v1.json"
	count := 0
	for _, line := range strings.Split(string(data), "\n") {
		if line == rule {
			count++
		}
	}
	if count != 1 {
		t.Fatalf("exact receipt ignore rule count = %d, want 1", count)
	}
}
