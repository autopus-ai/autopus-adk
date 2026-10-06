package healthband

import "strings"

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
