package cli

import (
	"context"
	"flag"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"testing"

	"github.com/stretchr/testify/require"
)

// sandboxCoverageFixtureCommand permits only the test harness's coverage output
// directory in addition to the caller's unchanged sandbox policy. Sharing the
// parent test.gocoverdir preserves child counters in the enclosing coverprofile.
func sandboxCoverageFixtureCommand(t *testing.T, ctx context.Context, profile, testName string) *exec.Cmd {
	t.Helper()
	arguments := []string{"-test.run=^" + testName + "$"}
	environment := os.Environ()
	if testing.CoverMode() != "" {
		coverageDir := ""
		if option := flag.Lookup("test.gocoverdir"); option != nil {
			coverageDir = option.Value.String()
		}
		if coverageDir == "" {
			coverageDir = t.TempDir()
		}
		coverageDir, err := filepath.EvalSymlinks(coverageDir)
		require.NoError(t, err)
		coverageDir, err = filepath.Abs(coverageDir)
		require.NoError(t, err)
		profile += "\n(allow file-write* (subpath " + strconv.Quote(coverageDir) + "))\n"
		arguments = append(arguments, "-test.gocoverdir="+coverageDir)
		environment = append(environment, "GOCOVERDIR="+coverageDir)
	}
	arguments = append([]string{"-p", profile, os.Args[0]}, arguments...)
	cmd := exec.CommandContext(ctx, "/usr/bin/sandbox-exec", arguments...)
	cmd.Env = environment
	return cmd
}
