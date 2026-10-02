package content

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func readOpus5ContractFile(t *testing.T, root, relativePath string) string {
	t.Helper()

	data, err := os.ReadFile(filepath.Join(root, relativePath))
	if err != nil {
		t.Fatalf("read %s: %v", relativePath, err)
	}
	return string(data)
}

func assertOpus5ContractFragments(t *testing.T, relativePath, document string, fragments []string) {
	t.Helper()

	for _, fragment := range fragments {
		if !strings.Contains(document, fragment) {
			t.Errorf("%s missing Opus 5 contract fragment %q", relativePath, fragment)
		}
	}
}

func TestOpus5Guidance_SourceAndGeneratedContracts(t *testing.T) {
	t.Parallel()

	repoRoot := filepath.Dir(repoContentDir(t))
	matrixFragments := []string{
		"| Claude Code provider | `opus` on v2.1.280+ | v2.1.219–v2.1.279 | Before v2.1.219 |",
		"| Anthropic API | Opus 5.5 | Opus 5 | Opus 4.8 on v2.1.154–v2.1.218 |",
		"| Claude Platform on AWS | Check `/model` | Opus 5 | Opus 4.8 on v2.1.207–v2.1.218; Opus 4.7 before v2.1.207 |",
		"| Amazon Bedrock | Check `/model` | Opus 5 | Opus 4.8 on v2.1.207–v2.1.218; Opus 4.6 before v2.1.207 |",
		"| Google Cloud Agent Platform | Check `/model` | Opus 5 | Opus 4.8 on v2.1.207–v2.1.218; Opus 4.6 before v2.1.207 |",
		"| Microsoft Foundry | Opus 4.6 | Opus 4.6 | Opus 4.6 |",
	}
	englishFragments := []string{
		"`claude-opus-5-5`",
		"Its default effort is\n`medium`",
		"writes\nits effort explicitly",
		"Thinking cannot\nbe disabled at any effort level",
		"`claude-opus-5` remains a valid explicit model",
	}
	koreanFragments := []string{
		"| `opus` | `claude-opus-5-5` | `opus` | `2.1.280` | 입력 $4 / 출력 $20 |",
		"가장 높은 요구 버전인 `2.1.284`",
		"기본 effort가 Opus 5의 `high`보다 한 단계 낮은 `medium`",
		"effort를 항상 명시합니다",
		"thinking은 어떤 effort에서도 끌 수 없고",
	}
	contractFiles := map[string][]string{
		"content/skills/adaptive-quality.md":                     englishFragments,
		"content/skills/using-autopus.md":                        koreanFragments,
		"templates/codex/skills/adaptive-quality.md.tmpl":        englishFragments,
		"templates/codex/skills/using-autopus.md.tmpl":           koreanFragments,
		"templates/gemini/skills/adaptive-quality/SKILL.md.tmpl": englishFragments,
		"templates/gemini/skills/using-autopus/SKILL.md.tmpl":    koreanFragments,
	}

	for relativePath, migrationFragments := range contractFiles {
		relativePath := relativePath
		migrationFragments := migrationFragments
		t.Run(relativePath, func(t *testing.T) {
			t.Parallel()

			document := readOpus5ContractFile(t, repoRoot, relativePath)
			assertOpus5ContractFragments(t, relativePath, document, matrixFragments)
			assertOpus5ContractFragments(t, relativePath, document, migrationFragments)
		})
	}
}

func TestWorkflowDoctorGuidance_UsesRouteAwarePins(t *testing.T) {
	t.Parallel()

	repoRoot := filepath.Dir(repoContentDir(t))
	files := map[string][]string{
		"content/skills/harness-workflow.md": {
			"`auto workflow doctor --route route_a`",
			"`RouteAMinVersion=2.1.246`",
			"`auto workflow doctor --route route_team`",
			"`RouteTeamMinVersion=2.1.284`",
		},
		"content/skills/using-autopus.md": {
			"`auto workflow doctor --route route_team`",
			"`route_a`",
			"`2.1.246`",
		},
		"templates/claude/commands/auto-workflows.md.tmpl": {
			"`auto workflow doctor --route route_a`",
			"`auto workflow doctor --route route_team`",
		},
	}

	for relativePath, fragments := range files {
		document := readOpus5ContractFile(t, repoRoot, relativePath)
		assertOpus5ContractFragments(t, relativePath, document, fragments)
	}
}
