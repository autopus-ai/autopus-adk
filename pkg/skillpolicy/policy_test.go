package skillpolicy

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestSelectExplicitEvidence(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "go.mod"), []byte("module fixture"), 0600); err != nil {
		t.Fatal(err)
	}
	policy := Policy{SchemaVersion: PolicySchema, Candidates: []Candidate{{ID: "go-test", AllowedTaskClasses: []string{"bugfix"}, ExcludedTaskClasses: []string{"deployment"}, RequiredFiles: []string{"go.mod"}, SupportedVersions: map[string][]string{"go": {"1.26"}}}}}
	for _, tt := range []struct{ name, class, version, status string }{
		{"positive", "bugfix", "1.26", "selected"},
		{"negative", "deployment", "1.26", "excluded"},
		{"incompatible", "bugfix", "1.25", "excluded"},
		{"unknown", "bugfix", "", "unknown"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			versions := map[string]string{}
			if tt.version != "" {
				versions["go"] = tt.version
			}
			result, err := Select(policy, Task{SchemaVersion: TaskSchema, Class: tt.class, DeclaredVersions: versions}, root)
			if err != nil {
				t.Fatal(err)
			}
			if result.Decisions[0].Status != tt.status {
				t.Fatalf("status = %s", result.Decisions[0].Status)
			}
			if tt.status == "selected" {
				if !reflect.DeepEqual(result.Selected, []string{"go-test"}) {
					t.Fatal(result.Selected)
				}
				seen := map[string]bool{}
				for _, evidence := range result.Decisions[0].Evidence {
					seen[evidence.Source] = true
				}
				if !seen["declared"] || !seen["local_file"] {
					t.Fatal(seen)
				}
			} else if len(result.Selected) != 0 {
				t.Fatal(result.Selected)
			}
		})
	}
}

func TestExcludeWinsAndMissingFile(t *testing.T) {
	policy := Policy{SchemaVersion: PolicySchema, Candidates: []Candidate{{ID: "test", AllowedTaskClasses: []string{"fix"}, ExcludedTaskClasses: []string{"fix"}}}}
	task := Task{SchemaVersion: TaskSchema, Class: "fix"}
	result, err := Select(policy, task, t.TempDir())
	if err != nil || result.Decisions[0].Status != "excluded" {
		t.Fatalf("%+v %v", result, err)
	}
	policy.Candidates[0].ExcludedTaskClasses = nil
	policy.Candidates[0].RequiredFiles = []string{"missing"}
	result, err = Select(policy, task, t.TempDir())
	if err != nil || result.Decisions[0].Status != "excluded" {
		t.Fatalf("%+v %v", result, err)
	}
}

func TestSelectRejectsUnsafeMarkers(t *testing.T) {
	root := t.TempDir()
	outside := t.TempDir()
	if err := os.WriteFile(filepath.Join(outside, "marker"), []byte("x"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(root, "link")); err != nil {
		t.Fatal(err)
	}
	for _, marker := range []string{"../marker", "/marker", "link/marker", ".", "a/../marker", `a\marker`} {
		t.Run(marker, func(t *testing.T) {
			policy := Policy{SchemaVersion: PolicySchema, Candidates: []Candidate{{ID: "test", AllowedTaskClasses: []string{"fix"}, RequiredFiles: []string{marker}}}}
			result, err := Select(policy, Task{SchemaVersion: TaskSchema, Class: "fix"}, root)
			if err == nil && len(result.Selected) != 0 {
				t.Fatalf("unsafe selected: %+v", result)
			}
		})
	}
}

func TestStrictDecode(t *testing.T) {
	for _, input := range []string{`{"schema_version":"skill_task.v1","class":"fix","extra":true}`, `{"schema_version":"skill_task.v1","class":"fix","class":"other"}`, `null`, `{}`, `{} {}`, strings.Repeat(" ", MaxJSONBytes+1)} {
		var task Task
		if err := Decode(strings.NewReader(input), &task); err == nil {
			t.Fatalf("accepted invalid input %.60s", input)
		}
	}
}

func TestPolicyReplayMismatch(t *testing.T) {
	policy := Policy{SchemaVersion: PolicySchema, Candidates: []Candidate{{ID: "test", AllowedTaskClasses: []string{"fix"}}}}
	cases := Cases{SchemaVersion: CasesSchema, Cases: []Case{{Name: "positive", Task: Task{SchemaVersion: TaskSchema, Class: "fix"}, ExpectedSelected: []string{"test"}}, {Name: "negative", Task: Task{SchemaVersion: TaskSchema, Class: "deploy"}, ExpectedSelected: []string{}}}}
	result, err := Check(policy, cases, t.TempDir())
	if err != nil || !result.Passed {
		t.Fatalf("%+v %v", result, err)
	}
	cases.Cases[0].ExpectedSelected = []string{}
	result, err = Check(policy, cases, t.TempDir())
	if err != nil || result.Passed {
		t.Fatalf("%+v %v", result, err)
	}
}
