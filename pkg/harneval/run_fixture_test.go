package harneval

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/insajin/autopus-adk/pkg/adapter"
	"github.com/insajin/autopus-adk/pkg/config"
)

// Digests of the run fixtures, computed outside Go by the scratch
// oracle_run.py (hashlib over hand-written canonical JSON). Its surface, agent,
// and standard set values coincide with the oracle_expect.py constants.
const (
	oracleGoneDigest     = "25230fe5f65e5d0bd0d50beb73dbf56f71a4eba6a2d670e5a57436bdefcd0865"
	oraclePlanDigest     = "20b040e3ff49b630e7c51415b26bf082a423fa43c2e0f0d0cd7468472929b443"
	oracleAgentTwoDigest = "5de0e5b0862ba6585cf78b0f097feefa89902f6703f6fde312e4e53dc938f278"
	oracleS3CurrentSet   = "3be52662afa9d0564a6157f7caa7b9e3df7fd3f9cce9f730fdaf8cfb6465341b"
	oracleS3StableSet    = "11088ea61bff2b60241fd1e89ddb870bad8fb5d1ed910b21aa879c0647f409ac"
	oracleS4BaseSet      = "19bc1ed581e98820a198501d569ee9d9dca0544211387900c2e6b870e28de534"
	oracleS4NeedleSet    = "552ba692c922794eab1c201a280f087739177b8cb59704c942634ffc43a2c572"
	oracleS4CorpusSet    = "a133f3bba8c2e14d3bde82252808082f5c978ab993be16667e81fb4a5178ddc5"
	oracleS4TestsSet     = "d37671cb88f1fd627212513461e044e2c3bee2553ac634d9dba463ca961d9610"
	oracleS5Set19        = "565e8c73bca7904ffc65fa0d40fec2d07596332a9a1a66522743d340cdb92a9e"
	oracleS5Set20        = "1af1b42bb9c409da028fd23a9f26bb4411d5195bbce88ae648a8f3fab1c58bc6"
)

// routerPath is the file every standard task asserts on claude-code.
const routerPath = ".claude/skills/auto/SKILL.md"

// fakeAdapter writes fixed files under its root and reports them as its own.
type fakeAdapter struct {
	adapter.PlatformAdapter
	name  string
	root  string
	files map[string]string
}

func (a fakeAdapter) Name() string { return a.name }

func (a fakeAdapter) Generate(context.Context, *config.HarnessConfig) (*adapter.PlatformFiles, error) {
	generated := &adapter.PlatformFiles{}
	for rel, body := range a.files {
		full := filepath.Join(a.root, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			return nil, err
		}
		if err := os.WriteFile(full, []byte(body), 0o644); err != nil {
			return nil, err
		}
		generated.Files = append(generated.Files, adapter.FileMapping{TargetPath: rel})
	}
	return generated, nil
}

// fakeAdapters is a fast stand-in surface: claude-code generates only its
// router, which holds the needles "plan" and "go"; the other platforms
// generate nothing.
func fakeAdapters(root string, _ Pins, _ []byte) []adapter.PlatformAdapter {
	adapters := make([]adapter.PlatformAdapter, 0, len(Platforms))
	for _, platform := range Platforms {
		files := map[string]string{}
		if platform == "claude-code" {
			files[routerPath] = "router: plan go\n"
		}
		adapters = append(adapters, fakeAdapter{name: platform, root: root, files: files})
	}
	return adapters
}

func noStaleTemplates(string) ([]string, error) { return nil, nil }

// fixedClock is 2026-10-07T01:02:03Z expressed in another zone.
func fixedClock() time.Time {
	return time.Date(2026, 10, 7, 10, 2, 3, 0, time.FixedZone("KST", 9*60*60))
}

// fakeRun runs the pipeline on the fake surface. Unless a test sets them, the
// template check is skipped and the clock is fixed.
func fakeRun(t *testing.T, root string, opts RunOptions) *Result {
	t.Helper()
	if opts.Adapters == nil {
		opts.Adapters = fakeAdapters
	}
	if opts.StaleCheck == nil {
		opts.StaleCheck = noStaleTemplates
	}
	if opts.Now == nil {
		opts.Now = fixedClock
	}
	result, err := Run(context.Background(), root, opts)
	require.NoError(t, err)
	require.NotNil(t, result)
	return result
}

func (f *fixture) remove(rel string) {
	f.t.Helper()
	require.NoError(f.t, os.Remove(filepath.Join(f.root, filepath.FromSlash(rel))))
}

// digestRow is a baseline row carrying an oracle expectation digest.
func digestRow(id, kind, state, result, digest string) map[string]any {
	row := baselineRow(id, kind, state, result)
	row["expectation_digest"] = digest
	return row
}

func (f *fixture) writeBaseline(setDigest string, rows ...map[string]any) {
	f.t.Helper()
	doc := baselineDoc(rows...)
	doc["set_digest"] = setDigest
	f.writeJSON(BaselinePath, doc)
}

// writeStandardBaseline pins the standard set: every surface task passed.
func (f *fixture) writeStandardBaseline() {
	f.t.Helper()
	f.writeBaseline(oracleSetDigest,
		digestRow("GT-AG-001", KindAgent, StateActive, ResultNotRun, oracleAgentDigest),
		digestRow("GT-FIX-A", KindSurface, StateActive, ResultPass, oracleSurfaceDigest),
		digestRow("GT-FIX-B", KindSurface, StateActive, ResultPass, oracleSurfaceDigest),
		digestRow("GT-FIX-C", KindSurface, StateActive, ResultPass, oracleSurfaceDigest),
	)
}

// surfaceTaskAt is a surface task whose single assertion is file_exists on a
// claude-code path.
func surfaceTaskAt(id, path string) map[string]any {
	task := surfaceTask(id)
	task["assertions"] = []any{map[string]any{"kind": AssertFileExists, "platform": "claude-code", "path": path}}
	return task
}

func retiredTask(task map[string]any) map[string]any {
	task["status"] = map[string]any{"state": StateRetired, "reason": "superseded"}
	return task
}

// failIfCalled is a template check that must not run.
func failIfCalled(t *testing.T) func(string) ([]string, error) {
	return func(string) ([]string, error) {
		t.Error("the template check ran after a load precondition failed")
		return nil, nil
	}
}

func requireRate(t *testing.T, want float64, got *float64) {
	t.Helper()
	require.NotNil(t, got)
	require.InDelta(t, want, *got, 1e-9)
}
