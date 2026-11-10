// Package securityscan runs the security scanners a project already has
// installed (gitleaks, govulncheck, npm/pnpm/yarn audit, cargo audit,
// pip-audit) and folds their JSON into one finding shape. It also holds the
// dependency-free secret patterns the verification Stop gate uses.
package securityscan

import (
	"fmt"
	"strings"
)

// Kind separates leaked credentials from vulnerable dependencies.
type Kind string

const (
	KindSecret Kind = "secret"
	KindVuln   Kind = "vuln"
)

// Normalized severities. Tools that do not grade their findings (govulncheck,
// cargo audit, pip-audit) report SeverityUnknown.
const (
	SeverityCritical = "critical"
	SeverityHigh     = "high"
	SeverityMedium   = "medium"
	SeverityLow      = "low"
	SeverityUnknown  = "unknown"
)

// Finding is one result in the common shape. It never carries a matched
// secret value: a secret is identified by file, line, rule, and fingerprint.
type Finding struct {
	Tool         string `json:"tool"`
	Kind         Kind   `json:"kind"`
	ID           string `json:"id"`
	Severity     string `json:"severity"`
	Package      string `json:"package,omitempty"`
	File         string `json:"file,omitempty"`
	Location     string `json:"location,omitempty"`
	FixedVersion string `json:"fixed_version,omitempty"`
	Summary      string `json:"summary,omitempty"`
	Fingerprint  string `json:"fingerprint,omitempty"`
}

// NormalizeSeverity maps a tool's severity word onto the normalized set.
func NormalizeSeverity(s string) string {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "critical":
		return SeverityCritical
	case "high":
		return SeverityHigh
	case "moderate", "medium":
		return SeverityMedium
	case "low", "info":
		return SeverityLow
	default:
		return SeverityUnknown
	}
}

// severityRank orders severities. An ungraded vulnerability ranks as high:
// the tools that do not grade still only report known, matched advisories,
// and govulncheck only those whose vulnerable code is reachable.
func severityRank(s string) int {
	switch s {
	case SeverityCritical:
		return 4
	case SeverityHigh, SeverityUnknown:
		return 3
	case SeverityMedium:
		return 2
	case SeverityLow:
		return 1
	}
	return 0
}

// FailOn is the threshold at which findings fail the scan.
type FailOn string

const (
	FailOnNone     FailOn = "none"
	FailOnSecret   FailOn = "secret"
	FailOnCritical FailOn = "critical"
	FailOnHigh     FailOn = "high"
	FailOnMedium   FailOn = "medium"
	FailOnLow      FailOn = "low"
)

// ParseFailOn validates a --fail-on value.
func ParseFailOn(s string) (FailOn, error) {
	switch f := FailOn(strings.ToLower(strings.TrimSpace(s))); f {
	case FailOnNone, FailOnSecret, FailOnCritical, FailOnHigh, FailOnMedium, FailOnLow:
		return f, nil
	}
	return "", fmt.Errorf("--fail-on %q: want none, secret, critical, high, medium, or low", s)
}

// Fails reports whether a finding reaches the threshold. Any secret fails
// every threshold except none; vulnerabilities fail at or above the named
// severity, and never under secret.
func (f FailOn) Fails(fd Finding) bool {
	switch f {
	case FailOnNone:
		return false
	case FailOnSecret:
		return fd.Kind == KindSecret
	}
	if fd.Kind == KindSecret {
		return true
	}
	return severityRank(fd.Severity) >= severityRank(string(f))
}
