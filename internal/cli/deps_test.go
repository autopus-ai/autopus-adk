package cli

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func runDeps(t *testing.T, args ...string) (string, error) {
	t.Helper()
	cmd := newDepsCmd()
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs(args)
	err := cmd.Execute()
	return out.String(), err
}

func depsProject(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "package.json"), []byte(`{"dependencies":{"react":"^17.0.2"}}`), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "compose.yaml"), []byte("services:\n  db:\n    image: postgres:18\n  q:\n    image: rabbitmq:3\n"), 0o644))
	return dir
}

func TestDepsLatest_PrefixBareNameAndErrors(t *testing.T) {
	useFixtureRegistry(t, 0)
	out, err := runDeps(t, "latest", "image:postgres:13")
	require.NoError(t, err)
	assert.Contains(t, out, "image:postgres 18.6")
	assert.Contains(t, out, "source")
	assert.Contains(t, out, "fetched")

	out, err = runDeps(t, "latest", "react", "--dir", depsProject(t))
	require.NoError(t, err, "a bare name resolves through the project's manifests")
	assert.Contains(t, out, "npm:react 19.3.1")

	_, err = runDeps(t, "latest", "maven:junit")
	assert.ErrorContains(t, err, "unknown ecosystem")
	_, err = runDeps(t, "latest", "leftpad", "--dir", t.TempDir())
	assert.ErrorContains(t, err, "name the ecosystem")
	_, err = runDeps(t, "latest", "npm:does-not-exist")
	assert.ErrorContains(t, err, "unknown: npm: HTTP 404")
}

func TestDepsOutdated_TableAndJSON(t *testing.T) {
	useFixtureRegistry(t, 0)
	dir := depsProject(t)
	out, err := runDeps(t, "outdated", "--dir", dir)
	require.NoError(t, err)
	assert.Contains(t, out, "package.json:1")
	assert.Regexp(t, `npm:react\s+\^17\.0\.2\s+19\.3\.1\s+major-behind \(2\)`, out)
	assert.Regexp(t, `image:postgres\s+18\s+18\.6\s+current`, out)
	assert.Contains(t, out, "image:rabbitmq", "a failed lookup is listed as unknown, not an error")
	assert.Contains(t, out, "3 pins: 1 major-behind, 0 minor-behind, 1 current, 1 unknown")

	out, err = runDeps(t, "outdated", "--dir", dir, "--json")
	require.NoError(t, err)
	var doc struct {
		Results []struct {
			Item struct {
				Name string `json:"name"`
			} `json:"item"`
			Status string `json:"status"`
		} `json:"results"`
	}
	require.NoError(t, json.Unmarshal([]byte(out), &doc), out)
	require.Len(t, doc.Results, 3)
	got := map[string]string{}
	for _, r := range doc.Results {
		got[r.Item.Name] = r.Status
	}
	assert.Equal(t, map[string]string{"react": "major-behind", "postgres": "current", "rabbitmq": "unknown"}, got)
}

// Offline with no cache, `outdated` still exits 0 and says unknown.
func TestDepsOutdated_OfflineNeverFails(t *testing.T) {
	out, err := runDeps(t, "outdated", "--dir", depsProject(t))
	require.NoError(t, err)
	assert.Contains(t, out, "3 unknown")
}
