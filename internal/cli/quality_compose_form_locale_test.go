package cli

// SPEC-MODELUX-002 T9 (REQ-MUX2-20; acceptance S17): every form title,
// option, and status notation follows the locale, while the values written
// stay the same in every locale.

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestComposeFormLocale_TextFollowsTheLocaleAndValuesDoNot(t *testing.T) {
	cases := []struct {
		lang string
		want []string
	}{
		{"en", []string{
			"Change the composition? \n1. keep\n2. edit the composition\n3. choose the default preset\n",
			"Which tool? \n1. same for all tools\n2. Claude Code\n3. Codex\n",
			"How? \n1. role groups\n2. per agent\n",
			"3. 6-sol · xhigh (substitute)  [opus]\n",
			"Apply? \n1. cancel\n2. apply\n3. keep editing\n",
		}},
		{"ko", []string{
			"구성을 바꿀까요? \n1. 유지\n2. 구성 편집\n3. 기본 프리셋 고르기\n",
			"어느 도구를 바꿀까요? \n1. 모든 도구 같게\n2. Claude Code\n3. Codex\n",
			"어떻게 고칠까요? \n1. 역할 그룹\n2. 에이전트별\n",
			"3. 6-sol · xhigh (대체)  [opus]\n",
			"적용할까요? \n1. 취소\n2. 적용\n3. 계속 편집\n",
		}},
	}
	for _, tc := range cases {
		t.Run(tc.lang, func(t *testing.T) {
			newComposeUpdateRun(t, true, "")
			composeSessionCatalog(t, "CAT-B")
			setComposeLanguage(t, tc.lang)
			useAccessibleComposeForms(t)
			dir := writeComposeFixture(t, "claude-code", "codex")

			out, err := runQualityForms(t, dir, nil, formInput(composeFormLines(composeFormS5, "2\n", "\n")...))

			require.NoError(t, err, out)
			for _, want := range tc.want {
				assert.Contains(t, out, want)
			}
			preset := composeBindLoad(t, dir).Quality.Presets["codex-custom"]
			assert.Equal(t, "Codex custom agent tiers", preset.Description)
			assert.Equal(t, "opus", preset.Agents["planner"])
		})
	}
}
