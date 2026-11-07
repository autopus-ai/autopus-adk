package cli

// SPEC-REA-001 acceptance S21 (REQ-REA-19): the init, update, and doctor help,
// docs/quality.md, docs/quality.ko.md, and the v0.50.127 CHANGELOG entry
// document every REA item.

import (
	"strconv"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/insajin/autopus-adk/pkg/config"
	"github.com/insajin/autopus-adk/pkg/reamcp"
)

// reaDocItemsKo names the REQ-REA-19 items as the Korean help states them:
// the default, both flags, the Node range, the lazy key and its old-binary
// limit, and the Windows limit. The 17-agent presets, reverse-engineer's
// session-model fallback, and the /auto reverse clean-room flow left with the
// reverse-engineer agent and its route.
var reaDocItemsKo = []string{
	"새 init은 기본으로 끄고", "--rea", "--no-rea", reamcp.NodeRange,
	"features.rea.enabled는 managed 항목이 결정을 담지 못할 때만", "v0.50.127보다 오래된 auto(v0.50.113–v0.50.125)",
	"Windows에서는 돌지 않고",
}

// reaRetiredHelpItems are claims the help must no longer make.
var reaRetiredHelpItems = []string{"reverse-engineer", "/auto reverse", "clean-room", "17번째 에이전트"}

func TestReaDocs_HelpDocumentsEveryItem(t *testing.T) {
	for _, command := range []string{"init", "update", "doctor"} {
		out, err := runReaCLI(t, "", command, "--help")
		require.NoError(t, err, command)
		help := flowText(out)
		for _, item := range reaDocItemsKo {
			assert.Contains(t, help, flowText(item), "auto %s --help is missing %q", command, item)
		}
		for _, item := range reaRetiredHelpItems {
			assert.NotContains(t, help, item, "auto %s --help still claims %q", command, item)
		}
	}
}

func TestReaDocs_ReadmesAndChangelogDocumentEveryItem(t *testing.T) {
	t.Parallel()
	agentCount := strconv.Itoa(len(config.CanonicalAgentNames()))
	docs := map[string][]string{
		// The quality references keep the REA items that still hold. The
		// 17-agent presets, reverse-engineer's session-model fallback, and the
		// /auto reverse clean-room flow and its runtime-capture consent left
		// with the reverse-engineer agent and the route.
		"docs/quality.md": {
			"registers the project-scope REA reverse-engineering MCP", "`--rea` and `--no-rea`", reamcp.NodeRange,
			"auto older than v0.50.127 cannot read features.rea", "v0.50.113–v0.50.125",
			"so they do not start on Windows", "OMP serves the " + agentCount + " Autopus agents",
			"OMP now uses the " + agentCount + " Autopus agents.",
		},
		"docs/quality.ko.md": {
			"프로젝트 범위 REA 역공학 MCP", "`--rea`·`--no-rea`", reamcp.NodeRange,
			"`features.rea는 v0.50.127보다 오래된 auto가 읽지 못합니다`", "v0.50.113–v0.50.125",
			"Windows에서는 시작되지 않습니다", "OMP는 Autopus 에이전트 " + agentCount + "개를",
			"`OMP는 이제 Autopus 에이전트 " + agentCount + "개를 씁니다.",
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
