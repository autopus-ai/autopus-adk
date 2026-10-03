package cli

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"

	qadiscover "github.com/insajin/autopus-adk/pkg/qa/discover"
	qarecord "github.com/insajin/autopus-adk/pkg/qa/record"
)

type qaDiscoverOptions struct {
	ProjectDir string
	Origin     string
	Journey    string
	ID         string
	MaxPages   int
	JSONOut    bool
	Format     string
}

func newQADiscoverCmd() *cobra.Command {
	return newQADiscoverCmdWith(nil)
}

// newQADiscoverCmdWith takes the process seam so tests can stand in for the
// node crawler; nil runs the real node.
func newQADiscoverCmdWith(runner qadiscover.Runner) *cobra.Command {
	var opts qaDiscoverOptions
	cmd := &cobra.Command{
		Use:   "discover",
		Short: "Crawl the app read-only and write a baseline candidate scenario",
		Long: "Runs a harness-generated Playwright crawler that only calls page.goto on same-origin links, " +
			"then writes one baseline candidate with a screen per page asserting its title and headings. " +
			"A baseline is a regression baseline, not a correctness proof. The origin must be allowed " +
			"by a Journey Pack unless it is passed explicitly with --origin.",
		Args:         cobra.NoArgs,
		SilenceUsage: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			// Naming the origin on the command line is the explicit consent
			// that lets discovery reach an origin no pack allows.
			explicit := cmd.Flags().Changed("origin") && strings.TrimSpace(opts.Origin) != ""
			return runQADiscover(cmd, opts, explicit, runner)
		},
	}
	cmd.Flags().StringVar(&opts.ProjectDir, "project-dir", ".", "Project directory")
	cmd.Flags().StringVar(&opts.Origin, "origin", "", "Origin to crawl (default: the Journey Pack's first allowed origin)")
	cmd.Flags().StringVar(&opts.Journey, "journey", "", "Journey Pack that runs the baseline")
	cmd.Flags().StringVar(&opts.ID, "id", qadiscover.DefaultID, "Candidate scenario id (lowercase kebab-case)")
	cmd.Flags().IntVar(&opts.MaxPages, "max-pages", qadiscover.DefaultMaxPages, "Maximum pages to crawl")
	addJSONFlags(cmd, &opts.JSONOut, &opts.Format)
	return cmd
}

func runQADiscover(cmd *cobra.Command, opts qaDiscoverOptions, explicit bool, runner qadiscover.Runner) error {
	jsonMode, err := resolveJSONMode(opts.JSONOut, opts.Format)
	if err != nil {
		return err
	}
	result, err := qadiscover.Run(qaRecordContext(cmd), opts.ProjectDir, qadiscover.Options{
		Origin: opts.Origin, Explicit: explicit, MaxPages: opts.MaxPages,
		Journey: opts.Journey, ID: opts.ID, Exec: runner,
	})
	if err != nil {
		code, setupGap := qarecord.CodeOf(err)
		if code == "" {
			code = "qa_discover_failed"
		}
		if jsonMode {
			return writeJSONResultAndExit(cmd, jsonStatusError, err, code, map[string]any{"setup_gap": setupGap}, nil, nil)
		}
		if setupGap {
			return fmt.Errorf("setup gap (%s): %w", code, err)
		}
		return err
	}
	if jsonMode {
		return writeJSONResult(cmd, jsonStatusOK, result, nil, nil)
	}
	out := cmd.OutOrStdout()
	verb := "unchanged"
	if result.Created {
		verb = "created"
	}
	fmt.Fprintf(out, "discovered %d page(s) on %s (journey %s)\n", result.Pages, result.Origin, result.Journey)
	fmt.Fprintf(out, "baseline candidate %s: %s  screens=%d steps=%d\n",
		verb, filepath.ToSlash(result.Path), result.Screens, result.Steps)
	for _, page := range result.Skipped {
		fmt.Fprintf(out, "skipped %s: %s\n", page.Path, page.Reason)
	}
	fmt.Fprintf(out, "note: this is a %s\n", result.Notice)
	fmt.Fprintln(out, "next: review every expectation, then promote it with auto qa scenario promote")
	return nil
}
