package rulecond

import (
	"regexp"
	"strings"
)

// StickyLanguage carries the resolved language.* values of autopus.yaml. The
// language-policy rule body only says "follow the configured language"; the
// values live in CLAUDE.md, which a long session reads once. Re-attaching the
// values with the rule is what keeps the response language from drifting to
// the language of the latest English tool or subagent output.
type StickyLanguage struct {
	Comments    string
	Commits     string
	AIResponses string
}

// languagePolicyRule is the sticky rule the configured values attach to.
const languagePolicyRule = "language-policy"

// languageCodePattern admits a short language tag (ko, en, pt-BR, zh_Hant).
// A config value is untrusted text on its way into model context, so anything
// else is dropped rather than injected.
var languageCodePattern = regexp.MustCompile(`^[A-Za-z]{2,3}([-_][A-Za-z0-9]{2,8})?$`)

var languageNames = map[string]string{
	"en": "English", "ko": "Korean", "ja": "Japanese", "zh": "Chinese",
}

// attachLanguageValues appends the configured values to the language-policy
// entry. Nothing is attached when that rule is not being injected or when no
// value is usable. The addition is bounded by the value pattern, and the
// aggregate cap still applies at assembly.
func attachLanguageValues(entries []injected, lang StickyLanguage) []injected {
	block := renderLanguageValues(lang)
	if block == "" {
		return entries
	}
	out := make([]injected, len(entries))
	copy(out, entries)
	for i := range out {
		if out[i].name == languagePolicyRule {
			out[i].body = strings.TrimRight(out[i].body, "\n") + "\n\n" + block
		}
	}
	return out
}

func renderLanguageValues(lang StickyLanguage) string {
	rows := []struct{ surface, value string }{
		{"code_comments", lang.Comments},
		{"commit_messages", lang.Commits},
		{"ai_responses", lang.AIResponses},
	}
	var b strings.Builder
	for _, row := range rows {
		if !languageCodePattern.MatchString(row.value) {
			continue
		}
		if b.Len() == 0 {
			b.WriteString("Configured values for this project (autopus.yaml):\n")
		}
		b.WriteString("- " + row.surface + ": " + describeLanguage(row.value) + "\n")
	}
	if b.Len() == 0 {
		return ""
	}
	if code := lang.AIResponses; languageCodePattern.MatchString(code) && !strings.EqualFold(code, "en") {
		b.WriteString("Write every message to the user in " + describeLanguage(code) +
			", including short progress notes and replies that follow English tool, test, or subagent output.\n")
	}
	return b.String()
}

func describeLanguage(code string) string {
	if name, ok := languageNames[strings.ToLower(code)]; ok {
		return code + " (" + name + ")"
	}
	return code
}
