package cli

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	contentfs "github.com/insajin/autopus-adk/content"
	"github.com/insajin/autopus-adk/pkg/config"
	"github.com/insajin/autopus-adk/pkg/content"
	"github.com/spf13/cobra"
)

type skillAuditEntry struct {
	MetadataTokenEstimate int    `json:"metadata_token_estimate"`
	BodyTokenEstimate     int    `json:"body_token_estimate"`
	Name                  string `json:"name"`
	Reason                string `json:"reason"`
	Target                string `json:"target,omitempty"`
	Compiled              bool   `json:"configured_compiled"`
	Visible               bool   `json:"configured_visible"`
	MetadataBytes         int    `json:"metadata_bytes"`
	BodyBytes             int    `json:"body_bytes"`
}

type skillAuditReport struct {
	SchemaVersion      int               `json:"schema_version"`
	Summary            skillAuditSummary `json:"summary"`
	TokenEstimateBasis string            `json:"token_estimate_basis"`
	Scope              string            `json:"scope"`
	SessionLoaded      string            `json:"session_loaded"`
	Platform           string            `json:"platform"`
	ConfigSource       string            `json:"config_source"`
	EstimateBasis      string            `json:"size_basis"`
	DefaultVisible     int               `json:"default_visible"`
	FullVisible        int               `json:"full_visible"`
	Catalog            []skillAuditEntry `json:"catalog"`
	Installed          []skillAuditFile  `json:"installed"`
	Missing            []string          `json:"missing_configured_paths"`
	Duplicates         [][]string        `json:"duplicate_content_paths"`
	Aliases            [][]string        `json:"same_name_paths"`
	Skipped            []string          `json:"skipped"`
}

func newSkillAuditCmd() *cobra.Command {
	var root, platform, format string
	var asJSON bool
	cmd := &cobra.Command{Use: "audit", Short: "Read-only skill exposure and local file diagnostics", Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			if format != "text" && format != "json" {
				return fmt.Errorf("unsupported format %q", format)
			}
			report, err := buildSkillAudit(root, platform)
			if err != nil {
				return err
			}
			if asJSON || format == "json" {
				enc := json.NewEncoder(cmd.OutOrStdout())
				enc.SetIndent("", "  ")
				return enc.Encode(report)
			}
			out := cmd.OutOrStdout()
			fmt.Fprintf(out, "Skill audit: %s | %s | session_loaded=UNKNOWN\n", report.Platform, report.Scope)
			fmt.Fprintf(out, "Config: %s; size basis: %s\nToken estimates: %s\n", report.ConfigSource, report.EstimateBasis, report.TokenEstimateBasis)
			fmt.Fprintf(out, "Configured compiled=%d visible=%d | local metadata=%dB body=%dB | local metadata_token_estimate=%d body_token_estimate=%d\n", report.Summary.ConfiguredCompiled, report.Summary.ConfiguredVisible, report.Summary.LocalMetadataBytes, report.Summary.LocalBodyBytes, report.Summary.LocalMetadataTokenEstimate, report.Summary.LocalBodyTokenEstimate)
			fmt.Fprintf(out, "Catalog default visible: %d | catalog full visible: %d | local files: %d | missing: %d\n", report.DefaultVisible, report.FullVisible, len(report.Installed), len(report.Missing))
			for _, e := range report.Catalog {
				fmt.Fprintf(out, "%s reason=%s compiled=%t visible=%t metadata=%dB body=%dB %s\n", e.Name, e.Reason, e.Compiled, e.Visible, e.MetadataBytes, e.BodyBytes, e.Target)
			}
			for _, f := range report.Installed {
				fmt.Fprintf(out, "local %s name=%s metadata=%dB body=%dB sha256=%s\n", f.Path, f.Name, f.MetadataBytes, f.BodyBytes, f.SHA256)
			}
			for _, p := range report.Missing {
				fmt.Fprintf(out, "missing %s\n", p)
			}
			for _, g := range report.Duplicates {
				fmt.Fprintf(out, "duplicate_content %v\n", g)
			}
			for _, g := range report.Aliases {
				fmt.Fprintf(out, "same_name %v\n", g)
			}
			for _, p := range report.Skipped {
				fmt.Fprintf(out, "skipped %s\n", p)
			}
			return nil
		}}
	cmd.Flags().StringVar(&root, "dir", ".", "Project directory")
	cmd.Flags().StringVar(&platform, "platform", "codex", "Platform to inspect")
	cmd.Flags().StringVar(&format, "format", "text", "Output format: text or json")
	cmd.Flags().BoolVar(&asJSON, "json", false, "Output JSON")
	return cmd
}

func buildSkillAudit(root, platform string) (skillAuditReport, error) {
	roots, err := skillAuditRoots(platform)
	if err != nil {
		return skillAuditReport{}, err
	}
	info, err := os.Stat(root)
	if err != nil {
		return skillAuditReport{}, err
	}
	if !info.IsDir() {
		return skillAuditReport{}, fmt.Errorf("not a directory: %s", root)
	}
	cfg := &config.HarnessConfig{}
	source := "defaults (no autopus.yaml)"
	if info, err := os.Lstat(filepath.Join(root, "autopus.yaml")); err == nil {
		if info.Mode()&os.ModeSymlink != 0 {
			return skillAuditReport{}, fmt.Errorf("symlink autopus.yaml is not allowed")
		}
		if !info.Mode().IsRegular() || info.Size() > skillAuditFileLimit {
			return skillAuditReport{}, fmt.Errorf("autopus.yaml must be a regular file at most %d bytes", skillAuditFileLimit)
		}
		cfg, err = config.LoadPreview(root)
		if err != nil {
			return skillAuditReport{}, err
		}
		source = "autopus.yaml"
	} else if !os.IsNotExist(err) {
		return skillAuditReport{}, err
	}
	catalog, err := content.LoadSkillCatalogFromFS(contentfs.FS, "skills")
	if err != nil {
		return skillAuditReport{}, err
	}
	registry, err := loadSkillRegistry("")
	if err != nil {
		return skillAuditReport{}, err
	}
	report := skillAuditReport{SchemaVersion: 1, TokenEstimateBasis: "heuristic ceil(UTF-8 bytes/4) per item, summed in summary; not measured provider tokens", Scope: "builtin_catalog_and_project_files_only; generated_route_selection_runtime_and_external_plugins_unobserved", SessionLoaded: "UNKNOWN", Platform: platform, ConfigSource: source, EstimateBasis: "UTF-8 bytes; canonical catalog and actual local files measured separately; not tokenizer counts", Catalog: []skillAuditEntry{}, Missing: []string{}}
	defaults := &config.HarnessConfig{Platforms: cfg.Platforms}
	full := &config.HarnessConfig{Platforms: cfg.Platforms}
	full.Skills.Compiler.Mode = config.SkillCompilerModeFull
	for _, s := range catalog.List() {
		selection := content.ExplainSkillSelection(s, platform, cfg)
		body, e := registry.Get(s.Name)
		if e != nil {
			return skillAuditReport{}, e
		}
		report.Catalog = append(report.Catalog, skillAuditEntry{Name: s.Name, Reason: selection.Reason, Target: selection.State.TargetPath, Compiled: selection.State.Compiled, Visible: selection.State.Visible, MetadataBytes: len(s.Name) + len(s.Description), BodyBytes: len(body.Level2Body), MetadataTokenEstimate: skillAuditTokenEstimate(len(s.Name) + len(s.Description)), BodyTokenEstimate: skillAuditTokenEstimate(len(body.Level2Body))})
		if content.ResolveCatalogSkillState(s, platform, defaults).Visible {
			report.DefaultVisible++
		}
		if content.ResolveCatalogSkillState(s, platform, full).Visible {
			report.FullVisible++
		}
	}
	report.Installed, report.Skipped, err = scanSkillAuditFiles(root, roots)
	if err != nil {
		return skillAuditReport{}, err
	}
	present := map[string]bool{}
	for _, f := range report.Installed {
		present[f.Path] = true
	}
	for _, e := range report.Catalog {
		if e.Compiled && !present[e.Target] {
			report.Missing = append(report.Missing, e.Target)
		}
	}
	report.Duplicates = skillAuditGroups(report.Installed, false)
	report.Aliases = skillAuditGroups(report.Installed, true)
	report.Summary = summarizeSkillAudit(report)
	return report, nil
}
