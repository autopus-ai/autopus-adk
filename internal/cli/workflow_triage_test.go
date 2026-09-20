package cli

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func runTriageFixture(args ...string) ([]byte, error) {
	cmd := NewWorkflowCmd(nil, nil)
	cmd.SetArgs(append([]string{"triage"}, args...))
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&bytes.Buffer{})
	err := cmd.Execute()
	return out.Bytes(), err
}

func TestWorkflowTriageMissingFactsIsGuidedAndReadOnly(t *testing.T) {
	root := t.TempDir()
	facts := filepath.Join(root, "facts.json")
	original := []byte(`{"version":1,"kind":"bugfix"}`)
	if err := os.WriteFile(facts, original, 0600); err != nil {
		t.Fatal(err)
	}
	output, err := runTriageFixture("--facts-json", facts)
	if err != nil {
		t.Fatal(err)
	}
	var report map[string]any
	if err := json.Unmarshal(output, &report); err != nil {
		t.Fatal(err)
	}
	if report["evidence_source"] != "caller_declared" || !bytes.Contains(output, []byte(`"guided"`)) {
		t.Fatalf("%s", output)
	}
	actual, err := os.ReadFile(facts)
	if err != nil || !bytes.Equal(actual, original) {
		t.Fatal("facts mutated")
	}
	entries, err := os.ReadDir(root)
	if err != nil || len(entries) != 1 {
		t.Fatal("triage wrote files")
	}
}

func TestWorkflowTriageRejectsInvalidInput(t *testing.T) {
	root := t.TempDir()
	facts := filepath.Join(root, "facts.json")
	if err := os.WriteFile(facts, []byte(`{"version":1,"kind":"bugfix"}`), 0600); err != nil {
		t.Fatal(err)
	}
	cases := [][]string{
		{}, {"--facts-json", facts, "extra"}, {"--facts-json", facts, "--format", "xml"},
		{"--facts-json", facts, "--facts-json", facts}, {"--facts-json", facts, "--format", "json", "--format", "human"},
		{"--facts-json", root},
	}
	for _, args := range cases {
		if _, err := runTriageFixture(args...); err == nil {
			t.Fatalf("invalid args accepted: %v", args)
		}
	}
	for _, data := range [][]byte{[]byte(`{"version":1,"version":1}`), []byte(`{"unknown":"private-secret"}`), make([]byte, (1<<20)+1)} {
		if err := os.WriteFile(facts, data, 0600); err != nil {
			t.Fatal(err)
		}
		_, err := runTriageFixture("--facts-json", facts)
		if err == nil || strings.Contains(err.Error(), "private-secret") {
			t.Fatalf("unsafe invalid input result: %v", err)
		}
	}
}

func TestWorkflowTriageRejectsSymlinkFacts(t *testing.T) {
	root := t.TempDir()
	facts := filepath.Join(root, "facts.json")
	link := filepath.Join(root, "link.json")
	if err := os.WriteFile(facts, []byte(`{"version":1,"kind":"bugfix"}`), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(facts, link); err != nil {
		t.Skip("symlinks unavailable")
	}
	if _, err := runTriageFixture("--facts-json", link); err == nil {
		t.Fatal("symlink accepted")
	}
}

func TestWorkflowTriageInlineAndPlannedRoutes(t *testing.T) {
	cases := []struct{ name, path, route string }{{"small", "pkg/widget/update.go", "inline"}, {"security", "pkg/auth/check.go", "planned"}}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "facts.json")
			facts := map[string]any{"version": 1, "kind": "bugfix", "paths": []string{tc.path}, "scope_complete": true, "requirements_clear": true, "acceptance_known": true, "estimated_changed_lines": 20, "risk": "low"}
			raw, err := json.Marshal(facts)
			if err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(path, raw, 0600); err != nil {
				t.Fatal(err)
			}
			output, err := runTriageFixture("--facts-json", path, "--format", "json")
			if err != nil {
				t.Fatal(err)
			}
			var report struct {
				ExecutionPerformed bool `json:"execution_performed"`
				Decision           struct {
					Route         string   `json:"route"`
					RequiredSteps []string `json:"required_steps"`
				} `json:"decision"`
			}
			if err := json.Unmarshal(output, &report); err != nil {
				t.Fatal(err)
			}
			if report.ExecutionPerformed || report.Decision.Route != tc.route || len(report.Decision.RequiredSteps) == 0 {
				t.Fatalf("%s", output)
			}
			human, err := runTriageFixture("--facts-json", path, "--format", "human")
			if err != nil {
				t.Fatal(err)
			}
			if !bytes.Contains(human, []byte(tc.route)) || !bytes.Contains(human, []byte("caller_declared")) || !bytes.Contains(human, []byte("No execution")) {
				t.Fatalf("%s", human)
			}
		})
	}
}
