package freshness

import (
	"fmt"
	"time"
)

// Ecosystem names where an item's latest version is looked up.
type Ecosystem string

const (
	EcoGo        Ecosystem = "go"
	EcoNPM       Ecosystem = "npm"
	EcoPyPI      Ecosystem = "pypi"
	EcoCrates    Ecosystem = "crates"
	EcoImage     Ecosystem = "image"
	EcoToolchain Ecosystem = "toolchain"
)

// Item is one version a project pins: a direct dependency, a toolchain or
// runtime pin, or a container image tag.
type Item struct {
	Ecosystem Ecosystem `json:"ecosystem"`
	Name      string    `json:"name"`
	Spec      string    `json:"spec"`
	// Field tells two pins of one name in one file apart, such as the go
	// and toolchain lines of go.mod or dependencies and devDependencies.
	Field string `json:"field,omitempty"`
	File  string `json:"file,omitempty"`
	Line  int    `json:"line,omitempty"`
	// Pinned is set when the manifest line carries a `version-pin:` comment.
	Pinned bool `json:"pinned,omitempty"`
	// Registry is an npm registry from the project's .npmrc, if any.
	Registry string `json:"-"`
}

// Key identifies the item within a file regardless of its version.
func (i Item) Key() string {
	if i.Field != "" {
		return string(i.Ecosystem) + ":" + i.Name + "@" + i.Field
	}
	return string(i.Ecosystem) + ":" + i.Name
}

// Latest is what a registry says is the newest stable release. Err is set,
// and Version empty, when the answer is unknown.
type Latest struct {
	Version   string    `json:"version,omitempty"`
	LTS       string    `json:"lts,omitempty"`
	Source    string    `json:"source,omitempty"`
	FetchedAt time.Time `json:"fetched_at"`
	Err       string    `json:"error,omitempty"`
}

// Status classifies a pin against the latest stable release.
type Status string

const (
	StatusCurrent Status = "current"
	StatusMinor   Status = "minor-behind"
	StatusMajor   Status = "major-behind"
	StatusUnknown Status = "unknown"
	StatusSkipped Status = "skipped"
)

// Result is one classified item.
type Result struct {
	Item        Item   `json:"item"`
	Latest      Latest `json:"latest"`
	Target      string `json:"target,omitempty"`
	Status      Status `json:"status"`
	LinesBehind int    `json:"lines_behind,omitempty"`
	Reason      string `json:"reason,omitempty"`
}

// usesLTS says the item is a Node runtime pin, judged against the active LTS
// line rather than the odd-numbered current release.
func usesLTS(i Item) bool {
	return i.Name == "node" && (i.Ecosystem == EcoToolchain || i.Ecosystem == EcoImage)
}

// minorLines says one release line is a minor version: Go and Python ship
// 1.N and 3.N lines that each carry their own support window.
func minorLines(i Item) bool {
	switch i.Ecosystem {
	case EcoToolchain:
		return i.Name == "go" || i.Name == "python"
	case EcoImage:
		return i.Name == "golang" || i.Name == "python"
	}
	return false
}

// Classify compares the item's spec with latest.
func Classify(i Item, latest Latest) Result {
	r := Result{Item: i, Latest: latest}
	if latest.Err != "" {
		r.Status, r.Reason = StatusUnknown, latest.Err
		return r
	}
	r.Target = latest.Version
	if usesLTS(i) && latest.LTS != "" {
		r.Target = latest.LTS
	}
	c, ok := ParseConstraint(i.Spec, i.Ecosystem == EcoCrates)
	if !ok {
		r.Status, r.Reason = StatusSkipped, "the spec names no comparable version"
		return r
	}
	lv, ok := ParseVersion(r.Target)
	if !ok {
		r.Status, r.Reason = StatusUnknown, fmt.Sprintf("unreadable latest version %q", r.Target)
		return r
	}
	r.Status = StatusCurrent
	if c.Kind == RangeAny || c.Kind == RangeFloor {
		return r
	}
	depth := 1
	if minorLines(i) || (c.Base.Major == 0 && lv.Major == 0) {
		depth = 2
	}
	if c.Kind == RangeCaret && depth == 2 && c.Base.Major > 0 {
		depth = 1 // ^3.11 allows every 3.x line
	}
	lines := lineDiff(c.Base, lv, depth)
	if c.Kind == RangeBelow {
		if Compare(lv, c.Base) < 0 {
			return r
		}
		// `<2` excludes the whole 2.x line; `<2.5` only part of it.
		if c.Base.Patch == 0 && (depth == 2 || c.Base.Minor == 0) {
			lines++
		}
		r.Status, r.LinesBehind = StatusMinor, 0
		if lines > 0 {
			r.Status, r.LinesBehind = StatusMajor, lines
		}
		return r
	}
	switch {
	case lines > 0:
		r.Status, r.LinesBehind = StatusMajor, lines
	case lines < 0:
		// Newer than the latest stable: a pre-release pin, not stale.
	case depth == 1 && c.Kind != RangeCaret && c.Base.Parts >= 2 && lv.Minor > c.Base.Minor:
		r.Status = StatusMinor
	case depth == 2 && c.Kind == RangeExact && c.Base.Parts >= 3 && lv.Patch > c.Base.Patch:
		r.Status = StatusMinor
	}
	return r
}

// lineDiff counts release lines from base to latest at the given depth.
func lineDiff(base, latest Version, depth int) int {
	if depth == 1 || base.Major != latest.Major {
		if d := latest.Major - base.Major; d != 0 || depth == 1 {
			return d
		}
	}
	if base.Parts < 2 {
		return 0
	}
	return latest.Minor - base.Minor
}
