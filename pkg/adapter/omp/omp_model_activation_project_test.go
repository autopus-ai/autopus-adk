package omp

import (
	"io/fs"
	"strings"
	"testing"

	"github.com/insajin/autopus-adk/pkg/config"
)

func TestMergeOMPProjectManagedConfig_PreservesUnmanagedBytesAndMode(t *testing.T) {
	t.Parallel()
	existing := []byte("# user header\nmodel: ${MODEL_ID}\ndisabledProviders:\n  - old\n# keep tail\nunknown: keep\n")
	prior, err := FingerprintOMPManagedValue([]any{"old"})
	if err != nil {
		t.Fatal(err)
	}
	result, err := MergeOMPProjectManagedConfig(OMPProjectManagedInput{
		Existing: existing,
		Mode:     fs.FileMode(0o640),
		Claims: []OMPManagedKeyClaim{{
			Path:               "disabledProviders",
			Value:              []any{"openai", "groq"},
			Complete:           true,
			FullArrayOwnership: true,
			PriorFingerprint:   prior,
		}},
	})
	if err != nil {
		t.Fatalf("merge project config: %v", err)
	}
	if result.Mode.Perm() != 0o640 {
		t.Fatalf("mode = %o, want 0640", result.Mode.Perm())
	}
	got := string(result.Bytes)
	for _, preserved := range []string{"# user header\nmodel: ${MODEL_ID}\n", "# keep tail\nunknown: keep\n"} {
		if !strings.Contains(got, preserved) {
			t.Fatalf("unmanaged bytes not preserved: %q\n%s", preserved, got)
		}
	}
	if !strings.Contains(got, "disabledProviders:\n  - openai\n  - groq\n") || strings.Contains(got, "- old") {
		t.Fatalf("array replacement mismatch:\n%s", got)
	}
	if result.ManagedFingerprints["disabledProviders"] == "" {
		t.Fatalf("managed fingerprint missing: %+v", result.ManagedFingerprints)
	}
}

func TestMergeOMPProjectManagedConfig_FailuresReturnOriginalBytes(t *testing.T) {
	t.Parallel()
	existing := []byte("disabledProviders:\n  - old\n")
	validPrior, err := FingerprintOMPManagedValue([]any{"old"})
	if err != nil {
		t.Fatal(err)
	}
	cases := []struct {
		name   string
		input  []byte
		claims []OMPManagedKeyClaim
	}{
		{
			name:  "prior conflict",
			input: existing,
			claims: []OMPManagedKeyClaim{{Path: "disabledProviders", Value: []any{"new"}, Complete: true,
				FullArrayOwnership: true, PriorFingerprint: OMPMissingManagedValueFingerprint()}},
		},
		{
			name:  "array ownership",
			input: existing,
			claims: []OMPManagedKeyClaim{{Path: "disabledProviders", Value: []any{"new"}, Complete: true,
				PriorFingerprint: validPrior}},
		},
		{
			name:  "duplicate key",
			input: []byte("task: {}\ntask: {}\n"),
			claims: []OMPManagedKeyClaim{{Path: config.OMPNativeAgentModelOverridesKey,
				Value: map[string]any{"task": "p/m"}, Complete: true, PriorFingerprint: validPrior}},
		},
		{
			name:  "incomplete claim",
			input: []byte("task: {}\n"),
			claims: []OMPManagedKeyClaim{{Path: config.OMPNativeAgentModelOverridesKey,
				Value: map[string]any{"task": "p/m"}, PriorFingerprint: validPrior}},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			result, mergeErr := MergeOMPProjectManagedConfig(OMPProjectManagedInput{Existing: tc.input, Mode: 0o600, Claims: tc.claims})
			if mergeErr == nil {
				t.Fatal("expected merge error")
			}
			if string(result.Bytes) != string(tc.input) || result.Changed {
				t.Fatalf("failed transaction mutated bytes: %+v", result)
			}
		})
	}
}

func TestMergeOMPProjectManagedConfig_InsertsClaimWithMissingFingerprint(t *testing.T) {
	t.Parallel()
	existing := []byte("# user\nunknown: keep\n")
	result, err := MergeOMPProjectManagedConfig(OMPProjectManagedInput{
		Existing: existing,
		Mode:     0o600,
		Claims: []OMPManagedKeyClaim{{
			Path:             config.OMPNativeAgentModelOverridesKey,
			Value:            map[string]any{"task": "p/m", "sonic": "q/n"},
			Complete:         true,
			PriorFingerprint: OMPMissingManagedValueFingerprint(),
		}},
	})
	if err != nil {
		t.Fatalf("insert claimed key: %v", err)
	}
	if !strings.HasPrefix(string(result.Bytes), string(existing)) {
		t.Fatalf("existing bytes changed:\n%s", result.Bytes)
	}
	if !strings.Contains(string(result.Bytes), "task:\n  agentModelOverrides:\n    sonic: q/n\n    task: p/m\n") {
		t.Fatalf("inserted mapping is not canonical:\n%s", result.Bytes)
	}
}
