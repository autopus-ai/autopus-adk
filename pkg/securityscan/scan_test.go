package securityscan

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func touch(t *testing.T, dir string, names ...string) {
	t.Helper()
	for _, name := range names {
		p := filepath.Join(dir, name)
		require.NoError(t, os.MkdirAll(filepath.Dir(p), 0o755))
		require.NoError(t, os.WriteFile(p, []byte("{}\n"), 0o644))
	}
}

func TestDetect_Ecosystems(t *testing.T) {
	for name, row := range map[string]struct {
		files []string
		want  []Ecosystem
	}{
		"empty":         {nil, nil},
		"go":            {[]string{"go.mod"}, []Ecosystem{{Name: "go", Manifest: "go.mod"}}},
		"npm":           {[]string{"package.json", "package-lock.json"}, []Ecosystem{{Name: "node", Manifest: "package-lock.json", Tool: "npm"}}},
		"pnpm":          {[]string{"package.json", "pnpm-lock.yaml"}, []Ecosystem{{Name: "node", Manifest: "pnpm-lock.yaml", Tool: "pnpm"}}},
		"yarn berry":    {[]string{"package.json", "yarn.lock", ".yarnrc.yml"}, []Ecosystem{{Name: "node", Manifest: "yarn.lock", Tool: "yarn-berry"}}},
		"yarn classic":  {[]string{"package.json", "yarn.lock"}, []Ecosystem{{Name: "node", Manifest: "yarn.lock", Tool: "yarn"}}},
		"node no lock":  {[]string{"package.json"}, []Ecosystem{{Name: "node", Manifest: "package.json"}}},
		"rust":          {[]string{"Cargo.toml", "Cargo.lock"}, []Ecosystem{{Name: "rust", Manifest: "Cargo.lock"}}},
		"python reqs":   {[]string{"requirements.txt", "requirements-dev.txt"}, []Ecosystem{{Name: "python", Manifest: "requirements-dev.txt, requirements.txt"}}},
		"python poetry": {[]string{"pyproject.toml", "poetry.lock"}, []Ecosystem{{Name: "python", Manifest: "pyproject.toml"}}},
		"mixed":         {[]string{"go.mod", "uv.lock"}, []Ecosystem{{Name: "go", Manifest: "go.mod"}, {Name: "python", Manifest: "uv.lock"}}},
	} {
		t.Run(name, func(t *testing.T) {
			dir := t.TempDir()
			touch(t, dir, row.files...)
			assert.Equal(t, row.want, Detect(dir))
		})
	}
}

// fakeTools answers LookPath for the named tools and replays fixtures.
type fakeTools struct {
	installed map[string]bool
	outputs   map[string][]byte
	calls     []string
}

func (f *fakeTools) lookPath(name string) (string, error) {
	if f.installed[name] {
		return "/fake/" + name, nil
	}
	return "", errors.New("not found")
}

func (f *fakeTools) exec(_ context.Context, _ string, bin string, args ...string) ([]byte, []byte, error) {
	call := filepath.Base(bin) + " " + strings.Join(args, " ")
	f.calls = append(f.calls, call)
	if strings.Contains(call, "--help") {
		return nil, nil, nil
	}
	for key, out := range f.outputs {
		if strings.HasPrefix(call, key) {
			if i := indexOf(args, "--report-path"); i >= 0 {
				return nil, nil, os.WriteFile(args[i+1], out, 0o600)
			}
			return out, nil, errors.New("exit status 1")
		}
	}
	return nil, []byte("boom"), errors.New("exit status 2")
}

func indexOf(args []string, want string) int {
	for i, a := range args {
		if a == want {
			return i
		}
	}
	return -1
}

func TestScan_RunsInstalledToolsAndNotesMissingOnes(t *testing.T) {
	dir := t.TempDir()
	touch(t, dir, "go.mod", "package.json", "package-lock.json", "Cargo.lock")
	require.NoError(t, os.Mkdir(filepath.Join(dir, ".git"), 0o755))
	tools := &fakeTools{
		installed: map[string]bool{"gitleaks": true, "govulncheck": true, "npm": true},
		outputs: map[string][]byte{
			"gitleaks git": fixture(t, "gitleaks.json"),
			"govulncheck":  fixture(t, "govulncheck.json"),
			"npm audit":    fixture(t, "npm-v7.json"),
		},
	}
	rep := Scan(context.Background(), Options{Dir: dir, Timeout: time.Minute, Staged: true, LookPath: tools.lookPath, Exec: tools.exec})

	status := map[string]Status{}
	for _, s := range rep.Scanners {
		status[s.Name] = s.Status
	}
	assert.Equal(t, map[string]Status{"gitleaks": StatusRan, "govulncheck": StatusRan, "npm audit": StatusRan,
		"cargo audit": StatusMissing}, status)
	assert.Len(t, rep.Findings, 5)
	assert.True(t, slicesHasPrefix(tools.calls, "gitleaks git --no-banner"), tools.calls)
	assert.True(t, slicesContains(tools.calls, "--staged"), "--staged reaches gitleaks")
	assert.True(t, slicesContains(tools.calls, "--redact"), "gitleaks is asked to redact")

	assert.True(t, rep.Failed(FailOnHigh))
	assert.True(t, rep.Failed(FailOnSecret))
	assert.False(t, rep.Failed(FailOnNone))
}

func TestScan_ErrorsAreReportedNotHidden(t *testing.T) {
	dir := t.TempDir()
	touch(t, dir, "go.mod")
	tools := &fakeTools{installed: map[string]bool{"govulncheck": true}}
	rep := Scan(context.Background(), Options{Dir: dir, Timeout: time.Minute, LookPath: tools.lookPath, Exec: tools.exec})
	for _, s := range rep.Scanners {
		if s.Name == "govulncheck" {
			assert.Equal(t, StatusError, s.Status)
			assert.Contains(t, s.Detail, "boom")
		}
	}
	assert.True(t, rep.HasErrors())
	assert.False(t, rep.Failed(FailOnHigh), "an error is not a finding")
}

func TestFailOn_Thresholds(t *testing.T) {
	secret := Finding{Kind: KindSecret, Severity: SeverityHigh}
	medium := Finding{Kind: KindVuln, Severity: SeverityMedium}
	unknown := Finding{Kind: KindVuln, Severity: SeverityUnknown}
	for _, row := range []struct {
		failOn             FailOn
		secret, med, unkwn bool
	}{
		{FailOnNone, false, false, false},
		{FailOnSecret, true, false, false},
		{FailOnCritical, true, false, false},
		{FailOnHigh, true, false, true},
		{FailOnMedium, true, true, true},
		{FailOnLow, true, true, true},
	} {
		assert.Equal(t, row.secret, row.failOn.Fails(secret), "%s secret", row.failOn)
		assert.Equal(t, row.med, row.failOn.Fails(medium), "%s medium", row.failOn)
		assert.Equal(t, row.unkwn, row.failOn.Fails(unknown), "%s unknown", row.failOn)
	}
	_, err := ParseFailOn("sometimes")
	assert.Error(t, err)
}

func TestWriteTable_RedactsAndSummarizes(t *testing.T) {
	findings, err := parseGitleaks(fixture(t, "gitleaks.json"))
	require.NoError(t, err)
	rep := Report{Dir: "/x", Scanners: []ScannerResult{{Name: "gitleaks", Status: StatusRan, Findings: 2},
		{Name: "govulncheck", Status: StatusMissing, Detail: "govulncheck not installed"}}, Findings: findings}
	var out bytes.Buffer
	WriteTable(&out, rep, FailOnHigh)
	text := out.String()
	assert.NotContains(t, text, "fixture-not-a-real-value-0001")
	assert.Contains(t, text, "config/settings.py:12")
	assert.Contains(t, text, "govulncheck not installed")
	assert.Contains(t, text, "FAIL")
}

func slicesHasPrefix(calls []string, prefix string) bool {
	for _, c := range calls {
		if strings.HasPrefix(c, prefix) {
			return true
		}
	}
	return false
}

func slicesContains(calls []string, part string) bool {
	for _, c := range calls {
		if strings.Contains(c, part) {
			return true
		}
	}
	return false
}
