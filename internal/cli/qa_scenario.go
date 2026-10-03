package cli

import (
	"fmt"
	"path/filepath"

	"github.com/spf13/cobra"

	"github.com/insajin/autopus-adk/pkg/qa/journey"
	qascenario "github.com/insajin/autopus-adk/pkg/qa/scenario"
)

type qaScenarioOptions struct {
	ProjectDir string
	Journey    string
	Origin     string
	DryRun     bool
	JSONOut    bool
	Format     string
}

func newQAScenarioCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "scenario",
		Short: "Author user scenarios and compile them into runner specs",
		Long: "Scenarios are the project's declaration of what a user sees. " +
			"'compile' renders them into Playwright specs under the project testDir, " +
			"tagged @explore so the gui-explore lane runs them.",
		SilenceUsage:  true,
		SilenceErrors: true,
	}
	cmd.AddCommand(newQAScenarioInitCmd())
	cmd.AddCommand(newQAScenarioCompileCmd())
	cmd.AddCommand(newQAScenarioGenerateCmd())
	cmd.AddCommand(newQAScenarioPromoteCmd())
	return cmd
}

func newQAScenarioInitCmd() *cobra.Command {
	var opts qaScenarioOptions
	cmd := &cobra.Command{
		Use:   "init",
		Short: "Write an example scenario to edit",
		RunE: func(cmd *cobra.Command, args []string) error {
			return runQAScenarioInit(cmd, opts)
		},
	}
	cmd.Flags().StringVar(&opts.ProjectDir, "project-dir", ".", "Project directory")
	cmd.Flags().StringVar(&opts.Journey, "journey", "", "Journey Pack that runs the scenario")
	addJSONFlags(cmd, &opts.JSONOut, &opts.Format)
	return cmd
}

func newQAScenarioCompileCmd() *cobra.Command {
	var opts qaScenarioOptions
	cmd := &cobra.Command{
		Use:   "compile",
		Short: "Compile scenarios into Playwright specs",
		RunE: func(cmd *cobra.Command, args []string) error {
			return runQAScenarioCompile(cmd, opts)
		},
	}
	cmd.Flags().StringVar(&opts.ProjectDir, "project-dir", ".", "Project directory")
	cmd.Flags().BoolVar(&opts.DryRun, "dry-run", false, "Validate and render without writing specs")
	addJSONFlags(cmd, &opts.JSONOut, &opts.Format)
	return cmd
}

func runQAScenarioInit(cmd *cobra.Command, opts qaScenarioOptions) error {
	jsonMode, err := resolveJSONMode(opts.JSONOut, opts.Format)
	if err != nil {
		return err
	}
	journeyID, origin := resolveScenarioJourney(opts.ProjectDir, opts.Journey)
	path, created, err := qascenario.WriteStarter(opts.ProjectDir, journeyID, origin)
	if err != nil {
		if jsonMode {
			return writeJSONResultAndExit(cmd, jsonStatusError, err, "qa_scenario_init_failed", nil, nil, nil)
		}
		return err
	}
	payload := map[string]any{
		"path":       filepath.ToSlash(path),
		"created":    created,
		"journey":    journeyID,
		"scenario":   qascenario.StarterID,
		"next_steps": []string{"auto qa scenario compile --project-dir " + opts.ProjectDir},
	}
	if jsonMode {
		return writeJSONResult(cmd, jsonStatusOK, payload, nil, nil)
	}
	out := cmd.OutOrStdout()
	verb := "exists"
	if created {
		verb = "created"
	}
	fmt.Fprintf(out, "scenario %s: %s\n", verb, filepath.ToSlash(path))
	fmt.Fprintf(out, "journey: %s\n", journeyID)
	fmt.Fprintf(out, "next: auto qa scenario compile --project-dir %s\n", opts.ProjectDir)
	return nil
}

func runQAScenarioCompile(cmd *cobra.Command, opts qaScenarioOptions) error {
	jsonMode, err := resolveJSONMode(opts.JSONOut, opts.Format)
	if err != nil {
		return err
	}
	result, err := qascenario.CompileProject(opts.ProjectDir, opts.DryRun)
	if err != nil {
		if jsonMode {
			return writeJSONResultAndExit(cmd, jsonStatusError, err, "qa_scenario_compile_failed", nil, nil, nil)
		}
		return err
	}
	if jsonMode {
		return writeJSONResult(cmd, jsonStatusOK, result, nil, nil)
	}
	writeQAScenarioText(cmd, result, opts.ProjectDir)
	return nil
}

func writeQAScenarioText(cmd *cobra.Command, result qascenario.Result, projectDir string) {
	out := cmd.OutOrStdout()
	mode := "wrote"
	if result.DryRun {
		mode = "would write"
	}
	fmt.Fprintf(out, "testDir: %s", result.TestDir)
	if result.ConfigRef != "" {
		fmt.Fprintf(out, " (from %s)", result.ConfigRef)
	}
	fmt.Fprintln(out)
	for _, item := range result.Compiled {
		fmt.Fprintf(out, "%s %s  screens=%d steps=%d bytes=%d\n",
			mode, item.SpecPath, item.Screens, item.Steps, item.Bytes)
	}
	// The screen matrix is what turns declared coverage into an enforced oracle,
	// so the count is surfaced even in text mode.
	for id, rows := range result.ScreenMatrix {
		fmt.Fprintf(out, "screen_matrix for %s: %d rows (paste into the pack to enforce coverage)\n", id, len(rows))
	}
	fmt.Fprintf(out, "next: auto qa explore --project-dir %s\n", projectDir)
}

// resolveScenarioJourney picks the Journey Pack a starter scenario targets. An
// explicit flag wins; otherwise the first pack that declares GUI origins is
// used, because those are the only packs a compiled scenario can run under.
func resolveScenarioJourney(projectDir, explicit string) (string, string) {
	packs, err := journey.LoadDir(projectDir)
	if err != nil {
		return explicit, ""
	}
	for _, pack := range packs {
		if explicit != "" && pack.ID != explicit {
			continue
		}
		if len(pack.GUI.AllowedOrigins) == 0 {
			continue
		}
		return pack.ID, pack.GUI.AllowedOrigins[0]
	}
	return explicit, ""
}
