package mutate

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const sizeProduct = `package main

import (
	"fmt"
	"os"
	"strconv"
)

func main() {
	n, _ := strconv.Atoi(os.Args[1])
	if n > 10 {
		fmt.Println("big")
	} else {
		fmt.Println("small")
	}
	verbose := false
	if verbose {
		fmt.Println("debug")
	}
}
`

// The cases check 20 and 3 but nothing at the boundary, so moving it
// survives; the other faults change output and are caught.
const sizeCases = `schema_version: qamesh.test-scenarios.v1
spec: INTENT-size
cases:
  - id: big
    ac: AC-1
    kind: happy
    title: above ten is big
    given: built
    when: size 20
    then: big
    automation:
      type: exec
      exec:
        build: [go, build, -o, $BIN/size, .]
        argv: [$BIN/size, "20"]
        expect: {stdout: "big\n"}
  - id: small
    ac: AC-2
    kind: happy
    title: below ten is small
    given: built
    when: size 3
    then: small
    automation:
      type: exec
      exec:
        build: [go, build, -o, $BIN/size, .]
        argv: [$BIN/size, "3"]
        expect: {stdout: "small\n"}
`

func TestRun_ReportsTheBoundaryNoCaseChecks(t *testing.T) {
	if _, err := exec.LookPath("go"); err != nil {
		t.Skip("go is not installed")
	}
	dir := t.TempDir()
	for name, body := range map[string]string{
		"go.mod": "module size\n\ngo 1.21\n", "main.go": sizeProduct,
		".autopus/qa/test-scenarios/size.yaml": sizeCases,
	} {
		p := filepath.Join(dir, filepath.FromSlash(name))
		require.NoError(t, os.MkdirAll(filepath.Dir(p), 0o755))
		require.NoError(t, os.WriteFile(p, []byte(body), 0o644))
	}
	git := exec.Command("git", "init", "-q")
	git.Dir = dir
	require.NoError(t, git.Run())

	report, err := Run(context.Background(), Options{ProjectDir: dir})
	require.NoError(t, err)

	assert.Equal(t, []string{"big", "small"}, report.Cases)
	verdicts := map[string]string{}
	for _, o := range report.Outcomes {
		verdicts[o.Operator+"@"+o.Mutated] = o.Verdict
	}
	assert.Equal(t, Survived, verdicts["boundary-gt@\tif n >= 10 {"], "no case runs 10")
	assert.Equal(t, Survived, verdicts["off-by-one@\tif n > 11 {"])
	assert.Equal(t, Killed, verdicts["flip-boolean@\tverbose := true"])
	assert.Equal(t, Killed, verdicts["off-by-one@\tn, _ := strconv.Atoi(os.Args[2])"], "a crash is caught")
	assert.Greater(t, report.Score, 0.0)
	assert.Less(t, report.Score, 1.0)

	data, err := os.ReadFile(filepath.Join(dir, "main.go"))
	require.NoError(t, err)
	assert.Equal(t, sizeProduct, string(data), "the project is never written")
}
