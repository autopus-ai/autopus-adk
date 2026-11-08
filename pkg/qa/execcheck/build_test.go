package execcheck

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/insajin/autopus-adk/pkg/qa/testscenario"
	"github.com/insajin/autopus-adk/pkg/qa/triage"
)

// A product that does not compile is what its user would hit first: the run
// is a product verdict that triages as a product defect, so the loop fixes it
// instead of stopping on an unclassified "cannot run".
func TestRun_ProductCompileErrorIsAProductDefect(t *testing.T) {
	project := newProduct(t)
	require.NoError(t, os.WriteFile(filepath.Join(project, "main.go"), []byte("package main\n\nfunc main() { undefinedName() }\n"), 0o644))

	res, err := Run(context.Background(), project, testscenario.Exec{
		Build: build(), Argv: []string{"$BIN/mywc"}, Expect: testscenario.ExecExpect{ExitCode: intp(0)},
	})
	require.NoError(t, err)
	require.Len(t, res.Mismatches, 1)
	assert.Equal(t, "build", res.Mismatches[0].Field)
	assert.Contains(t, res.Mismatches[0].Received, "main.go:3")

	var report bytes.Buffer
	WriteReport(&report, "builds", "AC-1", res)
	v := triage.Classify(triage.Input{
		JourneyID: "compiled-test-scenarios-intent-x-builds-custom-command",
		Adapter:   "custom-command", Status: "failed", FailureText: report.String(), ProjectDir: project,
	})
	assert.Equal(t, triage.ClassProductDefect, v.Class, "%+v", v)
}

func TestProductBuildFailure_OnlyProductSources(t *testing.T) {
	project := t.TempDir()
	for _, rel := range []string{"main.go", "cmd/tool/run.go", "main_test.go", "src/index.ts", "src/index.test.ts"} {
		require.NoError(t, os.MkdirAll(filepath.Dir(filepath.Join(project, rel)), 0o755))
		require.NoError(t, os.WriteFile(filepath.Join(project, rel), []byte("x"), 0o644))
	}
	for output, want := range map[string]bool{
		"./main.go:3:15: undefined: x":                                     true,
		"cmd/tool/run.go:9:2: syntax error":                                true,
		project + "/cmd/tool/run.go:9:2: syntax error":                     true,
		"./main_test.go:4:1: undefined: y":                                 false, // a test file is not the product
		"stat /tmp/p/nope: directory not found":                            false, // the case's build step is wrong
		"/usr/local/go/src/fmt/print.go:12: something":                     false,
		"go: cannot find main module, but found .git/config in /tmp/p":     false,
		"./gone.go:1:1: expected 'package'":                                false, // not in the project
		"src/index.ts(3,7): error TS2322: Type 'string' is not assignable": true,  // tsc
		"src/index.ts:3:7 - error TS2322: Type 'string'":                   true,  // tsc --pretty
		"src/index.test.ts(1,1): error TS2304: Cannot find name 'x'":       false,
		"tsconfig.json(2,3): error TS5023: Unknown compiler option":        false,
	} {
		assert.Equal(t, want, productBuildFailure(project, output), output)
	}
}
