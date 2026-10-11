package cli

import "github.com/insajin/autopus-adk/internal/cli/tui"

// composeLanguage reports the locale the composition text follows
// (SPEC-MODELUX-001 REQ-MUX-22). Tests replace it and restore it with
// t.Cleanup.
var composeLanguage = tui.DetectSystemLang

// composeText holds every operator-facing string of the composition step.
// Nothing written to autopus.yaml comes from here, so a locale changes what
// the operator reads and never what the file holds. Group labels, group
// displays, and table cells belong to the group editor and the table
// renderer, which receive the same locale.
type composeText struct {
	Title            string
	StatusCurrent    string
	StatusHistorical string
	StatusCustomized string // %d: canonical agents that differ from the current default
	Question         string
	OptionKeep       string
	OptionRefresh    string
	OptionGroups     string
	OptionAdvanced   string
	OptionTools      string
	InvalidKeep      string
	GroupsHeading    string
	GroupCurrent     string // %s: the group's display
	GroupKeep        string
	AdvancedHeading  string
	PreviewHeading   string
	ChangesHeading   string
	Confirm          string
	EditMenu         string
	Cancelled        string
	NoChange         string
	Applied          string
	Notice           string // %s: the historical preset names
}

// composeTextKO carries the SPEC wording verbatim.
var composeTextKO = composeText{
	Title:            "에이전트 모델 구성",
	StatusCurrent:    "현재 기본값",
	StatusHistorical: "이전 버전 기본값 (갱신 가능)",
	StatusCustomized: "사용자 수정 (%d개 다름)",
	Question:         "구성을 바꿀까요?",
	OptionKeep:       "[Enter] 유지",
	OptionRefresh:    "u) 최신 기본값으로 갱신",
	OptionGroups:     "g) 역할 그룹",
	OptionAdvanced:   "a) 고급",
	OptionTools:      "t) 도구별 설정",
	InvalidKeep:      "입력이 올바르지 않아 구성을 유지합니다.",
	GroupsHeading:    "역할 그룹마다 등급을 고르세요 (Enter = 그대로)",
	GroupCurrent:     "현재: %s",
	GroupKeep:        "[Enter] 그대로",
	AdvancedHeading:  "고급: 에이전트별 등급 (agent=tier, 빈 줄로 끝)",
	PreviewHeading:   "미리보기 (아직 바뀌지 않음)",
	ChangesHeading:   "바뀌는 설정:",
	Confirm:          "적용할까요? [y/N] · e) 계속 편집",
	EditMenu:         "계속 편집: g) 역할 그룹 · a) 고급 · [Enter] 미리보기로 돌아가기",
	Cancelled:        "취소했습니다. 설정을 바꾸지 않았습니다.",
	NoChange:         "변경 없음",
	Applied:          "구성을 적용했습니다.",
	Notice:           "참고: %s 프리셋이 이전 버전 기본값입니다. 대화형 auto update 또는 auto quality에서 갱신할 수 있습니다.",
}

var composeTextEN = composeText{
	Title:            "Agent model composition",
	StatusCurrent:    "current default",
	StatusHistorical: "previous-version default (can update)",
	StatusCustomized: "customized (%d differ)",
	Question:         "Change the composition?",
	OptionKeep:       "[Enter] keep",
	OptionRefresh:    "u) update to the latest defaults",
	OptionGroups:     "g) role groups",
	OptionAdvanced:   "a) advanced",
	OptionTools:      "t) per-tool settings",
	InvalidKeep:      "Invalid input; keeping the composition.",
	GroupsHeading:    "Choose a tier for each role group (Enter = keep)",
	GroupCurrent:     "current: %s",
	GroupKeep:        "[Enter] keep",
	AdvancedHeading:  "Advanced: per-agent tiers (agent=tier, empty line finishes)",
	PreviewHeading:   "Preview (nothing changed yet)",
	ChangesHeading:   "Settings that change:",
	Confirm:          "Apply? [y/N] · e) keep editing",
	EditMenu:         "Keep editing: g) role groups · a) advanced · [Enter] back to the preview",
	Cancelled:        "Cancelled; no settings changed.",
	NoChange:         "No changes; nothing written.",
	Applied:          "Composition applied.",
	Notice:           "Note: previous-version default presets: %s. Update them from an interactive auto update or auto quality.",
}

// composeTextFor returns Korean text for ko and English for every other
// locale.
func composeTextFor(lang string) composeText {
	if lang == "ko" {
		return composeTextKO
	}
	return composeTextEN
}

// currentComposeText returns the text of the operator's locale.
func currentComposeText() composeText {
	return composeTextFor(composeLanguage())
}
