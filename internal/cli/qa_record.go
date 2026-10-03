package cli

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"

	qarecord "github.com/insajin/autopus-adk/pkg/qa/record"
)

// qaRecordDefaultID names a recording when --id is absent and no file name
// can supply one.
const qaRecordDefaultID = "recorded-journey"

type qaRecordOptions struct {
	ProjectDir   string
	Origin       string
	ID           string
	Title        string
	Journey      string
	From         string
	InputFormat  string
	AllowPartial bool
	JSONOut      bool
	Format       string
}

func newQARecordCmd() *cobra.Command {
	return newQARecordCmdWith(nil)
}

// newQARecordCmdWith takes the process seam so tests can stand in for
// playwright codegen; nil runs the real npx.
func newQARecordCmdWith(runner qarecord.Runner) *cobra.Command {
	var opts qaRecordOptions
	cmd := &cobra.Command{
		Use:   "record",
		Short: "Record a journey with Playwright codegen as a candidate scenario",
		Long: "Opens playwright codegen on --origin, or on the Journey Pack's first allowed origin. " +
			"When the window closes, the recording becomes a v2 recording candidate under " +
			".autopus/qa/scenarios/candidates. 'record import' converts an existing codegen file " +
			"or a qamesh.recording.v1 agent log instead.",
		Args:         cobra.NoArgs,
		SilenceUsage: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runQARecordLive(cmd, opts, runner)
		},
	}
	addQARecordFlags(cmd, &opts)
	cmd.AddCommand(newQARecordImportCmd())
	return cmd
}

func newQARecordImportCmd() *cobra.Command {
	var opts qaRecordOptions
	cmd := &cobra.Command{
		Use:   "import",
		Short: "Import a codegen file or agent JSONL log as a candidate scenario",
		Long: "Converts Playwright codegen JavaScript/TypeScript (.js, .ts) or a qamesh.recording.v1 " +
			"log (.jsonl) into a v2 recording candidate. Lines with no scenario equivalent are listed " +
			"with their line numbers and fail the import unless --allow-partial drops them.",
		Args:         cobra.NoArgs,
		SilenceUsage: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runQARecordImport(cmd, opts)
		},
	}
	addQARecordFlags(cmd, &opts)
	cmd.Flags().StringVar(&opts.From, "from", "", "Codegen .js/.ts file or qamesh.recording.v1 .jsonl log")
	cmd.Flags().StringVar(&opts.InputFormat, "input-format", "auto", "Recording format (auto|codegen|jsonl)")
	_ = cmd.MarkFlagRequired("from")
	return cmd
}

func addQARecordFlags(cmd *cobra.Command, opts *qaRecordOptions) {
	cmd.Flags().StringVar(&opts.ProjectDir, "project-dir", ".", "Project directory")
	cmd.Flags().StringVar(&opts.Origin, "origin", "", "Origin to record against (default: the Journey Pack's first allowed origin)")
	cmd.Flags().StringVar(&opts.ID, "id", "", "Candidate scenario id (lowercase kebab-case)")
	cmd.Flags().StringVar(&opts.Journey, "journey", "", "Journey Pack that runs the scenario")
	cmd.Flags().StringVar(&opts.Title, "title", "", "Scenario title")
	cmd.Flags().BoolVar(&opts.AllowPartial, "allow-partial", false, "Drop unsupported lines instead of failing")
	addJSONFlags(cmd, &opts.JSONOut, &opts.Format)
}

func runQARecordLive(cmd *cobra.Command, opts qaRecordOptions, runner qarecord.Runner) error {
	jsonMode, err := resolveJSONMode(opts.JSONOut, opts.Format)
	if err != nil {
		return err
	}
	id := strings.TrimSpace(opts.ID)
	if id == "" {
		id = qaRecordDefaultID
	}
	result, err := qarecord.Live(qaRecordContext(cmd), opts.ProjectDir, qarecord.LiveOptions{
		Origin: opts.Origin, ID: id, Title: opts.Title, Journey: opts.Journey,
		AllowPartial: opts.AllowPartial, Exec: runner,
	})
	return reportQARecord(cmd, jsonMode, opts, result, err)
}

func runQARecordImport(cmd *cobra.Command, opts qaRecordOptions) error {
	jsonMode, err := resolveJSONMode(opts.JSONOut, opts.Format)
	if err != nil {
		return err
	}
	id := strings.TrimSpace(opts.ID)
	if id == "" {
		base := filepath.Base(opts.From)
		if id = qarecord.Slug(strings.TrimSuffix(base, filepath.Ext(base))); id == "" {
			id = qaRecordDefaultID
		}
	}
	result, err := qarecord.Import(opts.ProjectDir, qarecord.ImportOptions{
		From: opts.From, Format: opts.InputFormat, ID: id, Title: opts.Title,
		Journey: opts.Journey, Origin: opts.Origin, AllowPartial: opts.AllowPartial,
	})
	return reportQARecord(cmd, jsonMode, opts, result, err)
}

func reportQARecord(cmd *cobra.Command, jsonMode bool, opts qaRecordOptions, result qarecord.Result, err error) error {
	if err != nil {
		code, setupGap := qarecord.CodeOf(err)
		if code == "" {
			code = "qa_record_failed"
		}
		if jsonMode {
			data := map[string]any{"setup_gap": setupGap, "unsupported": result.Unsupported}
			if result.Recording != "" {
				data["recording"] = result.Recording
			}
			return writeJSONResultAndExit(cmd, jsonStatusError, err, code, data, nil, nil)
		}
		if result.Recording != "" && code == qarecord.CodeUnsupportedLines {
			fmt.Fprintf(cmd.ErrOrStderr(), "next: auto qa record import --from %s --id %s --allow-partial --project-dir %s\n",
				result.Recording, result.ID, opts.ProjectDir)
		}
		if setupGap {
			return fmt.Errorf("setup gap (%s): %w", code, err)
		}
		return err
	}
	if jsonMode {
		status, warnings := jsonStatusOK, []jsonMessage(nil)
		if n := len(result.Unsupported); n > 0 {
			status = jsonStatusWarn
			warnings = []jsonMessage{{Code: "qa_record_lines_dropped",
				Message: fmt.Sprintf("%d unsupported line(s) were dropped by --allow-partial", n)}}
		}
		return writeJSONResult(cmd, status, result, warnings, nil)
	}
	out := cmd.OutOrStdout()
	verb := "unchanged"
	if result.Created {
		verb = "created"
	}
	fmt.Fprintf(out, "recording candidate %s: %s\n", verb, filepath.ToSlash(result.Path))
	fmt.Fprintf(out, "journey: %s  origin: %s  screens=%d steps=%d\n", result.Journey, result.Origin, result.Screens, result.Steps)
	for _, line := range result.Unsupported {
		fmt.Fprintf(out, "dropped line %d: %s\n", line.Line, line.Text)
	}
	if result.ConfirmRequired > 0 {
		fmt.Fprintf(out, "confirm required: %d agent assertion(s) need a person or an ac before promotion\n", result.ConfirmRequired)
	}
	fmt.Fprintln(out, "next: review the candidate, then promote it with auto qa scenario promote")
	return nil
}

// qaRecordContext returns the command's context, or Background when the
// command runs outside Execute.
func qaRecordContext(cmd *cobra.Command) context.Context {
	if ctx := cmd.Context(); ctx != nil {
		return ctx
	}
	return context.Background()
}
