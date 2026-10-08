package healthband

import (
	"path"
	"regexp"
	"strings"
	"unicode"
)

// Patch Policy item 7 heuristics of the Phase 4 security review (L1, L2).
// Both are heuristics that refuse more than they must, never a proof that
// an accepted line is safe: a reviewer still reads the whole patch file.

// confusableScripts are the scripts whose letters look alike (L1).
var confusableScripts = []*unicode.RangeTable{unicode.Latin, unicode.Cyrillic, unicode.Greek}

// mixedScriptToken reports a token of text, a run of letters, marks,
// digits, and underscores, that holds letters of two or more of
// confusableScripts, such as аdmin with a Cyrillic а: it reads as another
// identifier. A token of one script, Cyrillic or Greek alone included, and
// a token of other scripts pass.
func mixedScriptToken(text string) bool {
	scripts := 0 // one bit per confusableScripts entry
	for _, r := range text + " " {
		if unicode.IsLetter(r) || unicode.IsDigit(r) || unicode.Is(unicode.M, r) || r == '_' {
			for i, table := range confusableScripts {
				if unicode.Is(table, r) {
					scripts |= 1 << i
				}
			}
			continue
		}
		if scripts&(scripts-1) != 0 {
			return true
		}
		scripts = 0
	}
	return false
}

// commentLine reports an added line of file that is a whole-line comment of
// the file's language, which the mixed-script check skips: after blanks it
// starts with # in Python and Ruby, with // or /* in the C-family languages
// of the allowlist, and with any of the three in PHP, but not with PHP's #[
// attribute, and a /* comment that closes on the line has nothing after it.
// A trailing comment and a block-comment continuation line are checked as
// code, so the skip never covers code.
func commentLine(file, line string) bool {
	trimmed := strings.TrimLeft(line, " \t")
	hash := strings.HasPrefix(trimmed, "#") && !strings.HasPrefix(trimmed, "#[")
	switch path.Ext(file) {
	case ".py", ".rb":
		return hash
	case ".php":
		if hash {
			return true
		}
	}
	if strings.HasPrefix(trimmed, "//") {
		return true
	}
	body, open := strings.CutPrefix(trimmed, "/*")
	_, after, closed := strings.Cut(body, "*/")
	return open && (!closed || strings.TrimSpace(after) == "")
}

// patchInjectionPatterns are item 7's own injection markers (L2), matched
// case-insensitively over the added lines joined by LF, with any run of
// blanks between words; 001's fixed markers of promptlayer still apply
// through Sanitize.
var patchInjectionPatterns = []*regexp.Regexp{
	regexp.MustCompile(`(?i)\b(ignore|disregard|forget|override)\s+((all|any|the|of)\s+)*(previous|prior|above|earlier|preceding)\s+(instructions?|prompts?|messages?|rules|directions)\b`),
	regexp.MustCompile(`(?i)\b(reveal|print|show|repeat|leak|output)\s+((me|the|your)\s+)*(system|developer|hidden)\s+(prompt|message|instructions?)\b`),
	regexp.MustCompile(`(?i)\bdeveloper\s+message\b`),
}

// injectionPhrase reports an item 7 injection marker in text.
func injectionPhrase(text string) bool {
	for _, pattern := range patchInjectionPatterns {
		if pattern.MatchString(text) {
			return true
		}
	}
	return false
}
