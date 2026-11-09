package cli

import (
	"context"
	"fmt"
	"path/filepath"
	"sort"

	"github.com/spf13/cobra"

	"github.com/insajin/autopus-adk/pkg/qa/execcheck"
	"github.com/insajin/autopus-adk/pkg/qa/testscenario"
)

// runQAExecAll runs every exec case in the project's test-scenarios files, the
// regression check `auto verify stop` and CI call after a change. A case whose
// expectation fails decides the exit status 1 over a case that could not run,
// which exits 2 only when nothing failed: a product verdict is the stronger
// signal, and an unrunnable case is not caused by the product change.
func runQAExecAll(cmd *cobra.Command, projectDir string) (int, error) {
	paths, _ := filepath.Glob(filepath.Join(projectDir, testscenario.DirRel, "*.yaml"))
	sort.Strings(paths)
	ctx := cmd.Context()
	if ctx == nil {
		ctx = context.Background()
	}
	out := cmd.OutOrStdout()
	passed, failed, broken := 0, 0, 0
	for _, path := range paths {
		rel, _ := filepath.Rel(projectDir, path)
		doc, err := testscenario.LoadFile(path)
		if err != nil {
			fmt.Fprintf(out, "%s: could not load: %v\n", filepath.ToSlash(rel), err)
			broken++
			continue
		}
		for _, c := range doc.Cases {
			if c.Automation.Type != testscenario.AutomationExec || c.Automation.Exec == nil {
				continue
			}
			res, err := execcheck.Run(ctx, projectDir, *c.Automation.Exec)
			switch {
			case err != nil:
				fmt.Fprintf(out, "exec case %s (%s): could not run: %v\n", c.ID, c.Ac, err)
				broken++
			case res.Passed():
				passed++
			default:
				execcheck.WriteReport(out, c.ID, c.Ac, res)
				failed++
			}
		}
	}
	fmt.Fprintf(out, "qa exec --all: %d passed, %d failed, %d could not run\n", passed, failed, broken)
	switch {
	case failed > 0:
		return qaExecExitMismatch, nil
	case broken > 0:
		return qaExecExitError, nil
	}
	return 0, nil
}
