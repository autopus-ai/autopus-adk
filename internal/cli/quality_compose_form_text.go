package cli

// composeFormText holds every operator-facing string the form flow adds to
// the composition step (SPEC-MODELUX-002 Form Text). Like composeText, a
// locale changes only what the operator reads: nothing written to
// autopus.yaml comes from here. Tool names, model IDs, efforts, and stored
// tiers stay the same in every locale.
type composeFormText struct {
	Menu, MenuKeep, MenuRefresh, MenuEdit, MenuPreset string
	Tab, TabAll                                       string
	Method, MethodGroups, MethodAgents                string
	List, ListDone, Keep                              string
	Confirm, ConfirmCancel, ConfirmApply, ConfirmEdit string
	Preset, PresetCurrent, PresetNone                 string
	OMP, OMPBack, OMPPin, OMPCurrent                  string // OMPCurrent: %s, the current policy
	OMPFamily, OMPProfileDefault, OMPThinking         string // OMPThinking: %s, the agent
	OMPRewrite, OMPDetectedOnly                       string
	OMPReloadFailed                                   string // %s: the cause
	OMPSessionKeep, OMPSessionModel, OMPFollowPlanner string // SPEC-MODELUX-003 OMP Autopus Tab
	OMPAttestedOnly, OMPOldBinary                     string
	OMPConvert                                        string // %s: the profile
	InitOMPUnavailable, InitOMPApplied                string
	Interrupted                                       string
	EffortAuto, EffortUnread                          string // SPEC-MODELUX-003 effort select
}

var composeFormTextKO = composeFormText{
	Menu: "구성을 바꿀까요?", MenuKeep: "유지", MenuRefresh: "최신 기본값으로 갱신",
	MenuEdit: "구성 편집", MenuPreset: "기본 프리셋 고르기",
	Tab: "어느 도구를 바꿀까요?", TabAll: "모든 도구 같게",
	Method: "어떻게 고칠까요?", MethodGroups: "역할 그룹", MethodAgents: "에이전트별",
	List: "에이전트를 고르세요", ListDone: "완료", Keep: "그대로",
	Confirm: "적용할까요?", ConfirmCancel: "취소", ConfirmApply: "적용", ConfirmEdit: "계속 편집",
	Preset: "기본 프리셋을 고르세요", PresetCurrent: "(현재)", PresetNone: "없음",
	OMP: "OMP 구성", OMPBack: "돌아가기", OMPPin: "에이전트별 지정 (balanced 위에)", OMPCurrent: "현재: %s",
	OMPFamily: "모델 계열", OMPProfileDefault: "프로필 기본값", OMPThinking: "%s thinking",
	OMPRewrite:      "OMP 설정은 autopus.yaml 전체를 다시 씁니다: 주석과 서식이 바뀔 수 있습니다(값은 그대로)",
	OMPDetectedOnly: "OMP는 이번 update에서 새로 설치됩니다. 모델은 다음 auto update나 auto quality에서 지정할 수 있습니다.",
	OMPReloadFailed: "OMP 설정은 적용됐지만 다시 읽지 못했습니다: %s",
	OMPSessionKeep:  "OMP 세션 모델 그대로", OMPSessionModel: "OMP 세션 모델", OMPFollowPlanner: "planner를 따름",
	OMPAttestedOnly:    "설치된 OMP catalog에 family·capability 정보가 없어 Autopus가 증명할 수 있는 model만 보입니다",
	OMPOldBinary:       "프로필 없는 role_model_policy.agents는 v0.50.126보다 오래된 auto가 읽지 못합니다",
	OMPConvert:         "OMP 프로필 %s의 route를 에이전트별 지정으로 옮깁니다. 바꾸지 않은 에이전트의 route는 그대로입니다.",
	InitOMPUnavailable: "OMP catalog를 읽지 못해 지금은 OMP 모델을 지정할 수 없습니다. init 뒤 auto quality에서 지정할 수 있습니다.",
	InitOMPApplied:     "OMP 모델 선택을 init 설정에 넣었습니다. 파일 생성 때 함께 씁니다.",
	Interrupted:        "중단했습니다. 구성을 더 바꾸지 않습니다.",
	EffortAuto:         "(자동)",
	EffortUnread:       "Codex catalog를 읽지 못해 effort는 자동입니다",
}

var composeFormTextEN = composeFormText{
	Menu: "Change the composition?", MenuKeep: "keep", MenuRefresh: "update to the latest defaults",
	MenuEdit: "edit the composition", MenuPreset: "choose the default preset",
	Tab: "Which tool?", TabAll: "same for all tools",
	Method: "How?", MethodGroups: "role groups", MethodAgents: "per agent",
	List: "Pick an agent", ListDone: "done", Keep: "keep",
	Confirm: "Apply?", ConfirmCancel: "cancel", ConfirmApply: "apply", ConfirmEdit: "keep editing",
	Preset: "Choose the default preset", PresetCurrent: "(current)", PresetNone: "none",
	OMP: "OMP models", OMPBack: "back", OMPPin: "pin per agent (on balanced)", OMPCurrent: "current: %s",
	OMPFamily: "Model family", OMPProfileDefault: "profile default", OMPThinking: "%s thinking",
	OMPRewrite:      "OMP settings rewrite all of autopus.yaml: comments and layout may change (values do not)",
	OMPDetectedOnly: "OMP is installed by this update; set its models with the next auto update or auto quality.",
	OMPReloadFailed: "OMP settings were applied but could not be re-read: %s",
	OMPSessionKeep:  "keep the OMP session model", OMPSessionModel: "OMP session model", OMPFollowPlanner: "follow planner",
	OMPAttestedOnly:    "The installed OMP catalog has no family or capability metadata; only models Autopus can attest are listed",
	OMPOldBinary:       "auto older than v0.50.126 cannot read role_model_policy.agents without a profile",
	OMPConvert:         "Moving the routes of OMP profile %s to per-agent pins; unchanged agents keep their routes.",
	InitOMPUnavailable: "Could not read the OMP catalog, so OMP models cannot be set now; set them after init with auto quality.",
	InitOMPApplied:     "OMP model choice added to the init config; it is written with the platform files.",
	Interrupted:        "Interrupted; the composition is not changed further.",
	EffortAuto:         "(automatic)",
	EffortUnread:       "Codex catalog unreadable; effort stays automatic",
}

// composeFormTextFor returns Korean for ko and English for every other
// locale, as composeTextFor does.
func composeFormTextFor(lang string) composeFormText {
	if lang == "ko" {
		return composeFormTextKO
	}
	return composeFormTextEN
}
