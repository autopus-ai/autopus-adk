package cli

import (
	"strconv"
	"strings"
)

// stableCurrentVersion reports the release this binary speaks for, and whether
// that claim is a stable release at all.
//
// A git-describe suffix (`-dirty`, `-20-gffda591a`, `-20-gffda591a-dirty`)
// still names its base release: such a build carries that release's surface
// plus local commits. Any other pre-release label does not. A candidate built
// as `0.50.109-canary` is not release 0.50.109 — treating it as one made the
// freshness gate install the newer official release over it and re-exec that
// binary, so the upgrade canary proved the released surface instead of the
// candidate's own.
func stableCurrentVersion(raw string) (string, bool) {
	value := strings.TrimPrefix(strings.TrimSpace(raw), "v")
	if index := strings.IndexByte(value, '+'); index != -1 {
		value = value[:index]
	}
	if index := strings.IndexByte(value, '-'); index != -1 {
		if !gitDescribeSuffix(value[index+1:]) {
			return "", false
		}
		value = value[:index]
	}
	parts := strings.Split(value, ".")
	if len(parts) != 3 {
		return "", false
	}
	for _, part := range parts {
		if part == "" {
			return "", false
		}
		if _, err := strconv.Atoi(part); err != nil {
			return "", false
		}
		for _, char := range part {
			if char < '0' || char > '9' {
				return "", false
			}
		}
	}
	return value, true
}

// gitDescribeSuffix reports whether suffix is one `git describe --tags --dirty`
// appends: bare `dirty`, `<commits>-g<sha>`, or `<commits>-g<sha>-dirty`.
func gitDescribeSuffix(suffix string) bool {
	if suffix == "dirty" {
		return true
	}
	suffix = strings.TrimSuffix(suffix, "-dirty")
	count, sha, found := strings.Cut(suffix, "-g")
	if !found || count == "" || len(sha) < 7 {
		return false
	}
	if _, err := strconv.Atoi(count); err != nil {
		return false
	}
	for _, char := range sha {
		if (char < '0' || char > '9') && (char < 'a' || char > 'f') {
			return false
		}
	}
	return true
}
