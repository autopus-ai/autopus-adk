package cli

import (
	"bytes"
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/insajin/autopus-adk/pkg/pipeline"
)

// statusFixture is a git project with SPECs and pipeline state for the status
// evidence tests (SPEC-STATUSEVIDENCE-001). Status sees the machine only
// through probe, which by default finds no AI CLI, no Desktop helper, and an
// empty home, so no test depends on what this machine has installed.
type statusFixture struct {
	t     *testing.T
	root  string
	probe statusProbe
}

// newStatusFixture makes a committed project. With platforms it also writes
// an autopus.yaml naming them.
func newStatusFixture(t *testing.T, platforms ...string) *statusFixture {
	t.Helper()
	f := &statusFixture{t: t, root: t.TempDir()}
	f.probe = stubStatusProbe(nil, false, t.TempDir())
	initTestGitRepo(t, f.root)
	if len(platforms) > 0 {
		f.write("autopus.yaml", "mode: full\nproject_name: fixture\nplatforms:\n  - "+strings.Join(platforms, "\n  - ")+"\n")
	}
	f.write("pkg/app/app.go", "package app\n")
	return f
}

// stubStatusProbe sees exactly these AI CLIs, Desktop helper, and home.
func stubStatusProbe(installed []string, helperInstalled bool, home string) statusProbe {
	return statusProbe{
		lookPath: func(name string) (string, error) {
			for _, candidate := range installed {
				if candidate == name {
					return "/stub/bin/" + name, nil
				}
			}
			return "", exec.ErrNotFound
		},
		desktopHelper: func() (string, error) {
			if helperInstalled {
				return "/stub/autopus-desktop-runtime", nil
			}
			return "", errRuntimeHelperNotFound
		},
		homeDir: func() (string, error) { return home, nil },
	}
}

func (f *statusFixture) write(rel, body string) {
	f.t.Helper()
	path := filepath.Join(f.root, filepath.FromSlash(rel))
	require.NoError(f.t, os.MkdirAll(filepath.Dir(path), 0o755))
	require.NoError(f.t, os.WriteFile(path, []byte(body), 0o644))
}

func (f *statusFixture) read(rel string) string {
	f.t.Helper()
	data, err := os.ReadFile(filepath.Join(f.root, filepath.FromSlash(rel)))
	require.NoError(f.t, err)
	return string(data)
}

// writeSpec writes a SPEC with the given declared status and returns its
// directory.
func (f *statusFixture) writeSpec(id, status string) string {
	f.t.Helper()
	base := ".autopus/specs/" + id + "/"
	f.write(base+"spec.md", "# "+id+": fixture feature\n\n**Status**: "+status+"\n\n## Requirements\n\nThe fixture works.\n")
	f.write(base+"plan.md", "# Plan\n\n## Tasks\n\n- T1: build it\n")
	f.write(base+"acceptance.md", "# Acceptance\n\n## Scenarios\n\nGiven the fixture, when it runs, then it works.\n")
	return filepath.Join(f.root, ".autopus", "specs", id)
}

func (f *statusFixture) commit(message string) string {
	f.t.Helper()
	runGitCommand(f.t, f.root, "add", "-A")
	runGitCommand(f.t, f.root, "commit", "-q", "--no-verify", "-m", message)
	return f.head()
}

func (f *statusFixture) head() string {
	f.t.Helper()
	return strings.TrimSpace(runGitCommand(f.t, f.root, "rev-parse", "HEAD"))
}

// saveCheckpoint writes a SPEC's canonical checkpoint, or its blocked
// sidecar when blocked is set.
func (f *statusFixture) saveCheckpoint(specID string, cp pipeline.Checkpoint, blocked bool) {
	f.t.Helper()
	name := specID + ".yaml"
	if blocked {
		name = specID + ".blocked.yaml"
	}
	dir := filepath.Join(f.root, pipelineStateDir)
	require.NoError(f.t, os.MkdirAll(dir, 0o755))
	require.NoError(f.t, cp.SaveFile(filepath.Join(dir, name)))
}

// runStatus runs `auto status --dir <root>` with extra arguments.
func (f *statusFixture) runStatus(args ...string) string {
	f.t.Helper()
	cmd := newStatusCmdWithProbe(f.probe)
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs(append([]string{"--dir", f.root}, args...))
	require.NoError(f.t, cmd.Execute(), out.String())
	return statusANSI.ReplaceAllString(out.String(), "")
}

// statusANSI matches terminal styling, which the assertions read past.
var statusANSI = regexp.MustCompile(`\x1b\[[0-9;]*m`)

// statusJSON runs `auto status --json` and returns the envelope's data.
func (f *statusFixture) statusJSON() map[string]any {
	f.t.Helper()
	var envelope struct {
		Data map[string]any `json:"data"`
	}
	out := f.runStatus("--json")
	require.NoError(f.t, json.Unmarshal([]byte(out), &envelope), out)
	return envelope.Data
}

// evidence reads one SPEC's evidence the way status does.
func (f *statusFixture) evidence(specID string) statusEvidence {
	f.t.Helper()
	specs := scanAllSpecs(f.root)
	reader := newStatusEvidenceReader(f.root, specs, collectStatusEnvironment(f.root, f.probe))
	for _, entry := range specs {
		if entry.id == specID {
			return reader.read(entry)
		}
	}
	require.FailNow(f.t, "SPEC not scanned", specID)
	return statusEvidence{}
}

// jsonSpec returns the JSON entry of one SPEC.
func jsonSpec(t *testing.T, data map[string]any, specID string) map[string]any {
	t.Helper()
	specs, _ := data["specs"].([]any)
	for _, raw := range specs {
		if spec, _ := raw.(map[string]any); spec["id"] == specID {
			return spec
		}
	}
	require.FailNow(t, "SPEC missing from status JSON", specID)
	return nil
}

// jsonPath walks nested JSON objects by key.
func jsonPath(value any, keys ...string) any {
	for _, key := range keys {
		object, _ := value.(map[string]any)
		value = object[key]
	}
	return value
}

// statusActions indexes the JSON next actions by id.
func statusActions(t *testing.T, data map[string]any) map[string]map[string]any {
	t.Helper()
	byID := map[string]map[string]any{}
	actions, _ := data["next_actions"].([]any)
	for _, raw := range actions {
		action, _ := raw.(map[string]any)
		byID[action["id"].(string)] = action
	}
	return byID
}

// removeIfExists deletes a file the test no longer wants, tolerating absence.
func removeIfExists(t *testing.T, path string) {
	t.Helper()
	if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
		require.NoError(t, err)
	}
}
