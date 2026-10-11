package cli

// composeTableLabels is every word the table prints. Values written to
// autopus.yaml never come from here, so only the screen depends on locale.
type composeTableLabels struct {
	group, agent, tier                                 string
	override, ompRequested                             string
	tierDefault, tierInvalid                           string
	platformDefault                                    string
	substituted, noCatalog, unverified, refused        string
	codexDefault, routeError                           string
	catalogRead, catalogUnread, supervisor, codexFails string
	noAgentModels, changedLegend                       string
}

var composeTableLabelsKo = composeTableLabels{
	group: "그룹", agent: "에이전트", tier: "등급",
	override: "개별 설정: %s", ompRequested: "요청 route · catalog 확인 전",
	tierDefault: "기본값(%s)", tierInvalid: "잘못된 값→%s",
	platformDefault: "플랫폼 기본값",
	substituted:     "(대체)", noCatalog: "(catalog 없음)", unverified: "(미확인)", refused: "생성 거부",
	codexDefault: "Codex 기본값", routeError: "route 오류",
	catalogRead: "Codex catalog: 읽음 · 대체 %d칸", catalogUnread: "Codex catalog: 읽지 못함",
	supervisor: "Codex 기본 세션: %s", codexFails: "Codex 생성이 실패합니다",
	noAgentModels: "설치된 플랫폼은 에이전트별 모델을 쓰지 않습니다", changedLegend: "* = 바뀌는 칸",
}

var composeTableLabelsEn = composeTableLabels{
	group: "Group", agent: "Agent", tier: "Tier",
	override: "override: %s", ompRequested: "requested route · before catalog check",
	tierDefault: "default(%s)", tierInvalid: "invalid→%s",
	platformDefault: "platform default",
	substituted:     "(substitute)", noCatalog: "(no catalog)", unverified: "(unverified)", refused: "generation refused",
	codexDefault: "Codex default", routeError: "route error",
	catalogRead: "Codex catalog: read · %d substituted", catalogUnread: "Codex catalog: unreadable",
	supervisor: "Codex default session: %s", codexFails: "Codex generation will fail",
	noAgentModels: "No installed platform uses per-agent models", changedLegend: "* = changed cell",
}

// composeTableLabelsFor answers Korean for "ko" and English otherwise.
func composeTableLabelsFor(lang string) composeTableLabels {
	if lang == "ko" {
		return composeTableLabelsKo
	}
	return composeTableLabelsEn
}
