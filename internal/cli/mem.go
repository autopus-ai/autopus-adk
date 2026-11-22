package cli

import (
	"errors"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"

	"github.com/insajin/autopus-adk/pkg/memindex"
)

func newMemCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:           "mem",
		Short:         "Build and query the decision/quality memory index",
		SilenceUsage:  true,
		SilenceErrors: true,
	}
	cmd.AddCommand(newMemRebuildCmd())
	cmd.AddCommand(newMemSearchCmd())
	cmd.AddCommand(newMemContextCmd())
	cmd.AddCommand(newMemStatusCmd())
	return cmd
}

func newMemRebuildCmd() *cobra.Command {
	var opts memCommandOptions
	cmd := &cobra.Command{
		Use:   "rebuild",
		Short: "Rebuild the decision/quality projection",
		RunE: func(cmd *cobra.Command, args []string) error {
			jsonMode, err := resolveJSONMode(opts.JSONOut, opts.Format)
			if err != nil {
				return err
			}
			result, err := memindex.Rebuild(memindex.Options{ProjectDir: opts.ProjectDir, IndexPath: opts.IndexPath})
			if err != nil {
				if jsonMode {
					return writeJSONResultAndExit(cmd, jsonStatusError, err, memErrorCode(err, "mem_rebuild_failed"), result, nil, nil)
				}
				return err
			}
			if jsonMode {
				return writeJSONResult(cmd, jsonStatusOK, result, nil, nil)
			}
			fmt.Fprintf(cmd.OutOrStdout(), "rebuilt %s\n", memDisplayPath(opts.ProjectDir, result.IndexPath))
			printMemSkips(cmd.OutOrStdout(), result.SkippedCountsByReason)
			return nil
		},
	}
	addMemProjectFlags(cmd, &opts)
	addJSONFlags(cmd, &opts.JSONOut, &opts.Format)
	return cmd
}

func newMemSearchCmd() *cobra.Command {
	var opts memSearchOptions
	cmd := &cobra.Command{
		Use:   "search <query>",
		Short: "Search the decision/quality projection",
		Args:  cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			jsonMode, err := resolveJSONMode(opts.JSONOut, opts.Format)
			if err != nil {
				return err
			}
			result, err := memindex.Search(memindex.SearchOptions{
				ProjectDir:   opts.ProjectDir,
				IndexPath:    opts.IndexPath,
				Query:        joinArgs(args),
				TopK:         opts.TopK,
				RequireFresh: opts.RequireFresh,
			})
			if err != nil {
				if jsonMode {
					return writeJSONResultAndExit(cmd, jsonStatusError, err, memErrorCode(err, "mem_search_failed"), result, nil, nil)
				}
				return err
			}
			if jsonMode {
				return writeJSONResult(cmd, jsonStatusOK, result, nil, nil)
			}
			for _, match := range result.Results {
				fmt.Fprintf(cmd.OutOrStdout(), "%d. %s %s\n", match.Rank, match.SourceRef, match.SnippetDigest)
			}
			return nil
		},
	}
	addMemProjectFlags(cmd, &opts.memCommandOptions)
	cmd.Flags().IntVar(&opts.TopK, "top-k", 10, "Maximum results")
	cmd.Flags().BoolVar(&opts.RequireFresh, "require-fresh", false, "Fail if indexed sources are stale")
	addJSONFlags(cmd, &opts.JSONOut, &opts.Format)
	return cmd
}

func newMemContextCmd() *cobra.Command {
	var opts memContextOptions
	cmd := &cobra.Command{
		Use:   "context",
		Short: "Render bounded quality recall context",
		RunE: func(cmd *cobra.Command, args []string) error {
			jsonMode, jsonErr := resolveJSONMode(opts.JSONOut, opts.Format)
			if jsonErr != nil && opts.Format != "prompt" {
				return jsonErr
			}
			if opts.Query == "" && len(args) > 0 {
				opts.Query = joinArgs(args)
			}
			if opts.Query == "" {
				err := fmt.Errorf("missing --query")
				if jsonMode {
					return writeJSONResultAndExit(cmd, jsonStatusError, err, "mem_context_missing_query", memindex.ContextResult{}, nil, nil)
				}
				return err
			}
			if opts.Format == "prompt" {
				result, err := memindex.Context(memindex.ContextOptions{
					ProjectDir:   opts.ProjectDir,
					IndexPath:    opts.IndexPath,
					Query:        opts.Query,
					BudgetTokens: opts.BudgetTokens,
					TopK:         opts.TopK,
				})
				if err != nil {
					return err
				}
				fmt.Fprint(cmd.OutOrStdout(), result.Prompt)
				return nil
			}
			result, err := memindex.Context(memindex.ContextOptions{
				ProjectDir:   opts.ProjectDir,
				IndexPath:    opts.IndexPath,
				Query:        opts.Query,
				BudgetTokens: opts.BudgetTokens,
				TopK:         opts.TopK,
			})
			if err != nil {
				if jsonMode {
					return writeJSONResultAndExit(cmd, jsonStatusError, err, memErrorCode(err, "mem_context_failed"), result, nil, nil)
				}
				return err
			}
			if jsonMode {
				return writeJSONResult(cmd, jsonStatusOK, result, nil, nil)
			}
			fmt.Fprint(cmd.OutOrStdout(), result.Prompt)
			return nil
		},
	}
	addMemProjectFlags(cmd, &opts.memCommandOptions)
	cmd.Flags().StringVar(&opts.Query, "query", "", "Recall query")
	cmd.Flags().IntVar(&opts.BudgetTokens, "budget-tokens", 800, "Approximate token budget")
	cmd.Flags().IntVar(&opts.TopK, "top-k", 20, "Maximum search results before budgeting")
	cmd.Flags().BoolVar(&opts.JSONOut, "json", false, "Output as JSON")
	cmd.Flags().StringVar(&opts.Format, "format", "text", "Output format (text|json|prompt)")
	return cmd
}

func newMemStatusCmd() *cobra.Command {
	var opts memCommandOptions
	cmd := &cobra.Command{
		Use:   "status",
		Short: "Report decision/quality projection status",
		RunE: func(cmd *cobra.Command, args []string) error {
			jsonMode, err := resolveJSONMode(opts.JSONOut, opts.Format)
			if err != nil {
				return err
			}
			result, err := memindex.Status(memindex.Options{ProjectDir: opts.ProjectDir, IndexPath: opts.IndexPath})
			if err != nil {
				if jsonMode {
					return writeJSONResultAndExit(cmd, jsonStatusError, err, memErrorCode(err, "mem_status_failed"), result, nil, nil)
				}
				return err
			}
			if jsonMode {
				return writeJSONResult(cmd, jsonStatusOK, result, nil, nil)
			}
			fmt.Fprintf(cmd.OutOrStdout(), "index: %s rebuild_recommended=%t\n", memDisplayPath(opts.ProjectDir, result.IndexPath), result.RebuildRecommended)
			printMemSkips(cmd.OutOrStdout(), result.SkippedCountsByReason)
			return nil
		},
	}
	addMemProjectFlags(cmd, &opts)
	addJSONFlags(cmd, &opts.JSONOut, &opts.Format)
	return cmd
}

type memCommandOptions struct {
	ProjectDir string
	IndexPath  string
	JSONOut    bool
	Format     string
}

type memSearchOptions struct {
	memCommandOptions
	TopK         int
	RequireFresh bool
}

type memContextOptions struct {
	memCommandOptions
	Query        string
	BudgetTokens int
	TopK         int
}

func addMemProjectFlags(cmd *cobra.Command, opts *memCommandOptions) {
	cmd.Flags().StringVar(&opts.ProjectDir, "project-dir", ".", "Project directory")
	cmd.Flags().StringVar(&opts.IndexPath, "index-path", "", "Projection SQLite path")
}

func memErrorCode(err error, fallback string) string {
	var memErr *memindex.Error
	if errors.As(err, &memErr) && memErr.Code != "" {
		return memErr.Code
	}
	return fallback
}

func joinArgs(args []string) string {
	out := ""
	for _, arg := range args {
		if out != "" {
			out += " "
		}
		out += arg
	}
	return out
}

func memDisplayPath(projectDir, path string) string {
	base, err := filepath.Abs(projectDir)
	if err != nil {
		return filepath.ToSlash(filepath.Base(path))
	}
	target, err := filepath.Abs(path)
	if err != nil {
		return filepath.ToSlash(filepath.Base(path))
	}
	rel, err := filepath.Rel(base, target)
	if err != nil || strings.HasPrefix(rel, ".."+string(filepath.Separator)) || rel == ".." {
		return filepath.ToSlash(filepath.Base(path))
	}
	return filepath.ToSlash(rel)
}
