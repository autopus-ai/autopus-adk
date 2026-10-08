package intake

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"reflect"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/insajin/autopus-adk/pkg/harneval"
)

// newPinnedProject writes the S6 golden set (surface task GT-FIX-A, agent
// task GT-AG-001 and its corpus) and the baseline that pins it, as
// `auto eval harness baseline --init` records it on the surface isolationRun
// generates. There is no intake area yet.
func newPinnedProject(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	writeRecord(t, root, harneval.ManifestPath, promoteManifest())
	writeRecord(t, root, SurfaceTaskDir+"/GT-FIX-A.json", validTaskDoc("GT-FIX-A"))
	writeFile(t, root, "bench/corpus_a.json", promoteCorpus)
	writeRecord(t, root, promoteAgentDir+"/GT-AG-001.json", promoteAgentTask())
	set, err := harneval.LoadSet(root)
	require.NoError(t, err)
	baseline := harneval.Baseline{
		SchemaVersion: harneval.BaselineSchemaV1, SetVersion: set.Manifest.SetVersion, SetDigest: harneval.SetDigest(set),
	}
	// Tasks are sorted by id, the order baseline rows need. The surface task's
	// one assertion holds on the generated surface; an agent task never runs.
	for _, task := range set.Tasks {
		result := harneval.ResultPass
		if task.Kind == harneval.KindAgent {
			result = harneval.ResultNotRun
		}
		baseline.Rows = append(baseline.Rows, harneval.BaselineRow{
			ID: task.ID, Kind: task.Kind, State: task.Status.State, Result: result,
			ExpectationDigest: harneval.ExpectationDigest(task),
		})
	}
	writeRecord(t, root, harneval.BaselinePath, baseline)
	return root
}

// isolationRun is `auto eval harness run --format json` on the project below
// root: the SPEC-HARNEVAL-001 run and its result encoding, with produced_at
// removed. The surface seam writes a fixed codex tree. Generation never reads
// the project root, so the golden set and the baseline are the only project
// files that reach the result.
func isolationRun(t *testing.T, root string) string {
	t.Helper()
	result, err := harneval.Run(context.Background(), root, promoteRun(".codex/hooks.json"))
	require.NoError(t, err)
	result.ProducedAt = ""
	data, err := harneval.EncodeResult(result)
	require.NoError(t, err)
	return string(data)
}

func TestIsolation_S8_IntakeAreaLeavesTheRunResultUnchanged(t *testing.T) {
	t.Parallel()
	// Given a golden set its baseline pins, so the run passes and any task
	// the intake area added would show as a set digest mismatch.
	root := newPinnedProject(t)
	pinned := isolationRun(t, root)
	var doc struct {
		Status    string `json:"status"`
		SetDigest string `json:"set_digest"`
	}
	require.NoError(t, json.Unmarshal([]byte(pinned), &doc))
	require.Equal(t, harneval.StatusPass, doc.Status, pinned)
	require.Regexp(t, `^[0-9a-f]{64}$`, doc.SetDigest)
	require.NoDirExists(t, root+"/"+IntakeDir)

	// When candidates are created, edited, rejected, and deleted, with a run
	// after each step.
	steps := []struct {
		name  string
		apply func(t *testing.T)
		left  []string
	}{
		{"created", func(t *testing.T) { runIntake(t, root, s3Entries(), nil) },
			[]string{"GTC-023e9302ff0b.json", "GTC-8e80c7a18029.json"}},
		{"edited", func(t *testing.T) { editCandidate(t, root, completeDraft) },
			[]string{"GTC-023e9302ff0b.json", "GTC-8e80c7a18029.json"}},
		{"rejected", func(t *testing.T) {
			_, err := rejectX(root, "not a harness issue")
			require.NoError(t, err)
		}, []string{"GTC-023e9302ff0b.json", "rejected"}},
		{"deleted", func(t *testing.T) { removeRel(t, root, promoteCandidateAt) }, []string{"rejected"}},
	}
	for _, step := range steps {
		step.apply(t)
		require.Equal(t, step.left, dirNames(t, root, IntakeDir), "%s: the step changed the intake area", step.name)

		// Then each result is the pinned one, byte for byte.
		assert.Equal(t, pinned, isolationRun(t, root), step.name)
	}
}

// goListDeps runs `go list -deps` on pkg for this host, or for the Windows
// build when goos is "windows", and returns the listed import paths.
func goListDeps(t *testing.T, pkg, goos string) []string {
	t.Helper()
	list := exec.Command("go", "list", "-deps", pkg)
	list.Env = os.Environ()
	if goos != "" {
		list.Env = append(list.Env, "GOOS="+goos, "GOARCH=amd64")
	}
	output, err := list.Output()
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) {
		err = fmt.Errorf("%w: %s", err, exitErr.Stderr)
	}
	require.NoError(t, err, "GOOS=%q", goos)
	return strings.Fields(string(output))
}

func TestIsolation_S8_HarnevalDependencyClosureLeavesOutTheIntakeSide(t *testing.T) {
	t.Parallel()
	harnevalPath := reflect.TypeOf(harneval.Task{}).PkgPath()
	module, found := strings.CutSuffix(harnevalPath, "/pkg/harneval")
	require.True(t, found, "pkg/harneval moved: %s", harnevalPath)
	for _, goos := range []string{"", "windows"} {
		// When go lists the production dependency closure of pkg/harneval.
		deps := goListDeps(t, harnevalPath, goos)

		// Then the closure is the real one, and no intake-side package, nor a
		// package below one, is in it.
		assert.Contains(t, deps, harnevalPath, "GOOS=%q", goos)
		assert.Contains(t, deps, module+"/pkg/content", "GOOS=%q", goos)
		for _, outside := range []string{"pkg/harneval/intake", "pkg/learn", "pkg/secretscan", "pkg/worker/security"} {
			for _, dep := range deps {
				inside := dep == module+"/"+outside || strings.HasPrefix(dep, module+"/"+outside+"/")
				assert.False(t, inside, "GOOS=%q: pkg/harneval depends on %s", goos, dep)
			}
		}
	}
}
