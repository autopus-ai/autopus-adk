// Package freshness looks up the latest stable versions of dependencies,
// toolchains and container images, reads the versions a project pins, and
// classifies how far behind each pin is. It never fails a caller because a
// registry is unreachable: an answer it cannot get is "unknown".
package freshness

import (
	"strconv"
	"strings"
)

// Version is a dotted numeric version. Parts counts how many numeric
// components the source spelled out, so `20` is not judged on its minor.
type Version struct {
	Major, Minor, Patch int
	Parts               int
	Pre                 string
}

// ParseVersion reads `v1.2.3`, `go1.22`, `1.2`, `8.0.36-1` and similar. A
// suffix after the numbers that is not build metadata is kept in Pre.
func ParseVersion(raw string) (Version, bool) {
	s := strings.TrimSpace(raw)
	s = strings.TrimPrefix(s, "go")
	s = strings.TrimPrefix(strings.TrimPrefix(s, "v"), "V")
	if i := strings.IndexByte(s, '+'); i >= 0 {
		s = s[:i]
	}
	var v Version
	num, rest := leadingDotted(s)
	if num == "" {
		return Version{}, false
	}
	parts := strings.Split(num, ".")
	if len(parts) > 3 {
		parts = parts[:3]
	}
	vals := [3]int{}
	for i, p := range parts {
		n, err := strconv.Atoi(p)
		if err != nil {
			return Version{}, false
		}
		vals[i] = n
	}
	v.Major, v.Minor, v.Patch, v.Parts = vals[0], vals[1], vals[2], len(parts)
	v.Pre = strings.TrimLeft(rest, "-.")
	return v, true
}

// leadingDotted splits "1.2.3-rc1" into "1.2.3" and "-rc1".
func leadingDotted(s string) (num, rest string) {
	end := 0
	for end < len(s) {
		c := s[end]
		if c >= '0' && c <= '9' {
			end++
			continue
		}
		if c == '.' && end > 0 && end+1 < len(s) && s[end+1] >= '0' && s[end+1] <= '9' {
			end++
			continue
		}
		break
	}
	return s[:end], s[end:]
}

// IsPrerelease reports a pre-release marker: a semver `-` suffix or a word
// such as rc, beta or alpha glued to the number (`3.14.0rc1`).
func (v Version) IsPrerelease() bool {
	if v.Pre == "" {
		return false
	}
	lower := strings.ToLower(v.Pre)
	for _, marker := range []string{"alpha", "beta", "rc", "pre", "dev", "next", "canary", "preview", "snapshot", "nightly", "a", "b"} {
		if strings.HasPrefix(lower, marker) {
			return true
		}
	}
	// Go pseudo-versions and other dash suffixes count as pre-release.
	return strings.ContainsAny(lower, "0123456789")
}

// Compare orders versions by their numbers; pre-release sorts first.
func Compare(a, b Version) int {
	for _, d := range [][2]int{{a.Major, b.Major}, {a.Minor, b.Minor}, {a.Patch, b.Patch}} {
		if d[0] != d[1] {
			if d[0] < d[1] {
				return -1
			}
			return 1
		}
	}
	switch {
	case a.Pre == b.Pre:
		return 0
	case a.Pre == "":
		return 1
	case b.Pre == "":
		return -1
	}
	return strings.Compare(a.Pre, b.Pre)
}

func (v Version) String() string {
	parts := []string{strconv.Itoa(v.Major)}
	if v.Parts >= 2 {
		parts = append(parts, strconv.Itoa(v.Minor))
	}
	if v.Parts >= 3 {
		parts = append(parts, strconv.Itoa(v.Patch))
	}
	s := strings.Join(parts, ".")
	if v.Pre != "" {
		s += "-" + v.Pre
	}
	return s
}

// MaxStable returns the highest non-pre-release version among raw tags.
func MaxStable(raw []string) (string, bool) {
	best, bestRaw, found := Version{}, "", false
	for _, r := range raw {
		v, ok := ParseVersion(r)
		if !ok || v.IsPrerelease() || v.Pre != "" {
			continue
		}
		if !found || Compare(v, best) > 0 {
			best, bestRaw, found = v, r, true
		}
	}
	return bestRaw, found
}
