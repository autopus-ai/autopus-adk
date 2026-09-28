// Package lore는 git commit 트레일러 기반의 의사결정 지식 관리를 제공한다.
package lore

import "time"

// LoreEntry는 하나의 Lore 의사결정 항목이다.
type LoreEntry struct {
	// 9개 트레일러 필드
	Constraint    string // 제약 사항
	Rejected      string // 거부된 대안
	Confidence    string // 신뢰도: low, medium, high
	ScopeRisk     string // 범위 리스크: local, module, system
	Reversibility string // 되돌릴 수 있는 정도: trivial, moderate, difficult
	Directive     string // 지시사항
	Tested        string // 테스트된 항목
	NotTested     string // 테스트되지 않은 항목
	Related       string // 관련 항목

	// 메타데이터 (git log에서 추출)
	CommitHash string    // 커밋 해시
	CommitDate time.Time // 커밋 날짜
	CommitMsg  string    // 커밋 메시지 (트레일러 제외)
	FilePath   string    // 관련 파일 경로
}

// LoreConfig는 Lore 설정이다.
type LoreConfig struct {
	RequiredTrailers   []string // 필수 트레일러 목록
	StaleThresholdDays int      // 오래된 항목 기준 (일)
	ForbiddenTrailers  []string // Trailer keys a commit message must not carry
}

// ValidationError는 검증 오류이다.
type ValidationError struct {
	Field   string // 오류 필드
	Message string // 오류 메시지
}

func (e ValidationError) Error() string {
	return e.Message
}
