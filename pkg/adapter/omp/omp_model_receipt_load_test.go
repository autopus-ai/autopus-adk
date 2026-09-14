package omp

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"
)

func TestLoadOMPModelResolutionReceipt_ValidCanonicalReceiptReturnsDefensiveCopies(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	receipt := writeLoadReceiptOverlay(t, root, modelReceiptFixture(time.Now().UTC()))
	written, err := WriteOMPModelResolutionReceipt(OMPModelReceiptWriteInput{
		WorkspaceRoot: root,
		Receipt:       receipt,
	})
	if err != nil {
		t.Fatalf("write canonical receipt: %v", err)
	}

	first, err := LoadOMPModelResolutionReceipt(root)
	if err != nil {
		t.Fatalf("load canonical receipt: %v", err)
	}
	if !reflect.DeepEqual(first, written.Receipt) {
		t.Fatalf("loaded receipt mismatch:\n got: %#v\nwant: %#v", first, written.Receipt)
	}

	first.Activation.Argv[0] = "mutated"
	mutatedAttempts := 0
	for index := range first.Roles {
		first.Roles[index].Agent = "mutated"
		for attempt := range first.Roles[index].FallbackAttempts {
			first.Roles[index].FallbackAttempts[attempt].Selector = "mutated/model"
			mutatedAttempts++
		}
	}
	if mutatedAttempts == 0 {
		t.Fatal("the fixture must carry a fallback attempt for the alias check to mean anything")
	}
	second, err := LoadOMPModelResolutionReceipt(root)
	if err != nil {
		t.Fatalf("reload canonical receipt: %v", err)
	}
	if !reflect.DeepEqual(second, written.Receipt) {
		t.Fatalf("loader leaked mutable aliases:\n got: %#v\nwant: %#v", second, written.Receipt)
	}
}

func TestLoadOMPModelResolutionReceipt_InvalidArtifactFailsClosed(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name    string
		prepare func(*testing.T) string
	}{
		{
			name: "missing",
			prepare: func(t *testing.T) string {
				return t.TempDir()
			},
		},
		{
			name: "symlink",
			prepare: func(t *testing.T) string {
				root := t.TempDir()
				outside := writeLoadReceiptFixture(t)
				if err := os.MkdirAll(filepath.Join(root, ".autopus"), 0o700); err != nil {
					t.Fatal(err)
				}
				if err := os.Symlink(loadReceiptPath(outside), loadReceiptPath(root)); err != nil {
					t.Skipf("symlink unavailable: %v", err)
				}
				return root
			},
		},
		{
			name: "world-readable mode",
			prepare: func(t *testing.T) string {
				root := writeLoadReceiptFixture(t)
				if err := os.Chmod(loadReceiptPath(root), 0o644); err != nil {
					t.Fatal(err)
				}
				return root
			},
		},
		{
			name: "unknown field",
			prepare: func(t *testing.T) string {
				root := writeLoadReceiptFixture(t)
				rewriteLoadReceipt(t, root, func(data []byte) []byte {
					return bytes.Replace(data, []byte("{\n"), []byte("{\n  \"unexpected\": true,\n"), 1)
				})
				return root
			},
		},
		{
			name: "duplicate field",
			prepare: func(t *testing.T) string {
				root := writeLoadReceiptFixture(t)
				rewriteLoadReceipt(t, root, func(data []byte) []byte {
					field := []byte("  \"schema_version\": \"" + OMPModelReceiptSchemaVersion + "\",\n")
					return bytes.Replace(data, field, append(append([]byte(nil), field...), field...), 1)
				})
				return root
			},
		},
		{
			name: "trailing JSON",
			prepare: func(t *testing.T) string {
				root := writeLoadReceiptFixture(t)
				rewriteLoadReceipt(t, root, func(data []byte) []byte {
					return append(append([]byte(nil), data...), []byte("{}\n")...)
				})
				return root
			},
		},
		{
			name: "oversized",
			prepare: func(t *testing.T) string {
				root := writeLoadReceiptFixture(t)
				rewriteLoadReceipt(t, root, func([]byte) []byte {
					return bytes.Repeat([]byte("x"), (1<<20)+1)
				})
				return root
			},
		},
		{
			name: "resolution digest drift",
			prepare: func(t *testing.T) string {
				root := writeLoadReceiptFixture(t)
				rewriteLoadReceipt(t, root, func(data []byte) []byte {
					var raw map[string]any
					if err := json.Unmarshal(data, &raw); err != nil {
						t.Fatal(err)
					}
					raw["resolution_digest"] = "sha256:" + strings.Repeat("d", 64)
					changed, err := json.Marshal(raw)
					if err != nil {
						t.Fatal(err)
					}
					return append(changed, '\n')
				})
				return root
			},
		},
		{
			name: "generated_at refreshed without digest",
			prepare: func(t *testing.T) string {
				root := writeLoadReceiptFixtureAt(t, time.Now().UTC().Add(-48*time.Hour))
				rewriteLoadReceipt(t, root, func(data []byte) []byte {
					var raw map[string]any
					if err := json.Unmarshal(data, &raw); err != nil {
						t.Fatal(err)
					}
					raw["generated_at"] = time.Now().UTC().Format(time.RFC3339Nano)
					changed, err := json.Marshal(raw)
					if err != nil {
						t.Fatal(err)
					}
					return append(changed, '\n')
				})
				return root
			},
		},
	}

	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			root := test.prepare(t)
			got, err := LoadOMPModelResolutionReceipt(root)
			if err == nil {
				t.Fatalf("unsafe receipt was accepted: %#v", got)
			}
			if !reflect.DeepEqual(got, OMPModelResolutionReceipt{}) {
				t.Fatalf("rejected receipt leaked partial authority: %#v", got)
			}
		})
	}
}

func TestLoadOMPModelResolutionReceiptAt_EnforcesTwentyFourHourFreshness(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, 8, 3, 12, 0, 0, 0, time.UTC)
	validFor := 24 * time.Hour

	boundaryRoot := writeLoadReceiptFixtureAt(t, now.Add(-validFor))
	boundary, err := LoadOMPModelResolutionReceiptAt(boundaryRoot, now, validFor)
	if err != nil {
		t.Fatalf("receipt at freshness boundary must remain valid: %v", err)
	}
	if !boundary.GeneratedAt.Equal(now.Add(-validFor)) {
		t.Fatalf("boundary generated_at = %s", boundary.GeneratedAt)
	}

	staleRoot := writeLoadReceiptFixtureAt(t, now.Add(-validFor-time.Nanosecond))
	stale, err := LoadOMPModelResolutionReceiptAt(staleRoot, now, validFor)
	if err == nil {
		t.Fatalf("stale receipt was accepted: %#v", stale)
	}
	if !reflect.DeepEqual(stale, OMPModelResolutionReceipt{}) {
		t.Fatalf("stale receipt leaked partial authority: %#v", stale)
	}
}

func TestLoadOMPModelResolutionReceipt_RejectsCrossWorkspaceReceiptWithoutBoundOverlay(t *testing.T) {
	t.Parallel()
	source := writeLoadReceiptFixture(t)
	target := t.TempDir()
	if err := os.MkdirAll(filepath.Dir(loadReceiptPath(target)), 0o700); err != nil {
		t.Fatal(err)
	}
	body, err := os.ReadFile(loadReceiptPath(source))
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(loadReceiptPath(target), body, 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadOMPModelResolutionReceipt(target); err == nil || !strings.Contains(err.Error(), "overlay binding") {
		t.Fatalf("cross-workspace receipt must fail closed, got %v", err)
	}
}

func writeLoadReceiptFixture(t *testing.T) string {
	t.Helper()
	return writeLoadReceiptFixtureAt(t, time.Now().UTC())
}

func writeLoadReceiptFixtureAt(t *testing.T, generatedAt time.Time) string {
	t.Helper()
	root := t.TempDir()
	receipt := writeLoadReceiptOverlay(t, root, modelReceiptFixture(generatedAt))
	if _, err := WriteOMPModelResolutionReceipt(OMPModelReceiptWriteInput{
		WorkspaceRoot: root,
		Receipt:       receipt,
	}); err != nil {
		t.Fatalf("write receipt fixture: %v", err)
	}
	return root
}

func writeLoadReceiptOverlay(t *testing.T, root string, receipt OMPModelResolutionReceipt) OMPModelResolutionReceipt {
	t.Helper()
	data := []byte("models:\n  test: provider/model\n")
	path := filepath.Join(root, filepath.FromSlash(DefaultOMPModelOverlayPath))
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}
	receipt.Activation.ConfigHash = OMPModelSHA256(data)
	return receipt
}

func loadReceiptPath(root string) string {
	return filepath.Join(root, filepath.FromSlash(OMPModelReceiptRelativePath))
}

func rewriteLoadReceipt(t *testing.T, root string, mutate func([]byte) []byte) {
	t.Helper()
	path := loadReceiptPath(root)
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, mutate(data), 0o600); err != nil {
		t.Fatal(err)
	}
}
