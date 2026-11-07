// Package cli implements the status command for SPEC dashboard display.
package cli

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"

	"github.com/insajin/autopus-adk/internal/cli/tui"
	"github.com/insajin/autopus-adk/pkg/spec"
)

// specEntry holds parsed data for a single SPEC.
type specEntry struct {
	module string
	id     string
	status string
	title  string
	path   string
}

func (s specEntry) displayID() string {
	if s.module == "" {
		return s.id
	}
	return fmt.Sprintf("[%s] %s", s.module, s.id)
}

func (s specEntry) isDone() bool {
	switch normalizeSpecStatus(s.status) {
	case "done", "completed", "implemented":
		return true
	default:
		return false
	}
}

// statusIcon returns the display icon for a given SPEC status.
func statusIcon(status string) string {
	switch {
	case specEntry{status: status}.isDone():
		return "✓"
	case normalizeSpecStatus(status) == "approved", normalizeSpecStatus(status) == "in-progress":
		return "→"
	default:
		return "○"
	}
}

// parseSpecFile reads a spec.md file and extracts status and title.
// Returns empty strings on parse failure (graceful degradation).
func parseSpecFile(path string) (status, title string) {
	content, err := os.ReadFile(path)
	if err == nil {
		meta := spec.ParseSpecMetadata(string(content))
		if meta.Status != "" || meta.Title != "" {
			return meta.Status, meta.Title
		}
	}

	f, err := os.Open(path)
	if err != nil {
		return "", ""
	}
	defer f.Close() //nolint:errcheck

	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := scanner.Text()

		// Extract title from H1 heading: "# SPEC-ID: Title"
		if title == "" && strings.HasPrefix(line, "# ") {
			parts := strings.SplitN(line[2:], ":", 2)
			if len(parts) == 2 {
				title = strings.TrimSpace(parts[1])
			}
		}

		// Extract status from bold markdown: "**Status**: value"
		if status == "" {
			lower := strings.ToLower(line)
			if strings.Contains(lower, "**status**") || strings.Contains(lower, "**status**: ") {
				// Match patterns like: **Status**: draft
				idx := strings.Index(lower, "**status**")
				if idx >= 0 {
					rest := line[idx+len("**status**"):]
					rest = strings.TrimLeft(rest, " *:") // strip ": " or ":"
					rest = strings.TrimSpace(rest)
					// Take first word as status value
					fields := strings.Fields(rest)
					if len(fields) > 0 {
						status = fields[0]
					}
				}
			}
		}

		// Stop scanning once both fields are found
		if status != "" && title != "" {
			break
		}
	}

	return status, title
}

// scanSpecs reads all SPEC directories under specsDir and returns parsed entries.
func scanSpecs(specsDir string) ([]specEntry, error) {
	entries, err := os.ReadDir(specsDir)
	if err != nil {
		return nil, err
	}

	var specs []specEntry
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		name := e.Name()
		if !strings.HasPrefix(name, "SPEC-") {
			continue
		}

		specFile := filepath.Join(specsDir, name, "spec.md")
		status, title := parseSpecFile(specFile)
		if status == "" {
			status = "draft"
		}

		specs = append(specs, specEntry{
			path:   specFile,
			id:     name,
			status: status,
			title:  title,
		})
	}

	return specs, nil
}

func newStatusCmd() *cobra.Command {
	return newStatusCmdWithOMPDependencies(defaultOMPPlatformDependencies())
}

func newStatusCmdWithOMPDependencies(deps ompPlatformDependencies) *cobra.Command {
	return newStatusCmdWith(deps, defaultStatusProbe())
}

// newStatusCmdWithProbe builds the status command against a given machine
// probe, so tests decide what is installed without touching PATH or home.
func newStatusCmdWithProbe(probe statusProbe) *cobra.Command {
	return newStatusCmdWith(defaultOMPPlatformDependencies(), probe)
}

func newStatusCmdWith(deps ompPlatformDependencies, probe statusProbe) *cobra.Command {
	var (
		dir        string
		platform   string
		jsonOutput bool
		format     string
	)

	cmd := &cobra.Command{
		Use:   "status",
		Short: "Show harness status and next verification steps",
		Long:  "SPEC 디렉터리를 스캔하여 모든 SPEC의 상태를 표시합니다.",
		RunE: func(cmd *cobra.Command, args []string) error {
			jsonMode, err := resolveJSONMode(jsonOutput, format)
			if err != nil {
				if jsonOutput {
					return writeJSONResultAndExit(
						cmd,
						jsonStatusError,
						err,
						"invalid_format",
						map[string]any{},
						nil,
						nil,
					)
				}
				return err
			}
			if dir == "" {
				dir, err = os.Getwd()
				if err != nil {
					if jsonMode {
						return writeJSONResultAndExit(
							cmd,
							jsonStatusError,
							fmt.Errorf("현재 디렉터리를 가져올 수 없음: %w", err),
							"cwd_unavailable",
							map[string]any{},
							nil,
							nil,
						)
					}
					return fmt.Errorf("현재 디렉터리를 가져올 수 없음: %w", err)
				}
			}
			selectedPlatform := strings.ToLower(strings.TrimSpace(platform))
			if selectedPlatform != "" {
				if selectedPlatform != "omp" {
					err := fmt.Errorf("unsupported status platform %q: must be omp", platform)
					if jsonMode {
						return writeJSONResultAndExit(
							cmd, jsonStatusError, err, "unsupported_platform",
							map[string]any{"platform": selectedPlatform}, nil, nil,
						)
					}
					return err
				}
				return runOMPPlatformStatus(cmd, dir, jsonMode, deps)
			}

			if jsonMode {
				return runStatusJSONWith(cmd, dir, probe)
			}
			return runStatusTextWith(cmd, dir, probe)
		},
	}

	cmd.Flags().StringVar(&dir, "dir", "", "프로젝트 루트 디렉터리 (기본값: 현재 디렉터리)")
	cmd.Flags().StringVar(&platform, "platform", "", "Platform operator view (omp)")
	addJSONFlags(cmd, &jsonOutput, &format)
	return cmd
}

func runStatusText(cmd *cobra.Command, dir string) error {
	return runStatusTextWith(cmd, dir, defaultStatusProbe())
}

func runStatusTextWith(cmd *cobra.Command, dir string, probe statusProbe) error {
	out := cmd.OutOrStdout()

	specs := scanAllSpecs(dir)
	report := collectStatusReport(dir, specs, probe)
	hygiene := collectStatusHygiene(dir)
	if len(specs) == 0 {
		// The harness verifies work rather than planning it, so an empty
		// project is pointed at review and QA, not at a SPEC.
		_, _ = fmt.Fprintln(out, "SPEC이 없습니다. 변경을 마친 뒤 아래 리뷰와 QA로 검증하세요.")
		renderStatusEnvironment(out, report.env)
		renderStatusNext(out, report.actions)
		renderHygieneText(out, hygiene)
		return nil
	}

	tui.BannerWithInfo(out, "Project Status", "specs")

	doneCount := 0
	for i, s := range specs {
		icon := statusIcon(s.status)

		var iconStyled string
		switch icon {
		case "✓":
			iconStyled = tui.SuccessLabelStyle.Render(icon)
			doneCount++
		case "→":
			iconStyled = tui.BrandStyle.Render(icon)
		default:
			iconStyled = tui.MutedStyle.Render(icon)
		}

		statusStyled := tui.MutedStyle.Render(fmt.Sprintf("%-8s", s.status))
		idStyled := tui.BoldStyle.Render(fmt.Sprintf("%-14s", s.displayID()))
		labelStyled := tui.MutedStyle.Render(fmt.Sprintf("%-15s", report.evidence[i].Label))
		_, _ = fmt.Fprintf(out, "  %s %s %s  %s  %s\n", idStyled, iconStyled, statusStyled, labelStyled, s.title)
		renderStatusEvidenceDetails(out, report.evidence[i])
	}

	_, _ = fmt.Fprintln(out)
	// The declared count is what the documents say; only the evidence line
	// below can say verified.
	summary := fmt.Sprintf("전체: %d/%d 완료 (문서에 선언된 상태)", doneCount, len(specs))
	_, _ = fmt.Fprintf(out, "  %s\n", tui.MutedStyle.Render(summary))
	_, _ = fmt.Fprintf(out, "  %s\n", tui.MutedStyle.Render(report.evidenceSummaryText()))
	renderStatusEnvironment(out, report.env)
	renderStatusNext(out, report.actions)
	renderHygieneText(out, hygiene)
	return nil
}
