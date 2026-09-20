package skillpolicy

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestReplayExampleFixtures(t *testing.T) {
	var policy Policy
	var cases Cases
	for path, target := range map[string]any{"testdata/policy.json": &policy, "testdata/cases.json": &cases} {
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		if err := Decode(strings.NewReader(string(data)), target); err != nil {
			t.Fatal(err)
		}
	}
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "go.mod"), []byte("module example"), 0600); err != nil {
		t.Fatal(err)
	}
	result, err := Check(policy, cases, root)
	if err != nil || !result.Passed || len(result.Cases) != 4 {
		t.Fatalf("%+v %v", result, err)
	}
}

func TestStrictPolicyAndReplayDocuments(t *testing.T) {
	for _, input := range []string{
		`{"schema_version":"skill_policy.v1","candidates":[]}`,
		`{"schema_version":"skill_policy.v1","candidates":[{"id":"x","allowed_task_classes":[]}]}`,
		`{"schema_version":"skill_policy.v1","candidates":[{"id":"x","allowed_task_classes":["fix"],"supported_versions":{"go":["1"],"go":["2"]}}]}`,
		`{"schema_version":"skill_policy.v1","candidates":[{"id":"x","allowed_task_classes":["fix"],"surprise":true}]}`,
		`{"schema_version":"skill_policy.v1","candidates":[{"id":"x","allowed_task_classes":["fix"]},{"id":"x","allowed_task_classes":["test"]}]}`,
	} {
		var policy Policy
		if err := Decode(strings.NewReader(input), &policy); err == nil {
			t.Fatalf("accepted %s", input)
		}
	}
	for _, suffix := range []string{"", `,"expected_selected":null`, `,"expected_selected":["x","x"]`} {
		var cases Cases
		input := `{"schema_version":"skill_policy_cases.v1","cases":[{"name":"negative","task":{"schema_version":"skill_task.v1","class":"fix"}` + suffix + `}]}`
		if err := Decode(strings.NewReader(input), &cases); err == nil {
			t.Fatalf("accepted %s", input)
		}
	}
}

func TestVersionMismatchWinsOverUnknown(t *testing.T) {
	policy := Policy{SchemaVersion: PolicySchema, Candidates: []Candidate{{ID: "test", AllowedTaskClasses: []string{"fix"}, SupportedVersions: map[string][]string{"a": {"1"}, "z": {"2"}}}}}
	for _, versions := range []map[string]string{{"a": "wrong"}, {"z": "wrong"}} {
		result, err := Select(policy, Task{SchemaVersion: TaskSchema, Class: "fix", DeclaredVersions: versions}, t.TempDir())
		if err != nil || result.Decisions[0].Status != "excluded" {
			t.Fatalf("%+v %v", result, err)
		}
	}
}

func TestSelectRejectsRootSymlinkAndDirectoryMarker(t *testing.T) {
	root := t.TempDir()
	link := filepath.Join(t.TempDir(), "root")
	if err := os.Symlink(root, link); err != nil {
		t.Fatal(err)
	}
	policy := Policy{SchemaVersion: PolicySchema, Candidates: []Candidate{{ID: "test", AllowedTaskClasses: []string{"fix"}}}}
	task := Task{SchemaVersion: TaskSchema, Class: "fix"}
	for _, directory := range []string{link, link + string(os.PathSeparator)} {
		if _, err := Select(policy, task, directory); err == nil {
			t.Fatalf("symlink root accepted: %s", directory)
		}
	}
	if err := os.Mkdir(filepath.Join(root, "marker"), 0700); err != nil {
		t.Fatal(err)
	}
	policy.Candidates[0].RequiredFiles = []string{"marker"}
	result, err := Select(policy, task, root)
	if err != nil || len(result.Selected) != 0 {
		t.Fatalf("%+v %v", result, err)
	}
}
