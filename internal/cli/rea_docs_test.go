package cli

// SPEC-REA-001 acceptance S21 (REQ-REA-19): the init, update, and doctor help,
// both READMEs, and the v0.50.127 CHANGELOG entry document every REA item.

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/insajin/autopus-adk/pkg/reamcp"
)

// reaDocItemsKo names the REQ-REA-19 items as the Korean help states them:
// the default, both flags, the Node range, the lazy key and its old-binary
// limit, the 17-agent presets, the session-model fallback, the Windows
// limit, the clean-room flow, and the runtime-capture consent.
var reaDocItemsKo = []string{
	"새 init은 기본으로 켜고", "--rea", "--no-rea", reamcp.NodeRange,
	"features.rea.enabled는 managed 항목이 결정을 담지 못할 때만", "v0.50.127보다 오래된 auto(v0.50.113–v0.50.125)",
	"이 버전의 17개 프리셋은 v0.50.125가 사용자 수정으로", "reverse-engineer만 세션 모델을 씁니다",
	"Windows에서는 돌지 않고", "clean-room", "runtime capture 전에 메인 세션 동의를",
}

func TestReaDocs_HelpDocumentsEveryItem(t *testing.T) {
	for _, command := range []string{"init", "update", "doctor"} {
		out, err := runReaCLI(t, "", command, "--help")
		require.NoError(t, err, command)
		help := flowText(out)
		for _, item := range reaDocItemsKo {
			assert.Contains(t, help, flowText(item), "auto %s --help is missing %q", command, item)
		}
	}
}

func TestReaDocs_ReadmesAndChangelogDocumentEveryItem(t *testing.T) {
	t.Parallel()
	docs := map[string][]string{
		"README.md": {
			"registers the project-scope REA reverse-engineering MCP", "`--rea` and `--no-rea`", reamcp.NodeRange,
			"auto older than v0.50.127 cannot read features.rea", "v0.50.113–v0.50.125",
			"v0.50.125 reads the 17-agent presets this release writes as customized",
			"only reverse-engineer falls back to the session model", "so they do not start on Windows",
			"**Clean-room flow**", "asks the main-session consent before any runtime capture",
			"OMP serves the 17 Autopus agents", "OMP now uses the 17 Autopus agents.",
		},
		"docs/README.ko.md": {
			"프로젝트 범위 REA 역공학 MCP", "`--rea`·`--no-rea`", reamcp.NodeRange,
			"`features.rea는 v0.50.127보다 오래된 auto가 읽지 못합니다`", "v0.50.113–v0.50.125",
			"이 버전이 쓴 17개 프리셋은 v0.50.125가 사용자 수정으로", "reverse-engineer만 세션 모델을 쓰고",
			"Windows에서는 시작되지 않습니다", "**clean-room 흐름**", "runtime capture 전에 메인 세션이 동의를",
			"Autopus 에이전트 17개를 씁니다(SPEC-MODELUX-003, SPEC-REA-001)", "`OMP는 이제 Autopus 에이전트 17개를 씁니다.",
		},
		"CHANGELOG.md": {
			"v0.50.127: REA 역공학 MCP", "기본 on", "`--rea`·`--no-rea`", reamcp.NodeRange,
			"lazy `features.rea`", "v0.50.113–v0.50.125", "이 버전이 쓴 17개 프리셋은 v0.50.125가 사용자 수정으로",
			"reverse-engineer만 route나 model 줄 없이 세션 모델을", "POSIX `/bin/bash -l -c", "clean-room 흐름",
			"메인 세션의 동의 질문", "headless managed OMP RPC backend(`/autopus-pipeline`)는 이 route를",
		},
	}
	for path, items := range docs {
		text := flowText(readRepoFile(t, strings.Split(path, "/")...))
		for _, item := range items {
			assert.Contains(t, text, flowText(item), "%s is missing %q", path, item)
		}
	}
}
