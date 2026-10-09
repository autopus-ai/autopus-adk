package editguard

import (
	"net/url"
	"path/filepath"
	"slices"
	"strings"
	"unicode/utf8"
)

// geminiSpellings returns the file_path a Gemini CLI 0.52.0 write_file or
// replace call names and every other spelling the host may write it as, each
// once, so a deny of any of them denies the call. Gemini does not write the
// path as sent: resolveDefensiveToolPath removes NUL bytes and, unless a file
// of the literal name exists, a leading @ with the separators after it, and
// resolveToRealPath converts a file:// URL and percent-decodes the absolute
// path, keeping a spelling that does not decode as it is. Both the @ and the
// stripped spelling are judged, so no file created in between changes the
// decision, and so is a file:// URL whichever branch reaches it. A spelling
// that still holds a NUL byte is never written and is left out. decodesFirst
// is set for an absolute replace path, which the host decodes before it
// resolves the `..` in it (bundle chunk-7LQRUKPT.js:307921 and 308375), so
// that spelling is judged too; write_file and a relative replace path are
// resolved before they are decoded. Every decoded spelling is decoded again
// until it stops changing (decodedRounds), so a host that decodes a path
// once more than 0.52.0 does is covered as well.
func geminiSpellings(cwd, raw string, decodesFirst bool) []string {
	var spellings []string
	add := func(p string) {
		if p != "" && !strings.ContainsRune(p, 0) && !slices.Contains(spellings, p) {
			spellings = append(spellings, p)
		}
	}
	bases := geminiBases(raw)
	for _, base := range bases {
		add(base)
		if converted, ok := fileURLPath(base); ok {
			add(converted)
			for _, decoded := range decodedRounds(converted) {
				add(decoded)
			}
		}
		for _, decoded := range decodedRounds(lexicalAbs(cwd, base)) {
			add(decoded)
		}
		if decodesFirst {
			for _, decoded := range decodedRounds(base) {
				add(lexicalAbs(cwd, decoded))
			}
		}
	}
	if len(spellings) == 0 {
		return bases[:1] // an empty path: the malformed entry is dropped
	}
	return spellings
}

// maxDecodeRounds bounds decodedRounds: a path is decoded at most this often.
const maxDecodeRounds = 4

// decodedRounds returns s percent-decoded once, then that result decoded
// again, and so on while a round decodes and changes the path, at most
// maxDecodeRounds rounds. A round that finds no escape, or escapes that do not
// decode, ends it.
func decodedRounds(s string) []string {
	var rounds []string
	for len(rounds) < maxDecodeRounds {
		decoded, ok := decodeURIComponent(s)
		if !ok || decoded == s {
			break
		}
		rounds = append(rounds, decoded)
		s = decoded
	}
	return rounds
}

// geminiBases are the spellings resolveDefensiveToolPath may return: the path
// without NUL bytes and, after a leading @, without the @ and the separators
// that follow it.
func geminiBases(raw string) []string {
	clean := strings.ReplaceAll(raw, "\x00", "")
	bases := []string{clean}
	if rest, ok := strings.CutPrefix(clean, "@"); ok {
		if stripped := strings.TrimLeft(rest, `/\`); stripped != "" {
			bases = append(bases, stripped)
		}
	}
	return bases
}

// lexicalAbs is path.resolve(cwd, p) for an absolute cwd; a relative p stays
// relative without one.
func lexicalAbs(cwd, p string) string {
	switch {
	case filepath.IsAbs(p):
		return filepath.Clean(p)
	case filepath.IsAbs(cwd):
		return filepath.Join(cwd, p)
	}
	return p
}

// decodeURIComponent decodes every percent-escape of s as JavaScript does. ok
// is false when s has no escape or when the escapes are malformed or decode to
// invalid UTF-8, where JavaScript throws.
func decodeURIComponent(s string) (string, bool) {
	if !strings.Contains(s, "%") {
		return "", false
	}
	decoded, err := url.PathUnescape(s)
	if err != nil || !utf8.ValidString(decoded) {
		return "", false
	}
	return decoded, true
}

// fileURLPath is Node's fileURLToPath on POSIX: the decoded path of a file://
// URL with an empty or localhost host and no encoded slash.
func fileURLPath(s string) (string, bool) {
	if !strings.HasPrefix(s, "file://") {
		return "", false
	}
	u, err := url.Parse(s)
	if err != nil || u.Host != "" && u.Host != "localhost" || u.Path == "" ||
		strings.Contains(strings.ToLower(u.EscapedPath()), "%2f") || !utf8.ValidString(u.Path) {
		return "", false
	}
	return u.Path, true
}
