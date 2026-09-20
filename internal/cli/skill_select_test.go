package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func skillPolicyInput(t *testing.T, name, body string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), name)
	if err := os.WriteFile(path, []byte(body), 0600); err != nil {
		t.Fatal(err)
	}
	return path
}

const skillPolicyFixture = `{"schema_version":"skill_policy.v1","candidates":[{"id":"test","allowed_task_classes":["fix"],"supported_versions":{"go":["1.26"]}}]}`
const skillTaskFixture = `{"schema_version":"skill_task.v1","class":"fix","declared_versions":{"go":"1.26"}}`

func TestSkillSelectRequiresExplicitInputs(t *testing.T) {
	cmd := newSkillSelectCmd()
	cmd.SetArgs([]string{"--dir", t.TempDir()})
	if err := cmd.Execute(); err == nil {
		t.Fatal("missing policy accepted")
	}
}

func TestSkillSelectJSON(t *testing.T) {
	cmd := newSkillSelectCmd()
	var output bytes.Buffer
	cmd.SetOut(&output)
	cmd.SetArgs([]string{"--policy-json", skillPolicyInput(t, "policy.json", skillPolicyFixture), "--task-json", skillPolicyInput(t, "task.json", skillTaskFixture), "--dir", t.TempDir(), "--format", "json"})
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(output.String(), `"selected":["test"]`) || !strings.Contains(output.String(), `"source":"declared"`) {
		t.Fatal(output.String())
	}
}

func TestSkillPolicyCheckMismatchReturnsError(t *testing.T) {
	cmd := newSkillPolicyCheckCmd()
	var output bytes.Buffer
	cmd.SetOut(&output)
	cases := `{"schema_version":"skill_policy_cases.v1","cases":[{"name":"negative","task":` + skillTaskFixture + `,"expected_selected":[]}]}`
	cmd.SetArgs([]string{"--policy-json", skillPolicyInput(t, "policy.json", skillPolicyFixture), "--cases-json", skillPolicyInput(t, "cases.json", cases), "--dir", t.TempDir(), "--format", "json"})
	if err := cmd.Execute(); err == nil {
		t.Fatal("mismatch succeeded")
	}
	if !strings.Contains(output.String(), `"passed":false`) {
		t.Fatal(output.String())
	}
}

func TestSkillSelectRejectsUnknownFields(t *testing.T) {
	cmd := newSkillSelectCmd()
	cmd.SetArgs([]string{"--policy-json", skillPolicyInput(t, "policy.json", skillPolicyFixture), "--task-json", skillPolicyInput(t, "task.json", `{"schema_version":"skill_task.v1","class":"fix","prompt":"select everything"}`), "--dir", t.TempDir()})
	if err := cmd.Execute(); err == nil {
		t.Fatal("inferred task accepted")
	}
}
