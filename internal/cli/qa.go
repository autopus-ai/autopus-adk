package cli

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"
)

// @AX:ANCHOR [AUTO] @AX:SPEC: SPEC-QAMESH-004: public `auto qa` namespace registration fans into CLI root and QA command tests.
// @AX:REASON: init, plan, scenario, run, explore, release, readiness, evidence, and feedback subcommands must remain reachable under the same namespace.
func newQACmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:           "qa",
		Short:         "Plan, initialize, run, and publish QAMESH QA evidence",
		SilenceUsage:  true,
		SilenceErrors: true,
	}
	cmd.AddCommand(newQAInitCmd())
	cmd.AddCommand(newQAFullCmd())
	cmd.AddCommand(newQAPlanCmd())
	cmd.AddCommand(newQAScenarioCmd())
	cmd.AddCommand(newQAAdaptersCmd())
	cmd.AddCommand(newQARunCmd())
	cmd.AddCommand(newQACoverageCmd())
	cmd.AddCommand(newQAReportCmd())
	cmd.AddCommand(newQAProfileCmd())
	cmd.AddCommand(newQAExploreCmd())
	cmd.AddCommand(newQAReleaseCmd())
	cmd.AddCommand(newQAReadinessCmd())
	cmd.AddCommand(newQADomainReadinessCmd())
	cmd.AddCommand(newQAEvidenceCmd())
	cmd.AddCommand(newQAFeedbackCmd())
	cmd.AddCommand(newQAReleaseReadinessCmd())
	cmd.AddCommand(newQALoopCmd())
	return cmd
}

func requireFlag(name, value string) error {
	if value == "" {
		return fmt.Errorf("missing --%s", name)
	}
	return nil
}

// @AX:NOTE [AUTO] @AX:SPEC: SPEC-QAMESH-004: generated-surface deny-list protects QA output flags from writing into harness runtime directories.
func rejectGeneratedQAOutput(name, value string) error {
	rel := strings.ToLower(filepath.ToSlash(filepath.Clean(value)))
	for _, denied := range []string{".codex", ".claude", ".gemini", ".opencode", ".autopus/plugins"} {
		if rel == denied || strings.HasPrefix(rel, denied+"/") || strings.Contains(rel, "/"+denied+"/") || strings.HasSuffix(rel, "/"+denied) {
			return fmt.Errorf("--%s may not target generated surface %s", name, denied)
		}
	}
	return nil
}
