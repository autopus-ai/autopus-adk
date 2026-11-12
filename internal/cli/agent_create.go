package cli

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"text/template"

	"gopkg.in/yaml.v3"

	"github.com/spf13/cobra"
)

// @AX:NOTE [AUTO] @AX:EXCLUDE @AX:REASON: agent template format must match .claude/agents/ conventions; TODO strings inside agentTemplate are user-facing placeholders, not code defects
const agentTemplate = `---
name: {{.Name}}
description: {{.Description}}
effort: {{.Effort}}
tools:
{{- range .Tools}}
  - {{.}}
{{- end}}
---

# {{.Name}} Agent

{{.Description}}

## 역할

TODO: 이 에이전트의 역할과 책임을 정의하세요.

## 작업 지침

TODO: 작업 처리 지침을 작성하세요.

## 완료 기준

- [ ] TODO: 완료 기준을 정의하세요.
`

type agentTemplateData struct {
	Name        string
	Description string
	// Effort is the default effort tier for this agent role (e.g. "medium", "max").
	Effort string
	Tools  []string
}

// agentFrontmatter mirrors the required fields of an agent .md frontmatter block.
type agentFrontmatter struct {
	Name        string `yaml:"name"`
	Description string `yaml:"description"`
	Effort      string `yaml:"effort,omitempty"`
}

// newAgentCmd creates the `auto agent` parent command with subcommands.
func newAgentCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "agent",
		Short: "Autopus 에이전트 관리",
		Long:  "에이전트 파일 생성 및 관리 도구입니다.",
	}

	cmd.AddCommand(newAgentCreateSubCmd())
	cmd.AddCommand(newAgentRunSubCmd())
	return cmd
}

// newAgentCreateSubCmd creates the `auto agent create <name>` subcommand.
func newAgentCreateSubCmd() *cobra.Command {
	var (
		description string
		tools       string
		write       bool
	)

	cmd := &cobra.Command{
		Use:   "create <name>",
		Short: "새 에이전트 파일 생성",
		Long:  "새 에이전트 .md 파일을 생성합니다. 기본값은 dry-run(stdout 출력)이며, --write 플래그로 파일을 저장합니다.",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runAgentCreate(cmd, args[0], description, tools, write)
		},
	}

	cmd.Flags().StringVar(&description, "description", "", "에이전트 설명 (필수)")
	cmd.Flags().StringVar(&tools, "tools", "Read,Write,Bash", "사용 도구 목록 (쉼표 구분)")
	cmd.Flags().BoolVar(&write, "write", false, "파일로 저장 (.claude/agents/autopus/)")
	_ = cmd.MarkFlagRequired("description")

	return cmd
}

func runAgentCreate(cmd *cobra.Command, name, description, tools string, write bool) error {
	if strings.ContainsAny(name, "/\\") || filepath.Base(name) != name {
		return fmt.Errorf("invalid agent name %q: must not contain path separators", name)
	}

	toolList := parseTools(tools)

	// Validate: check name uniqueness among existing agents
	if write {
		agentsDir := ".claude/agents/autopus"
		existing := filepath.Join(agentsDir, name+".md")
		if _, err := os.Stat(existing); err == nil {
			return fmt.Errorf("agent %q already exists at %s", name, existing)
		}
	}

	data := agentTemplateData{
		Name:        name,
		Description: description,
		Effort:      defaultEffortForRole(name),
		Tools:       toolList,
	}

	tmpl, err := template.New("agent").Parse(agentTemplate)
	if err != nil {
		return fmt.Errorf("template parse error: %w", err)
	}

	// Render to buffer first so we can validate the frontmatter before writing.
	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, data); err != nil {
		return fmt.Errorf("template render error: %w", err)
	}

	if err := validateAgentFrontmatter(buf.Bytes()); err != nil {
		return err
	}

	if !write {
		_, err := cmd.OutOrStdout().Write(buf.Bytes())
		return err
	}

	agentsDir := ".claude/agents/autopus"
	if err := os.MkdirAll(agentsDir, 0o755); err != nil {
		return fmt.Errorf("failed to create agents directory: %w", err)
	}

	outPath := filepath.Join(agentsDir, name+".md")
	if err := os.WriteFile(outPath, buf.Bytes(), 0o644); err != nil {
		return fmt.Errorf("failed to write agent file: %w", err)
	}

	fmt.Fprintf(cmd.OutOrStdout(), "Agent created: %s\n", outPath)
	return nil
}

// validateAgentFrontmatter extracts and parses the YAML frontmatter block,
// verifying that name and description fields are present.
func validateAgentFrontmatter(content []byte) error {
	fm, err := extractFrontmatter(content)
	if err != nil {
		return fmt.Errorf("agent frontmatter invalid YAML: %w", err)
	}
	var a agentFrontmatter
	if err := yaml.Unmarshal(fm, &a); err != nil {
		return fmt.Errorf("agent frontmatter parse error: %w", err)
	}
	if a.Name == "" {
		return fmt.Errorf("agent frontmatter missing required field: name")
	}
	if a.Description == "" {
		return fmt.Errorf("agent frontmatter missing required field: description")
	}
	return nil
}

func parseTools(tools string) []string {
	if tools == "" {
		return []string{"Read", "Write", "Bash"}
	}
	parts := strings.Split(tools, ",")
	result := make([]string, 0, len(parts))
	for _, p := range parts {
		if t := strings.TrimSpace(p); t != "" {
			result = append(result, t)
		}
	}
	return result
}

// maxEffortRoles lists Opus-class roles that require max reasoning effort by default.
// Roles not in a special set default to "medium" (safe fallback).
var maxEffortRoles = map[string]bool{
	"spec-writer":      true,
	"planner":          true,
	"security-auditor": true,
	"architect":        true,
	"deep-worker":      true,
}

// highEffortRoles lists Sonnet-class roles that still need higher reasoning effort.
var highEffortRoles = map[string]bool{
	"reviewer": true,
}

// defaultEffortForRole returns the default effort tier string for a given agent role name.
// Returns "max" for Opus-class roles, "high" for review roles, and "medium" for all others.
func defaultEffortForRole(role string) string {
	if maxEffortRoles[role] {
		return "max"
	}
	if highEffortRoles[role] {
		return "high"
	}
	return "medium"
}
