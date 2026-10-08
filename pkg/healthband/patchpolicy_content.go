package healthband

import (
	"strings"
	"unicode/utf8"
)

// Patch Policy items 7–8 over the added lines that the hunk headers count.
// The raw check comes first (CD-3 H1, N1), because Sanitize strips control
// and format characters silently while git apply applies the raw diff.

// minEncodedRun is the shortest run of base64 or hex characters refused.
const minEncodedRun = 40

// deniedContent is item 7: per added line the raw check and then the
// mixed-script check outside whole-line comments, then Sanitize, the
// injection phrases, and the encoded runs over all added lines.
func deniedContent(files []*diffFile) string {
	var added []string
	for _, file := range files {
		for _, line := range file.lines {
			if !cleanLine(line) {
				return PatchCodeControlChar
			}
			if !commentLine(file.path, line.text) && mixedScriptToken(line.text) {
				return PatchCodeConfusable
			}
			added = append(added, line.text)
		}
	}
	text := strings.Join(added, "\n")
	evidence := Sanitize(text, SanitizeOptions{Cut: KeepHead, Limit: len(text) + 1})
	for _, reason := range evidence.Reasons {
		if reason == ReasonSecretRisk || reason == ReasonInjectionRisk {
			return PatchCodeContentDenied
		}
	}
	if injectionPhrase(text) {
		return PatchCodeContentDenied
	}
	for _, line := range added {
		if hasEncodedRun(line) {
			return PatchCodeContentDenied
		}
	}
	return ""
}

// cleanLine is the raw check of one added line: valid UTF-8 and no code
// point of the item 7 set, except TAB and a CR that is the one byte before
// the LF that ends the line in the diff text. A line that a no-newline
// marker leaves without its LF keeps no CR exception.
func cleanLine(line addedLine) bool {
	text := line.text
	if !utf8.ValidString(text) {
		return false
	}
	if strings.HasSuffix(text, "\r") && !line.noEOL {
		text = text[:len(text)-1]
	}
	for _, r := range text {
		if r != '\t' && inControlSet(r) {
			return false
		}
	}
	return true
}

// hasEncodedRun reports a run of 40 or more characters of the RFC 4648
// base64 and base64url alphabets together (+, /, -, and _), which hold
// every hex digit. It is a heuristic: a 40-character identifier, camelCase
// or snake_case, is refused as well, an intended fail-closed limitation.
func hasEncodedRun(line string) bool {
	run := 0
	for i := 0; i < len(line); i++ {
		switch c := line[i]; {
		case 'A' <= c && c <= 'Z', 'a' <= c && c <= 'z', '0' <= c && c <= '9', c == '+', c == '/', c == '-', c == '_':
			if run++; run >= minEncodedRun {
				return true
			}
		default:
			run = 0
		}
	}
	return false
}

// tooLarge is item 8: at most 10 files and 400 changed lines.
func tooLarge(files []*diffFile) string {
	changed := 0
	for _, file := range files {
		changed += file.added + file.removed
	}
	if len(files) > PatchMaxFiles || changed > PatchMaxChangedLines {
		return PatchCodeTooLarge
	}
	return ""
}
