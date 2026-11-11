package cireport

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// maxSinceDays bounds --since; the runs API keeps about 400 days anyway.
const maxSinceDays = 400

var (
	sincePattern = regexp.MustCompile(`^(\d+)([hdw])$`)
	repoPattern  = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9-]*/[A-Za-z0-9._-]+$`)
	// remotePattern matches the owner/name part of an SSH, scp-like, or
	// HTTPS github.com remote URL.
	remotePattern = regexp.MustCompile(`^(?:https?://(?:[^@/]+@)?github\.com/|ssh://git@github\.com/|git@github\.com:)([^/]+/[^/]+?)(?:\.git)?/?$`)
)

// ParseSince reads a window start: Nh, Nd, Nw before now, or a YYYY-MM-DD
// date in UTC.
func ParseSince(value string, now time.Time) (time.Time, error) {
	value = strings.TrimSpace(value)
	if m := sincePattern.FindStringSubmatch(value); m != nil {
		n, _ := strconv.Atoi(m[1])
		unit := map[string]time.Duration{"h": time.Hour, "d": 24 * time.Hour, "w": 7 * 24 * time.Hour}[m[2]]
		d := time.Duration(n) * unit
		if n <= 0 || d > maxSinceDays*24*time.Hour {
			return time.Time{}, fmt.Errorf("--since %q must be above 0 and at most %d days", value, maxSinceDays)
		}
		return now.Add(-d), nil
	}
	if t, err := time.Parse("2006-01-02", value); err == nil && t.Before(now) {
		return t, nil
	}
	return time.Time{}, fmt.Errorf("--since %q must be Nh, Nd, Nw, or a past YYYY-MM-DD date", value)
}

// ParseGitHubRemote reads owner/name from a github.com remote URL.
func ParseGitHubRemote(remote string) (string, error) {
	m := remotePattern.FindStringSubmatch(strings.TrimSpace(remote))
	if m == nil || !ValidRepo(m[1]) {
		return "", fmt.Errorf("remote %q is not a github.com repository; pass --repo owner/name", remote)
	}
	return m[1], nil
}

// ValidRepo reports whether repo is a plain owner/name, safe to put in an
// API path.
func ValidRepo(repo string) bool {
	return repoPattern.MatchString(repo) && !strings.Contains(repo, "..")
}
