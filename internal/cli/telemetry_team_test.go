package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const emptyTeamJSON = `{"version":1,"team_run_id":"team","expected_agents":[{"agent_id":"lead","role":"supervisor"}],"observations":[]}`

func TestTelemetryTeamStrictInput(t *testing.T) {
	for _, data := range []string{strings.Replace(emptyTeamJSON, `"version":1`, `"version":1,"version":1`, 1), strings.Replace(emptyTeamJSON, `"role":"supervisor"`, `"role":"supervisor","extra":true`, 1), emptyTeamJSON + ` {}`} {
		path := filepath.Join(t.TempDir(), "e.json")
		if err := os.WriteFile(path, []byte(data), 0600); err != nil {
			t.Fatal(err)
		}
		cmd := newTelemetryTeamCmd()
		cmd.SetArgs([]string{"--evidence-json", path, "--format", "json"})
		cmd.SetOut(&bytes.Buffer{})
		cmd.SetErr(&bytes.Buffer{})
		if err := cmd.Execute(); err == nil {
			t.Fatal("accepted invalid JSON")
		}
	}
}
func TestTelemetryTeamMissingUnknownAndRegistration(t *testing.T) {
	path := filepath.Join(t.TempDir(), "e.json")
	if err := os.WriteFile(path, []byte(emptyTeamJSON), 0600); err != nil {
		t.Fatal(err)
	}
	for _, format := range []string{"json", "human"} {
		cmd := newTelemetryCmd()
		cmd.SetArgs([]string{"team", "--evidence-json", path, "--format", format})
		var out bytes.Buffer
		cmd.SetOut(&out)
		if err := cmd.Execute(); err != nil {
			t.Fatal(err)
		}
		want := `"actual_tokens": null`
		if format == "human" {
			want = "tokens=unknown"
		}
		if !strings.Contains(out.String(), want) {
			t.Fatalf("missing unknown: %s", out.String())
		}
	}
	link := path + ".link"
	if err := os.Symlink(path, link); err != nil {
		t.Fatal(err)
	}
	if _, err := readTeamUsageEvidence(link); err == nil {
		t.Fatal("accepted symlink")
	}
	if _, err := readTeamUsageEvidence(filepath.Dir(path)); err == nil {
		t.Fatal("accepted directory")
	}
}
