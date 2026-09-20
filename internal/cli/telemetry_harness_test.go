package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestTelemetryHarnessStrictEvidence(t *testing.T) {
	for _, data := range []string{`{"version":1,"version":1,"expected_task_ids":["t"],"observations":[]}`, `{"version":1,"expected_task_ids":["t"],"observations":[],"typo":true}`, `{"version":1,"expected_task_ids":["t"],"observations":[]} {}`, `{"version":1,"expected_task_ids":["t","t"],"observations":[]}`} {
		file := filepath.Join(t.TempDir(), "e.json")
		if err := os.WriteFile(file, []byte(data), 0600); err != nil {
			t.Fatal(err)
		}
		cmd := newTelemetryHarnessCmd()
		cmd.SetArgs([]string{"--evidence-json", file, "--format", "json"})
		cmd.SetOut(&bytes.Buffer{})
		cmd.SetErr(&bytes.Buffer{})
		if err := cmd.Execute(); err == nil {
			t.Fatalf("accepted %s", data)
		}
	}
}
func TestTelemetryHarnessMissingObservationsAreUnknown(t *testing.T) {
	file := filepath.Join(t.TempDir(), "e.json")
	if err := os.WriteFile(file, []byte(`{"version":1,"expected_task_ids":["t"],"observations":[]}`), 0600); err != nil {
		t.Fatal(err)
	}
	for _, format := range []string{"json", "human", "text"} {
		cmd := newTelemetryCmd()
		cmd.SetArgs([]string{"harness", "--evidence-json", file, "--format", format})
		var out bytes.Buffer
		cmd.SetOut(&out)
		if err := cmd.Execute(); err != nil {
			t.Fatal(err)
		}
		want := "unknown"
		if format == "json" {
			want = `"actual_tokens": null`
		}
		if !strings.Contains(out.String(), want) {
			t.Fatalf("missing unknown in %s", out.String())
		}
	}
}
func TestTelemetryHarnessNestedDuplicateKeys(t *testing.T) {
	if err := rejectHarnessDuplicateKeys([]byte(`{"a":[{"x":1,"x":2}]}`)); err == nil {
		t.Fatal("accepted nested duplicate")
	}
}

func TestTelemetryHarnessRejectsNonRegularInputs(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "e.json")
	if err := os.WriteFile(file, []byte(`{"version":1,"expected_task_ids":["t"],"observations":[]}`), 0600); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(dir, "link.json")
	if err := os.Symlink(file, link); err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{link, dir} {
		if _, err := readHarnessEvidence(path); err == nil {
			t.Fatalf("accepted nonregular input: %s", path)
		}
	}
	if _, err := readHarnessEvidence(file); err != nil {
		t.Fatalf("regular input rejected: %v", err)
	}
}
