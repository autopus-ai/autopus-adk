package cli

import (
	"fmt"
	"sort"
	"strings"

	"github.com/spf13/cobra"

	contentfs "github.com/insajin/autopus-adk/content"
	"github.com/insajin/autopus-adk/pkg/content"
)

// newSkillCmd는 skill 서브커맨드를 생성한다.
func newSkillCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "skill",
		Short: "Autopus 빌트인 스킬 관리",
		Long:  "autopus-adk에 포함된 빌트인 스킬 목록을 조회하고 상세 정보를 확인합니다.",
	}

	var skillsDir string

	// list 서브커맨드
	listCmd := &cobra.Command{
		Use:   "list",
		Short: "빌트인 스킬 목록 표시",
		RunE: func(cmd *cobra.Command, args []string) error {
			return runSkillList(cmd, skillsDir)
		},
	}
	listCmd.Flags().StringVar(&skillsDir, "skills-dir", "", "스킬 디렉토리 경로 (기본값: 빌트인)")
	listCmd.Flags().String("category", "", "카테고리 필터")

	// info 서브커맨드
	infoCmd := &cobra.Command{
		Use:   "info <name>",
		Short: "스킬 상세 정보 표시",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runSkillInfo(cmd, args[0], skillsDir)
		},
	}
	infoCmd.Flags().StringVar(&skillsDir, "skills-dir", "", "스킬 디렉토리 경로 (기본값: 빌트인)")

	cmd.AddCommand(listCmd)
	cmd.AddCommand(infoCmd)
	cmd.AddCommand(newSkillCreateCmd())
	cmd.AddCommand(newSkillSelectCmd())
	cmd.AddCommand(newSkillPolicyCheckCmd())
	cmd.AddCommand(newSkillAuditCmd())
	// @AX:ANCHOR [AUTO] @AX:SPEC: SPEC-SKILL-EVOLVE-001: exposes `auto skill evolve` under the existing skill namespace.
	// @AX:REASON: Removing this registration hides candidate, replay, promote, and archive commands from CLI users and smoke tests.
	cmd.AddCommand(newSkillEvolveCmd())

	return cmd
}

// runSkillList는 스킬 목록을 출력한다.
func runSkillList(cmd *cobra.Command, skillsDir string) error {
	registry, err := loadSkillRegistry(skillsDir)
	if err != nil {
		return err
	}

	// 카테고리 필터
	categoryFilter, _ := cmd.Flags().GetString("category")

	var skills []content.SkillDefinition
	if categoryFilter != "" {
		skills = registry.ListByCategory(categoryFilter)
	} else {
		skills = registry.List()
	}

	if len(skills) == 0 {
		fmt.Fprintln(cmd.OutOrStdout(), "등록된 스킬이 없습니다.")
		return nil
	}

	// 이름 순 정렬
	sort.Slice(skills, func(i, j int) bool {
		return skills[i].Name < skills[j].Name
	})

	// 헤더 출력
	fmt.Fprintf(cmd.OutOrStdout(), "%-25s %-15s %s\n", "이름", "카테고리", "설명")
	fmt.Fprintf(cmd.OutOrStdout(), "%s\n", strings.Repeat("-", 80))

	for _, s := range skills {
		desc := s.Description
		if len(desc) > 40 {
			desc = desc[:37] + "..."
		}
		fmt.Fprintf(cmd.OutOrStdout(), "%-25s %-15s %s\n", s.Name, s.Category, desc)
	}

	fmt.Fprintf(cmd.OutOrStdout(), "\n총 %d개 스킬\n", len(skills))
	return nil
}

// runSkillInfo는 스킬 상세 정보를 출력한다.
func runSkillInfo(cmd *cobra.Command, name, skillsDir string) error {
	registry, err := loadSkillRegistry(skillsDir)
	if err != nil {
		return err
	}

	skill, err := registry.Get(name)
	if err != nil {
		return fmt.Errorf("스킬 %q를 찾을 수 없습니다", name)
	}

	out := cmd.OutOrStdout()
	fmt.Fprintf(out, "# %s\n\n", skill.Name)
	fmt.Fprintf(out, "설명: %s\n", skill.Description)
	fmt.Fprintf(out, "카테고리: %s\n", skill.Category)

	if len(skill.Triggers) > 0 {
		fmt.Fprintf(out, "트리거: %s\n", strings.Join(skill.Triggers, ", "))
	}

	if skill.Level1Metadata != "" {
		fmt.Fprintf(out, "메타데이터: %s\n", skill.Level1Metadata)
	}

	if len(skill.Level3Resources) > 0 {
		fmt.Fprintf(out, "참고 자료:\n")
		for _, r := range skill.Level3Resources {
			fmt.Fprintf(out, "  - %s\n", r)
		}
	}

	if skill.Level2Body != "" {
		fmt.Fprintf(out, "\n%s\n", skill.Level2Body)
	}

	return nil
}

// loadSkillRegistry loads skill registry from embedded FS or user-specified directory.
func loadSkillRegistry(skillsDir string) (*content.SkillRegistry, error) {
	registry := &content.SkillRegistry{}

	if skillsDir != "" {
		// User-specified directory: load from disk.
		if err := registry.Load(skillsDir); err != nil {
			return nil, fmt.Errorf("스킬 로드 실패: %w", err)
		}
		return registry, nil
	}

	// Default: load from embedded filesystem.
	if err := registry.LoadFromFS(contentfs.FS, "skills"); err != nil {
		return nil, fmt.Errorf("빌트인 스킬 로드 실패: %w", err)
	}
	return registry, nil
}
