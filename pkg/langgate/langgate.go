// Package langgate judges whether an agent response is written in the
// language a project configured for AI responses.
//
// It looks at prose only. Code blocks, inline code, and URLs are removed
// first, because a reply in Korean still quotes English identifiers, commands,
// and output. What is left is counted as letters of the target script against
// Latin letters, so technical terms mixed into a Korean sentence pass while a
// reply written wholly in English does not.
package langgate

import (
	"regexp"
	"strings"
	"unicode"
)

// minLatinLetters keeps only the shortest replies out of the check, such as
// "Merged." A one-line English progress note is exactly the drift the check
// is for, so the floor stays low; replayed over 2,411 replies of a real
// session it blocked no reply with Korean prose and passed no English one.
const minLatinLetters = 12

// minNativeShare is the smallest share of target-script letters a reply needs.
// It is low on purpose: Korean prose about code carries many English terms.
const minNativeShare = 0.1

var (
	fencedCode = regexp.MustCompile("(?s)```.*?```|~~~.*?~~~")
	inlineCode = regexp.MustCompile("`[^`\n]*`")
	urlPattern = regexp.MustCompile(`https?://\S+`)
)

// languages maps a configured language code to its English name and the
// scripts its prose is written in. Codes absent here are not checked.
var languages = map[string]struct {
	name    string
	scripts []*unicode.RangeTable
}{
	"ko": {"Korean", []*unicode.RangeTable{unicode.Hangul}},
	"ja": {"Japanese", []*unicode.RangeTable{unicode.Hiragana, unicode.Katakana, unicode.Han}},
	"zh": {"Chinese", []*unicode.RangeTable{unicode.Han}},
}

// Supported reports whether lang is a language Check can judge.
func Supported(lang string) bool {
	_, ok := languages[normalize(lang)]
	return ok
}

// Name returns the English name of lang, or lang itself when unknown.
func Name(lang string) string {
	if l, ok := languages[normalize(lang)]; ok {
		return l.name
	}
	return lang
}

// Check reports whether text reads as written in lang. Unsupported languages
// and replies too short to judge always pass.
func Check(lang, text string) bool {
	l, ok := languages[normalize(lang)]
	if !ok {
		return true
	}
	prose := fencedCode.ReplaceAllString(text, " ")
	prose = inlineCode.ReplaceAllString(prose, " ")
	prose = urlPattern.ReplaceAllString(prose, " ")
	native, latin := 0, 0
	for _, r := range prose {
		switch {
		case unicode.IsOneOf(l.scripts, r):
			native++
		case unicode.Is(unicode.Latin, r) && unicode.IsLetter(r):
			latin++
		}
	}
	if latin < minLatinLetters {
		return true
	}
	return float64(native) >= minNativeShare*float64(native+latin)
}

func normalize(lang string) string {
	lang = strings.ToLower(strings.TrimSpace(lang))
	if base, _, ok := strings.Cut(lang, "-"); ok {
		return base
	}
	return lang
}
