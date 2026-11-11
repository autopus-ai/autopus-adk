package freshness

import (
	"regexp"
	"strings"
)

// RangeKind says how much of the version line a spec allows.
type RangeKind int

const (
	// RangeExact pins one version (or one line when fewer parts are given).
	RangeExact RangeKind = iota
	// RangeCaret allows any minor of the major line (`^1.2`, cargo `1.2`).
	RangeCaret
	// RangeTilde allows any patch of the minor line (`~1.2`, `~=1.2.3`).
	RangeTilde
	// RangeFloor has a lower bound only (`>=18`): it already allows latest.
	RangeFloor
	// RangeBelow has an upper bound (`<2`): it allows only lines under Base.
	RangeBelow
	// RangeAny is `*`, `latest` or `x`.
	RangeAny
)

// Constraint is a parsed version spec.
type Constraint struct {
	Base Version
	Kind RangeKind
}

var (
	upperBound = regexp.MustCompile(`<=?\s*v?(\d[\w.\-]*)`)
	opPrefix   = regexp.MustCompile(`^(\^|~=|~>|~|===|==|=|>=|>|<=|<|!=)?\s*`)
)

// ParseConstraint reads npm, PyPI, cargo, Go and plain image-tag specs. ok is
// false for specs that name no comparable version (a URL, a path, a tag word).
// defaultCaret says whether a bare `1.2` means a caret range (cargo) or an
// exact pin (everything else).
func ParseConstraint(raw string, defaultCaret bool) (Constraint, bool) {
	s := strings.TrimSpace(raw)
	switch strings.ToLower(s) {
	case "", "*", "x", "latest", "any":
		return Constraint{Kind: RangeAny}, s != ""
	}
	if strings.Contains(s, "||") {
		// The last alternative is the newest one an npm range allows.
		alts := strings.Split(s, "||")
		s = strings.TrimSpace(alts[len(alts)-1])
	}
	if m := upperBound.FindStringSubmatch(s); m != nil {
		if v, ok := ParseVersion(m[1]); ok {
			return Constraint{Base: v, Kind: RangeBelow}, true
		}
	}
	// Several comparators without an upper bound: the first one decides.
	first := strings.FieldsFunc(s, func(r rune) bool { return r == ',' || r == ' ' })
	if len(first) == 0 {
		return Constraint{}, false
	}
	s = first[0]
	if len(first) > 1 && opPrefix.FindString(s) == s {
		s += first[1] // `>= 18` split by the space
	}
	op := strings.TrimSpace(opPrefix.FindString(s))
	rest := strings.TrimSpace(s[len(opPrefix.FindString(s)):])
	rest = strings.TrimSuffix(strings.TrimSuffix(rest, ".x"), ".*")
	if rest == "x" || rest == "*" {
		return Constraint{Kind: RangeAny}, true
	}
	v, ok := ParseVersion(rest)
	if !ok || (len(rest) > 0 && !startsNumeric(rest)) {
		return Constraint{}, false
	}
	c := Constraint{Base: v}
	switch op {
	case "^":
		c.Kind = RangeCaret
	case "~", "~>":
		c.Kind = RangeTilde
	case "~=":
		// PEP 440: ~=1.2 is >=1.2,<2; ~=1.2.3 is >=1.2.3,<1.3.
		c.Kind = RangeTilde
		if v.Parts == 2 {
			c.Kind = RangeCaret
		}
	case ">=", ">":
		c.Kind = RangeFloor
	case "", "=", "==", "===":
		c.Kind = RangeExact
		if op == "" && defaultCaret {
			c.Kind = RangeCaret
		}
	default:
		return Constraint{}, false
	}
	return c, true
}

func startsNumeric(s string) bool {
	s = strings.TrimPrefix(strings.TrimPrefix(s, "go"), "v")
	return s != "" && s[0] >= '0' && s[0] <= '9'
}
