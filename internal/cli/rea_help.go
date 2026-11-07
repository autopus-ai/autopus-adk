package cli

import "github.com/insajin/autopus-adk/pkg/reamcp"

// reaHelpSummary is the REA paragraph the init, update, and doctor help share,
// so each documents every REQ-REA-19 item (SPEC-REA-001).
const reaHelpSummary = "REA 역공학 MCP(SPEC-REA-001): 새 init은 기본으로 끄고(v0.51.0부터 opt-in) --rea·--no-rea로 정할 수 있으며, Node\n" +
	reamcp.NodeRange + "일 때만 Claude Code·Codex·Antigravity에 프로젝트 범위\n" +
	reaPackage + " MCP를 등록합니다. features.rea.enabled는 managed 항목이 결정을 담지 못할 때만 쓰고,\n" +
	"이 key가 든 파일은 v0.50.127보다 오래된 auto(v0.50.113–v0.50.125)가 읽지 못합니다. Claude Code·Antigravity\n" +
	"서버는 POSIX /bin/bash로 시작해 Windows에서는 돌지 않고, OMP·OpenCode에는 등록하지 않습니다.\n"

// reaInitHelp is the REA part of `auto init --help`.
const reaInitHelp = reaHelpSummary +
	"init은 claude-code·codex·antigravity-cli가 있으면 대화형 wizard에서 한 번 묻고(기본 No) 비대화형 실행은 --rea가 있을 때만 켭니다.\n" +
	"--rea와 --no-rea를 함께 쓰면 아무것도 쓰기 전에 끝납니다."

// reaUpdateHelp is the REA part of `auto update --help`.
const reaUpdateHelp = reaHelpSummary +
	"update는 결정이 없으면 대화형 실행(TTY, --yes·--auto 없음, truthy CI 없음)에서만 프로젝트 설정 질문 뒤 한 번\n" +
	"묻고(기본 No), 그 밖의 실행은 stdin을 읽지 않고 아무것도 더하지 않으며 auto update --rea를 알리는 hint 한\n" +
	"줄만 씁니다. --rea·--no-rea는 묻지 않고 적용합니다. 둘을 함께 쓰거나 --workspace와 섞으면 쓰기 전에 끝나므로\n" +
	"repo마다 --local로 실행하세요. Node가 맞지 않으면 항목을 더하지 않고 기존 항목과 결정은 그대로 둡니다.\n" +
	"--no-rea는 선택되지 않은 대상 파일의 managed 항목도 지우고 사용자 소유·다른 pin 항목은 건드리지 않습니다."

// reaDoctorHelp is the REA part of `auto doctor --help`.
const reaDoctorHelp = reaHelpSummary +
	"doctor는 결정과 출처(features.rea·managed 항목·none), Node probe 한 번의 판정, 선택된 플랫폼마다 rea 항목\n" +
	"분류(managed·absent·other_pin·user_owned), OMP·OpenCode 안내를 보고하고(--json의 data.rea와 doctor.rea.* check)\n" +
	"REA 서버를 띄우거나 네트워크를 쓰지 않습니다."
