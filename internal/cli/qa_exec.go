package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"

	"github.com/spf13/cobra"

	"github.com/insajin/autopus-adk/pkg/qa/execcheck"
	"github.com/insajin/autopus-adk/pkg/qa/testscenario"
)

// qaExecExitMismatch is the exit status of a case whose expectations did not
// hold. Any other failure, such as a build that does not compile, exits 2, so
// a journey can tell a product verdict from a case that never ran.
const (
	qaExecExitMismatch = 1
	qaExecExitError    = 2
)

func newQAExecCmd() *cobra.Command {
	var file, caseID, projectDir string
	var jsonOut bool
	cmd := &cobra.Command{
		Use:   "exec",
		Short: "Run one exec test case against the product and check what a user observes",
		Long: "Loads a qamesh.test-scenarios.v1 file, builds the product if the case asks, runs it in a fresh " +
			"directory holding only the case's files, and checks the exit code, stdout, stderr, and files the " +
			"case expects. A failed expectation prints Expected:/Received: lines and exits 1; a case that cannot " +
			"run exits 2. 'auto qa run' and 'auto qa loop' call this for every exec case.",
		Args:          cobra.NoArgs,
		SilenceUsage:  true,
		SilenceErrors: true,
		RunE: func(cmd *cobra.Command, _ []string) error {
			code, err := runQAExec(cmd, projectDir, file, caseID, jsonOut)
			if err != nil {
				fmt.Fprintln(cmd.ErrOrStderr(), "auto qa exec:", err)
			}
			if code != 0 {
				return &qaExecExit{code: code}
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&file, "file", "", "Test-scenarios file, relative to the project (required)")
	cmd.Flags().StringVar(&caseID, "case", "", "Case id inside the file (required)")
	cmd.Flags().StringVar(&projectDir, "project-dir", ".", "Project directory")
	cmd.Flags().BoolVar(&jsonOut, "json", false, "Print the observed result as JSON")
	return cmd
}

func runQAExec(cmd *cobra.Command, projectDir, file, caseID string, jsonOut bool) (int, error) {
	if file == "" || caseID == "" {
		return qaExecExitError, fmt.Errorf("--file and --case are required")
	}
	path := file
	if !filepath.IsAbs(path) {
		path = filepath.Join(projectDir, file)
	}
	doc, err := testscenario.LoadFile(path)
	if err != nil {
		return qaExecExitError, err
	}
	var found *testscenario.Case
	for i := range doc.Cases {
		if doc.Cases[i].ID == caseID {
			found = &doc.Cases[i]
		}
	}
	if found == nil || found.Automation.Type != testscenario.AutomationExec || found.Automation.Exec == nil {
		return qaExecExitError, fmt.Errorf("%s has no exec case %q", file, caseID)
	}
	ctx := cmd.Context()
	if ctx == nil {
		ctx = context.Background()
	}
	res, err := execcheck.Run(ctx, projectDir, *found.Automation.Exec)
	if err != nil {
		return qaExecExitError, err
	}
	out := cmd.OutOrStdout()
	if jsonOut {
		data, _ := json.MarshalIndent(res, "", "  ")
		fmt.Fprintln(out, string(data))
	}
	if res.Passed() {
		if !jsonOut {
			fmt.Fprintf(out, "exec case %s (%s): passed\n", caseID, found.Ac)
		}
		return 0, nil
	}
	execcheck.WriteReport(out, caseID, found.Ac, res)
	return qaExecExitMismatch, nil
}

// qaExecExit ends the command with code after it reported its own outcome.
type qaExecExit struct{ code int }

func (e *qaExecExit) Error() string    { return fmt.Sprintf("qa exec exited %d", e.code) }
func (e *qaExecExit) ExitCode() int    { return e.code }
func (e *qaExecExit) alreadyReported() {}
