package lore

import (
	"fmt"
	"strings"
)

var (
	validConfidence    = map[string]bool{"low": true, "medium": true, "high": true}
	validScopeRisk     = map[string]bool{"local": true, "module": true, "system": true}
	validReversibility = map[string]bool{"trivial": true, "moderate": true, "difficult": true}
)

// Validate는 커밋 메시지의 Lore 트레일러를 검증한다.
func Validate(commitMsg string, config LoreConfig) []ValidationError {
	entry, err := ParseTrailers(commitMsg)
	if err != nil {
		return []ValidationError{{Field: "parse", Message: fmt.Sprintf("파싱 실패: %v", err)}}
	}

	var errs []ValidationError

	// 필수 트레일러 검사
	for _, required := range config.RequiredTrailers {
		if !hasField(*entry, required) {
			errs = append(errs, ValidationError{
				Field:   required,
				Message: fmt.Sprintf("필수 트레일러 '%s'가 없습니다", required),
			})
		}
	}

	// 값 형식 검사
	if entry.Confidence != "" && !validConfidence[strings.ToLower(entry.Confidence)] {
		errs = append(errs, ValidationError{
			Field:   "Confidence",
			Message: fmt.Sprintf("유효하지 않은 Confidence 값: %q (low, medium, high 중 하나여야 합니다)", entry.Confidence),
		})
	}

	if entry.ScopeRisk != "" && !validScopeRisk[strings.ToLower(entry.ScopeRisk)] {
		errs = append(errs, ValidationError{
			Field:   "Scope-risk",
			Message: fmt.Sprintf("유효하지 않은 Scope-risk 값: %q (local, module, system 중 하나여야 합니다)", entry.ScopeRisk),
		})
	}

	if entry.Reversibility != "" && !validReversibility[strings.ToLower(entry.Reversibility)] {
		errs = append(errs, ValidationError{
			Field:   "Reversibility",
			Message: fmt.Sprintf("유효하지 않은 Reversibility 값: %q (trivial, moderate, difficult 중 하나여야 합니다)", entry.Reversibility),
		})
	}

	errs = append(errs, forbiddenTrailerErrors(commitMsg, config.ForbiddenTrailers)...)

	return errs
}

// forbiddenTrailerErrors reports each forbidden trailer the message carries.
// The subject line is skipped because it is never a trailer, and keys compare
// case-insensitively because git treats "Co-authored-by" and
// "Co-Authored-By" as the same trailer.
func forbiddenTrailerErrors(commitMsg string, forbidden []string) []ValidationError {
	var errs []ValidationError
	lines := strings.Split(commitMsg, "\n")
	for _, key := range forbidden {
		key = strings.TrimSpace(key)
		if key == "" {
			continue
		}
		for _, line := range lines[1:] {
			name, _, ok := strings.Cut(strings.TrimSpace(line), ":")
			if ok && strings.EqualFold(strings.TrimSpace(name), key) {
				errs = append(errs, ValidationError{
					Field:   key,
					Message: fmt.Sprintf("금지된 트레일러 '%s'가 있습니다 (lore.forbidden_trailers)", key),
				})
				break
			}
		}
	}
	return errs
}
