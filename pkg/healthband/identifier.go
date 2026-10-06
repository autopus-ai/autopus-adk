package healthband

import (
	"crypto/sha256"
	"encoding/hex"
	"strings"
)

// H8 returns the first 8 hex digits of the SHA-256 of s.
func H8(s string) string {
	sum := sha256.Sum256([]byte(s))
	return hex.EncodeToString(sum[:hashSuffixLen/2])
}

// SanitizeIdentifier filters an untrusted workflow name or canary target to
// [A-Za-z0-9 ._:+-] and at most 80 characters (Untrusted Input Contract item
// 7). A result that differs from raw, or is empty, gets "#" plus the H8 of
// raw so distinct raw names stay distinct; changed reports that, and callers
// record ReasonIdentifierSanitized. Only the result may be stored or printed.
func SanitizeIdentifier(raw string) (id string, changed bool) {
	var b strings.Builder
	for i := 0; i < len(raw) && b.Len() < identifierMaxLen; i++ {
		if isIdentifierByte(raw[i]) {
			b.WriteByte(raw[i])
		}
	}
	filtered := b.String()
	if filtered == raw && filtered != "" {
		return filtered, false
	}
	return filtered + "#" + H8(raw), true
}

// CISeriesID returns ci.failure_rate:<filtered workflow name>.
func CISeriesID(workflowName string) (string, bool) {
	id, changed := SanitizeIdentifier(workflowName)
	return SeriesPrefixCI + id, changed
}

// CanarySeriesID returns canary.failure_rate:<filtered target>, where the
// target is the joined host set or "local".
func CanarySeriesID(target string) (string, bool) {
	id, changed := SanitizeIdentifier(target)
	return SeriesPrefixCanary + id, changed
}

// identifierMaxLen bounds a filtered workflow name or canary target.
const identifierMaxLen = 80

// hashSuffixLen is the hex length of the #<h8> suffix of a filtered name.
const hashSuffixLen = 8

// isIdentifierByte reports membership in [A-Za-z0-9 ._:+-].
func isIdentifierByte(c byte) bool {
	switch {
	case c >= 'a' && c <= 'z', c >= 'A' && c <= 'Z', c >= '0' && c <= '9':
		return true
	}
	return strings.IndexByte(" ._:+-", c) >= 0
}

func allIdentifierBytes(s string) bool {
	for i := 0; i < len(s); i++ {
		if !isIdentifierByte(s[i]) {
			return false
		}
	}
	return true
}

func isLowerHex(s string) bool {
	for i := 0; i < len(s); i++ {
		if !(s[i] >= '0' && s[i] <= '9' || s[i] >= 'a' && s[i] <= 'f') {
			return false
		}
	}
	return true
}

// validIdentifier accepts what the ingest filter can emit: 1–80 identifier
// bytes, or 0–80 of them followed by #<8 lowercase hex>. '#' is outside the
// identifier set, so only the suffix can hold it.
func validIdentifier(id string) bool {
	name, suffix, hashed := strings.Cut(id, "#")
	if len(name) > identifierMaxLen || !allIdentifierBytes(name) {
		return false
	}
	if !hashed {
		return name != ""
	}
	return len(suffix) == hashSuffixLen && isLowerHex(suffix)
}

// ValidSeriesID reports whether series is a known series prefix followed by
// a filtered identifier. The store rejects anything else at read and write,
// so no unfiltered name reaches events, state, or terminal output.
func ValidSeriesID(series string) bool {
	for _, prefix := range []string{SeriesPrefixCI, SeriesPrefixCanary} {
		if rest, ok := strings.CutPrefix(series, prefix); ok {
			return validIdentifier(rest)
		}
	}
	return false
}
